package frameworks

import (
	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"fmt"
	"path/filepath"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type NetHTTPDetector struct {
	packageStates map[string]*netHTTPPackageState
}

type netHTTPPackageState struct {
	serveMuxFields map[string]map[string]bool
}

func NewNetHTTPDetector() *NetHTTPDetector {
	return &NetHTTPDetector{
		packageStates: map[string]*netHTTPPackageState{},
	}
}

func (d *NetHTTPDetector) Name() string {
	return "net/http"
}

func (d *NetHTTPDetector) PreparePackage(files []boundary.ParsedGoFile, _ []symbol.Symbol) []symbol.Diagnostic {
	if len(files) == 0 {
		return nil
	}
	state := &netHTTPPackageState{
		serveMuxFields: structFieldsMatchingType(files, func(rawType string, aliases map[string]string) bool {
			return matchesQualifiedType(rawType, aliases, "net/http", "ServeMux")
		}),
	}
	d.packageStates[d.packageKey(files[0])] = state
	return nil
}

func (d *NetHTTPDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	state := d.packageStates[d.packageKey(file)]
	imports := fileImportAliases(file)

	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	walk(file.Root, func(node *tree_sitter.Node) bool {
		switch node.Kind() {
		case "function_declaration", "method_declaration", "func_literal":
			body := node.ChildByFieldName("body")
			if body == nil || body.Kind() != "block" {
				return false
			}
			scope := d.initialScope(node, file.Content, imports)
			receiverEnv := receiverAliasesForDeclaration(node, file.Content)
			foundRoots, foundDiags := d.processBlock(body, file, symbols, imports, scope, receiverEnv, state)
			roots = append(roots, foundRoots...)
			diags = append(diags, foundDiags...)
			return false
		default:
			return true
		}
	})

	return roots, diags
}

func (d *NetHTTPDetector) processBlock(block *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, imports map[string]string, inheritedScope map[string]bool, receiverEnv map[string]string, state *netHTTPPackageState) ([]boundaryroot.Root, []symbol.Diagnostic) {
	scope := copyBoolScope(inheritedScope)
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	statementList := statementListNode(block)
	for i := 0; i < int(statementList.NamedChildCount()); i++ {
		stmt := statementList.NamedChild(uint(i))
		if stmt == nil {
			continue
		}

		d.bindServeMuxScope(stmt, file.Content, imports, scope, receiverEnv, state)

		if call := callExpressionFromStatement(stmt); call != nil {
			root, diag := d.handleCall(call, file, symbols, imports, scope, receiverEnv, state)
			if root != nil {
				roots = append(roots, *root)
			}
			if diag.Category != "" {
				diags = append(diags, diag)
			}
		}

		for _, child := range nestedBlocks(stmt) {
			nestedRoots, nestedDiags := d.processBlock(child, file, symbols, imports, scope, receiverEnv, state)
			roots = append(roots, nestedRoots...)
			diags = append(diags, nestedDiags...)
		}
	}
	return roots, diags
}

func (d *NetHTTPDetector) initialScope(node *tree_sitter.Node, content []byte, imports map[string]string) map[string]bool {
	scope := map[string]bool{}
	for name, rawType := range declarationParameterTypes(node, content) {
		if matchesQualifiedType(rawType, imports, "net/http", "ServeMux") {
			scope[name] = true
		}
	}
	return scope
}

func (d *NetHTTPDetector) bindServeMuxScope(stmt *tree_sitter.Node, content []byte, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *netHTTPPackageState) {
	switch stmt.Kind() {
	case "short_var_declaration", "short_variable_declaration", "assignment_statement":
		left := stmt.ChildByFieldName("left")
		right := stmt.ChildByFieldName("right")
		if (left == nil || right == nil) && stmt.NamedChildCount() >= 2 {
			left = stmt.NamedChild(0)
			right = stmt.NamedChild(1)
		}
		if left == nil || right == nil {
			return
		}
		leftItems := expressionItems(left)
		rightItems := expressionItems(right)
		count := minInt(len(leftItems), len(rightItems))
		for i := 0; i < count; i++ {
			name := identifierName(leftItems[i], content)
			if name == "" {
				continue
			}
			if d.resolveServeMuxExpr(rightItems[i], content, imports, scope, receiverEnv, state) {
				scope[name] = true
			} else {
				delete(scope, name)
			}
		}
	case "var_declaration":
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			spec := stmt.NamedChild(uint(i))
			if spec == nil || spec.Kind() != "var_spec" {
				continue
			}
			nameList := spec.ChildByFieldName("name")
			valueList := spec.ChildByFieldName("value")
			if nameList == nil || valueList == nil {
				continue
			}
			nameItems := expressionItems(nameList)
			valueItems := expressionItems(valueList)
			count := minInt(len(nameItems), len(valueItems))
			for j := 0; j < count; j++ {
				name := identifierName(nameItems[j], content)
				if name == "" {
					continue
				}
				if d.resolveServeMuxExpr(valueItems[j], content, imports, scope, receiverEnv, state) {
					scope[name] = true
				} else {
					delete(scope, name)
				}
			}
		}
	}
}

func (d *NetHTTPDetector) handleCall(call *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *netHTTPPackageState) (*boundaryroot.Root, symbol.Diagnostic) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Kind() != "selector_expression" {
		return nil, symbol.Diagnostic{}
	}

	field := fn.ChildByFieldName("field")
	if field == nil {
		return nil, symbol.Diagnostic{}
	}
	method := nodeText(field, file.Content)
	if method != "HandleFunc" && method != "Handle" {
		return nil, symbol.Diagnostic{}
	}

	receiver := fn.ChildByFieldName("operand")
	if !d.resolveServeMuxReceiver(receiver, file.Content, imports, scope, receiverEnv, state) {
		return nil, boundaryDiagnostic("boundary_unproven_receiver", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because receiver provenance was not proven", method))
	}

	args := call.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil, boundaryDiagnostic("boundary_insufficient_args", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because it did not provide path and handler arguments", method))
	}

	pathArg := args.NamedChild(0)
	if !isStringLiteral(pathArg) {
		return nil, boundaryDiagnostic("boundary_nonliteral_path", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because the path is not a string literal", method))
	}

	handlerArg := args.NamedChild(args.NamedChildCount() - 1)
	handlerTarget := resolveHandlerTarget(handlerArg, file.Content, symbols)
	root := boundaryroot.Root{
		Kind:            boundaryroot.KindHTTP,
		Framework:       "net/http",
		Method:          "ANY",
		Path:            cleanPath(getStringValue(pathArg, file.Content)),
		CanonicalName:   fmt.Sprintf("ANY %s", cleanPath(getStringValue(pathArg, file.Content))),
		HandlerTarget:   handlerTarget,
		RepositoryID:    file.RepositoryID,
		SourceFile:      file.Path,
		SourceStartByte: uint32(call.StartByte()),
		SourceEndByte:   uint32(call.EndByte()),
		SourceExpr:      nodeText(call, file.Content),
		Confidence:      "high",
	}
	root.ID = boundaryroot.StableID(root)

	if handlerTarget == "" {
		return &root, boundaryDiagnostic("boundary_unresolved_handler_target", file.Path, root.SourceExpr, fmt.Sprintf("route %s has no exact handler target", root.CanonicalName))
	}
	return &root, symbol.Diagnostic{}
}

func (d *NetHTTPDetector) resolveServeMuxReceiver(expr *tree_sitter.Node, content []byte, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *netHTTPPackageState) bool {
	expr = unwrapParens(expr)
	if expr == nil {
		return false
	}

	switch expr.Kind() {
	case "identifier":
		name := nodeText(expr, content)
		return importAliasMatches(imports, name, "net/http") || scope[name]
	case "selector_expression":
		fieldName, receiverType, ok := receiverField(expr, content, receiverEnv)
		return ok && state != nil && state.hasServeMuxField(receiverType, fieldName)
	default:
		return false
	}
}

func (d *NetHTTPDetector) resolveServeMuxExpr(expr *tree_sitter.Node, content []byte, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *netHTTPPackageState) bool {
	expr = unwrapParens(expr)
	if expr == nil {
		return false
	}

	switch expr.Kind() {
	case "identifier":
		return scope[nodeText(expr, content)]
	case "selector_expression":
		fieldName, receiverType, ok := receiverField(expr, content, receiverEnv)
		return ok && state != nil && state.hasServeMuxField(receiverType, fieldName)
	case "call_expression":
		fn := expr.ChildByFieldName("function")
		if fn == nil || fn.Kind() != "selector_expression" {
			return false
		}
		field := fn.ChildByFieldName("field")
		operand := fn.ChildByFieldName("operand")
		if field == nil || operand == nil || nodeText(field, content) != "NewServeMux" {
			return false
		}
		return operand.Kind() == "identifier" && importAliasMatches(imports, nodeText(operand, content), "net/http")
	default:
		return false
	}
}

func (d *NetHTTPDetector) packageKey(file boundary.ParsedGoFile) string {
	dir := filepath.ToSlash(filepath.Dir(file.Path))
	if dir == "." {
		dir = ""
	}
	return strings.Join([]string{file.RepositoryID, dir, file.PackageName}, "|")
}

func (s *netHTTPPackageState) hasServeMuxField(receiverType, fieldName string) bool {
	fields := s.serveMuxFields[receiverType]
	return fields != nil && fields[fieldName]
}

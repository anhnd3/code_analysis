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

type GRPCGatewayDetector struct {
	packageStates map[string]*grpcGatewayPackageState
}

type grpcGatewayPackageState struct {
	gatewayMuxFields map[string]map[string]bool
}

func NewGRPCGatewayDetector() *GRPCGatewayDetector {
	return &GRPCGatewayDetector{
		packageStates: map[string]*grpcGatewayPackageState{},
	}
}

func (d *GRPCGatewayDetector) Name() string {
	return "grpc-gateway"
}

func (d *GRPCGatewayDetector) PreparePackage(files []boundary.ParsedGoFile, _ []symbol.Symbol) []symbol.Diagnostic {
	if len(files) == 0 {
		return nil
	}
	state := &grpcGatewayPackageState{
		gatewayMuxFields: structFieldsMatchingType(files, func(rawType string, aliases map[string]string) bool {
			return matchesQualifiedTypeFunc(rawType, aliases, "ServeMux", isGRPCGatewayRuntimeImport)
		}),
	}
	d.packageStates[d.packageKey(files[0])] = state
	return nil
}

func (d *GRPCGatewayDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
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

func (d *GRPCGatewayDetector) processBlock(block *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, imports map[string]string, inheritedScope map[string]bool, receiverEnv map[string]string, state *grpcGatewayPackageState) ([]boundaryroot.Root, []symbol.Diagnostic) {
	scope := copyBoolScope(inheritedScope)
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	statementList := statementListNode(block)
	for i := 0; i < int(statementList.NamedChildCount()); i++ {
		stmt := statementList.NamedChild(uint(i))
		if stmt == nil {
			continue
		}

		d.bindGatewayMuxScope(stmt, file.Content, imports, scope, receiverEnv, state)

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

func (d *GRPCGatewayDetector) initialScope(node *tree_sitter.Node, content []byte, imports map[string]string) map[string]bool {
	scope := map[string]bool{}
	for name, rawType := range declarationParameterTypes(node, content) {
		if matchesQualifiedTypeFunc(rawType, imports, "ServeMux", isGRPCGatewayRuntimeImport) {
			scope[name] = true
		}
	}
	return scope
}

func (d *GRPCGatewayDetector) bindGatewayMuxScope(stmt *tree_sitter.Node, content []byte, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *grpcGatewayPackageState) {
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
			if d.resolveGatewayMuxExpr(rightItems[i], content, imports, scope, receiverEnv, state) {
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
				if d.resolveGatewayMuxExpr(valueItems[j], content, imports, scope, receiverEnv, state) {
					scope[name] = true
				} else {
					delete(scope, name)
				}
			}
		}
	}
}

func (d *GRPCGatewayDetector) handleCall(call *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *grpcGatewayPackageState) (*boundaryroot.Root, symbol.Diagnostic) {
	fn := call.ChildByFieldName("function")
	if fn == nil {
		return nil, symbol.Diagnostic{}
	}

	name := gatewayRegisterName(fn, file.Content)
	if !isGatewayRegisterName(name) {
		return nil, symbol.Diagnostic{}
	}

	args := call.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil, boundaryDiagnostic("boundary_insufficient_args", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because it did not provide a proven gateway mux", name))
	}

	muxArg := args.NamedChild(1)
	if !d.resolveGatewayMuxExpr(muxArg, file.Content, imports, scope, receiverEnv, state) {
		return nil, boundaryDiagnostic("boundary_unproven_receiver", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because gateway mux provenance was not proven", name))
	}

	handlerTarget := resolveHandlerTarget(fn, file.Content, symbols)
	if handlerTarget == "" || handlerTarget == nodeText(fn, file.Content) {
		handlerTarget = name
	}

	root := boundaryroot.Root{
		Kind:            boundaryroot.KindHTTPGateway,
		Framework:       "grpc-gateway",
		Method:          "PROXY",
		Path:            name,
		CanonicalName:   "PROXY " + name,
		HandlerTarget:   handlerTarget,
		RepositoryID:    file.RepositoryID,
		SourceFile:      file.Path,
		SourceStartByte: uint32(call.StartByte()),
		SourceEndByte:   uint32(call.EndByte()),
		SourceExpr:      nodeText(call, file.Content),
		Confidence:      "high",
	}
	root.ID = boundaryroot.StableID(root)
	return &root, symbol.Diagnostic{}
}

func gatewayRegisterName(fn *tree_sitter.Node, content []byte) string {
	switch fn.Kind() {
	case "selector_expression":
		if field := fn.ChildByFieldName("field"); field != nil {
			return nodeText(field, content)
		}
	case "identifier":
		return nodeText(fn, content)
	}
	return ""
}

func isGatewayRegisterName(name string) bool {
	if !strings.HasPrefix(name, "Register") {
		return false
	}
	switch {
	case strings.HasSuffix(name, "Handler"),
		strings.HasSuffix(name, "HandlerServer"),
		strings.HasSuffix(name, "HandlerClient"),
		strings.HasSuffix(name, "HandlerFromEndpoint"):
		return true
	default:
		return false
	}
}

func isGRPCGatewayRuntimeImport(path string) bool {
	return strings.Contains(path, "grpc-gateway") && strings.HasSuffix(path, "/runtime")
}

func (d *GRPCGatewayDetector) resolveGatewayMuxExpr(expr *tree_sitter.Node, content []byte, imports map[string]string, scope map[string]bool, receiverEnv map[string]string, state *grpcGatewayPackageState) bool {
	expr = unwrapParens(expr)
	if expr == nil {
		return false
	}

	switch expr.Kind() {
	case "identifier":
		return scope[nodeText(expr, content)]
	case "selector_expression":
		fieldName, receiverType, ok := receiverField(expr, content, receiverEnv)
		return ok && state != nil && state.hasGatewayMuxField(receiverType, fieldName)
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
		return operand.Kind() == "identifier" && importAliasMatchesFunc(imports, nodeText(operand, content), isGRPCGatewayRuntimeImport)
	default:
		return false
	}
}

func (d *GRPCGatewayDetector) packageKey(file boundary.ParsedGoFile) string {
	dir := filepath.ToSlash(filepath.Dir(file.Path))
	if dir == "." {
		dir = ""
	}
	return strings.Join([]string{file.RepositoryID, dir, file.PackageName}, "|")
}

func (s *grpcGatewayPackageState) hasGatewayMuxField(receiverType, fieldName string) bool {
	fields := s.gatewayMuxFields[receiverType]
	return fields != nil && fields[fieldName]
}

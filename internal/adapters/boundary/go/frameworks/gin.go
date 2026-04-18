package frameworks

import (
	"analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type GinDetector struct{}

type ginContext struct {
	prefix     string
	aliasDepth int
}

var ginRouteMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"DELETE":  true,
	"PATCH":   true,
	"HEAD":    true,
	"OPTIONS": true,
	"Any":     true,
}

func NewGinDetector() *GinDetector {
	return &GinDetector{}
}

func (d *GinDetector) Name() string {
	return "gin"
}

func (d *GinDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	walk(file.Root, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "function_declaration", "method_declaration", "func_literal":
			body := n.ChildByFieldName("body")
			if body != nil && body.Kind() == "block" {
				roots = append(roots, d.processBlock(body, file, symbols, map[string]ginContext{})...)
			}
			return false
		default:
			return true
		}
	})

	return roots, diags
}

func (d *GinDetector) processBlock(block *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, inherited map[string]ginContext) []boundaryroot.Root {
	scope := cloneGinScope(inherited)
	var roots []boundaryroot.Root

	statementList := block
	if block.NamedChildCount() == 1 {
		if child := block.NamedChild(0); child != nil && child.Kind() == "statement_list" {
			statementList = child
		}
	}

	for i := 0; i < int(statementList.NamedChildCount()); i++ {
		stmt := statementList.NamedChild(uint(i))
		if stmt == nil {
			continue
		}

		d.bindGinContext(stmt, file.Content, scope)

		if call := callExpressionFromStatement(stmt); call != nil {
			if root := d.handleRouteCall(call, file, scope, symbols); root != nil {
				roots = append(roots, *root)
			}
		}

		d.processNestedBlocks(stmt, file, symbols, scope, &roots)
	}

	return roots
}

func (d *GinDetector) processNestedBlocks(node *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, scope map[string]ginContext, roots *[]boundaryroot.Root) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "block":
			*roots = append(*roots, d.processBlock(child, file, symbols, scope)...)
		case "func_literal":
			// Route registration inside handler closures is out of scope for boundary detection.
			continue
		default:
			d.processNestedBlocks(child, file, symbols, scope, roots)
		}
	}
}

func (d *GinDetector) bindGinContext(stmt *tree_sitter.Node, content []byte, scope map[string]ginContext) {
	switch stmt.Kind() {
	case "short_var_declaration", "short_variable_declaration", "assignment_statement":
		d.bindAssignment(stmt, content, scope)
	case "var_declaration":
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			spec := stmt.NamedChild(uint(i))
			if spec != nil && spec.Kind() == "var_spec" {
				d.bindVarSpec(spec, content, scope)
			}
		}
	case "declaration_statement":
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			child := stmt.NamedChild(uint(i))
			if child != nil && child.Kind() == "var_declaration" {
				d.bindGinContext(child, content, scope)
			}
		}
	}
}

func (d *GinDetector) bindAssignment(stmt *tree_sitter.Node, content []byte, scope map[string]ginContext) {
	left := stmt.ChildByFieldName("left")
	right := stmt.ChildByFieldName("right")
	if (left == nil || right == nil) && stmt.NamedChildCount() >= 2 {
		left = stmt.NamedChild(0)
		right = stmt.NamedChild(1)
	}
	if left == nil || right == nil {
		return
	}

	count := minNamedChildren(left.NamedChildCount(), right.NamedChildCount())
	for i := 0; i < count; i++ {
		name := identifierName(left.NamedChild(uint(i)), content)
		if name == "" {
			continue
		}
		ctx, ok := d.resolveAssignedContext(right.NamedChild(uint(i)), content, scope)
		if ok {
			scope[name] = ctx
			continue
		}
		delete(scope, name)
	}
}

func (d *GinDetector) bindVarSpec(spec *tree_sitter.Node, content []byte, scope map[string]ginContext) {
	nameList := spec.ChildByFieldName("name")
	valueList := spec.ChildByFieldName("value")
	if nameList == nil || valueList == nil {
		return
	}

	count := minNamedChildren(nameList.NamedChildCount(), valueList.NamedChildCount())
	for i := 0; i < count; i++ {
		name := identifierName(nameList.NamedChild(uint(i)), content)
		if name == "" {
			continue
		}
		ctx, ok := d.resolveAssignedContext(valueList.NamedChild(uint(i)), content, scope)
		if ok {
			scope[name] = ctx
			continue
		}
		delete(scope, name)
	}
}

func (d *GinDetector) resolveAssignedContext(expr *tree_sitter.Node, content []byte, scope map[string]ginContext) (ginContext, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return ginContext{}, false
	}

	if expr.Kind() == "identifier" {
		source, ok := scope[nodeText(expr, content)]
		if !ok || source.aliasDepth >= 1 {
			return ginContext{}, false
		}
		return ginContext{
			prefix:     source.prefix,
			aliasDepth: source.aliasDepth + 1,
		}, true
	}

	ctx, ok := d.resolveContextExpression(expr, content, scope)
	if !ok {
		return ginContext{}, false
	}
	ctx.aliasDepth = 0
	return ctx, true
}

func (d *GinDetector) resolveContextExpression(expr *tree_sitter.Node, content []byte, scope map[string]ginContext) (ginContext, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return ginContext{}, false
	}

	switch expr.Kind() {
	case "identifier":
		ctx, ok := scope[nodeText(expr, content)]
		return ctx, ok
	case "call_expression":
		fn := expr.ChildByFieldName("function")
		if fn == nil || fn.Kind() != "selector_expression" {
			return ginContext{}, false
		}

		field := fn.ChildByFieldName("field")
		if field == nil {
			return ginContext{}, false
		}

		methodName := nodeText(field, content)
		operand := fn.ChildByFieldName("operand")
		args := expr.ChildByFieldName("arguments")

		switch methodName {
		case "New", "Default":
			if operand != nil && nodeText(operand, content) == "gin" {
				return ginContext{prefix: ""}, true
			}
		case "Group":
			parent, ok := d.resolveContextExpression(operand, content, scope)
			if !ok || args == nil || args.NamedChildCount() == 0 {
				return ginContext{}, false
			}
			prefixArg := args.NamedChild(0)
			if !isStringLiteral(prefixArg) {
				return ginContext{}, false
			}
			return ginContext{
				prefix: cleanPath(parent.prefix + "/" + getStringValue(prefixArg, content)),
			}, true
		}
	}

	return ginContext{}, false
}

func (d *GinDetector) handleRouteCall(n *tree_sitter.Node, file boundary.ParsedGoFile, scope map[string]ginContext, symbols []symbol.Symbol) *boundaryroot.Root {
	fn := n.ChildByFieldName("function")
	if fn == nil || fn.Kind() != "selector_expression" {
		return nil
	}

	field := fn.ChildByFieldName("field")
	if field == nil {
		return nil
	}

	method := nodeText(field, file.Content)
	if !ginRouteMethods[method] {
		return nil
	}

	receiver := fn.ChildByFieldName("operand")
	ctx, ok := d.resolveContextExpression(receiver, file.Content, scope)
	if !ok {
		return nil
	}

	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil
	}

	pathArg := args.NamedChild(0)
	if !isStringLiteral(pathArg) {
		return nil
	}

	fullPath := cleanPath(ctx.prefix + "/" + getStringValue(pathArg, file.Content))
	handlerArg := args.NamedChild(args.NamedChildCount() - 1)
	handlerTarget := resolveHandlerTarget(handlerArg, file.Content, symbols)

	root := boundaryroot.Root{
		Kind:            boundaryroot.KindHTTP,
		Framework:       "gin",
		Method:          method,
		Path:            fullPath,
		CanonicalName:   fmt.Sprintf("%s %s", method, fullPath),
		HandlerTarget:   handlerTarget,
		RepositoryID:    file.RepositoryID,
		SourceFile:      file.Path,
		SourceStartByte: uint32(n.StartByte()),
		SourceEndByte:   uint32(n.EndByte()),
		SourceExpr:      nodeText(n, file.Content),
		Confidence:      "high",
	}
	root.ID = boundaryroot.StableID(root)
	return &root
}

// resolveHandlerTarget extracts the most meaningful identifier from a handler argument node.
//
//   - Closure literal -> look up by symbol location, fall back to raw text.
//   - Wrapper / factory call (e.g. auth.Required(h)) -> keep the outer callee name.
//   - Bare identifier / selector -> return raw text.
//
// TODO(phase1-pr2): refine boundary wrapper attribution only if it materially improves entrypoint quality.
func resolveHandlerTarget(handlerArg *tree_sitter.Node, content []byte, symbols []symbol.Symbol) string {
	if handlerArg == nil {
		return ""
	}

	// Priority 1: symbol table lookup for closures.
	for _, sym := range symbols {
		if sym.Location.StartLine == uint32(handlerArg.StartPosition().Row+1) &&
			sym.Location.StartCol == uint32(handlerArg.StartPosition().Column+1) {
			return string(sym.ID)
		}
	}

	// Priority 2: if the argument is itself a call expression, use its callee name.
	// This preserves exact wrapper/factory registration knowledge without inventing inner business targets.
	if handlerArg.Kind() == "call_expression" {
		fn := handlerArg.ChildByFieldName("function")
		if fn != nil {
			switch fn.Kind() {
			case "selector_expression", "identifier":
				return nodeText(fn, content)
			}
		}
	}

	// Priority 3: raw text.
	return nodeText(handlerArg, content)
}

func cloneGinScope(scope map[string]ginContext) map[string]ginContext {
	cloned := make(map[string]ginContext, len(scope))
	for name, ctx := range scope {
		cloned[name] = ctx
	}
	return cloned
}

func callExpressionFromStatement(stmt *tree_sitter.Node) *tree_sitter.Node {
	switch stmt.Kind() {
	case "expression_statement":
		if stmt.NamedChildCount() == 0 {
			return nil
		}
		expr := stmt.NamedChild(0)
		if expr != nil && expr.Kind() == "call_expression" {
			return expr
		}
	case "call_expression":
		return stmt
	}
	return nil
}

func unwrapParens(n *tree_sitter.Node) *tree_sitter.Node {
	for n != nil && n.Kind() == "parenthesized_expression" && n.NamedChildCount() == 1 {
		n = n.NamedChild(0)
	}
	return n
}

func identifierName(n *tree_sitter.Node, content []byte) string {
	n = unwrapParens(n)
	if n == nil || n.Kind() != "identifier" {
		return ""
	}
	return nodeText(n, content)
}

func nodeText(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	return string(content[n.StartByte():n.EndByte()])
}

func minNamedChildren(left, right uint) int {
	if left < right {
		return int(left)
	}
	return int(right)
}

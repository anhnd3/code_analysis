package frameworks

import (
	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type GinDetector struct {
	packageStates map[string]*ginPackageState
}

const ginHelperExpansionMaxDepth = 3

type ginContext struct {
	prefix     string
	aliasDepth int
}

type handlerBinding struct {
	packageToken string
	receiverType string
	aliasDepth   int
}

type ginCallable struct {
	file         boundary.ParsedGoFile
	node         *tree_sitter.Node
	receiverType string
}

type ginPackageState struct {
	packageToken           string
	fieldContexts          map[string]map[string]ginContext
	methodProviders        map[string]map[string]ginContext
	functions              map[string]ginContext
	handlerFunctions       map[string]handlerBinding
	handlerMethodProviders map[string]map[string]handlerBinding
	packageFunctions       map[string][]symbol.Symbol
	packageMethods         map[string]map[string][]symbol.Symbol
	functionDecls          map[string][]ginCallable
	methodDecls            map[string]map[string][]ginCallable
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
	return &GinDetector{
		packageStates: map[string]*ginPackageState{},
	}
}

func (d *GinDetector) Name() string {
	return "gin"
}

func (d *GinDetector) PreparePackage(files []boundary.ParsedGoFile, symbols []symbol.Symbol) []symbol.Diagnostic {
	if len(files) == 0 {
		return nil
	}

	state := newGinPackageState()
	state.packageToken = filePackageToken(files[0])
	indexGinPackageSymbols(state, symbols)
	indexGinPackageCallables(state, files)
	for iteration := 0; iteration < 3; iteration++ {
		changed := false
		for _, file := range files {
			changed = d.prepareFile(file, state) || changed
		}
		if !changed {
			break
		}
	}

	d.packageStates[d.packageKey(files[0])] = state
	return nil
}

func (d *GinDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	state := d.packageStates[d.packageKey(file)]
	imports := fileImportAliases(file)

	walk(file.Root, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "function_declaration", "method_declaration", "func_literal":
			body := n.ChildByFieldName("body")
			if body == nil || body.Kind() != "block" {
				return false
			}
			if n.Kind() != "func_literal" && declarationHasGinContextParams(n, file.Content, imports) {
				return false
			}
			scope := initialGinScopeForDeclaration(n, file.Content, imports, nil)
			receiverEnv := declarationTypeAliases(n, file.Content)
			foundRoots, foundDiags := d.processBlock(body, file, symbols, imports, scope, map[string]handlerBinding{}, receiverEnv, state, 0, map[string]bool{})
			roots = append(roots, foundRoots...)
			diags = append(diags, foundDiags...)
			return false
		default:
			return true
		}
	})

	return dedupeGinRoots(roots), sortGinDiagnostics(diags)
}

func (d *GinDetector) prepareFile(file boundary.ParsedGoFile, state *ginPackageState) bool {
	changed := false
	imports := fileImportAliases(file)
	walk(file.Root, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "function_declaration", "method_declaration":
			body := n.ChildByFieldName("body")
			if body == nil || body.Kind() != "block" {
				return false
			}
			scope := initialGinScopeForDeclaration(n, file.Content, imports, nil)
			receiverEnv := declarationTypeAliases(n, file.Content)
			receiverType := declarationReceiverType(n, file.Content)
			changed = d.prepareBlock(body, file, imports, scope, map[string]handlerBinding{}, receiverEnv, receiverType, state) || changed
			return false
		default:
			return true
		}
	})
	return changed
}

func (d *GinDetector) prepareBlock(block *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, inheritedScope map[string]ginContext, inheritedHandlers map[string]handlerBinding, inheritedReceivers map[string]string, receiverType string, state *ginPackageState) bool {
	scope := cloneGinScope(inheritedScope)
	handlerEnv := cloneHandlerBindings(inheritedHandlers)
	receiverEnv := cloneReceiverAliases(inheritedReceivers)
	changed := false

	statementList := statementListNode(block)
	for i := 0; i < int(statementList.NamedChildCount()); i++ {
		stmt := statementList.NamedChild(uint(i))
		if stmt == nil {
			continue
		}

		if stmt.Kind() == "block" {
			changed = d.prepareBlock(stmt, file, imports, scope, handlerEnv, receiverEnv, receiverType, state) || changed
			continue
		}

		d.bindReceiverAliases(stmt, file.Content, receiverEnv)
		changed = d.bindGinContext(stmt, file.Content, scope, receiverEnv, state) || changed
		changed = d.bindHandlerBindings(stmt, file, imports, handlerEnv, receiverEnv, state) || changed
		changed = d.captureCompositeLiteralContexts(stmt, file.Content, scope, receiverEnv, state) || changed
		changed = d.captureProviderReturn(stmt, file.Content, scope, receiverEnv, receiverType, state) || changed
		changed = d.captureHandlerProviderReturn(stmt, file, imports, handlerEnv, receiverEnv, receiverType, state) || changed

		for _, child := range nestedBlocks(stmt) {
			changed = d.prepareBlock(child, file, imports, scope, handlerEnv, receiverEnv, receiverType, state) || changed
		}
	}

	return changed
}

func (d *GinDetector) processBlock(block *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, imports map[string]string, inheritedScope map[string]ginContext, inheritedHandlers map[string]handlerBinding, inheritedReceivers map[string]string, state *ginPackageState, helperDepth int, helperStack map[string]bool) ([]boundaryroot.Root, []symbol.Diagnostic) {
	scope := cloneGinScope(inheritedScope)
	handlerEnv := cloneHandlerBindings(inheritedHandlers)
	receiverEnv := cloneReceiverAliases(inheritedReceivers)
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	statementList := statementListNode(block)
	for i := 0; i < int(statementList.NamedChildCount()); i++ {
		stmt := statementList.NamedChild(uint(i))
		if stmt == nil {
			continue
		}

		if stmt.Kind() == "block" {
			nestedRoots, nestedDiags := d.processBlock(stmt, file, symbols, imports, scope, handlerEnv, receiverEnv, state, helperDepth, helperStack)
			roots = append(roots, nestedRoots...)
			diags = append(diags, nestedDiags...)
			continue
		}

		d.bindReceiverAliases(stmt, file.Content, receiverEnv)
		d.bindGinContext(stmt, file.Content, scope, receiverEnv, state)
		d.bindHandlerBindings(stmt, file, imports, handlerEnv, receiverEnv, state)

		if call := callExpressionFromStatement(stmt); call != nil {
			root, diag := d.handleRouteCall(call, file, scope, handlerEnv, receiverEnv, state, symbols)
			if root != nil {
				roots = append(roots, *root)
			}
			if diag.Category != "" {
				diags = append(diags, diag)
			}
			if helperDepth < ginHelperExpansionMaxDepth {
				helperRoots, helperDiags := d.expandHelperCall(call, file, imports, scope, handlerEnv, receiverEnv, state, helperDepth, helperStack)
				roots = append(roots, helperRoots...)
				diags = append(diags, helperDiags...)
			}
		}

		for _, child := range nestedBlocks(stmt) {
			nestedRoots, nestedDiags := d.processBlock(child, file, symbols, imports, scope, handlerEnv, receiverEnv, state, helperDepth, helperStack)
			roots = append(roots, nestedRoots...)
			diags = append(diags, nestedDiags...)
		}
	}

	return roots, diags
}

func (d *GinDetector) bindReceiverAliases(stmt *tree_sitter.Node, content []byte, receiverEnv map[string]string) {
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
			lhs := leftItems[i]
			rhs := rightItems[i]
			name := identifierName(lhs, content)
			if name == "" {
				continue
			}
			receiverType, ok := resolveReceiverType(rhs, content, receiverEnv)
			if ok {
				receiverEnv[name] = receiverType
				continue
			}
			delete(receiverEnv, name)
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
				receiverType, ok := resolveReceiverType(valueItems[j], content, receiverEnv)
				if ok {
					receiverEnv[name] = receiverType
					continue
				}
				delete(receiverEnv, name)
			}
		}
	case "declaration_statement":
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			child := stmt.NamedChild(uint(i))
			if child != nil && child.Kind() == "var_declaration" {
				d.bindReceiverAliases(child, content, receiverEnv)
			}
		}
	}
}

func (d *GinDetector) bindGinContext(stmt *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, state *ginPackageState) bool {
	switch stmt.Kind() {
	case "short_var_declaration", "short_variable_declaration", "assignment_statement":
		return d.bindAssignment(stmt, content, scope, receiverEnv, state)
	case "var_declaration":
		changed := false
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			spec := stmt.NamedChild(uint(i))
			if spec != nil && spec.Kind() == "var_spec" {
				changed = d.bindVarSpec(spec, content, scope, receiverEnv, state) || changed
			}
		}
		return changed
	case "declaration_statement":
		changed := false
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			child := stmt.NamedChild(uint(i))
			if child != nil && child.Kind() == "var_declaration" {
				changed = d.bindGinContext(child, content, scope, receiverEnv, state) || changed
			}
		}
		return changed
	default:
		return false
	}
}

func (d *GinDetector) bindAssignment(stmt *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, state *ginPackageState) bool {
	left := stmt.ChildByFieldName("left")
	right := stmt.ChildByFieldName("right")
	if (left == nil || right == nil) && stmt.NamedChildCount() >= 2 {
		left = stmt.NamedChild(0)
		right = stmt.NamedChild(1)
	}
	if left == nil || right == nil {
		return false
	}

	changed := false
	leftItems := expressionItems(left)
	rightItems := expressionItems(right)
	count := minInt(len(leftItems), len(rightItems))
	for i := 0; i < count; i++ {
		lhs := leftItems[i]
		rhs := rightItems[i]

		if name := identifierName(lhs, content); name != "" {
			ctx, ok := d.resolveAssignedContext(rhs, content, scope, receiverEnv, state)
			if ok {
				scope[name] = ctx
				continue
			}
			delete(scope, name)
			continue
		}

		fieldName, receiverType, ok := receiverField(lhs, content, receiverEnv)
		if !ok {
			continue
		}
		ctx, resolved := d.resolveAssignedContext(rhs, content, scope, receiverEnv, state)
		if !resolved {
			continue
		}
		if setFieldContext(state, receiverType, fieldName, ctx) {
			changed = true
		}
	}

	return changed
}

func (d *GinDetector) bindVarSpec(spec *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, state *ginPackageState) bool {
	nameList := spec.ChildByFieldName("name")
	valueList := spec.ChildByFieldName("value")
	if nameList == nil || valueList == nil {
		return false
	}

	changed := false
	nameItems := expressionItems(nameList)
	valueItems := expressionItems(valueList)
	count := minInt(len(nameItems), len(valueItems))
	for i := 0; i < count; i++ {
		name := identifierName(nameItems[i], content)
		if name == "" {
			continue
		}
		ctx, ok := d.resolveAssignedContext(valueItems[i], content, scope, receiverEnv, state)
		if ok {
			scope[name] = ctx
			continue
		}
		delete(scope, name)
	}
	return changed
}

func (d *GinDetector) captureCompositeLiteralContexts(stmt *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, state *ginPackageState) bool {
	changed := false
	for _, literal := range compositeLiterals(stmt) {
		typeName := compositeLiteralTypeName(literal, content)
		if typeName == "" {
			continue
		}
		walk(literal, func(current *tree_sitter.Node) bool {
			switch current.Kind() {
			case "func_literal":
				return false
			case "composite_literal":
				return current == literal
			case "keyed_element":
				keyNode, valueNode := keyedElementParts(current)
				if keyNode == nil || valueNode == nil {
					return false
				}
				key := fieldNameFromNode(keyNode, content)
				if key == "" {
					return false
				}
				ctx, ok := d.resolvePreparedContextExpression(valueNode, content, scope, receiverEnv, state, 0)
				if !ok {
					return false
				}
				if setFieldContext(state, typeName, key, ctx) {
					changed = true
				}
				return false
			default:
				return true
			}
		})
	}
	return changed
}

func (d *GinDetector) captureProviderReturn(stmt *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, receiverType string, state *ginPackageState) bool {
	if stmt == nil || stmt.Kind() != "return_statement" || stmt.NamedChildCount() == 0 {
		return false
	}

	expr := stmt.NamedChild(0)
	if expr != nil && expr.Kind() == "expression_list" && expr.NamedChildCount() > 0 {
		expr = expr.NamedChild(0)
	}
	ctx, ok := d.resolvePreparedContextExpression(expr, content, scope, receiverEnv, state, 0)
	if !ok {
		return false
	}

	owner := enclosingCallable(stmt)
	if owner == nil {
		return false
	}

	name := callableName(owner, content)
	if name == "" {
		return false
	}

	if owner.Kind() == "method_declaration" && receiverType != "" {
		return setMethodProvider(state, receiverType, name, ctx)
	}
	return setFunctionProvider(state, name, ctx)
}

func (d *GinDetector) resolveAssignedContext(expr *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, state *ginPackageState) (ginContext, bool) {
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

	ctx, ok := d.resolvePreparedContextExpression(expr, content, scope, receiverEnv, state, 0)
	if !ok {
		return ginContext{}, false
	}
	ctx.aliasDepth = 0
	return ctx, true
}

func (d *GinDetector) resolvePreparedContextExpression(expr *tree_sitter.Node, content []byte, scope map[string]ginContext, receiverEnv map[string]string, state *ginPackageState, providerDepth int) (ginContext, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return ginContext{}, false
	}

	switch expr.Kind() {
	case "identifier":
		ctx, ok := scope[nodeText(expr, content)]
		return ctx, ok
	case "selector_expression":
		fieldName, receiverType, ok := receiverField(expr, content, receiverEnv)
		if !ok || state == nil {
			return ginContext{}, false
		}
		ctx, ok := state.fieldContext(receiverType, fieldName)
		return ctx, ok
	case "call_expression":
		fn := expr.ChildByFieldName("function")
		args := expr.ChildByFieldName("arguments")
		switch {
		case fn == nil:
			return ginContext{}, false
		case fn.Kind() == "identifier":
			if args != nil && args.NamedChildCount() > 0 {
				return ginContext{}, false
			}
			if providerDepth >= 1 || state == nil {
				return ginContext{}, false
			}
			ctx, ok := state.functions[nodeText(fn, content)]
			return ctx, ok
		case fn.Kind() != "selector_expression":
			return ginContext{}, false
		}

		field := fn.ChildByFieldName("field")
		operand := fn.ChildByFieldName("operand")
		if field == nil {
			return ginContext{}, false
		}

		methodName := nodeText(field, content)
		switch methodName {
		case "New", "Default":
			if operand != nil && nodeText(operand, content) == "gin" {
				return ginContext{prefix: ""}, true
			}
			return ginContext{}, false
		case "Group":
			parent, ok := d.resolvePreparedContextExpression(operand, content, scope, receiverEnv, state, providerDepth)
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
		default:
			if providerDepth >= 1 || state == nil || args == nil || args.NamedChildCount() != 0 {
				return ginContext{}, false
			}
			receiverType, ok := resolveReceiverType(operand, content, receiverEnv)
			if !ok {
				return ginContext{}, false
			}
			ctx, ok := state.methodProvider(receiverType, methodName)
			return ctx, ok
		}
	default:
		return ginContext{}, false
	}
}

func (d *GinDetector) handleRouteCall(call *tree_sitter.Node, file boundary.ParsedGoFile, scope map[string]ginContext, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState, symbols []symbol.Symbol) (*boundaryroot.Root, symbol.Diagnostic) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Kind() != "selector_expression" {
		return nil, symbol.Diagnostic{}
	}

	field := fn.ChildByFieldName("field")
	if field == nil {
		return nil, symbol.Diagnostic{}
	}

	method := nodeText(field, file.Content)
	if !ginRouteMethods[method] {
		return nil, symbol.Diagnostic{}
	}

	receiver := fn.ChildByFieldName("operand")
	ctx, ok := d.resolvePreparedContextExpression(receiver, file.Content, scope, receiverEnv, state, 0)
	if !ok {
		category := "boundary_unproven_receiver"
		if receiver != nil && receiver.Kind() == "call_expression" {
			category = "boundary_unsupported_accessor_pattern"
		}
		return nil, boundaryDiagnostic(category, file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because receiver provenance was not proven", method))
	}

	args := call.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil, boundaryDiagnostic("boundary_insufficient_args", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because it did not provide path and handler arguments", method))
	}

	pathArg := args.NamedChild(0)
	if !isStringLiteral(pathArg) {
		return nil, boundaryDiagnostic("boundary_nonliteral_path", file.Path, nodeText(call, file.Content), fmt.Sprintf("rejected %s route registration because the path is not a string literal", method))
	}

	fullPath := cleanPath(ctx.prefix + "/" + getStringValue(pathArg, file.Content))
	handlerArg := args.NamedChild(args.NamedChildCount() - 1)
	handlerTarget, handlerTargetKind := d.resolveGinHandlerTarget(handlerArg, file, symbols, handlerEnv, receiverEnv, state)

	root := boundaryroot.Root{
		Kind:              boundaryroot.KindHTTP,
		Framework:         "gin",
		Method:            method,
		Path:              fullPath,
		CanonicalName:     fmt.Sprintf("%s %s", method, fullPath),
		HandlerTarget:     handlerTarget,
		HandlerTargetKind: handlerTargetKind,
		RepositoryID:      file.RepositoryID,
		SourceFile:        file.Path,
		SourceStartByte:   uint32(call.StartByte()),
		SourceEndByte:     uint32(call.EndByte()),
		SourceExpr:        nodeText(call, file.Content),
		Confidence:        "high",
	}
	root.ID = boundaryroot.StableID(root)

	if handlerTarget == "" {
		return &root, boundaryDiagnostic("boundary_unresolved_handler_target", file.Path, root.SourceExpr, fmt.Sprintf("route %s has no exact handler target", root.CanonicalName))
	}
	return &root, symbol.Diagnostic{}
}

func (d *GinDetector) expandHelperCall(call *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, scope map[string]ginContext, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState, helperDepth int, helperStack map[string]bool) ([]boundaryroot.Root, []symbol.Diagnostic) {
	callable, ok := d.resolveHelperCallable(call, file, handlerEnv, receiverEnv, state)
	if !ok {
		return nil, nil
	}
	body := callable.node.ChildByFieldName("body")
	if body == nil || body.Kind() != "block" {
		return nil, nil
	}

	args := call.ChildByFieldName("arguments")
	if args == nil {
		return nil, nil
	}

	calleeImports := fileImportAliases(callable.file)
	ginOverrides := map[string]ginContext{}
	handlerOverrides := map[string]handlerBinding{}
	receiverOverrides := map[string]string{}
	hasGinBinding := false
	ordinaryParams := ordinaryDeclarationParameters(callable.node)
	for i, param := range ordinaryParams {
		if i >= int(args.NamedChildCount()) {
			break
		}
		name := parameterName(param, callable.file.Content)
		if name == "" {
			continue
		}
		arg := args.NamedChild(uint(i))
		if arg == nil {
			continue
		}
		if ctx, ok := d.resolvePreparedContextExpression(arg, file.Content, scope, receiverEnv, state, 0); ok && isGinRouterType(parameterTypeNode(param), callable.file.Content, calleeImports) {
			ginOverrides[name] = ginContext{prefix: ctx.prefix}
			hasGinBinding = true
		}
		if binding, ok := d.resolveAssignedHandlerBinding(arg, file, imports, handlerEnv, receiverEnv, state); ok {
			handlerOverrides[name] = binding
		}
		if receiverType, ok := resolveReceiverType(arg, file.Content, receiverEnv); ok {
			receiverOverrides[name] = receiverType
		}
	}
	if !hasGinBinding {
		return nil, nil
	}

	calleeScope := initialGinScopeForDeclaration(callable.node, callable.file.Content, calleeImports, ginOverrides)
	calleeReceivers := declarationTypeAliases(callable.node, callable.file.Content)
	for name, receiverType := range receiverOverrides {
		calleeReceivers[name] = receiverType
	}

	frameKey := helperFrameKey(callable, calleeScope, handlerOverrides)
	if helperStack[frameKey] {
		return nil, nil
	}
	nextStack := map[string]bool{}
	for key, value := range helperStack {
		nextStack[key] = value
	}
	nextStack[frameKey] = true

	return d.processBlock(body, callable.file, nil, calleeImports, calleeScope, handlerOverrides, calleeReceivers, state, helperDepth+1, nextStack)
}

func (d *GinDetector) resolveHelperCallable(call *tree_sitter.Node, file boundary.ParsedGoFile, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) (ginCallable, bool) {
	if state == nil || call == nil {
		return ginCallable{}, false
	}
	fn := call.ChildByFieldName("function")
	if fn == nil {
		return ginCallable{}, false
	}

	switch fn.Kind() {
	case "identifier":
		return state.uniqueFunctionDecl(nodeText(fn, file.Content))
	case "selector_expression":
		field := fn.ChildByFieldName("field")
		operand := fn.ChildByFieldName("operand")
		if field == nil || operand == nil {
			return ginCallable{}, false
		}
		methodName := nodeText(field, file.Content)
		if receiverType, ok := ginLocalReceiverType(operand, file.Content, file.PackageName, handlerEnv, receiverEnv); ok {
			return state.uniqueMethodDecl(receiverType, methodName)
		}
	}
	return ginCallable{}, false
}

func helperFrameKey(callable ginCallable, scope map[string]ginContext, handlerEnv map[string]handlerBinding) string {
	parts := []string{callable.file.Path, callable.receiverType, callableName(callable.node, callable.file.Content)}
	scopeNames := make([]string, 0, len(scope))
	for name := range scope {
		scopeNames = append(scopeNames, name)
	}
	sort.Strings(scopeNames)
	for _, name := range scopeNames {
		parts = append(parts, "ctx:"+name+"="+scope[name].prefix)
	}
	handlerNames := make([]string, 0, len(handlerEnv))
	for name := range handlerEnv {
		handlerNames = append(handlerNames, name)
	}
	sort.Strings(handlerNames)
	for _, name := range handlerNames {
		binding := handlerEnv[name]
		parts = append(parts, "handler:"+name+"="+binding.packageToken+"."+binding.receiverType)
	}
	return strings.Join(parts, "|")
}

func ordinaryDeclarationParameters(node *tree_sitter.Node) []*tree_sitter.Node {
	if node == nil {
		return nil
	}
	parameters := node.ChildByFieldName("parameters")
	if parameters == nil {
		return nil
	}
	var out []*tree_sitter.Node
	for i := 0; i < int(parameters.NamedChildCount()); i++ {
		child := parameters.NamedChild(uint(i))
		if child != nil && child.Kind() == "parameter_declaration" {
			out = append(out, child)
		}
	}
	return out
}

func resolveHandlerTarget(handlerArg *tree_sitter.Node, content []byte, symbols []symbol.Symbol) string {
	if handlerArg == nil {
		return ""
	}

	for _, sym := range symbols {
		if sym.Location.StartLine == uint32(handlerArg.StartPosition().Row+1) &&
			sym.Location.StartCol == uint32(handlerArg.StartPosition().Column+1) {
			return string(sym.ID)
		}
	}

	handlerArg = unwrapParens(handlerArg)
	if handlerArg == nil {
		return ""
	}

	switch handlerArg.Kind() {
	case "identifier", "selector_expression":
		return nodeText(handlerArg, content)
	case "call_expression":
		fn := handlerArg.ChildByFieldName("function")
		if fn != nil {
			switch fn.Kind() {
			case "selector_expression", "identifier":
				return nodeText(fn, content)
			}
		}
	case "func_literal":
		return nodeText(handlerArg, content)
	}

	return nodeText(handlerArg, content)
}

func (d *GinDetector) packageKey(file boundary.ParsedGoFile) string {
	dir := filepath.ToSlash(filepath.Dir(file.Path))
	if dir == "." {
		dir = ""
	}
	return strings.Join([]string{file.RepositoryID, dir, file.PackageName}, "|")
}

func newGinPackageState() *ginPackageState {
	return &ginPackageState{
		fieldContexts:          map[string]map[string]ginContext{},
		methodProviders:        map[string]map[string]ginContext{},
		functions:              map[string]ginContext{},
		handlerFunctions:       map[string]handlerBinding{},
		handlerMethodProviders: map[string]map[string]handlerBinding{},
		packageFunctions:       map[string][]symbol.Symbol{},
		packageMethods:         map[string]map[string][]symbol.Symbol{},
		functionDecls:          map[string][]ginCallable{},
		methodDecls:            map[string]map[string][]ginCallable{},
	}
}

func filePackageToken(file boundary.ParsedGoFile) string {
	return normalizeTypeName(file.PackageName)
}

func indexGinPackageCallables(state *ginPackageState, files []boundary.ParsedGoFile) {
	if state == nil {
		return
	}
	for _, file := range files {
		walk(file.Root, func(node *tree_sitter.Node) bool {
			switch node.Kind() {
			case "function_declaration", "method_declaration":
				name := callableName(node, file.Content)
				if name == "" {
					return false
				}
				callable := ginCallable{
					file:         file,
					node:         node,
					receiverType: declarationReceiverType(node, file.Content),
				}
				if node.Kind() == "method_declaration" && callable.receiverType != "" {
					if state.methodDecls[callable.receiverType] == nil {
						state.methodDecls[callable.receiverType] = map[string][]ginCallable{}
					}
					state.methodDecls[callable.receiverType][name] = append(state.methodDecls[callable.receiverType][name], callable)
					return false
				}
				state.functionDecls[name] = append(state.functionDecls[name], callable)
				return false
			default:
				return true
			}
		})
	}
}

func (s *ginPackageState) uniqueFunctionDecl(name string) (ginCallable, bool) {
	if s == nil {
		return ginCallable{}, false
	}
	candidates := s.functionDecls[name]
	if len(candidates) != 1 {
		return ginCallable{}, false
	}
	return candidates[0], true
}

func (s *ginPackageState) uniqueMethodDecl(receiverType, methodName string) (ginCallable, bool) {
	if s == nil {
		return ginCallable{}, false
	}
	methods := s.methodDecls[receiverType]
	if methods == nil {
		return ginCallable{}, false
	}
	candidates := methods[methodName]
	if len(candidates) != 1 {
		return ginCallable{}, false
	}
	return candidates[0], true
}

func (s *ginPackageState) fieldContext(receiverType, fieldName string) (ginContext, bool) {
	fields := s.fieldContexts[receiverType]
	if fields == nil {
		return ginContext{}, false
	}
	ctx, ok := fields[fieldName]
	return ctx, ok
}

func (s *ginPackageState) methodProvider(receiverType, methodName string) (ginContext, bool) {
	methods := s.methodProviders[receiverType]
	if methods == nil {
		return ginContext{}, false
	}
	ctx, ok := methods[methodName]
	return ctx, ok
}

func setFieldContext(state *ginPackageState, receiverType, fieldName string, ctx ginContext) bool {
	if receiverType == "" || fieldName == "" {
		return false
	}
	if state.fieldContexts[receiverType] == nil {
		state.fieldContexts[receiverType] = map[string]ginContext{}
	}
	existing, ok := state.fieldContexts[receiverType][fieldName]
	if ok && existing.prefix == ctx.prefix {
		return false
	}
	state.fieldContexts[receiverType][fieldName] = ginContext{prefix: ctx.prefix}
	return true
}

func setMethodProvider(state *ginPackageState, receiverType, methodName string, ctx ginContext) bool {
	if receiverType == "" || methodName == "" {
		return false
	}
	if state.methodProviders[receiverType] == nil {
		state.methodProviders[receiverType] = map[string]ginContext{}
	}
	existing, ok := state.methodProviders[receiverType][methodName]
	if ok && existing.prefix == ctx.prefix {
		return false
	}
	state.methodProviders[receiverType][methodName] = ginContext{prefix: ctx.prefix}
	return true
}

func setFunctionProvider(state *ginPackageState, functionName string, ctx ginContext) bool {
	if functionName == "" {
		return false
	}
	existing, ok := state.functions[functionName]
	if ok && existing.prefix == ctx.prefix {
		return false
	}
	state.functions[functionName] = ginContext{prefix: ctx.prefix}
	return true
}

func boundaryDiagnostic(category, filePath, evidence, message string) symbol.Diagnostic {
	return symbol.Diagnostic{
		Category: category,
		FilePath: filePath,
		Evidence: evidence,
		Message:  message,
	}
}

func sortGinDiagnostics(diags []symbol.Diagnostic) []symbol.Diagnostic {
	seen := map[string]symbol.Diagnostic{}
	for _, diag := range diags {
		key := strings.Join([]string{diag.FilePath, diag.Category, diag.Message, diag.Evidence}, "|")
		seen[key] = diag
	}

	out := make([]symbol.Diagnostic, 0, len(seen))
	for _, diag := range seen {
		out = append(out, diag)
	}
	sort.Slice(out, func(i, j int) bool {
		leftKey := strings.Join([]string{out[i].FilePath, out[i].Category, out[i].Message, out[i].Evidence}, "|")
		rightKey := strings.Join([]string{out[j].FilePath, out[j].Category, out[j].Message, out[j].Evidence}, "|")
		return leftKey < rightKey
	})
	return out
}

func dedupeGinRoots(roots []boundaryroot.Root) []boundaryroot.Root {
	seen := map[string]boundaryroot.Root{}
	for _, root := range roots {
		if root.ID == "" {
			root.ID = boundaryroot.StableID(root)
		}
		seen[root.ID] = root
	}
	out := make([]boundaryroot.Root, 0, len(seen))
	for _, root := range seen {
		out = append(out, root)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CanonicalName != out[j].CanonicalName {
			return out[i].CanonicalName < out[j].CanonicalName
		}
		if out[i].SourceFile != out[j].SourceFile {
			return out[i].SourceFile < out[j].SourceFile
		}
		if out[i].SourceStartByte != out[j].SourceStartByte {
			return out[i].SourceStartByte < out[j].SourceStartByte
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func cloneGinScope(scope map[string]ginContext) map[string]ginContext {
	cloned := make(map[string]ginContext, len(scope))
	for name, ctx := range scope {
		cloned[name] = ctx
	}
	return cloned
}

func cloneReceiverAliases(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneHandlerBindings(in map[string]handlerBinding) map[string]handlerBinding {
	out := make(map[string]handlerBinding, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func statementListNode(block *tree_sitter.Node) *tree_sitter.Node {
	if block == nil {
		return nil
	}
	if block.NamedChildCount() == 1 {
		if child := block.NamedChild(0); child != nil && child.Kind() == "statement_list" {
			return child
		}
	}
	return block
}

func nestedBlocks(node *tree_sitter.Node) []*tree_sitter.Node {
	var blocks []*tree_sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "block":
			blocks = append(blocks, child)
		case "func_literal":
			continue
		default:
			blocks = append(blocks, nestedBlocks(child)...)
		}
	}
	return blocks
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

func declarationTypeAliases(node *tree_sitter.Node, content []byte) map[string]string {
	aliases := map[string]string{}
	for _, param := range declarationParameters(node) {
		name := parameterName(param, content)
		typeNode := parameterTypeNode(param)
		if name == "" || typeNode == nil {
			continue
		}
		aliases[name] = normalizeTypeName(nodeText(typeNode, content))
	}
	return aliases
}

func declarationHasGinContextParams(node *tree_sitter.Node, content []byte, imports map[string]string) bool {
	for _, param := range declarationParameters(node) {
		if typeNode := parameterTypeNode(param); isGinRouterType(typeNode, content, imports) {
			return true
		}
	}
	return false
}

func initialGinScopeForDeclaration(node *tree_sitter.Node, content []byte, imports map[string]string, overrides map[string]ginContext) map[string]ginContext {
	scope := map[string]ginContext{}
	for key, value := range overrides {
		scope[key] = value
	}
	for _, param := range declarationParameters(node) {
		name := parameterName(param, content)
		if name == "" {
			continue
		}
		typeNode := parameterTypeNode(param)
		if !isGinRouterType(typeNode, content, imports) {
			continue
		}
		if _, ok := scope[name]; ok {
			continue
		}
		scope[name] = ginContext{prefix: ""}
	}
	return scope
}

func receiverAliasesForDeclaration(node *tree_sitter.Node, content []byte) map[string]string {
	aliases := map[string]string{}
	if node == nil || node.Kind() != "method_declaration" {
		return aliases
	}

	receiver := node.ChildByFieldName("receiver")
	if receiver == nil {
		return aliases
	}
	for i := 0; i < int(receiver.NamedChildCount()); i++ {
		param := receiver.NamedChild(uint(i))
		if param == nil || param.Kind() != "parameter_declaration" {
			continue
		}
		name := ""
		if nameNode := param.ChildByFieldName("name"); nameNode != nil {
			name = nodeText(nameNode, content)
		} else if param.NamedChildCount() > 0 {
			name = identifierName(param.NamedChild(0), content)
		}
		typeNode := param.ChildByFieldName("type")
		if typeNode == nil && param.NamedChildCount() > 0 {
			typeNode = param.NamedChild(uint(param.NamedChildCount() - 1))
		}
		if name != "" && typeNode != nil {
			aliases[name] = normalizeTypeName(nodeText(typeNode, content))
		}
	}
	return aliases
}

func declarationParameters(node *tree_sitter.Node) []*tree_sitter.Node {
	var params []*tree_sitter.Node
	if node == nil {
		return params
	}
	appendParams := func(list *tree_sitter.Node) {
		if list == nil {
			return
		}
		for i := 0; i < int(list.NamedChildCount()); i++ {
			child := list.NamedChild(uint(i))
			if child != nil && child.Kind() == "parameter_declaration" {
				params = append(params, child)
			}
		}
	}
	if receiver := node.ChildByFieldName("receiver"); receiver != nil {
		appendParams(receiver)
	}
	if parameters := node.ChildByFieldName("parameters"); parameters != nil {
		appendParams(parameters)
	}
	return params
}

func parameterName(param *tree_sitter.Node, content []byte) string {
	if param == nil {
		return ""
	}
	if nameNode := param.ChildByFieldName("name"); nameNode != nil {
		return nodeText(nameNode, content)
	}
	for i := 0; i < int(param.NamedChildCount()); i++ {
		child := param.NamedChild(uint(i))
		if child == nil {
			continue
		}
		if name := identifierName(child, content); name != "" && child.Kind() != "selector_expression" {
			return name
		}
	}
	return ""
}

func parameterTypeNode(param *tree_sitter.Node) *tree_sitter.Node {
	if param == nil {
		return nil
	}
	if typeNode := param.ChildByFieldName("type"); typeNode != nil {
		return typeNode
	}
	if param.NamedChildCount() > 0 {
		return param.NamedChild(uint(param.NamedChildCount() - 1))
	}
	return nil
}

func isGinRouterType(typeNode *tree_sitter.Node, content []byte, imports map[string]string) bool {
	if typeNode == nil {
		return false
	}
	raw := strings.TrimSpace(nodeText(typeNode, content))
	raw = strings.TrimPrefix(raw, "*")
	raw = strings.TrimPrefix(raw, "[]")
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[:idx]
	}
	if raw == "" || !strings.Contains(raw, ".") {
		return false
	}
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 {
		return false
	}
	if imports[parts[0]] != "github.com/gin-gonic/gin" {
		return false
	}
	switch parts[1] {
	case "Engine", "RouterGroup", "IRouter", "IRoutes":
		return true
	default:
		return false
	}
}

func declarationReceiverType(node *tree_sitter.Node, content []byte) string {
	for _, receiverType := range receiverAliasesForDeclaration(node, content) {
		return receiverType
	}
	return ""
}

func resolveReceiverType(expr *tree_sitter.Node, content []byte, receiverEnv map[string]string) (string, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return "", false
	}

	switch expr.Kind() {
	case "identifier":
		receiverType, ok := receiverEnv[nodeText(expr, content)]
		return receiverType, ok
	default:
		return "", false
	}
}

func receiverField(expr *tree_sitter.Node, content []byte, receiverEnv map[string]string) (fieldName string, receiverType string, ok bool) {
	expr = unwrapParens(expr)
	if expr == nil || expr.Kind() != "selector_expression" {
		return "", "", false
	}

	field := expr.ChildByFieldName("field")
	operand := expr.ChildByFieldName("operand")
	if field == nil || operand == nil {
		return "", "", false
	}

	receiverType, ok = resolveReceiverType(operand, content, receiverEnv)
	if !ok {
		return "", "", false
	}
	return nodeText(field, content), receiverType, true
}

func enclosingCallable(node *tree_sitter.Node) *tree_sitter.Node {
	for current := node; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "function_declaration", "method_declaration":
			return current
		}
	}
	return nil
}

func callableName(node *tree_sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	name := node.ChildByFieldName("name")
	if name == nil {
		return ""
	}
	return nodeText(name, content)
}

func compositeLiterals(node *tree_sitter.Node) []*tree_sitter.Node {
	var out []*tree_sitter.Node
	walk(node, func(current *tree_sitter.Node) bool {
		switch current.Kind() {
		case "func_literal":
			return false
		case "composite_literal":
			out = append(out, current)
			return false
		default:
			return true
		}
	})
	return out
}

func compositeLiteralTypeName(node *tree_sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		return normalizeTypeName(nodeText(typeNode, content))
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		if child != nil && child.Kind() != "literal_value" {
			return normalizeTypeName(nodeText(child, content))
		}
	}
	return ""
}

func literalValueNode(node *tree_sitter.Node) *tree_sitter.Node {
	if node == nil {
		return nil
	}
	if body := node.ChildByFieldName("body"); body != nil {
		return body
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		if child != nil && child.Kind() == "literal_value" {
			return child
		}
	}
	return nil
}

func normalizeTypeName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "*")
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[:idx]
	}
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		raw = raw[idx+1:]
	}
	return raw
}

func fieldNameFromNode(node *tree_sitter.Node, content []byte) string {
	node = unwrapParens(node)
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier", "field_identifier":
		return nodeText(node, content)
	default:
		return strings.TrimSpace(nodeText(node, content))
	}
}

func keyedElementParts(node *tree_sitter.Node) (*tree_sitter.Node, *tree_sitter.Node) {
	if node == nil || node.Kind() != "keyed_element" {
		return nil, nil
	}
	keyNode := node.ChildByFieldName("key")
	valueNode := node.ChildByFieldName("value")
	if keyNode != nil && valueNode != nil {
		return keyNode, valueNode
	}
	if node.NamedChildCount() >= 2 {
		return node.NamedChild(0), node.NamedChild(1)
	}
	return nil, nil
}

func unwrapParens(n *tree_sitter.Node) *tree_sitter.Node {
	for n != nil {
		switch {
		case n.Kind() == "parenthesized_expression" && n.NamedChildCount() == 1:
			n = n.NamedChild(0)
		case n.Kind() == "literal_element" && n.NamedChildCount() == 1:
			n = n.NamedChild(0)
		case n.Kind() == "expression_list" && n.NamedChildCount() == 1:
			n = n.NamedChild(0)
		default:
			return n
		}
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

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func expressionItems(node *tree_sitter.Node) []*tree_sitter.Node {
	if node == nil {
		return nil
	}
	if node.Kind() == "expression_list" {
		items := make([]*tree_sitter.Node, 0, node.NamedChildCount())
		for i := 0; i < int(node.NamedChildCount()); i++ {
			items = append(items, node.NamedChild(uint(i)))
		}
		return items
	}
	return []*tree_sitter.Node{node}
}

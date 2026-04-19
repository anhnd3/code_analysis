package frameworks

import (
	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/domain/targetref"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func indexGinPackageSymbols(state *ginPackageState, symbols []symbol.Symbol) {
	if state == nil {
		return
	}
	for _, sym := range symbols {
		if !isGinExecutableSymbol(sym) {
			continue
		}
		if sym.Receiver == "" {
			state.packageFunctions[sym.Name] = append(state.packageFunctions[sym.Name], sym)
			continue
		}
		if state.packageMethods[sym.Receiver] == nil {
			state.packageMethods[sym.Receiver] = map[string][]symbol.Symbol{}
		}
		state.packageMethods[sym.Receiver][sym.Name] = append(state.packageMethods[sym.Receiver][sym.Name], sym)
	}
}

func isGinExecutableSymbol(sym symbol.Symbol) bool {
	switch sym.Kind {
	case symbol.KindFunction, symbol.KindMethod, symbol.KindRouteHandler, symbol.KindGRPCHandler:
		return true
	default:
		return false
	}
}

func (d *GinDetector) bindHandlerBindings(stmt *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) bool {
	switch stmt.Kind() {
	case "short_var_declaration", "short_variable_declaration", "assignment_statement":
		return d.bindHandlerAssignment(stmt, file, imports, handlerEnv, receiverEnv, state)
	case "var_declaration":
		changed := false
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			spec := stmt.NamedChild(uint(i))
			if spec != nil && spec.Kind() == "var_spec" {
				changed = d.bindHandlerVarSpec(spec, file, imports, handlerEnv, receiverEnv, state) || changed
			}
		}
		return changed
	case "declaration_statement":
		changed := false
		for i := 0; i < int(stmt.NamedChildCount()); i++ {
			child := stmt.NamedChild(uint(i))
			if child != nil && child.Kind() == "var_declaration" {
				changed = d.bindHandlerBindings(child, file, imports, handlerEnv, receiverEnv, state) || changed
			}
		}
		return changed
	default:
		return false
	}
}

func (d *GinDetector) bindHandlerAssignment(stmt *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) bool {
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
		name := identifierName(leftItems[i], file.Content)
		if name == "" {
			continue
		}
		binding, ok := d.resolveAssignedHandlerBinding(rightItems[i], file, imports, handlerEnv, receiverEnv, state)
		if ok {
			existing, exists := handlerEnv[name]
			handlerEnv[name] = binding
			if !exists || existing != binding {
				changed = true
			}
			continue
		}
		if _, exists := handlerEnv[name]; exists {
			delete(handlerEnv, name)
			changed = true
		}
	}
	return changed
}

func (d *GinDetector) bindHandlerVarSpec(spec *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) bool {
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
		name := identifierName(nameItems[i], file.Content)
		if name == "" {
			continue
		}
		binding, ok := d.resolveAssignedHandlerBinding(valueItems[i], file, imports, handlerEnv, receiverEnv, state)
		if ok {
			existing, exists := handlerEnv[name]
			handlerEnv[name] = binding
			if !exists || existing != binding {
				changed = true
			}
			continue
		}
		if _, exists := handlerEnv[name]; exists {
			delete(handlerEnv, name)
			changed = true
		}
	}
	return changed
}

func (d *GinDetector) captureHandlerProviderReturn(stmt *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, receiverType string, state *ginPackageState) bool {
	if stmt == nil || stmt.Kind() != "return_statement" || stmt.NamedChildCount() == 0 {
		return false
	}

	expr := stmt.NamedChild(0)
	if expr != nil && expr.Kind() == "expression_list" && expr.NamedChildCount() > 0 {
		expr = expr.NamedChild(0)
	}
	binding, ok := d.resolvePreparedHandlerBindingExpression(expr, file, imports, handlerEnv, receiverEnv, state, 1)
	if !ok {
		return false
	}

	owner := enclosingCallable(stmt)
	if owner == nil {
		return false
	}
	name := callableName(owner, file.Content)
	if name == "" {
		return false
	}
	if owner.Kind() == "method_declaration" && receiverType != "" {
		return setHandlerMethodProvider(state, receiverType, name, binding)
	}
	return setHandlerFunctionProvider(state, name, binding)
}

func (d *GinDetector) resolveAssignedHandlerBinding(expr *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) (handlerBinding, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return handlerBinding{}, false
	}

	if expr.Kind() == "identifier" {
		source, ok := handlerEnv[nodeText(expr, file.Content)]
		if !ok || source.aliasDepth >= 1 {
			return handlerBinding{}, false
		}
		source.aliasDepth++
		return source, true
	}

	binding, ok := d.resolvePreparedHandlerBindingExpression(expr, file, imports, handlerEnv, receiverEnv, state, 0)
	if !ok {
		return handlerBinding{}, false
	}
	binding.aliasDepth = 0
	return binding, true
}

func (d *GinDetector) resolvePreparedHandlerBindingExpression(expr *tree_sitter.Node, file boundary.ParsedGoFile, imports map[string]string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState, providerDepth int) (handlerBinding, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return handlerBinding{}, false
	}

	switch expr.Kind() {
	case "identifier":
		binding, ok := handlerEnv[nodeText(expr, file.Content)]
		return binding, ok
	case "composite_literal":
		typeName := compositeLiteralTypeName(expr, file.Content)
		if typeName == "" {
			return handlerBinding{}, false
		}
		return handlerBinding{
			packageToken: targetref.NormalizePackageToken(file.PackageName),
			receiverType: typeName,
		}, true
	case "unary_expression":
		if operand := expr.ChildByFieldName("operand"); operand != nil {
			return d.resolvePreparedHandlerBindingExpression(operand, file, imports, handlerEnv, receiverEnv, state, providerDepth)
		}
		if expr.NamedChildCount() > 0 {
			return d.resolvePreparedHandlerBindingExpression(expr.NamedChild(uint(expr.NamedChildCount()-1)), file, imports, handlerEnv, receiverEnv, state, providerDepth)
		}
		return handlerBinding{}, false
	case "call_expression":
		fn := expr.ChildByFieldName("function")
		switch {
		case fn == nil:
			return handlerBinding{}, false
		case fn.Kind() == "identifier":
			if providerDepth >= 1 || state == nil {
				return handlerBinding{}, false
			}
			return state.handlerFunction(nodeText(fn, file.Content))
		case fn.Kind() != "selector_expression":
			return handlerBinding{}, false
		}

		field := fn.ChildByFieldName("field")
		operand := fn.ChildByFieldName("operand")
		if field == nil || operand == nil {
			return handlerBinding{}, false
		}
		methodName := nodeText(field, file.Content)

		if importAliasMatchesFunc(imports, nodeText(operand, file.Content), func(string) bool { return true }) {
			if binding, ok := d.importedHandlerFunction(nodeText(operand, file.Content), methodName); ok {
				return binding, true
			}
			if looksLikeHandlerConstructor(methodName) {
				return handlerBinding{packageToken: targetref.NormalizePackageToken(nodeText(operand, file.Content))}, true
			}
		}

		if providerDepth >= 1 || state == nil {
			return handlerBinding{}, false
		}
		receiverType, ok := resolveReceiverType(operand, file.Content, receiverEnv)
		if !ok {
			return handlerBinding{}, false
		}
		return state.handlerMethodProvider(receiverType, methodName)
	default:
		return handlerBinding{}, false
	}
}

func looksLikeHandlerConstructor(name string) bool {
	for _, prefix := range []string{"New", "Make", "Create", "Build", "Init"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func setHandlerFunctionProvider(state *ginPackageState, name string, binding handlerBinding) bool {
	if state == nil || name == "" || binding.packageToken == "" {
		return false
	}
	existing, ok := state.handlerFunctions[name]
	if ok && existing == binding {
		return false
	}
	state.handlerFunctions[name] = binding
	return true
}

func setHandlerMethodProvider(state *ginPackageState, receiverType, methodName string, binding handlerBinding) bool {
	if state == nil || receiverType == "" || methodName == "" || binding.packageToken == "" {
		return false
	}
	if state.handlerMethodProviders[receiverType] == nil {
		state.handlerMethodProviders[receiverType] = map[string]handlerBinding{}
	}
	existing, ok := state.handlerMethodProviders[receiverType][methodName]
	if ok && existing == binding {
		return false
	}
	state.handlerMethodProviders[receiverType][methodName] = binding
	return true
}

func (s *ginPackageState) handlerFunction(name string) (handlerBinding, bool) {
	if s == nil {
		return handlerBinding{}, false
	}
	binding, ok := s.handlerFunctions[name]
	return binding, ok
}

func (s *ginPackageState) handlerMethodProvider(receiverType, methodName string) (handlerBinding, bool) {
	if s == nil {
		return handlerBinding{}, false
	}
	methods := s.handlerMethodProviders[receiverType]
	if methods == nil {
		return handlerBinding{}, false
	}
	binding, ok := methods[methodName]
	return binding, ok
}

func (d *GinDetector) importedHandlerFunction(packageToken, functionName string) (handlerBinding, bool) {
	packageToken = targetref.NormalizePackageToken(packageToken)
	if packageToken == "" || functionName == "" {
		return handlerBinding{}, false
	}

	var found handlerBinding
	var foundAny bool
	for _, state := range d.packageStates {
		if state == nil || state.packageToken != packageToken {
			continue
		}
		binding, ok := state.handlerFunction(functionName)
		if !ok {
			continue
		}
		if !foundAny {
			found = binding
			foundAny = true
			continue
		}
		if found != binding {
			return handlerBinding{}, false
		}
	}
	return found, foundAny
}

func (s *ginPackageState) uniqueFunction(name string) (symbol.Symbol, bool) {
	if s == nil {
		return symbol.Symbol{}, false
	}
	candidates := s.packageFunctions[name]
	if len(candidates) != 1 {
		return symbol.Symbol{}, false
	}
	return candidates[0], true
}

func (s *ginPackageState) uniqueMethod(receiverType, methodName string) (symbol.Symbol, bool) {
	if s == nil {
		return symbol.Symbol{}, false
	}
	methods := s.packageMethods[receiverType]
	if methods == nil {
		return symbol.Symbol{}, false
	}
	candidates := methods[methodName]
	if len(candidates) != 1 {
		return symbol.Symbol{}, false
	}
	return candidates[0], true
}

func (d *GinDetector) resolveGinHandlerTarget(handlerArg *tree_sitter.Node, file boundary.ParsedGoFile, symbols []symbol.Symbol, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) (string, targetref.Kind) {
	if symbolID, ok := exactSymbolIDAtNode(handlerArg, symbols); ok {
		return symbolID, targetref.KindExactSymbolID
	}

	handlerArg = unwrapParens(handlerArg)
	if handlerArg == nil {
		return "", targetref.KindUnknown
	}

	switch handlerArg.Kind() {
	case "identifier":
		if sym, ok := state.uniqueFunction(nodeText(handlerArg, file.Content)); ok {
			return sym.CanonicalName, targetref.KindExactCanonical
		}
	case "selector_expression":
		return d.resolveGinSelectorTarget(handlerArg, file, handlerEnv, receiverEnv, state)
	case "call_expression":
		fn := handlerArg.ChildByFieldName("function")
		if fn == nil {
			return "", targetref.KindUnknown
		}
		switch fn.Kind() {
		case "identifier":
			if sym, ok := state.uniqueFunction(nodeText(fn, file.Content)); ok {
				return sym.CanonicalName, targetref.KindExactCanonical
			}
		case "selector_expression":
			return d.resolveGinSelectorTarget(fn, file, handlerEnv, receiverEnv, state)
		}
	}
	return "", targetref.KindUnknown
}

func exactSymbolIDAtNode(node *tree_sitter.Node, symbols []symbol.Symbol) (string, bool) {
	if node == nil {
		return "", false
	}
	for _, sym := range symbols {
		if sym.Location.StartLine == uint32(node.StartPosition().Row+1) &&
			sym.Location.StartCol == uint32(node.StartPosition().Column+1) {
			return string(sym.ID), true
		}
	}
	return "", false
}

func (d *GinDetector) resolveGinSelectorTarget(selector *tree_sitter.Node, file boundary.ParsedGoFile, handlerEnv map[string]handlerBinding, receiverEnv map[string]string, state *ginPackageState) (string, targetref.Kind) {
	if selector == nil || selector.Kind() != "selector_expression" {
		return "", targetref.KindUnknown
	}
	field := selector.ChildByFieldName("field")
	operand := selector.ChildByFieldName("operand")
	if field == nil || operand == nil {
		return "", targetref.KindUnknown
	}
	methodName := nodeText(field, file.Content)

	if receiverType, ok := ginLocalReceiverType(operand, file.Content, file.PackageName, handlerEnv, receiverEnv); ok {
		if sym, ok := state.uniqueMethod(receiverType, methodName); ok {
			return sym.CanonicalName, targetref.KindExactCanonical
		}
		return "", targetref.KindUnknown
	}

	binding, ok := ginHandlerBindingForOperand(operand, file.Content, handlerEnv)
	if !ok || binding.packageToken == "" {
		return "", targetref.KindUnknown
	}
	if binding.receiverType != "" && targetref.NormalizePackageToken(binding.packageToken) == targetref.NormalizePackageToken(file.PackageName) {
		if sym, ok := state.uniqueMethod(binding.receiverType, methodName); ok {
			return sym.CanonicalName, targetref.KindExactCanonical
		}
	}
	hint := targetref.BuildPackageMethodHint(binding.packageToken, methodName)
	if hint == "" {
		return "", targetref.KindUnknown
	}
	return hint, targetref.KindPackageMethodHint
}

func ginLocalReceiverType(expr *tree_sitter.Node, content []byte, currentPackage string, handlerEnv map[string]handlerBinding, receiverEnv map[string]string) (string, bool) {
	expr = unwrapParens(expr)
	if expr == nil {
		return "", false
	}
	switch expr.Kind() {
	case "identifier":
		name := nodeText(expr, content)
		if receiverType, ok := receiverEnv[name]; ok {
			return receiverType, true
		}
		if binding, ok := handlerEnv[name]; ok && targetref.NormalizePackageToken(binding.packageToken) == targetref.NormalizePackageToken(currentPackage) && binding.receiverType != "" {
			return binding.receiverType, true
		}
	case "composite_literal":
		typeName := compositeLiteralTypeName(expr, content)
		if typeName != "" {
			return typeName, true
		}
	case "unary_expression":
		if operand := expr.ChildByFieldName("operand"); operand != nil {
			return ginLocalReceiverType(operand, content, currentPackage, handlerEnv, receiverEnv)
		}
	}
	return "", false
}

func ginHandlerBindingForOperand(expr *tree_sitter.Node, content []byte, handlerEnv map[string]handlerBinding) (handlerBinding, bool) {
	expr = unwrapParens(expr)
	if expr == nil || expr.Kind() != "identifier" {
		return handlerBinding{}, false
	}
	binding, ok := handlerEnv[nodeText(expr, content)]
	return binding, ok
}

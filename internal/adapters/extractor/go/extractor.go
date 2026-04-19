package goextractor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/domain/targetref"
	extractorport "analysis-module/internal/ports/extractor"
	"analysis-module/pkg/ids"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Extractor struct {
	parser              *treesitter.GoParser
	packageTypeEnvCache map[string]goFileTypeEnv
}

type scope struct {
	start, end uint32
	sym        symbol.Symbol
}

type syntheticSpanIndex struct {
	bySpan map[string]symbol.Symbol
}

type semanticHintMatch struct {
	startByte uint32
	hint      executionhint.Hint
}

type goFieldRef struct {
	DeclaredType string
	PackageToken string
}

type goFileTypeEnv struct {
	structFields map[string]map[string]goFieldRef
}

type goCallEnv struct {
	packageName           string
	receiverType          string
	receiverAliases       map[string]string
	importAliases         map[string]string
	importBindingsByAlias map[string]symbol.ImportBinding
	typeEnv               goFileTypeEnv
}

func newSyntheticSpanIndex() syntheticSpanIndex {
	return syntheticSpanIndex{bySpan: map[string]symbol.Symbol{}}
}

func (i *syntheticSpanIndex) Add(node *tree_sitter.Node, sym symbol.Symbol) {
	if node == nil {
		return
	}
	i.bySpan[spanKey(node)] = sym
}

func (i syntheticSpanIndex) Find(node *tree_sitter.Node) (symbol.Symbol, bool) {
	if node == nil {
		return symbol.Symbol{}, false
	}
	sym, ok := i.bySpan[spanKey(node)]
	return sym, ok
}

func spanKey(node *tree_sitter.Node) string {
	return fmt.Sprintf("%d:%d", node.StartByte(), node.EndByte())
}

func orderedHints(matches ...[]semanticHintMatch) []executionhint.Hint {
	var combined []semanticHintMatch
	for _, matchSet := range matches {
		combined = append(combined, matchSet...)
	}
	sort.SliceStable(combined, func(i, j int) bool {
		if combined[i].startByte != combined[j].startByte {
			return combined[i].startByte < combined[j].startByte
		}
		if combined[i].hint.Kind != combined[j].hint.Kind {
			return combined[i].hint.Kind < combined[j].hint.Kind
		}
		return combined[i].hint.Evidence < combined[j].hint.Evidence
	})
	ordered := make([]executionhint.Hint, 0, len(combined))
	for idx, match := range combined {
		match.hint.OrderIndex = idx
		ordered = append(ordered, match.hint)
	}
	return ordered
}

func isReturnedFuncLiteral(node *tree_sitter.Node) bool {
	if node == nil || node.Kind() != "func_literal" {
		return false
	}
	parent := node.Parent()
	if parent == nil {
		return false
	}
	if parent.Kind() == "return_statement" {
		return true
	}
	if parent.Kind() == "expression_list" {
		grandparent := parent.Parent()
		return grandparent != nil && grandparent.Kind() == "return_statement" && parent.NamedChildCount() == 1
	}
	return false
}

func isInNestedFuncLiteral(root, node *tree_sitter.Node) bool {
	if root == nil || node == nil || node == root {
		return false
	}
	for parent := node.Parent(); parent != nil && parent != root; parent = parent.Parent() {
		if parent.Kind() == "func_literal" {
			return true
		}
	}
	return false
}

func New() extractorport.SymbolExtractor {
	return &Extractor{
		parser:              treesitter.NewGoParser(),
		packageTypeEnvCache: map[string]goFileTypeEnv{},
	}
}

func (e *Extractor) Supports(lang string) bool {
	return strings.EqualFold(lang, "go")
}

func (e *Extractor) ExtractFile(file symbol.FileRef) (symbol.FileExtractionResult, error) {
	content, err := os.ReadFile(file.AbsolutePath)
	if err != nil {
		return symbol.FileExtractionResult{}, err
	}
	tree, err := e.parser.Parse(content)
	if err != nil {
		return symbol.FileExtractionResult{}, err
	}
	// The official bindings keep parse trees backed by C memory until Close.
	defer tree.Close()
	root := tree.RootNode()
	result := symbol.FileExtractionResult{
		FilePath: file.RelativePath,
		Language: "go",
		Warnings: []string{},
	}
	importAliases := map[string]string{}
	importBindingsByAlias := map[string]symbol.ImportBinding{}
	modulePath := readGoModulePath(file.RepositoryRoot)
	walk(root, func(node *tree_sitter.Node) {
		switch node.Kind() {
		case "package_clause":
			if name := packageClauseName(node); name != nil {
				result.PackageName = name.Utf8Text(content)
			}
		case "import_spec":
			pathNode := node.ChildByFieldName("path")
			if pathNode != nil {
				importPath := strings.Trim(pathNode.Utf8Text(content), "\"")
				result.Imports = append(result.Imports, importPath)
				alias := filepath.Base(importPath)
				if aliasNode := node.ChildByFieldName("name"); aliasNode != nil {
					alias = aliasNode.Utf8Text(content)
				}
				importAliases[alias] = importPath
				resolvedPath, local := resolveGoImportPath(file.RepositoryRoot, modulePath, importPath)
				binding := symbol.ImportBinding{
					Source:       importPath,
					Alias:        alias,
					ResolvedPath: resolvedPath,
					IsNamespace:  true,
					IsLocal:      local,
				}
				importBindingsByAlias[alias] = binding
				result.ImportBindings = append(result.ImportBindings, binding)
			}
		}
	})
	typeEnv := e.packageTypeEnv(file, result.PackageName, root, content)
	walk(root, func(node *tree_sitter.Node) {
		switch node.Kind() {
		case "function_declaration", "method_declaration":
			sym := buildSymbol(file, result.PackageName, node, content)
			result.Symbols = append(result.Symbols, sym)
			body := node.ChildByFieldName("body")
			callEnv := goCallEnv{
				packageName:           result.PackageName,
				receiverType:          sym.Receiver,
				receiverAliases:       declarationReceiverAliases(node, content),
				importAliases:         importAliases,
				importBindingsByAlias: importBindingsByAlias,
				typeEnv:               typeEnv,
			}

			closureCount := 0
			inlineCount := 0
			var scopes []scope
			syntheticIndex := newSyntheticSpanIndex()

			// First pass: identify closures and inline handlers inside this function body
			walk(body, func(inner *tree_sitter.Node) {
				if inner == nil || inner.Kind() != "func_literal" {
					return
				}

				var synth symbol.Symbol
				if isReturnedFuncLiteral(inner) {
					synth = GenerateClosureSymbol(file, result.PackageName, sym.CanonicalName, closureCount, inner)
					closureCount++
				} else {
					synth = GenerateInlineSymbol(file, result.PackageName, sym.CanonicalName, inlineCount, inner)
					inlineCount++
				}
				result.Symbols = append(result.Symbols, synth)
				scopes = append(scopes, scope{
					start: uint32(inner.StartByte()),
					end:   uint32(inner.EndByte()),
					sym:   synth,
				})
				syntheticIndex.Add(inner, synth)
			})

			// Semantic extraction runs after synthetic symbol creation so hints can bind to exact IDs.
			result.Hints = append(result.Hints, orderedHints(
				extractClosureHints(body, content, sym, syntheticIndex),
				extractAsyncHints(body, content, sym, callEnv, syntheticIndex),
				extractControlHints(body, content, sym),
			)...)

			// Second pass: extract calls with nearest scope affiliation
			walk(body, func(inner *tree_sitter.Node) {
				if inner == nil || inner.Kind() != "call_expression" {
					return
				}

				// Determine active symbol (nested scopes take priority since they are physically inside)
				activeSym := sym
				start, end := uint32(inner.StartByte()), uint32(inner.EndByte())
				var tightest *scope
				for i := range scopes {
					s := &scopes[i]
					if start >= s.start && end <= s.end {
						if tightest == nil || (s.end-s.start < tightest.end-tightest.start) {
							tightest = s
						}
					}
				}
				if tightest != nil {
					activeSym = tightest.sym
				}

				candidate := buildCallCandidate(activeSym.ID, inner, content, callEnv)
				if candidate.TargetCanonicalName != "" {
					result.Relations = append(result.Relations, candidate)
				}
			})
		case "type_spec":
			if typeNode := node.ChildByFieldName("type"); typeNode != nil {
				kind := symbol.KindStruct
				if typeNode.Kind() == "interface_type" {
					kind = symbol.KindInterface
				}
				if nameNode := node.ChildByFieldName("name"); nameNode != nil {
					name := nameNode.Utf8Text(content)
					result.Symbols = append(result.Symbols, symbol.Symbol{
						ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, name, string(kind))),
						RepositoryID:  file.RepositoryID,
						FilePath:      file.RelativePath,
						PackageName:   result.PackageName,
						Name:          name,
						CanonicalName: canonicalName(result.PackageName, "", name),
						Kind:          kind,
						Signature:     strings.TrimSpace(node.Utf8Text(content)),
						Location:      locationFromNode(file.RelativePath, node),
					})
				}
			}
		}
	})
	return result, nil
}

func newGoFileTypeEnv() goFileTypeEnv {
	return goFileTypeEnv{structFields: map[string]map[string]goFieldRef{}}
}

func cloneGoFileTypeEnv(env goFileTypeEnv) goFileTypeEnv {
	cloned := newGoFileTypeEnv()
	for structName, fields := range env.structFields {
		fieldCopy := make(map[string]goFieldRef, len(fields))
		for fieldName, fieldRef := range fields {
			fieldCopy[fieldName] = fieldRef
		}
		cloned.structFields[structName] = fieldCopy
	}
	return cloned
}

func (e *Extractor) packageTypeEnv(file symbol.FileRef, packageName string, root *tree_sitter.Node, content []byte) goFileTypeEnv {
	env := newGoFileTypeEnv()
	collectTypeSpecsFromRoot(root, content, packageName, &env)

	if file.RepositoryRoot == "" || packageName == "" {
		return env
	}

	cacheKey := packageTypeEnvCacheKey(file, packageName)
	if cached, ok := e.packageTypeEnvCache[cacheKey]; ok {
		return cloneGoFileTypeEnv(cached)
	}

	dir := filepath.Join(file.RepositoryRoot, filepath.Dir(file.RelativePath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.packageTypeEnvCache[cacheKey] = cloneGoFileTypeEnv(env)
		return env
	}

	currentPath := filepath.Clean(file.AbsolutePath)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		absPath := filepath.Join(dir, name)
		if filepath.Clean(absPath) == currentPath {
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		tree, err := e.parser.Parse(data)
		if err != nil {
			continue
		}
		if packageNameForRoot(tree.RootNode(), data) != packageName {
			tree.Close()
			continue
		}
		collectTypeSpecsFromRoot(tree.RootNode(), data, packageName, &env)
		tree.Close()
	}

	e.packageTypeEnvCache[cacheKey] = cloneGoFileTypeEnv(env)
	return env
}

func packageTypeEnvCacheKey(file symbol.FileRef, packageName string) string {
	return filepath.ToSlash(filepath.Join(file.RepositoryRoot, filepath.Dir(file.RelativePath))) + "|" + packageName
}

func collectTypeSpecsFromRoot(root *tree_sitter.Node, content []byte, packageName string, env *goFileTypeEnv) {
	walk(root, func(node *tree_sitter.Node) {
		if node != nil && node.Kind() == "type_spec" {
			collectGoStructFields(node, content, packageName, env)
		}
	})
}

func packageNameForRoot(root *tree_sitter.Node, content []byte) string {
	if root == nil {
		return ""
	}
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child == nil || child.Kind() != "package_clause" {
			continue
		}
		if name := packageClauseName(child); name != nil {
			return name.Utf8Text(content)
		}
	}
	return ""
}

func packageClauseName(node *tree_sitter.Node) *tree_sitter.Node {
	if node == nil {
		return nil
	}
	if name := node.ChildByFieldName("name"); name != nil {
		return name
	}
	if node.NamedChildCount() > 0 {
		return node.NamedChild(0)
	}
	return nil
}

func buildSymbol(file symbol.FileRef, pkg string, node *tree_sitter.Node, content []byte) symbol.Symbol {
	nameNode := node.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = nameNode.Utf8Text(content)
	}
	receiver := ""
	kind := symbol.KindFunction
	if node.Kind() == "method_declaration" {
		kind = symbol.KindMethod
		if receiverNode := node.ChildByFieldName("receiver"); receiverNode != nil {
			receiver = normalizeReceiver(receiverNode.Utf8Text(content))
		}
	}
	if strings.HasPrefix(name, "Test") {
		kind = symbol.KindTestFunction
	}
	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, receiver, name)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   pkg,
		Name:          name,
		Receiver:      receiver,
		CanonicalName: canonicalName(pkg, receiver, name),
		Kind:          kind,
		Signature:     strings.TrimSpace(node.Utf8Text(content)),
		Location:      locationFromNode(file.RelativePath, node),
	}
}

func buildCallCandidate(source symbol.ID, node *tree_sitter.Node, content []byte, env goCallEnv) symbol.RelationCandidate {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return symbol.RelationCandidate{}
	}
	candidate := resolveCallTarget(fnNode, content, env)
	candidate.SourceSymbolID = source
	candidate.Relationship = "calls"
	candidate.EvidenceSource = strings.TrimSpace(fnNode.Utf8Text(content))
	candidate.ExtractionMethod = "tree-sitter-go"
	return candidate
}

func resolveCallTarget(node *tree_sitter.Node, content []byte, env goCallEnv) symbol.RelationCandidate {
	node = unwrapGoParens(node)
	if node == nil {
		return symbol.RelationCandidate{}
	}
	switch node.Kind() {
	case "identifier":
		return symbol.RelationCandidate{
			TargetCanonicalName: canonicalName(env.packageName, "", node.Utf8Text(content)),
			TargetKind:          targetref.KindExactCanonical,
			EvidenceType:        "identifier",
			ConfidenceScore:     0.95,
		}
	case "selector_expression":
		operand := node.ChildByFieldName("operand")
		field := node.ChildByFieldName("field")
		if operand == nil || field == nil {
			return symbol.RelationCandidate{}
		}
		left := operand.Utf8Text(content)
		right := field.Utf8Text(content)
		if importPath, ok := env.importAliases[left]; ok {
			candidate := symbol.RelationCandidate{
				TargetCanonicalName: importPath + "." + right,
				TargetKind:          targetref.KindExactCanonical,
				EvidenceType:        "import_selector",
				ConfidenceScore:     0.75,
			}
			if binding, ok := env.importBindingsByAlias[left]; ok && binding.ResolvedPath != "" {
				candidate.TargetFilePath = binding.ResolvedPath
				candidate.TargetExportName = right
			}
			return candidate
		}
		if receiverType, ok := env.receiverAliases[left]; ok {
			return symbol.RelationCandidate{
				TargetCanonicalName: canonicalName(env.packageName, receiverType, right),
				TargetKind:          targetref.KindExactCanonical,
				EvidenceType:        "receiver_selector",
				ConfidenceScore:     0.9,
			}
		}
		if packageToken, ok := env.receiverFieldPackageToken(operand, content); ok {
			hint := targetref.BuildPackageMethodHint(packageToken, right)
			if hint == "" {
				return symbol.RelationCandidate{}
			}
			return symbol.RelationCandidate{
				TargetCanonicalName: hint,
				TargetKind:          targetref.KindPackageMethodHint,
				EvidenceType:        "receiver_field_selector",
				ConfidenceScore:     0.7,
			}
		}
		return symbol.RelationCandidate{}
	default:
		return symbol.RelationCandidate{}
	}
}

func canonicalName(pkg, receiver, name string) string {
	if receiver != "" {
		return fmt.Sprintf("%s.%s.%s", pkg, receiver, name)
	}
	return fmt.Sprintf("%s.%s", pkg, name)
}

func normalizeReceiver(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")
	raw = strings.ReplaceAll(raw, "*", "")
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func declarationReceiverAliases(node *tree_sitter.Node, content []byte) map[string]string {
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
		typeNode := param.ChildByFieldName("type")
		if typeNode == nil && param.NamedChildCount() > 0 {
			typeNode = param.NamedChild(uint(param.NamedChildCount() - 1))
		}
		if typeNode == nil {
			continue
		}
		receiverType := normalizeReceiver(typeNode.Utf8Text(content))
		if receiverType == "" {
			continue
		}
		if nameNode := param.ChildByFieldName("name"); nameNode != nil {
			aliases[nameNode.Utf8Text(content)] = receiverType
			continue
		}
		if param.NamedChildCount() > 0 {
			name := param.NamedChild(0).Utf8Text(content)
			if name != receiverType {
				aliases[name] = receiverType
			}
		}
	}
	return aliases
}

func collectGoStructFields(typeSpec *tree_sitter.Node, content []byte, packageName string, env *goFileTypeEnv) {
	if typeSpec == nil || env == nil {
		return
	}
	typeNode := typeSpec.ChildByFieldName("type")
	nameNode := typeSpec.ChildByFieldName("name")
	if typeNode == nil || nameNode == nil || typeNode.Kind() != "struct_type" {
		return
	}
	structName := nameNode.Utf8Text(content)
	if structName == "" {
		return
	}
	if env.structFields[structName] == nil {
		env.structFields[structName] = map[string]goFieldRef{}
	}
	walk(typeNode, func(node *tree_sitter.Node) {
		if node == nil || node.Kind() != "field_declaration" {
			return
		}
		fieldType := node.ChildByFieldName("type")
		if fieldType == nil && node.NamedChildCount() > 0 {
			fieldType = node.NamedChild(uint(node.NamedChildCount() - 1))
		}
		if fieldType == nil {
			return
		}
		fieldRef := goFieldRef{
			DeclaredType: strings.TrimSpace(fieldType.Utf8Text(content)),
			PackageToken: targetref.PackageTokenFromTypeText(fieldType.Utf8Text(content), packageName),
		}
		for _, fieldName := range goFieldDeclarationNames(node, fieldType, content) {
			env.structFields[structName][fieldName] = fieldRef
		}
	})
}

func goFieldDeclarationNames(fieldDecl, typeNode *tree_sitter.Node, content []byte) []string {
	if fieldDecl == nil {
		return nil
	}
	if nameNode := fieldDecl.ChildByFieldName("name"); nameNode != nil {
		return goNamedFieldNames(nameNode, content)
	}
	names := []string{}
	for i := 0; i < int(fieldDecl.NamedChildCount()); i++ {
		child := fieldDecl.NamedChild(uint(i))
		if child == nil || child == typeNode {
			continue
		}
		switch child.Kind() {
		case "identifier", "field_identifier", "type_identifier":
			names = append(names, child.Utf8Text(content))
		case "identifier_list", "expression_list", "parameter_list":
			names = append(names, goNamedFieldNames(child, content)...)
		}
	}
	return names
}

func goNamedFieldNames(node *tree_sitter.Node, content []byte) []string {
	if node == nil {
		return nil
	}
	if node.Kind() == "identifier" || node.Kind() == "field_identifier" || node.Kind() == "type_identifier" {
		return []string{node.Utf8Text(content)}
	}
	names := []string{}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "identifier", "field_identifier", "type_identifier":
			names = append(names, child.Utf8Text(content))
		}
	}
	return names
}

func goTypePackageToken(node *tree_sitter.Node, content []byte, currentPkg string) string {
	node = unwrapGoParens(node)
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "pointer_type", "slice_type", "array_type", "variadic_type", "channel_type":
		if child := node.ChildByFieldName("type"); child != nil {
			return goTypePackageToken(child, content, currentPkg)
		}
		if node.NamedChildCount() > 0 {
			return goTypePackageToken(node.NamedChild(uint(node.NamedChildCount()-1)), content, currentPkg)
		}
	case "map_type":
		if value := node.ChildByFieldName("value"); value != nil {
			return goTypePackageToken(value, content, currentPkg)
		}
		if node.NamedChildCount() > 1 {
			return goTypePackageToken(node.NamedChild(1), content, currentPkg)
		}
	case "generic_type":
		if typeNode := node.ChildByFieldName("type"); typeNode != nil {
			return goTypePackageToken(typeNode, content, currentPkg)
		}
		if node.NamedChildCount() > 0 {
			return goTypePackageToken(node.NamedChild(0), content, currentPkg)
		}
	case "selector_expression":
		if operand := node.ChildByFieldName("operand"); operand != nil {
			return operand.Utf8Text(content)
		}
	case "type_identifier", "identifier":
		return currentPkg
	}
	return currentPkg
}

func (e goCallEnv) receiverFieldPackageToken(expr *tree_sitter.Node, content []byte) (string, bool) {
	expr = unwrapGoParens(expr)
	if expr == nil || expr.Kind() != "selector_expression" {
		return "", false
	}
	operand := expr.ChildByFieldName("operand")
	field := expr.ChildByFieldName("field")
	if operand == nil || field == nil {
		return "", false
	}
	receiverType, ok := e.receiverAliases[operand.Utf8Text(content)]
	if !ok {
		return "", false
	}
	fields := e.typeEnv.structFields[receiverType]
	if fields == nil {
		return "", false
	}
	fieldRef, ok := fields[field.Utf8Text(content)]
	if !ok {
		return "", false
	}
	if fieldRef.PackageToken != "" {
		return targetref.NormalizePackageToken(fieldRef.PackageToken), true
	}
	return "", false
}

func unwrapGoParens(node *tree_sitter.Node) *tree_sitter.Node {
	for node != nil {
		switch {
		case node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1:
			node = node.NamedChild(0)
		case node.Kind() == "expression_list" && node.NamedChildCount() == 1:
			node = node.NamedChild(0)
		default:
			return node
		}
	}
	return nil
}

func readGoModulePath(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func resolveGoImportPath(repoRoot, modulePath, importPath string) (string, bool) {
	if modulePath == "" || !strings.HasPrefix(importPath, modulePath) {
		return "", false
	}
	relDir := strings.TrimPrefix(strings.TrimPrefix(importPath, modulePath), "/")
	if relDir == "" {
		return "", false
	}
	dir := filepath.Join(repoRoot, filepath.FromSlash(relDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	candidates := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		candidates = append(candidates, filepath.Join(dir, name))
	}
	if len(candidates) != 1 {
		return "", false
	}
	rel, err := filepath.Rel(repoRoot, candidates[0])
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func locationFromNode(path string, node *tree_sitter.Node) symbol.CodeLocation {
	start := node.StartPosition()
	end := node.EndPosition()
	return symbol.CodeLocation{
		FilePath:  path,
		StartLine: uint32(start.Row) + 1,
		StartCol:  uint32(start.Column) + 1,
		EndLine:   uint32(end.Row) + 1,
		EndCol:    uint32(end.Column) + 1,
		StartByte: uint32(node.StartByte()),
		EndByte:   uint32(node.EndByte()),
	}
}

func walk(node *tree_sitter.Node, visit func(*tree_sitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		walk(node.NamedChild(i), visit)
	}
}

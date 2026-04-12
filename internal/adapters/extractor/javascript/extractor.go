package jsextractor

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/symbol"
	extractorport "analysis-module/internal/ports/extractor"
	"analysis-module/pkg/ids"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Extractor struct {
	jsParser  *treesitter.JavaScriptParser
	tsParser  *treesitter.TypeScriptParser
	tsxParser *treesitter.TSXParser
}

func New() extractorport.SymbolExtractor {
	return &Extractor{
		jsParser:  treesitter.NewJavaScriptParser(),
		tsParser:  treesitter.NewTypeScriptParser(),
		tsxParser: treesitter.NewTSXParser(),
	}
}

func (e *Extractor) Supports(lang string) bool {
	return strings.EqualFold(lang, "javascript") || strings.EqualFold(lang, "typescript")
}

func (e *Extractor) ExtractFile(file symbol.FileRef) (symbol.FileExtractionResult, error) {
	content, err := os.ReadFile(file.AbsolutePath)
	if err != nil {
		return symbol.FileExtractionResult{}, err
	}
	tree, err := e.parse(file, content)
	if err != nil {
		return symbol.FileExtractionResult{}, err
	}
	defer tree.Close()

	moduleName := jsModulePath(file.RelativePath)
	result := symbol.FileExtractionResult{
		FilePath:    file.RelativePath,
		Language:    file.Language,
		PackageName: moduleName,
		Warnings:    []string{},
	}

	imports, importsByAlias := extractJSImportBindings(file, content, tree.RootNode())
	result.ImportBindings = imports
	result.Imports = collectImportSources(imports)

	contexts := make([]jsSymbolContext, 0, 8)
	collectJSSymbols(file, content, tree.RootNode(), moduleName, "", &result, &contexts)
	result.Exports = append(result.Exports, collectJSExports(moduleName, content, tree.RootNode(), result.Symbols)...)
	sort.Slice(result.Symbols, func(i, j int) bool {
		return result.Symbols[i].CanonicalName < result.Symbols[j].CanonicalName
	})

	for _, ctx := range contexts {
		extractJSRelations(content, moduleName, ctx, importsByAlias, &result)
	}
	return result, nil
}

func (e *Extractor) parse(file symbol.FileRef, content []byte) (*tree_sitter.Tree, error) {
	ext := strings.ToLower(filepath.Ext(file.RelativePath))
	switch {
	case strings.EqualFold(file.Language, "typescript") && ext == ".tsx":
		return e.tsxParser.Parse(content)
	case strings.EqualFold(file.Language, "typescript"):
		return e.tsParser.Parse(content)
	default:
		return e.jsParser.Parse(content)
	}
}

type jsSymbolContext struct {
	symbol   symbol.Symbol
	body     *tree_sitter.Node
	receiver string
}

func collectJSSymbols(file symbol.FileRef, content []byte, node *tree_sitter.Node, moduleName, className string, result *symbol.FileExtractionResult, contexts *[]jsSymbolContext) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "class_declaration":
		nameNode := node.ChildByFieldName("name")
		bodyNode := node.ChildByFieldName("body")
		if nameNode == nil {
			return
		}
		name := strings.TrimSpace(nameNode.Utf8Text(content))
		sym := buildJSSymbol(file, moduleName, name, "", symbol.KindClass, node, content)
		result.Symbols = append(result.Symbols, sym)
		collectJSSymbols(file, content, bodyNode, moduleName, name, result, contexts)
		return
	case "function_declaration", "generator_function_declaration":
		nameNode := node.ChildByFieldName("name")
		bodyNode := node.ChildByFieldName("body")
		if nameNode == nil || bodyNode == nil {
			return
		}
		name := strings.TrimSpace(nameNode.Utf8Text(content))
		kind := detectJSTestKind(file.RelativePath, name)
		sym := buildJSSymbol(file, moduleName, name, "", kind, node, content)
		result.Symbols = append(result.Symbols, sym)
		*contexts = append(*contexts, jsSymbolContext{symbol: sym, body: bodyNode})
		return
	case "method_definition":
		nameNode := node.ChildByFieldName("name")
		bodyNode := node.ChildByFieldName("body")
		if nameNode == nil || bodyNode == nil || className == "" {
			return
		}
		name := strings.TrimSpace(nameNode.Utf8Text(content))
		sym := buildJSSymbol(file, moduleName, name, className, detectJSTestKind(file.RelativePath, name), node, content)
		result.Symbols = append(result.Symbols, sym)
		*contexts = append(*contexts, jsSymbolContext{symbol: sym, body: bodyNode, receiver: className})
		return
	case "lexical_declaration":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child == nil || child.Kind() != "variable_declarator" {
				continue
			}
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")
			if nameNode == nil || valueNode == nil {
				continue
			}
			if valueNode.Kind() != "arrow_function" && valueNode.Kind() != "function" && valueNode.Kind() != "function_expression" {
				continue
			}
			bodyNode := valueNode.ChildByFieldName("body")
			if bodyNode == nil {
				continue
			}
			name := strings.TrimSpace(nameNode.Utf8Text(content))
			sym := buildJSSymbol(file, moduleName, name, "", detectJSTestKind(file.RelativePath, name), child, content)
			result.Symbols = append(result.Symbols, sym)
			*contexts = append(*contexts, jsSymbolContext{symbol: sym, body: bodyNode})
		}
	case "call_expression":
		callee := node.ChildByFieldName("function")
		if callee == nil {
			break
		}
		name := callIdentifier(callee, content)
		if name != "test" && name != "it" {
			break
		}
		args := node.ChildByFieldName("arguments")
		if args == nil {
			break
		}
		callback := findCallbackArgument(args)
		if callback == nil {
			break
		}
		bodyNode := callback.ChildByFieldName("body")
		if bodyNode == nil {
			break
		}
		syntheticName := buildSyntheticTestName(node, content)
		sym := buildJSSymbol(file, moduleName, syntheticName, "", symbol.KindTestFunction, callback, content)
		result.Symbols = append(result.Symbols, sym)
		*contexts = append(*contexts, jsSymbolContext{symbol: sym, body: bodyNode})
		return
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		collectJSSymbols(file, content, node.NamedChild(i), moduleName, className, result, contexts)
	}
}

func buildJSSymbol(file symbol.FileRef, moduleName, name, receiver string, kind symbol.Kind, node *tree_sitter.Node, content []byte) symbol.Symbol {
	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, receiver, name)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   moduleName,
		Name:          name,
		Receiver:      receiver,
		CanonicalName: jsCanonicalName(moduleName, receiver, name),
		Kind:          kind,
		Signature:     strings.TrimSpace(node.Utf8Text(content)),
		Location:      locationFromNode(file.RelativePath, node),
	}
}

func extractJSImportBindings(file symbol.FileRef, content []byte, root *tree_sitter.Node) ([]symbol.ImportBinding, map[string]symbol.ImportBinding) {
	imports := make([]symbol.ImportBinding, 0, 8)
	byAlias := map[string]symbol.ImportBinding{}
	walk(root, func(node *tree_sitter.Node) {
		if node == nil || node.Kind() != "import_statement" {
			return
		}
		text := strings.TrimSpace(node.Utf8Text(content))
		source := importSource(text)
		if source == "" {
			return
		}
		resolvedPath, local := resolveJSModulePath(file.RepositoryRoot, file.RelativePath, source)
		clause := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "import"), ";"))
		if idx := strings.Index(clause, " from "); idx >= 0 {
			clause = strings.TrimSpace(clause[:idx])
		}
		switch {
		case strings.HasPrefix(clause, "{"):
			for _, part := range splitCommaSeparated(strings.Trim(clause, "{}")) {
				fields := strings.SplitN(part, " as ", 2)
				exportName := strings.TrimSpace(fields[0])
				alias := exportName
				if len(fields) == 2 && strings.TrimSpace(fields[1]) != "" {
					alias = strings.TrimSpace(fields[1])
				}
				binding := symbol.ImportBinding{
					Source:       source,
					Alias:        alias,
					ImportedName: exportName,
					ExportName:   exportName,
					ResolvedPath: resolvedPath,
					IsLocal:      local,
				}
				imports = append(imports, binding)
				byAlias[alias] = binding
			}
		case strings.HasPrefix(clause, "* as "):
			alias := strings.TrimSpace(strings.TrimPrefix(clause, "* as "))
			binding := symbol.ImportBinding{
				Source:       source,
				Alias:        alias,
				ResolvedPath: resolvedPath,
				IsNamespace:  true,
				IsLocal:      local,
			}
			imports = append(imports, binding)
			byAlias[alias] = binding
		default:
			defaultAlias := clause
			namespaceAlias := ""
			if strings.Contains(clause, ",") {
				parts := splitCommaSeparated(clause)
				if len(parts) > 0 {
					defaultAlias = parts[0]
				}
				if len(parts) > 1 && strings.HasPrefix(parts[1], "* as ") {
					namespaceAlias = strings.TrimSpace(strings.TrimPrefix(parts[1], "* as "))
				}
			}
			defaultAlias = strings.TrimSpace(defaultAlias)
			if defaultAlias != "" {
				binding := symbol.ImportBinding{
					Source:       source,
					Alias:        defaultAlias,
					ExportName:   "default",
					ResolvedPath: resolvedPath,
					IsDefault:    true,
					IsLocal:      local,
				}
				imports = append(imports, binding)
				byAlias[defaultAlias] = binding
			}
			if namespaceAlias != "" {
				binding := symbol.ImportBinding{
					Source:       source,
					Alias:        namespaceAlias,
					ResolvedPath: resolvedPath,
					IsNamespace:  true,
					IsLocal:      local,
				}
				imports = append(imports, binding)
				byAlias[namespaceAlias] = binding
			}
		}
	})
	return dedupeImportBindings(imports), byAlias
}

func collectJSExports(moduleName string, content []byte, root *tree_sitter.Node, symbols []symbol.Symbol) []symbol.ExportBinding {
	exports := make([]symbol.ExportBinding, 0, 8)
	symbolByName := map[string]symbol.Symbol{}
	for _, sym := range symbols {
		if sym.Receiver == "" {
			symbolByName[sym.Name] = sym
		}
	}
	walk(root, func(node *tree_sitter.Node) {
		if node == nil || node.Kind() != "export_statement" {
			return
		}
		text := strings.TrimSpace(node.Utf8Text(content))
		switch {
		case strings.HasPrefix(text, "export default function "):
			if declaration := firstNamedChild(node); declaration != nil {
				nameNode := declaration.ChildByFieldName("name")
				if nameNode != nil {
					name := strings.TrimSpace(nameNode.Utf8Text(content))
					if sym, ok := symbolByName[name]; ok {
						exports = append(exports, symbol.ExportBinding{Name: "default", CanonicalName: sym.CanonicalName, IsDefault: true})
					}
				}
			}
		case strings.HasPrefix(text, "export default "):
			name := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(text, "export default ")), ";")
			if sym, ok := symbolByName[name]; ok {
				exports = append(exports, symbol.ExportBinding{Name: "default", CanonicalName: sym.CanonicalName, IsDefault: true})
			}
		case strings.HasPrefix(text, "export {"):
			for _, part := range splitCommaSeparated(strings.Trim(strings.TrimPrefix(strings.TrimSuffix(text, ";"), "export "), "{}")) {
				fields := strings.SplitN(part, " as ", 2)
				name := strings.TrimSpace(fields[0])
				exportName := name
				if len(fields) == 2 && strings.TrimSpace(fields[1]) != "" {
					exportName = strings.TrimSpace(fields[1])
				}
				if sym, ok := symbolByName[name]; ok {
					exports = append(exports, symbol.ExportBinding{Name: exportName, CanonicalName: sym.CanonicalName, IsDefault: exportName == "default"})
				}
			}
		default:
			if declaration := firstNamedChild(node); declaration != nil {
				name := exportDeclarationName(declaration, content)
				if name == "" {
					return
				}
				if sym, ok := symbolByName[name]; ok {
					exports = append(exports, symbol.ExportBinding{Name: name, CanonicalName: sym.CanonicalName})
				} else {
					exports = append(exports, symbol.ExportBinding{Name: name, CanonicalName: jsCanonicalName(moduleName, "", name)})
				}
			}
		}
	})
	return dedupeExportBindings(exports)
}

func extractJSRelations(content []byte, moduleName string, ctx jsSymbolContext, imports map[string]symbol.ImportBinding, result *symbol.FileExtractionResult) {
	walk(ctx.body, func(node *tree_sitter.Node) {
		if node == nil || node.Kind() != "call_expression" {
			return
		}
		functionNode := node.ChildByFieldName("function")
		if functionNode == nil {
			return
		}
		relation, diagnostic := resolveJSCall(moduleName, ctx, functionNode, content, imports)
		if diagnostic != nil {
			diagnostic.SymbolID = ctx.symbol.ID
			diagnostic.FilePath = ctx.symbol.FilePath
			result.Diagnostics = append(result.Diagnostics, *diagnostic)
			return
		}
		if relation.TargetCanonicalName == "" && relation.TargetFilePath == "" {
			return
		}
		relation.SourceSymbolID = ctx.symbol.ID
		relation.Relationship = "calls"
		relation.EvidenceType = "call_expression"
		relation.EvidenceSource = strings.TrimSpace(functionNode.Utf8Text(content))
		relation.ExtractionMethod = "tree-sitter-js"
		if relation.ConfidenceScore == 0 {
			relation.ConfidenceScore = 0.8
		}
		result.Relations = append(result.Relations, relation)
	})
}

func resolveJSCall(moduleName string, ctx jsSymbolContext, node *tree_sitter.Node, content []byte, imports map[string]symbol.ImportBinding) (symbol.RelationCandidate, *symbol.Diagnostic) {
	switch node.Kind() {
	case "identifier":
		name := strings.TrimSpace(node.Utf8Text(content))
		if name == "" {
			return symbol.RelationCandidate{}, nil
		}
		if binding, ok := imports[name]; ok {
			if !binding.IsLocal || binding.ResolvedPath == "" {
				return symbol.RelationCandidate{}, &symbol.Diagnostic{
					Category: "unresolved_import",
					Message:  "js/ts import could not be resolved to a local module",
					Evidence: name,
				}
			}
			relation := symbol.RelationCandidate{
				TargetFilePath:   binding.ResolvedPath,
				TargetExportName: binding.ExportName,
				ConfidenceScore:  0.9,
			}
			if relation.TargetExportName == "" {
				relation.TargetExportName = binding.ImportedName
			}
			if relation.TargetExportName != "" && relation.TargetExportName != "default" {
				relation.TargetCanonicalName = jsModulePath(binding.ResolvedPath) + "." + relation.TargetExportName
			}
			return relation, nil
		}
		return symbol.RelationCandidate{
			TargetCanonicalName: jsCanonicalName(moduleName, "", name),
			ConfidenceScore:     0.9,
		}, nil
	case "member_expression":
		objectNode := node.ChildByFieldName("object")
		propertyNode := node.ChildByFieldName("property")
		if objectNode == nil || propertyNode == nil {
			return symbol.RelationCandidate{}, &symbol.Diagnostic{
				Category: "unsupported_construct",
				Message:  "js/ts member call shape is not statically supported",
				Evidence: strings.TrimSpace(node.Utf8Text(content)),
			}
		}
		objectText := strings.TrimSpace(objectNode.Utf8Text(content))
		propertyName := strings.TrimSpace(propertyNode.Utf8Text(content))
		if objectText == "this" && ctx.receiver != "" {
			return symbol.RelationCandidate{
				TargetCanonicalName: jsCanonicalName(moduleName, ctx.receiver, propertyName),
				ConfidenceScore:     0.85,
			}, nil
		}
		rootName := objectText
		if idx := strings.Index(objectText, "."); idx >= 0 {
			rootName = objectText[:idx]
		}
		if binding, ok := imports[rootName]; ok {
			if !binding.IsLocal || binding.ResolvedPath == "" {
				return symbol.RelationCandidate{}, &symbol.Diagnostic{
					Category: "unresolved_import",
					Message:  "js/ts imported member call could not be resolved locally",
					Evidence: strings.TrimSpace(node.Utf8Text(content)),
				}
			}
			return symbol.RelationCandidate{
				TargetCanonicalName: jsModulePath(binding.ResolvedPath) + "." + propertyName,
				TargetFilePath:      binding.ResolvedPath,
				TargetExportName:    propertyName,
				ConfidenceScore:     0.85,
			}, nil
		}
		return symbol.RelationCandidate{}, &symbol.Diagnostic{
			Category: "ambiguous_relation",
			Message:  "js/ts member call target is not statically attributable",
			Evidence: strings.TrimSpace(node.Utf8Text(content)),
		}
	default:
		return symbol.RelationCandidate{}, &symbol.Diagnostic{
			Category: "unsupported_construct",
			Message:  "js/ts dynamic call target is outside the supported static ceiling",
			Evidence: strings.TrimSpace(node.Utf8Text(content)),
		}
	}
}

func resolveJSModulePath(repoRoot, relativePath, spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" || !strings.HasPrefix(spec, ".") {
		return "", false
	}
	currentDir := filepath.Dir(relativePath)
	base := filepath.Clean(filepath.Join(repoRoot, currentDir, spec))
	candidates := []string{
		base + ".ts",
		base + ".tsx",
		base + ".js",
		base + ".jsx",
		base + ".mjs",
		base + ".cjs",
		filepath.Join(base, "index.ts"),
		filepath.Join(base, "index.tsx"),
		filepath.Join(base, "index.js"),
		filepath.Join(base, "index.jsx"),
		filepath.Join(base, "index.mjs"),
		filepath.Join(base, "index.cjs"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(repoRoot, candidate)
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

func importSource(text string) string {
	for _, quote := range []string{"\"", "'"} {
		start := strings.Index(text, quote)
		end := strings.LastIndex(text, quote)
		if start >= 0 && end > start {
			return text[start+1 : end]
		}
	}
	return ""
}

func callIdentifier(node *tree_sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier":
		return strings.TrimSpace(node.Utf8Text(content))
	case "member_expression":
		property := node.ChildByFieldName("property")
		if property != nil {
			return strings.TrimSpace(property.Utf8Text(content))
		}
	}
	return ""
}

func findCallbackArgument(arguments *tree_sitter.Node) *tree_sitter.Node {
	for i := uint(0); i < arguments.NamedChildCount(); i++ {
		child := arguments.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Kind() == "arrow_function" || child.Kind() == "function" || child.Kind() == "function_expression" {
			return child
		}
	}
	return nil
}

func buildSyntheticTestName(node *tree_sitter.Node, content []byte) string {
	args := node.ChildByFieldName("arguments")
	label := ""
	if args != nil && args.NamedChildCount() > 0 {
		first := args.NamedChild(0)
		if first != nil {
			label = sanitizeIdentifier(strings.Trim(first.Utf8Text(content), "\"'`"))
		}
	}
	if label == "" {
		label = "callback"
	}
	return "__test_" + label + "_" + strconv.Itoa(int(node.StartPosition().Row)+1)
}

func sanitizeIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, raw)
	raw = strings.Trim(raw, "_")
	if raw == "" {
		return ""
	}
	return raw
}

func exportDeclarationName(node *tree_sitter.Node, content []byte) string {
	switch node.Kind() {
	case "function_declaration", "generator_function_declaration", "class_declaration":
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			return strings.TrimSpace(nameNode.Utf8Text(content))
		}
	case "lexical_declaration":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child == nil || child.Kind() != "variable_declarator" {
				continue
			}
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				return strings.TrimSpace(nameNode.Utf8Text(content))
			}
		}
	}
	return ""
}

func firstNamedChild(node *tree_sitter.Node) *tree_sitter.Node {
	if node == nil {
		return nil
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			return child
		}
	}
	return nil
}

func dedupeImportBindings(bindings []symbol.ImportBinding) []symbol.ImportBinding {
	seen := map[string]symbol.ImportBinding{}
	for _, binding := range bindings {
		key := binding.Source + "|" + binding.Alias + "|" + binding.ImportedName + "|" + binding.ResolvedPath + "|" + binding.ExportName
		seen[key] = binding
	}
	result := make([]symbol.ImportBinding, 0, len(seen))
	for _, binding := range seen {
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Alias == result[j].Alias {
			return result[i].Source < result[j].Source
		}
		return result[i].Alias < result[j].Alias
	})
	return result
}

func dedupeExportBindings(bindings []symbol.ExportBinding) []symbol.ExportBinding {
	seen := map[string]symbol.ExportBinding{}
	for _, binding := range bindings {
		key := binding.Name + "|" + binding.CanonicalName
		seen[key] = binding
	}
	result := make([]symbol.ExportBinding, 0, len(seen))
	for _, binding := range seen {
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func collectImportSources(bindings []symbol.ImportBinding) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Source == "" {
			continue
		}
		if _, ok := seen[binding.Source]; ok {
			continue
		}
		seen[binding.Source] = struct{}{}
		result = append(result, binding.Source)
	}
	sort.Strings(result)
	return result
}

func splitCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func jsCanonicalName(moduleName, receiver, name string) string {
	if receiver != "" {
		return moduleName + "." + receiver + "." + name
	}
	return moduleName + "." + name
}

func jsModulePath(relativePath string) string {
	withoutExt := strings.TrimSuffix(filepath.ToSlash(relativePath), filepath.Ext(relativePath))
	withoutIndex := strings.TrimSuffix(withoutExt, "/index")
	if withoutIndex != "" && withoutIndex != withoutExt {
		return strings.ReplaceAll(withoutIndex, "/", ".")
	}
	return strings.ReplaceAll(withoutExt, "/", ".")
}

func detectJSTestKind(path, name string) symbol.Kind {
	lower := strings.ToLower(filepath.Base(path))
	if strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") || strings.HasPrefix(strings.ToLower(name), "test") {
		return symbol.KindTestFunction
	}
	return symbol.KindFunction
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

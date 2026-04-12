package pythonextractor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/symbol"
	extractorport "analysis-module/internal/ports/extractor"
	"analysis-module/pkg/ids"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Extractor struct {
	parser *treesitter.PythonParser
}

func New() extractorport.SymbolExtractor {
	return &Extractor{parser: treesitter.NewPythonParser()}
}

func (e *Extractor) Supports(lang string) bool {
	return strings.EqualFold(lang, "python")
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
	defer tree.Close()

	moduleName := pythonModulePath(file.RelativePath)
	result := symbol.FileExtractionResult{
		FilePath:    file.RelativePath,
		Language:    "python",
		PackageName: moduleName,
		Warnings:    []string{},
	}

	imports, importsByAlias := extractPythonImports(file, content, tree.RootNode())
	result.ImportBindings = imports
	result.Imports = collectImportSources(imports)

	contexts := make([]pythonSymbolContext, 0, 8)
	collectPythonSymbols(file, content, tree.RootNode(), moduleName, "", false, &result, &contexts)
	sort.Slice(result.Symbols, func(i, j int) bool {
		return result.Symbols[i].CanonicalName < result.Symbols[j].CanonicalName
	})

	for _, ctx := range contexts {
		extractPythonRelations(content, moduleName, ctx, importsByAlias, &result)
	}

	return result, nil
}

type pythonSymbolContext struct {
	symbol   symbol.Symbol
	body     *tree_sitter.Node
	receiver string
}

func collectPythonSymbols(file symbol.FileRef, content []byte, node *tree_sitter.Node, moduleName, className string, inFunction bool, result *symbol.FileExtractionResult, contexts *[]pythonSymbolContext) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "class_definition":
		nameNode := node.ChildByFieldName("name")
		bodyNode := node.ChildByFieldName("body")
		if nameNode == nil {
			return
		}
		name := nameNode.Utf8Text(content)
		sym := buildPythonSymbol(file, moduleName, name, "", symbol.KindClass, node, content)
		result.Symbols = append(result.Symbols, sym)
		if !inFunction {
			result.Exports = append(result.Exports, symbol.ExportBinding{Name: name, CanonicalName: sym.CanonicalName})
		}
		collectPythonSymbols(file, content, bodyNode, moduleName, name, false, result, contexts)
		return
	case "function_definition":
		nameNode := node.ChildByFieldName("name")
		bodyNode := node.ChildByFieldName("body")
		if nameNode == nil || bodyNode == nil {
			return
		}
		name := nameNode.Utf8Text(content)
		kind := symbol.KindFunction
		receiver := ""
		if className != "" {
			receiver = className
			kind = symbol.KindMethod
		}
		if strings.HasPrefix(name, "test_") || strings.HasPrefix(filepath.Base(file.RelativePath), "test_") || strings.HasSuffix(filepath.Base(file.RelativePath), "_test.py") {
			kind = symbol.KindTestFunction
		}
		sym := buildPythonSymbol(file, moduleName, name, receiver, kind, node, content)
		result.Symbols = append(result.Symbols, sym)
		if !inFunction && receiver == "" {
			result.Exports = append(result.Exports, symbol.ExportBinding{Name: name, CanonicalName: sym.CanonicalName})
		}
		*contexts = append(*contexts, pythonSymbolContext{symbol: sym, body: bodyNode, receiver: receiver})
		return
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		nextClass := className
		nextInFunction := inFunction
		if node.Kind() == "function_definition" {
			nextInFunction = true
		}
		collectPythonSymbols(file, content, child, moduleName, nextClass, nextInFunction, result, contexts)
	}
}

func buildPythonSymbol(file symbol.FileRef, moduleName, name, receiver string, kind symbol.Kind, node *tree_sitter.Node, content []byte) symbol.Symbol {
	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, receiver, name)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   moduleName,
		Name:          name,
		Receiver:      receiver,
		CanonicalName: pythonCanonicalName(moduleName, receiver, name),
		Kind:          kind,
		Signature:     strings.TrimSpace(node.Utf8Text(content)),
		Location:      locationFromNode(file.RelativePath, node),
	}
}

func extractPythonImports(file symbol.FileRef, content []byte, root *tree_sitter.Node) ([]symbol.ImportBinding, map[string]symbol.ImportBinding) {
	imports := make([]symbol.ImportBinding, 0, 8)
	byAlias := map[string]symbol.ImportBinding{}
	walk(root, func(node *tree_sitter.Node) {
		switch node.Kind() {
		case "import_statement":
			statement := strings.TrimSpace(node.Utf8Text(content))
			statement = strings.TrimPrefix(statement, "import ")
			for _, part := range splitCommaSeparated(statement) {
				fields := strings.SplitN(part, " as ", 2)
				moduleSpec := strings.TrimSpace(fields[0])
				if moduleSpec == "" {
					continue
				}
				alias := filepath.Base(strings.ReplaceAll(moduleSpec, ".", "/"))
				if len(fields) == 2 && strings.TrimSpace(fields[1]) != "" {
					alias = strings.TrimSpace(fields[1])
				}
				resolvedPath, local := resolvePythonModulePath(file.RepositoryRoot, file.RelativePath, moduleSpec)
				binding := symbol.ImportBinding{
					Source:       moduleSpec,
					Alias:        alias,
					ResolvedPath: resolvedPath,
					IsNamespace:  true,
					IsLocal:      local,
				}
				imports = append(imports, binding)
				byAlias[alias] = binding
			}
		case "import_from_statement":
			statement := strings.TrimSpace(node.Utf8Text(content))
			statement = strings.TrimPrefix(statement, "from ")
			idx := strings.Index(statement, " import ")
			if idx < 0 {
				return
			}
			moduleSpec := strings.TrimSpace(statement[:idx])
			importSection := strings.TrimSpace(statement[idx+len(" import "):])
			resolvedPath, local := resolvePythonModulePath(file.RepositoryRoot, file.RelativePath, moduleSpec)
			for _, part := range splitCommaSeparated(strings.Trim(importSection, "()")) {
				fields := strings.SplitN(part, " as ", 2)
				exportName := strings.TrimSpace(fields[0])
				if exportName == "" || exportName == "*" {
					continue
				}
				alias := exportName
				if len(fields) == 2 && strings.TrimSpace(fields[1]) != "" {
					alias = strings.TrimSpace(fields[1])
				}
				binding := symbol.ImportBinding{
					Source:       moduleSpec,
					Alias:        alias,
					ImportedName: exportName,
					ExportName:   exportName,
					ResolvedPath: resolvedPath,
					IsLocal:      local,
				}
				imports = append(imports, binding)
				byAlias[alias] = binding
			}
		}
	})
	return dedupeImportBindings(imports), byAlias
}

func extractPythonRelations(content []byte, moduleName string, ctx pythonSymbolContext, imports map[string]symbol.ImportBinding, result *symbol.FileExtractionResult) {
	walk(ctx.body, func(node *tree_sitter.Node) {
		if node == nil || node.Kind() != "call" {
			return
		}
		functionNode := node.ChildByFieldName("function")
		if functionNode == nil {
			return
		}
		relation, diagnostic := resolvePythonCall(moduleName, ctx, functionNode, content, imports)
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
		relation.ExtractionMethod = "tree-sitter-python"
		if relation.ConfidenceScore == 0 {
			relation.ConfidenceScore = 0.8
		}
		result.Relations = append(result.Relations, relation)
	})
}

func resolvePythonCall(moduleName string, ctx pythonSymbolContext, node *tree_sitter.Node, content []byte, imports map[string]symbol.ImportBinding) (symbol.RelationCandidate, *symbol.Diagnostic) {
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
					Message:  "python import could not be resolved to a local module",
					Evidence: name,
				}
			}
			targetExport := binding.ExportName
			if targetExport == "" {
				targetExport = binding.ImportedName
			}
			canonical := ""
			if targetExport != "" {
				canonical = pythonModulePath(binding.ResolvedPath) + "." + targetExport
			}
			return symbol.RelationCandidate{
				TargetCanonicalName: canonical,
				TargetFilePath:      binding.ResolvedPath,
				TargetExportName:    targetExport,
				ConfidenceScore:     0.9,
			}, nil
		}
		return symbol.RelationCandidate{
			TargetCanonicalName: pythonCanonicalName(moduleName, "", name),
			ConfidenceScore:     0.9,
		}, nil
	case "attribute":
		objectNode := node.ChildByFieldName("object")
		attrNode := node.ChildByFieldName("attribute")
		if objectNode == nil || attrNode == nil {
			return symbol.RelationCandidate{}, &symbol.Diagnostic{
				Category: "unsupported_construct",
				Message:  "python attribute call shape is not statically supported",
				Evidence: strings.TrimSpace(node.Utf8Text(content)),
			}
		}
		objectText := strings.TrimSpace(objectNode.Utf8Text(content))
		attrName := strings.TrimSpace(attrNode.Utf8Text(content))
		if attrName == "" {
			return symbol.RelationCandidate{}, nil
		}
		if objectText == "self" || objectText == "cls" {
			if ctx.receiver == "" {
				return symbol.RelationCandidate{}, &symbol.Diagnostic{
					Category: "ambiguous_relation",
					Message:  "python self/cls call outside method scope is ambiguous",
					Evidence: strings.TrimSpace(node.Utf8Text(content)),
				}
			}
			return symbol.RelationCandidate{
				TargetCanonicalName: pythonCanonicalName(moduleName, ctx.receiver, attrName),
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
					Message:  "python imported module attribute call could not be resolved locally",
					Evidence: strings.TrimSpace(node.Utf8Text(content)),
				}
			}
			return symbol.RelationCandidate{
				TargetCanonicalName: pythonModulePath(binding.ResolvedPath) + "." + attrName,
				TargetFilePath:      binding.ResolvedPath,
				TargetExportName:    attrName,
				ConfidenceScore:     0.85,
			}, nil
		}
		return symbol.RelationCandidate{}, &symbol.Diagnostic{
			Category: "ambiguous_relation",
			Message:  "python attribute call target is not statically attributable",
			Evidence: strings.TrimSpace(node.Utf8Text(content)),
		}
	default:
		return symbol.RelationCandidate{}, &symbol.Diagnostic{
			Category: "unsupported_construct",
			Message:  "python dynamic call target is outside the supported static ceiling",
			Evidence: strings.TrimSpace(node.Utf8Text(content)),
		}
	}
}

func resolvePythonModulePath(repoRoot, relativePath, moduleSpec string) (string, bool) {
	moduleSpec = strings.TrimSpace(moduleSpec)
	if moduleSpec == "" {
		return "", false
	}
	module := moduleSpec
	if strings.HasPrefix(moduleSpec, ".") {
		currentPackage := strings.Split(pythonModulePath(relativePath), ".")
		if len(currentPackage) > 0 {
			currentPackage = currentPackage[:len(currentPackage)-1]
		}
		dots := 0
		for dots < len(moduleSpec) && moduleSpec[dots] == '.' {
			dots++
		}
		module = strings.TrimPrefix(moduleSpec[dots:], ".")
		ascend := dots - 1
		if ascend < 0 {
			ascend = 0
		}
		if ascend > len(currentPackage) {
			return "", false
		}
		baseParts := currentPackage[:len(currentPackage)-ascend]
		if module != "" {
			baseParts = append(baseParts, strings.Split(module, ".")...)
		}
		module = strings.Join(baseParts, ".")
	}
	modulePath := filepath.Join(repoRoot, filepath.FromSlash(strings.ReplaceAll(module, ".", "/")))
	candidates := []string{
		modulePath + ".py",
		filepath.Join(modulePath, "__init__.py"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(repoRoot, candidate)
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

func pythonCanonicalName(moduleName, receiver, name string) string {
	if receiver != "" {
		return moduleName + "." + receiver + "." + name
	}
	return moduleName + "." + name
}

func pythonModulePath(relativePath string) string {
	withoutExt := strings.TrimSuffix(filepath.ToSlash(relativePath), filepath.Ext(relativePath))
	withoutInit := strings.TrimSuffix(withoutExt, "/__init__")
	return strings.ReplaceAll(withoutInit, "/", ".")
}

func dedupeImportBindings(bindings []symbol.ImportBinding) []symbol.ImportBinding {
	seen := map[string]symbol.ImportBinding{}
	for _, binding := range bindings {
		key := binding.Source + "|" + binding.Alias + "|" + binding.ImportedName + "|" + binding.ResolvedPath
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

package goextractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/symbol"
	extractorport "analysis-module/internal/ports/extractor"
	"analysis-module/pkg/ids"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Extractor struct {
	parser *treesitter.GoParser
}

func New() extractorport.SymbolExtractor {
	return &Extractor{parser: treesitter.NewGoParser()}
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
			}
		case "function_declaration", "method_declaration":
			sym := buildSymbol(file, result.PackageName, node, content)
			result.Symbols = append(result.Symbols, sym)
			body := node.ChildByFieldName("body")
			walk(body, func(inner *tree_sitter.Node) {
				if inner == nil || inner.Kind() != "call_expression" {
					return
				}
				candidate := buildCallCandidate(sym.ID, inner, content, result.PackageName, importAliases)
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

func buildCallCandidate(source symbol.ID, node *tree_sitter.Node, content []byte, pkg string, importAliases map[string]string) symbol.RelationCandidate {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return symbol.RelationCandidate{}
	}
	target, confidence, evidence := resolveCallTarget(fnNode, content, pkg, importAliases)
	return symbol.RelationCandidate{
		SourceSymbolID:      source,
		TargetCanonicalName: target,
		Relationship:        "calls",
		EvidenceType:        evidence,
		EvidenceSource:      strings.TrimSpace(fnNode.Utf8Text(content)),
		ExtractionMethod:    "tree-sitter-go",
		ConfidenceScore:     confidence,
	}
}

func resolveCallTarget(node *tree_sitter.Node, content []byte, pkg string, importAliases map[string]string) (string, float64, string) {
	switch node.Kind() {
	case "identifier":
		return canonicalName(pkg, "", node.Utf8Text(content)), 0.95, "identifier"
	case "selector_expression":
		operand := node.ChildByFieldName("operand")
		field := node.ChildByFieldName("field")
		if operand == nil || field == nil {
			return "", 0, ""
		}
		left := operand.Utf8Text(content)
		right := field.Utf8Text(content)
		if importPath, ok := importAliases[left]; ok {
			return importPath + "." + right, 0.75, "import_selector"
		}
		return pkg + "." + right, 0.55, "selector"
	default:
		raw := strings.TrimSpace(node.Utf8Text(content))
		if raw == "" {
			return "", 0, ""
		}
		return pkg + "." + raw, 0.3, "expression"
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

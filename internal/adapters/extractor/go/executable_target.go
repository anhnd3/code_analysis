package goextractor

import (
	"fmt"
	"analysis-module/internal/domain/symbol"
	"analysis-module/pkg/ids"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type TargetCategory string

const (
	TargetDirect  TargetCategory = "direct"
	TargetClosure TargetCategory = "closure_return"
	TargetInline  TargetCategory = "inline_handler"
)

// ExecutableTarget represents a resolved handler expression
type ExecutableTarget struct {
	Category        TargetCategory
	CanonicalTarget string // canonical name of the symbol to trace
}

// GenerateClosureSymbol creates a synthetic representation of a returned function literal.
func GenerateClosureSymbol(file symbol.FileRef, pkg string, parentCanonicalName string, index int, node *tree_sitter.Node) symbol.Symbol {
	syntheticName := fmt.Sprintf("$closure_return_%d", index)
	canonical := fmt.Sprintf("%s.%s", parentCanonicalName, syntheticName)

	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, canonical)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   pkg,
		Name:          syntheticName,
		CanonicalName: canonical,
		Kind:          symbol.KindFunction,
		Signature:     "func()", // Exact sig could be extracted if needed
		Location:      locationFromNode(file.RelativePath, node),
		Properties: map[string]string{
			"synthetic":        "true",
			"synthetic_kind":   string(TargetClosure),
			"parent_canonical": parentCanonicalName,
		},
	}
}

// GenerateInlineSymbol creates a synthetic representation of an inline function literal.
func GenerateInlineSymbol(file symbol.FileRef, pkg string, callContextCanonical string, index int, node *tree_sitter.Node) symbol.Symbol {
	syntheticName := fmt.Sprintf("$inline_handler_%d", index)
	canonical := fmt.Sprintf("%s.%s", callContextCanonical, syntheticName)

	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, canonical)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   pkg,
		Name:          syntheticName,
		CanonicalName: canonical,
		Kind:          symbol.KindFunction,
		Signature:     "func(...)",
		Location:      locationFromNode(file.RelativePath, node),
		Properties: map[string]string{
			"synthetic":      "true",
			"synthetic_kind": string(TargetInline),
			"parent_context": callContextCanonical,
		},
	}
}

package goextractor

import (
	"strings"

	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractControlHints detects if/else, switch, and for/range constructs,
// emitting HintBranch with the condition text as the label.
func extractControlHints(node *tree_sitter.Node, content []byte, src symbol.Symbol) []executionhint.Hint {
	var hints []executionhint.Hint
	walk(node, func(inner *tree_sitter.Node) {
		if inner == nil {
			return
		}
		switch inner.Kind() {
		case "if_statement":
			label := branchLabel(inner, content, "condition")
			hints = append(hints, executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbol:   "",
				Kind:           executionhint.HintBranch,
				Evidence:       "if " + label,
			})

		case "switch_statement", "expression_switch_statement", "type_switch_statement":
			label := branchLabel(inner, content, "value")
			hints = append(hints, executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbol:   "",
				Kind:           executionhint.HintBranch,
				Evidence:       "switch " + label,
			})

		case "for_statement":
			hints = append(hints, executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbol:   "",
				Kind:           executionhint.HintBranch,
				Evidence:       "for loop at line " + lineOf(inner),
			})

		case "range_statement", "for_range_statement":
			label := branchLabel(inner, content, "right")
			hints = append(hints, executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbol:   "",
				Kind:           executionhint.HintBranch,
				Evidence:       "range " + label,
			})
		}
	})
	return hints
}

// branchLabel extracts the text of the named field on the node (e.g. "condition", "value"),
// falling back to the first named child, and finally to a friendly fallback string.
func branchLabel(node *tree_sitter.Node, content []byte, field string) string {
	if fn := node.ChildByFieldName(field); fn != nil {
		return strings.TrimSpace(fn.Utf8Text(content))
	}
	if node.NamedChildCount() > 0 {
		if child := node.NamedChild(0); child != nil {
			return strings.TrimSpace(child.Utf8Text(content))
		}
	}
	return "unknown"
}

func lineOf(node *tree_sitter.Node) string {
	return locationLabel(node) // reuse helper from semantic_closure.go
}

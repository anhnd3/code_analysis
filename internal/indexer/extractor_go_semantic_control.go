package indexer

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractControlHints detects if/else, switch, and for/range constructs, emitting HintBranch.
func extractControlHints(node *tree_sitter.Node, content []byte, src Symbol) []semanticHintMatch {
	var hints []semanticHintMatch

	walk(node, func(inner *tree_sitter.Node) bool {
		if inner == nil || isInNestedFuncLiteral(node, inner) {
			return true
		}
		switch inner.Kind() {
		case "if_statement":
			label := branchLabel(inner, content, "condition")
			hints = append(hints, semanticHintMatch{
				startByte: uint32(inner.StartByte()),
				hint: Hint{
					SourceSymbolID: string(src.ID),
					TargetSymbolID: string(src.ID),
					TargetSymbol:   src.CanonicalName,
					Kind:           HintBranch,
					Evidence:       "if " + label,
				},
			})

		case "switch_statement", "expression_switch_statement", "type_switch_statement":
			label := branchLabel(inner, content, "value")
			hints = append(hints, semanticHintMatch{
				startByte: uint32(inner.StartByte()),
				hint: Hint{
					SourceSymbolID: string(src.ID),
					TargetSymbolID: string(src.ID),
					TargetSymbol:   src.CanonicalName,
					Kind:           HintBranch,
					Evidence:       "switch " + label,
				},
			})

		case "for_statement":
			hints = append(hints, semanticHintMatch{
				startByte: uint32(inner.StartByte()),
				hint: Hint{
					SourceSymbolID: string(src.ID),
					TargetSymbolID: string(src.ID),
					TargetSymbol:   src.CanonicalName,
					Kind:           HintBranch,
					Evidence:       "for loop at line " + lineOf(inner),
				},
			})

		case "range_statement", "for_range_statement":
			label := branchLabel(inner, content, "right")
			hints = append(hints, semanticHintMatch{
				startByte: uint32(inner.StartByte()),
				hint: Hint{
					SourceSymbolID: string(src.ID),
					TargetSymbolID: string(src.ID),
					TargetSymbol:   src.CanonicalName,
					Kind:           HintBranch,
					Evidence:       "range " + label,
				},
			})
		}
		return true
	})
	return hints
}

// branchLabel extracts the text of the named field on the node (e.g. "condition", "value").
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

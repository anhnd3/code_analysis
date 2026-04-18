package goextractor

import (
	"fmt"
	"strings"

	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractClosureHints detects direct returned func literals and binds them to
// the already-created synthetic closure symbols from the enclosing function.
//
// TODO(phase2): add alias-chain resolution for returned closures only if it
// materially improves flow generation without weakening target precision.
func extractClosureHints(node *tree_sitter.Node, content []byte, src symbol.Symbol, syntheticIndex syntheticSpanIndex) []semanticHintMatch {
	var hints []semanticHintMatch
	walk(node, func(inner *tree_sitter.Node) {
		if inner == nil || inner.Kind() != "return_statement" || isInNestedFuncLiteral(node, inner) {
			return
		}
		funcLiteral := returnedFuncLiteral(inner)
		if funcLiteral == nil {
			return
		}
		target, ok := syntheticIndex.Find(funcLiteral)
		if !ok {
			return
		}
		hints = append(hints, semanticHintMatch{
			startByte: uint32(inner.StartByte()),
			hint: executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbolID: string(target.ID),
				TargetSymbol:   target.CanonicalName,
				Kind:           executionhint.HintReturnHandler,
				Evidence:       semanticEvidence(inner, content),
			},
		})
	})
	return hints
}

func returnedFuncLiteral(node *tree_sitter.Node) *tree_sitter.Node {
	if node == nil || node.Kind() != "return_statement" {
		return nil
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Kind() == "func_literal" {
			return child
		}
		if child.Kind() == "expression_list" && child.NamedChildCount() == 1 {
			expr := child.NamedChild(0)
			if expr != nil && expr.Kind() == "func_literal" {
				return expr
			}
		}
	}
	return nil
}

func semanticEvidence(node *tree_sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(strings.Fields(node.Utf8Text(content)), " "))
}

func locationLabel(node *tree_sitter.Node) string {
	pos := node.StartPosition()
	return fmt.Sprintf("line %d", pos.Row+1)
}

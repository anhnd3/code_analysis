package goextractor

import (
	"fmt"

	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractClosureHints detects returned func literals and emits HintReturnHandler.
// It mirrors the closure-counting logic in extractor.go so that the synthetic
// canonical name matches the node created by GenerateClosureSymbol.
func extractClosureHints(node *tree_sitter.Node, content []byte, src symbol.Symbol) []executionhint.Hint {
	var hints []executionhint.Hint
	closureIdx := 0
	walk(node, func(inner *tree_sitter.Node) {
		if inner == nil || inner.Kind() != "func_literal" {
			return
		}
		parent := inner.Parent()
		isReturn := parent != nil &&
			(parent.Kind() == "return_statement" || parent.Kind() == "expression_list")
		if !isReturn {
			return
		}
		syntheticName := fmt.Sprintf("$closure_return_%d", closureIdx)
		closureCanonical := fmt.Sprintf("%s.%s", src.CanonicalName, syntheticName)
		closureIdx++
		hints = append(hints, executionhint.Hint{
			SourceSymbolID: string(src.ID),
			TargetSymbol:   closureCanonical,
			Kind:           executionhint.HintReturnHandler,
			Evidence:       "return func_literal at " + locationLabel(inner),
		})
	})
	return hints
}

func locationLabel(node *tree_sitter.Node) string {
	pos := node.StartPosition()
	return fmt.Sprintf("line %d", pos.Row+1)
}

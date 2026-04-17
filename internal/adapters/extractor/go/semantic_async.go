package goextractor

import (
	"fmt"
	"strings"

	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractAsyncHints detects:
//   - go <expr>     → HintSpawn
//   - defer <expr>  → HintDefer
//   - wg.Wait()     → HintWait
func extractAsyncHints(node *tree_sitter.Node, content []byte, src symbol.Symbol, pkg string, importAliases map[string]string) []executionhint.Hint {
	var hints []executionhint.Hint
	inlineIdx := 0 // mirrors extractor.go's inlineCount

	walk(node, func(inner *tree_sitter.Node) {
		if inner == nil {
			return
		}
		switch inner.Kind() {
		case "go_statement":
			target, evidence := resolveAsyncTarget(inner, content, pkg, importAliases, inlineIdx, src.CanonicalName)
			if isFuncLiteral(inner) {
				inlineIdx++
			}
			hints = append(hints, executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbol:   target,
				Kind:           executionhint.HintSpawn,
				Evidence:       "go " + evidence,
			})

		case "defer_statement":
			target, evidence := resolveAsyncTarget(inner, content, pkg, importAliases, inlineIdx, src.CanonicalName)
			if isFuncLiteral(inner) {
				inlineIdx++
			}
			hints = append(hints, executionhint.Hint{
				SourceSymbolID: string(src.ID),
				TargetSymbol:   target,
				Kind:           executionhint.HintDefer,
				Evidence:       "defer " + evidence,
			})

		case "call_expression":
			// Detect wg.Wait() – any selector call whose field is "Wait"
			if isWaitCall(inner, content) {
				hints = append(hints, executionhint.Hint{
					SourceSymbolID: string(src.ID),
					TargetSymbol:   "",
					Kind:           executionhint.HintWait,
					Evidence:       fmt.Sprintf("wg.Wait() at line %d", inner.StartPosition().Row+1),
				})
			}
		}
	})
	return hints
}

// resolveAsyncTarget returns (targetCanonical, evidenceText) for a go/defer node.
// If the body is a func_literal, it maps to the synthetic inline canonical name.
func resolveAsyncTarget(node *tree_sitter.Node, content []byte, pkg string, importAliases map[string]string, inlineIdx int, parentCanonical string) (string, string) {
	// The immediate child of go_statement / defer_statement is typically a call_expression
	// or a func_literal (for anonymous goroutines).
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "func_literal":
			syntheticName := fmt.Sprintf("$inline_handler_%d", inlineIdx)
			return fmt.Sprintf("%s.%s", parentCanonical, syntheticName), "func_literal"
		case "call_expression":
			fnNode := child.ChildByFieldName("function")
			if fnNode == nil {
				break
			}
			target, _, evidence := resolveCallTarget(fnNode, content, pkg, importAliases)
			return target, evidence
		}
	}
	raw := strings.TrimSpace(node.Utf8Text(content))
	return pkg + "." + raw, "expression"
}

// isFuncLiteral reports whether a go/defer statement directly spawns an anonymous func.
func isFuncLiteral(node *tree_sitter.Node) bool {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Kind() == "func_literal" {
			return true
		}
	}
	return false
}

// isWaitCall returns true when inner is a call_expression of the form <anything>.Wait().
func isWaitCall(node *tree_sitter.Node, content []byte) bool {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil || fnNode.Kind() != "selector_expression" {
		return false
	}
	field := fnNode.ChildByFieldName("field")
	return field != nil && field.Utf8Text(content) == "Wait"
}

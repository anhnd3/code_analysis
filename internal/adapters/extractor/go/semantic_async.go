package goextractor

import (
	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractAsyncHints detects:
//   - go <expr>     -> HintSpawn
//   - defer <expr>  -> HintDefer
//   - wg.Wait()     -> HintWait
//
// TODO(phase1-pr2): model explicit join nodes if WAITS_ON later needs a
// first-class barrier target instead of a stable self-edge.
func extractAsyncHints(node *tree_sitter.Node, content []byte, src symbol.Symbol, env goCallEnv, syntheticIndex syntheticSpanIndex) []semanticHintMatch {
	var hints []semanticHintMatch

	walk(node, func(inner *tree_sitter.Node) {
		if inner == nil || isInNestedFuncLiteral(node, inner) {
			return
		}
		switch inner.Kind() {
			case "go_statement":
				if hint, ok := asyncHint(inner, content, src, env, syntheticIndex, executionhint.HintSpawn); ok {
					hints = append(hints, hint)
				}

			case "defer_statement":
				if hint, ok := asyncHint(inner, content, src, env, syntheticIndex, executionhint.HintDefer); ok {
					hints = append(hints, hint)
				}

		case "call_expression":
			if isWaitCall(inner, content) {
				hints = append(hints, semanticHintMatch{
					startByte: uint32(inner.StartByte()),
					hint: executionhint.Hint{
						SourceSymbolID: string(src.ID),
						TargetSymbolID: string(src.ID),
						TargetSymbol:   src.CanonicalName,
						Kind:           executionhint.HintWait,
						Evidence:       semanticEvidence(inner, content),
					},
				})
			}
		}
	})
	return hints
}

func asyncHint(node *tree_sitter.Node, content []byte, src symbol.Symbol, env goCallEnv, syntheticIndex syntheticSpanIndex, kind executionhint.HintKind) (semanticHintMatch, bool) {
	call := asyncCallExpression(node)
	if call == nil {
		return semanticHintMatch{}, false
	}
	fnNode := call.ChildByFieldName("function")
	if fnNode == nil {
		return semanticHintMatch{}, false
	}
	hint := executionhint.Hint{
		SourceSymbolID: string(src.ID),
		Kind:           kind,
		Evidence:       semanticEvidence(node, content),
	}
	if fnNode.Kind() == "func_literal" {
		target, ok := syntheticIndex.Find(fnNode)
		if !ok {
			return semanticHintMatch{}, false
		}
		hint.TargetSymbolID = string(target.ID)
		hint.TargetSymbol = target.CanonicalName
		return semanticHintMatch{startByte: uint32(node.StartByte()), hint: hint}, true
	}
	target := resolveCallTarget(fnNode, content, env)
	if target.TargetCanonicalName == "" {
		return semanticHintMatch{}, false
	}
	hint.TargetSymbol = target.TargetCanonicalName
	return semanticHintMatch{startByte: uint32(node.StartByte()), hint: hint}, true
}

func asyncCallExpression(node *tree_sitter.Node) *tree_sitter.Node {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Kind() == "call_expression" {
			return child
		}
	}
	return nil
}

func isWaitCall(node *tree_sitter.Node, content []byte) bool {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil || fnNode.Kind() != "selector_expression" {
		return false
	}
	args := node.ChildByFieldName("arguments")
	if args != nil && args.NamedChildCount() != 0 {
		return false
	}
	field := fnNode.ChildByFieldName("field")
	return field != nil && field.Utf8Text(content) == "Wait"
}

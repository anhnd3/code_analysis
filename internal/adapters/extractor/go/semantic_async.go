package goextractor

import (
	"analysis-module/internal/domain/executionhint"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractAsyncHints(node *tree_sitter.Node, content []byte) []executionhint.Hint {
	var hints []executionhint.Hint
	// TODO: Match AST against go_statement and wg.Wait()
	return hints
}

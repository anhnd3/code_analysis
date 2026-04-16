package goextractor

import (
	"analysis-module/internal/domain/executionhint"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractClosureHints(node *tree_sitter.Node, content []byte) []executionhint.Hint {
	var hints []executionhint.Hint
	// TODO: Match AST against func_literal inside return_statement
	return hints
}

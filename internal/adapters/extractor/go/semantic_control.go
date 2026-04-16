package goextractor

import (
	"analysis-module/internal/domain/executionhint"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractControlHints(node *tree_sitter.Node, content []byte) []executionhint.Hint {
	var hints []executionhint.Hint
	// TODO: Match AST against defer_statement and if_statement / switch_statement
	return hints
}

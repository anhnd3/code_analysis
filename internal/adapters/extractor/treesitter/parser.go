package treesitter

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type GoParser struct {
	language *tree_sitter.Language
}

func NewGoParser() *GoParser {
	return &GoParser{
		language: tree_sitter.NewLanguage(tree_sitter_go.Language()),
	}
}

func (p *GoParser) Parse(content []byte) (*tree_sitter.Tree, error) {
	parser := tree_sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(p.language); err != nil {
		return nil, err
	}

	return parser.Parse(content, nil), nil
}

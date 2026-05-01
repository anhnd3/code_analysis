package treesitter

import (
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type LanguageParser struct {
	language *tree_sitter.Language
}

func newParser(ptr unsafe.Pointer) *LanguageParser {
	return &LanguageParser{language: tree_sitter.NewLanguage(ptr)}
}

func (p *LanguageParser) Parse(content []byte) (*tree_sitter.Tree, error) {
	parser := tree_sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(p.language); err != nil {
		return nil, err
	}

	return parser.Parse(content, nil), nil
}

type GoParser struct {
	*LanguageParser
}

func NewGoParser() *GoParser {
	return &GoParser{LanguageParser: newParser(tree_sitter_go.Language())}
}

type PythonParser struct {
	*LanguageParser
}

func NewPythonParser() *PythonParser {
	return &PythonParser{LanguageParser: newParser(tree_sitter_python.Language())}
}

type JavaScriptParser struct {
	*LanguageParser
}

func NewJavaScriptParser() *JavaScriptParser {
	return &JavaScriptParser{LanguageParser: newParser(tree_sitter_javascript.Language())}
}

type TypeScriptParser struct {
	*LanguageParser
}

func NewTypeScriptParser() *TypeScriptParser {
	return &TypeScriptParser{LanguageParser: newParser(tree_sitter_typescript.LanguageTypescript())}
}

type TSXParser struct {
	*LanguageParser
}

func NewTSXParser() *TSXParser {
	return &TSXParser{LanguageParser: newParser(tree_sitter_typescript.LanguageTSX())}
}

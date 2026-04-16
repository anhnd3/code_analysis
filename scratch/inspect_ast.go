package main

import (
	"fmt"
	"os"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func main() {
	content, _ := os.ReadFile("testdata/workspaces/gin_route_closure/main.go")
	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(tree_sitter_go.Language()))
	tree := parser.Parse(content, nil)
	
	walk(tree.RootNode(), 0, content)
}

func walk(n *tree_sitter.Node, depth int, content []byte) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	
	field := ""
	// In the newer go-tree-sitter API, Parent().FieldNameForChild(i) or similar might be needed
    // But we just want to see the node kind and text for call_expressions
	
	if n.Kind() == "call_expression" {
		fmt.Printf("%s%s [%s]\n", indent, n.Kind(), string(content[n.StartByte():n.EndByte()]))
		for i := 0; i < int(n.ChildCount()); i++ {
			c := n.Child(uint(i))
			fmt.Printf("%s  child %d: %s (field: %s)\n", indent, i, c.Kind(), n.FieldNameForChild(uint(i)))
		}
	}
	
	for i := 0; i < int(n.ChildCount()); i++ {
		walk(n.Child(uint(i)), depth+1, content)
	}
}

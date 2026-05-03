package indexer

import (
	"os"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestDebugNodeTypeInStructField(t *testing.T) {
	workspace := t.TempDir()
	path := workspace + "/handler.go"
	data := []byte(`package handler

import "example.com/demo/repo"

type handler struct {
	repo    *repo.CameraRepo
}
`)
	os.WriteFile(path, data, 0o644)
	path2 := workspace + "/go.mod"
	os.WriteFile(path2, []byte("module example.com/demo\n\ngo 1.22\n"), 0o644)

	parser := NewGoParser()
	tree, _ := parser.Parse(data)
	defer tree.Close()

	root := tree.RootNode()
	var printAll func(n *tree_sitter.Node, indent string)
	printAll = func(n *tree_sitter.Node, indent string) {
		text := n.Utf8Text(data)
		t.Logf("%s%s %q", indent, n.Kind(), text)
		for i := uint(0); i < n.NamedChildCount(); i++ {
			printAll(n.NamedChild(i), indent+"  ")
		}
	}

	// Find the struct type and print its children
	goWalk(root, func(node *tree_sitter.Node) {
		if node != nil && node.Kind() == "struct_type" {
			t.Log("=== STRUCT TYPE ===")
			printAll(node, "")
		}
	})
}

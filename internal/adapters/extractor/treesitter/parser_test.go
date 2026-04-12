package treesitter

import "testing"

func TestGoParserParsesSource(t *testing.T) {
	parser := NewGoParser()
	tree, err := parser.Parse([]byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		t.Fatal("expected root node")
	}
	if root.Kind() != "source_file" {
		t.Fatalf("expected source_file root, got %q", root.Kind())
	}
}

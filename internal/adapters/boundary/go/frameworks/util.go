package frameworks

import (
	"strings"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func walk(n *tree_sitter.Node, fn func(*tree_sitter.Node) bool) {
	if n == nil {
		return
	}
	if !fn(n) {
		return
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		walk(n.Child(uint(i)), fn)
	}
}

func isStringLiteral(n *tree_sitter.Node) bool {
	if n == nil {
		return false
	}
	k := n.Kind()
	return k == "interpreted_string_literal" || k == "raw_string_literal" || k == "string_literal"
}

func getStringValue(n *tree_sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	s := string(content[n.StartByte():n.EndByte()])
	if strings.HasPrefix(s, "`") {
		return strings.Trim(s, "`")
	}
	return strings.Trim(s, "\"")
}

func cleanPath(p string) string {
	p = strings.ReplaceAll(p, "//", "/")
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}


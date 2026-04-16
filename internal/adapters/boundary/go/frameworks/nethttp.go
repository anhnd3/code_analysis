package frameworks

import (
	"analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type NetHTTPDetector struct{}

func NewNetHTTPDetector() *NetHTTPDetector {
	return &NetHTTPDetector{}
}

func (d *NetHTTPDetector) Name() string {
	return "net/http"
}

func (d *NetHTTPDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	walk(file.Root, func(n *tree_sitter.Node) bool {
		if n.Kind() == "call_expression" {
			root := d.handleCall(n, file.Content)
			if root != nil {
				root.SourceFile = file.Path
				roots = append(roots, *root)
			}
		}
		return true
	})

	return roots, diags
}

func (d *NetHTTPDetector) handleCall(n *tree_sitter.Node, content []byte) *boundaryroot.Root {
	fn := n.ChildByFieldName("function")
	if fn == nil || fn.Kind() != "selector_expression" {
		return nil
	}

	field := fn.ChildByFieldName("field")
	if field == nil {
		return nil
	}
	name := string(content[field.StartByte():field.EndByte()])

	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil
	}

	pathArg := args.NamedChild(0)
	handlerArg := args.NamedChild(args.NamedChildCount() - 1)

	if !isStringLiteral(pathArg) {
		return nil
	}

	if name == "HandleFunc" || name == "Handle" {
		path := getStringValue(pathArg, content)
		handlerName := string(content[handlerArg.StartByte():handlerArg.EndByte()])

		return &boundaryroot.Root{
			ID:            fmt.Sprintf("nethttp:%s", path),
			Kind:          boundaryroot.KindHTTP,
			Framework:     "net/http",
			Method:        "ANY",
			Path:          path,
			CanonicalName: fmt.Sprintf("HTTP %s", path),
			HandlerTarget: handlerName,
			SourceExpr:    string(content[n.StartByte():n.EndByte()]),
			Confidence:    "medium",
		}
	}
	return nil
}

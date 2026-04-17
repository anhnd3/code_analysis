package frameworks

import (
	"analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type GinDetector struct{}

func NewGinDetector() *GinDetector {
	return &GinDetector{}
}

func (d *GinDetector) Name() string {
	return "gin"
}

func (d *GinDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	// map from variable name to calculated path prefix
	groups := make(map[string]string)

	methods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"DELETE":  true,
		"PATCH":   true,
		"HEAD":    true,
		"OPTIONS": true,
		"Any":     true,
	}

	walk(file.Root, func(n *tree_sitter.Node) bool {
		// 1. Detect Group assignments: v1 := r.Group("/v1")
		if n.Kind() == "short_variable_declaration" || n.Kind() == "assignment_statement" {
			d.handleAssignment(n, file.Content, groups)
		}

		// 2. Detect Method calls: r.GET("/path", handler)
		if n.Kind() == "call_expression" {
			root := d.handleCall(n, file.Content, groups, methods, symbols)
			if root != nil {
				root.SourceFile = file.Path
				roots = append(roots, *root)
			}
		}

		return true
	})

	return roots, diags
}

func (d *GinDetector) handleAssignment(n *tree_sitter.Node, content []byte, groups map[string]string) {
	var right *tree_sitter.Node
	var leftVar string

	if n.Kind() == "short_variable_declaration" {
		left := n.ChildByFieldName("left")
		rightList := n.ChildByFieldName("right")
		if left != nil && rightList != nil && left.NamedChildCount() > 0 && rightList.NamedChildCount() > 0 {
			leftVar = string(content[left.NamedChild(0).StartByte():left.NamedChild(0).EndByte()])
			right = rightList.NamedChild(0)
		}
	}

	if right != nil && right.Kind() == "call_expression" {
		fn := right.ChildByFieldName("function")
		if fn != nil && fn.Kind() == "selector_expression" {
			field := fn.ChildByFieldName("field")
			if field != nil && string(content[field.StartByte():field.EndByte()]) == "Group" {
				operand := fn.ChildByFieldName("operand")
				parentPrefix := ""
				if operand != nil {
					parentVar := string(content[operand.StartByte():operand.EndByte()])
					parentPrefix = groups[parentVar]
				}

				args := right.ChildByFieldName("arguments")
				if args != nil && args.NamedChildCount() > 0 {
					pathArg := args.NamedChild(0)
					if isStringLiteral(pathArg) {
						path := getStringValue(pathArg, content)
						groups[leftVar] = cleanPath(parentPrefix + "/" + path)
					}
				}
			}
		}
	}
}

func (d *GinDetector) handleCall(n *tree_sitter.Node, content []byte, groups map[string]string, methods map[string]bool, symbols []symbol.Symbol) *boundaryroot.Root {
	fn := n.ChildByFieldName("function")
	if fn == nil || fn.Kind() != "selector_expression" {
		return nil
	}

	field := fn.ChildByFieldName("field")
	if field == nil {
		return nil
	}
	method := string(content[field.StartByte():field.EndByte()])
	if !methods[method] {
		return nil
	}

	operand := fn.ChildByFieldName("operand")
	prefix := ""
	if operand != nil {
		prefix = groups[string(content[operand.StartByte():operand.EndByte()])]
	}

	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil
	}

	pathArg := args.NamedChild(0)
	handlerArg := args.NamedChild(args.NamedChildCount() - 1)

	// Accept string literals as-is; use a tagged placeholder for dynamic expressions
	// (e.g. fmt.Sprintf("/v1/%s", id)) so routes are not silently dropped.
	var path string
	confidence := "high"
	if isStringLiteral(pathArg) {
		path = getStringValue(pathArg, content)
	} else {
		path = fmt.Sprintf("<dynamic:%s>", string(content[pathArg.StartByte():pathArg.EndByte()]))
		confidence = "medium" // Lower confidence for dynamic paths
	}
	fullPath := cleanPath(prefix + "/" + path)

	// Resolve handler target.
	// Priority 1: anonymous function / closure matched by symbol location.
	// Priority 2: wrapper/factory call_expression — use the outermost callee name.
	// Priority 3: raw source text.
	handlerTarget := resolveHandlerTarget(handlerArg, content, symbols)

	return &boundaryroot.Root{
		ID:            fmt.Sprintf("gin:%s:%s", method, fullPath),
		Kind:          boundaryroot.KindHTTP,
		Framework:     "gin",
		Method:        method,
		Path:          fullPath,
		CanonicalName: fmt.Sprintf("%s %s", method, fullPath),
		HandlerTarget: handlerTarget,
		SourceExpr:    string(content[n.StartByte():n.EndByte()]),
		Confidence:    confidence,
	}
}

// resolveHandlerTarget extracts the most meaningful identifier from a handler argument node.
//
//   - Closure literal → look up by symbol location, fall back to raw text.
//   - Wrapper / factory call (e.g. auth.Required(h)) → use the callee function name so
//     the graph can trace execution into the wrapper.
//   - Bare identifier / selector → return raw text.
func resolveHandlerTarget(handlerArg *tree_sitter.Node, content []byte, symbols []symbol.Symbol) string {
	if handlerArg == nil {
		return ""
	}

	// Priority 1: symbol table lookup for closures.
	for _, sym := range symbols {
		if sym.Location.StartLine == uint32(handlerArg.StartPosition().Row+1) &&
			sym.Location.StartCol == uint32(handlerArg.StartPosition().Column+1) {
			return string(sym.ID)
		}
	}

	// Priority 2: if the argument is itself a call expression, use its callee name.
	// This handles middleware wrappers like auth.Required(myHandler) or MakeHandler().
	if handlerArg.Kind() == "call_expression" {
		fn := handlerArg.ChildByFieldName("function")
		if fn != nil {
			switch fn.Kind() {
			case "selector_expression":
				// e.g. auth.Required → use fully-qualified name
				return string(content[fn.StartByte():fn.EndByte()])
			case "identifier":
				return string(content[fn.StartByte():fn.EndByte()])
			}
		}
	}

	// Priority 3: raw text.
	return string(content[handlerArg.StartByte():handlerArg.EndByte()])
}



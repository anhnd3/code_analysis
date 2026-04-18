package frameworks

import (
	"analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type GRPCGatewayDetector struct{}

func NewGRPCGatewayDetector() *GRPCGatewayDetector {
	return &GRPCGatewayDetector{}
}

func (d *GRPCGatewayDetector) Name() string {
	return "grpc-gateway"
}

func (d *GRPCGatewayDetector) DetectBoundaries(file boundary.ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	var roots []boundaryroot.Root
	var diags []symbol.Diagnostic

	walk(file.Root, func(n *tree_sitter.Node) bool {
		if n.Kind() == "call_expression" {
			root := d.handleCall(n, file.Content)
			if root != nil {
				root.RepositoryID = file.RepositoryID
				root.SourceFile = file.Path
				root.SourceStartByte = uint32(n.StartByte())
				root.SourceEndByte = uint32(n.EndByte())
				root.ID = boundaryroot.StableID(*root)
				roots = append(roots, *root)
			}
		}
		return true
	})

	return roots, diags
}

func (d *GRPCGatewayDetector) handleCall(n *tree_sitter.Node, content []byte) *boundaryroot.Root {
	fn := n.ChildByFieldName("function")
	if fn == nil {
		return nil
	}

	var name string
	if fn.Kind() == "selector_expression" {
		field := fn.ChildByFieldName("field")
		if field != nil {
			name = string(content[field.StartByte():field.EndByte()])
		}
	} else if fn.Kind() == "identifier" {
		name = string(content[fn.StartByte():fn.EndByte()])
	}

	// grpc-gateway common patterns: RegisterXXXHandlerFromEndpoint or RegisterXXXHandler
	if !strings.HasPrefix(name, "Register") || !strings.Contains(name, "Handler") {
		return nil
	}

	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() < 2 {
		return nil
	}

	// For grpc-gateway, the "handler" is effectively the gRPC service being proxied.
	// The actual HTTP paths are defined in the .proto file, which we don't see here.
	// But we can mark this as a gateway entrypoint.

	return &boundaryroot.Root{
		ID:            fmt.Sprintf("grpc-gateway:%s", name),
		Kind:          boundaryroot.KindHTTPGateway,
		Framework:     "grpc-gateway",
		Method:        "PROXY",
		Path:          "mapped-from-proto",
		CanonicalName: name,
		HandlerTarget: "grpc-service", // Placeholder
		SourceExpr:    string(content[n.StartByte():n.EndByte()]),
		Confidence:    "low",
	}
}

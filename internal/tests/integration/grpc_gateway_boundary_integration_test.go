package integration

import (
	"testing"

	"analysis-module/internal/domain/graph"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
)

func TestGRPCGatewayRegistrationPersistsProxyRoots(t *testing.T) {
	app := newGraphTestApplication(t)

	result, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "grpc_gateway_registration"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	foundEndpoint := false
	foundBoundaryEdge := false
	for _, node := range result.Snapshot.Nodes {
		if node.Kind == graph.NodeEndpoint && node.CanonicalName == "PROXY RegisterUsersHandlerFromEndpoint" {
			foundEndpoint = true
		}
	}
	for _, edge := range result.Snapshot.Edges {
		if edge.Kind != graph.EdgeRegistersBoundary {
			continue
		}
		if edge.From == "" || edge.To == "" {
			t.Fatalf("expected non-empty boundary edge endpoints, got %+v", edge)
		}
		foundBoundaryEdge = true
	}

	if !foundEndpoint {
		t.Fatal("expected grpc-gateway endpoint node to persist")
	}
	if !foundBoundaryEdge {
		t.Fatal("expected grpc-gateway REGISTERS_BOUNDARY edge to persist")
	}
}

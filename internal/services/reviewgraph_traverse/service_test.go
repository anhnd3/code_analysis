package reviewgraph_traverse

import (
	"testing"

	"analysis-module/internal/domain/reviewgraph"
)

func TestTraverseFollowsCrossServiceCalls(t *testing.T) {
	service := New()
	graph := service.BuildGraph(
		[]reviewgraph.Node{
			{ID: "node:api.handle", SnapshotID: "snap", Service: "api", Kind: reviewgraph.NodeFunction, Symbol: "api.Handle", NodeRole: reviewgraph.RoleEntrypoint},
			{ID: "node:worker.process", SnapshotID: "snap", Service: "worker", Kind: reviewgraph.NodeFunction, Symbol: "worker.Process", NodeRole: reviewgraph.RoleNormal},
		},
		[]reviewgraph.Edge{
			{ID: "edge:api->worker", SnapshotID: "snap", SrcID: "node:api.handle", DstID: "node:worker.process", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		},
	)

	result, err := service.Traverse(graph, "node:api.handle", reviewgraph.TraversalFullFlow, false, 0, 0, reviewgraph.DefaultTraversalCaps())
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.CoveredNodeIDs) != 2 {
		t.Fatalf("expected both service nodes to be covered, got %v", result.CoveredNodeIDs)
	}
	if len(result.CrossServices) != 1 || result.CrossServices[0] != "worker" {
		t.Fatalf("expected worker cross-service boundary, got %v", result.CrossServices)
	}
	if len(result.SyncDownstreamPaths) == 0 || len(result.SyncDownstreamPaths[0].NodeIDs) != 2 {
		t.Fatalf("expected stitched downstream path across services, got %+v", result.SyncDownstreamPaths)
	}
}

func TestTraverseContinuesPastCrossServiceHandlerBoundary(t *testing.T) {
	service := New()
	graph := service.BuildGraph(
		[]reviewgraph.Node{
			{ID: "node:api.handle", SnapshotID: "snap", Service: "api", Kind: reviewgraph.NodeFunction, Symbol: "api.Handle", NodeRole: reviewgraph.RoleEntrypoint},
			{ID: "node:scan.predict", SnapshotID: "snap", Service: "scan", Kind: reviewgraph.NodeGRPCMethod, Symbol: "scan.Predict", NodeRole: reviewgraph.RoleBoundary},
			{ID: "node:scan.service.predict", SnapshotID: "snap", Service: "scan", Kind: reviewgraph.NodeFunction, Symbol: "scan.ServicePredict", NodeRole: reviewgraph.RoleNormal},
		},
		[]reviewgraph.Edge{
			{ID: "edge:api->scan.handler", SnapshotID: "snap", SrcID: "node:api.handle", DstID: "node:scan.predict", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
			{ID: "edge:scan.handler->impl", SnapshotID: "snap", SrcID: "node:scan.predict", DstID: "node:scan.service.predict", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		},
	)

	result, err := service.Traverse(graph, "node:api.handle", reviewgraph.TraversalFullFlow, false, 0, 0, reviewgraph.DefaultTraversalCaps())
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.SyncDownstreamPaths) == 0 {
		t.Fatalf("expected downstream paths, got %+v", result)
	}
	path := result.SyncDownstreamPaths[0]
	if len(path.NodeIDs) != 3 {
		t.Fatalf("expected cross-service handler plus implementation to be stitched, got %+v", path)
	}
	if path.NodeIDs[1] != "node:scan.predict" || path.NodeIDs[2] != "node:scan.service.predict" {
		t.Fatalf("expected handler and implementation in downstream path, got %+v", path)
	}
}

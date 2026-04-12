package graphstore

import "analysis-module/internal/domain/graph"

type Store interface {
	SaveSnapshot(snapshot graph.GraphSnapshot) error
	GetSnapshot(snapshotID string) (graph.GraphSnapshot, error)
	GetNodes(snapshotID string) ([]graph.Node, error)
	GetEdges(snapshotID string) ([]graph.Edge, error)
	FindNode(snapshotID, canonicalName string) (graph.Node, error)
}

type Provider interface {
	ForWorkspace(workspaceID string) (Store, error)
}

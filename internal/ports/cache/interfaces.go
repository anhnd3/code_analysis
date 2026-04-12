package cache

import "analysis-module/internal/domain/graph"

type SnapshotCache interface {
	Put(snapshot graph.GraphSnapshot)
	Get(snapshotID string) (graph.GraphSnapshot, bool)
}

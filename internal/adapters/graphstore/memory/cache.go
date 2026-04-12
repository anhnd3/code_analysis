package memory

import (
	"sync"

	"analysis-module/internal/domain/graph"
	cacheport "analysis-module/internal/ports/cache"
)

type SnapshotCache struct {
	mu        sync.RWMutex
	snapshots map[string]graph.GraphSnapshot
}

func NewSnapshotCache() cacheport.SnapshotCache {
	return &SnapshotCache{snapshots: map[string]graph.GraphSnapshot{}}
}

func (c *SnapshotCache) Put(snapshot graph.GraphSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshots[snapshot.ID] = snapshot
}

func (c *SnapshotCache) Get(snapshotID string) (graph.GraphSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snapshot, ok := c.snapshots[snapshotID]
	return snapshot, ok
}

package cache

type SnapshotCache interface {
	Put(snapshot graph.GraphSnapshot)
	Get(snapshotID, ignoreSignature string) (graph.GraphSnapshot, bool)
}

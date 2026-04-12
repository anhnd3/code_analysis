package graph_query

import (
	"container/list"
	"database/sql"

	"analysis-module/internal/domain/graph"
	graphstoreport "analysis-module/internal/ports/graphstore"
	queryport "analysis-module/internal/ports/query"
)

type Service struct {
	stores graphstoreport.Provider
}

func New(stores graphstoreport.Provider) Service {
	return Service{stores: stores}
}

func (s Service) BlastRadius(req queryport.BlastRadiusRequest) (queryport.BlastRadiusResult, error) {
	nodes, edges, target, err := s.load(req.WorkspaceID, req.SnapshotID, req.Target)
	if err != nil {
		return queryport.BlastRadiusResult{}, err
	}
	impacted := bfs(nodes, edges, target.ID, req.MaxDepth, func(node graph.Node) bool { return true })
	return queryport.BlastRadiusResult{SnapshotID: req.SnapshotID, Target: req.Target, Impacted: impacted}, nil
}

func (s Service) ImpactedTests(req queryport.ImpactedTestsRequest) (queryport.ImpactedTestsResult, error) {
	nodes, edges, target, err := s.load(req.WorkspaceID, req.SnapshotID, req.Target)
	if err != nil {
		return queryport.ImpactedTestsResult{}, err
	}
	tests := bfs(nodes, edges, target.ID, req.MaxDepth, func(node graph.Node) bool {
		return node.Kind == graph.NodeTest
	})
	return queryport.ImpactedTestsResult{SnapshotID: req.SnapshotID, Target: req.Target, Tests: tests}, nil
}

func (s Service) load(workspaceID, snapshotID, target string) (map[string]graph.Node, []graph.Edge, graph.Node, error) {
	store, err := s.stores.ForWorkspace(workspaceID)
	if err != nil {
		return nil, nil, graph.Node{}, err
	}
	nodes, err := store.GetNodes(snapshotID)
	if err != nil {
		return nil, nil, graph.Node{}, err
	}
	edges, err := store.GetEdges(snapshotID)
	if err != nil {
		return nil, nil, graph.Node{}, err
	}
	targetNode, err := store.FindNode(snapshotID, target)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, graph.Node{}, err
		}
		return nil, nil, graph.Node{}, err
	}
	nodeMap := map[string]graph.Node{}
	for _, node := range nodes {
		nodeMap[node.ID] = node
	}
	return nodeMap, edges, targetNode, nil
}

func bfs(nodes map[string]graph.Node, edges []graph.Edge, start string, maxDepth int, include func(graph.Node) bool) []queryport.ImpactedEntity {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	adj := map[string][]string{}
	for _, edge := range edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		adj[edge.To] = append(adj[edge.To], edge.From)
	}
	type step struct {
		NodeID string
		Depth  int
		Path   []string
	}
	seen := map[string]bool{start: true}
	queue := list.New()
	queue.PushBack(step{NodeID: start, Depth: 0, Path: []string{start}})
	result := []queryport.ImpactedEntity{}
	for queue.Len() > 0 {
		raw := queue.Remove(queue.Front()).(step)
		if raw.Depth > maxDepth {
			continue
		}
		node := nodes[raw.NodeID]
		if raw.NodeID != start && include(node) {
			result = append(result, queryport.ImpactedEntity{
				Node:     node,
				Distance: raw.Depth,
				Path:     graph.Path{NodeIDs: append([]string(nil), raw.Path...)},
				Confidence: graph.Confidence{
					Tier:  graph.ConfidenceInferred,
					Score: 1.0 / float64(raw.Depth+1),
				},
			})
		}
		for _, next := range adj[raw.NodeID] {
			if seen[next] {
				continue
			}
			seen[next] = true
			nextPath := append(append([]string(nil), raw.Path...), next)
			queue.PushBack(step{NodeID: next, Depth: raw.Depth + 1, Path: nextPath})
		}
	}
	return result
}

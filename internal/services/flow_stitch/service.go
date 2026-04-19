package flow_stitch

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
)

// Service turns raw static graph into execution-shaped intra-repo flows.
type Service struct{}

// New creates a flow stitcher.
func New() Service {
	return Service{}
}

// Build walks from each resolved entrypoint through the graph,
// producing ordered execution chains and marking boundary nodes.
func (s Service) Build(snapshot graph.GraphSnapshot, entrypoints entrypoint.Result, inventory repository.Inventory) (flow.Bundle, error) {
	idx := newSnapshotIndex(snapshot)

	var chains []flow.Chain
	var markers []flow.BoundaryMarker

	for _, root := range entrypoints.Roots {
		chain, visited := s.buildRootChain(root, idx)
		if len(chain.Steps) > 0 {
			chains = append(chains, chain)
		}
		markers = append(markers, s.detectBoundaries(visited, idx)...)
	}

	markers = deduplicateMarkers(markers)
	sort.Slice(markers, func(i, j int) bool {
		return markers[i].NodeID < markers[j].NodeID
	})

	return flow.Bundle{
		Chains:          chains,
		BoundaryMarkers: markers,
	}, nil
}

func (s Service) buildRootChain(root entrypoint.Root, idx *snapshotIndex) (flow.Chain, map[string]bool) {
	if root.RootType == entrypoint.RootHTTP {
		if chain, visited, ok := s.walkSemanticSpine(root, idx); ok {
			return chain, visited
		}
	}
	return s.walkLegacyChain(root, idx)
}

func newChain(root entrypoint.Root) flow.Chain {
	return flow.Chain{
		RootNodeID:   root.NodeID,
		RepositoryID: root.RepositoryID,
		ServiceID:    root.ServiceID,
	}
}

// walkLegacyChain preserves the previous deterministic DFS traversal as a fallback.
func (s Service) walkLegacyChain(root entrypoint.Root, idx *snapshotIndex) (flow.Chain, map[string]bool) {
	chain := newChain(root)
	visited := map[string]bool{}

	var stack []string
	stack = append(stack, root.NodeID)

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[current] {
			continue
		}
		visited[current] = true

		outgoing := idx.outgoingLegacy(current)
		sort.Slice(outgoing, func(i, j int) bool {
			wi := edgeOrderWeight(outgoing[i].Kind)
			wj := edgeOrderWeight(outgoing[j].Kind)
			if wi != wj {
				return wi < wj
			}
			if outgoing[i].Confidence.Tier != outgoing[j].Confidence.Tier {
				return outgoing[i].Confidence.Tier == graph.ConfidenceConfirmed
			}
			return outgoing[i].To < outgoing[j].To
		})

		for _, edge := range outgoing {
			if visited[edge.To] {
				continue
			}

			chain.Steps = append(chain.Steps, newStep(current, edge, idx))
			stack = append(stack, edge.To)
		}
	}

	return chain, visited
}

// detectBoundaries checks visited nodes for boundary protocol indicators.
func (s Service) detectBoundaries(visited map[string]bool, idx *snapshotIndex) []flow.BoundaryMarker {
	var markers []flow.BoundaryMarker
	for nodeID := range visited {
		node, ok := idx.nodeByID[nodeID]
		if !ok {
			continue
		}
		kind := nodeSymbolKind(node)
		switch kind {
		case string(symbol.KindRouteHandler):
			markers = append(markers, flow.BoundaryMarker{
				NodeID:   nodeID,
				Protocol: "http",
				Role:     "server",
			})
		case string(symbol.KindGRPCHandler):
			markers = append(markers, flow.BoundaryMarker{
				NodeID:   nodeID,
				Protocol: "grpc",
				Role:     "server",
			})
		case string(symbol.KindConsumer):
			markers = append(markers, flow.BoundaryMarker{
				NodeID:   nodeID,
				Protocol: "kafka",
				Role:     "consumer",
			})
		case string(symbol.KindProducer):
			markers = append(markers, flow.BoundaryMarker{
				NodeID:   nodeID,
				Protocol: "kafka",
				Role:     "producer",
			})
		}

		for _, edge := range idx.outgoing[nodeID] {
			switch edge.Kind {
			case graph.EdgeCallsHTTP:
				markers = append(markers, flow.BoundaryMarker{
					NodeID:   nodeID,
					Protocol: "http",
					Role:     "client",
					Detail:   edge.Evidence.Details,
				})
			case graph.EdgeCallsGRPC:
				markers = append(markers, flow.BoundaryMarker{
					NodeID:   nodeID,
					Protocol: "grpc",
					Role:     "client",
					Detail:   edge.Evidence.Details,
				})
			case graph.EdgeProducesTopic:
				markers = append(markers, flow.BoundaryMarker{
					NodeID:   nodeID,
					Protocol: "kafka",
					Role:     "producer",
					Detail:   edge.Evidence.Details,
				})
			case graph.EdgeSubscribesTopic:
				markers = append(markers, flow.BoundaryMarker{
					NodeID:   nodeID,
					Protocol: "kafka",
					Role:     "consumer",
					Detail:   edge.Evidence.Details,
				})
			}
		}
	}
	return markers
}

func newStep(fromNodeID string, edge graph.Edge, idx *snapshotIndex) flow.Step {
	return flow.Step{
		FromNodeID: fromNodeID,
		ToNodeID:   edge.To,
		EdgeID:     edge.ID,
		Kind:       classifyStep(edge, idx),
		Label:      stepLabel(edge, idx),
		Inferred:   edge.Confidence.Tier == graph.ConfidenceInferred,
	}
}

func classifyStep(edge graph.Edge, idx *snapshotIndex) flow.StepKind {
	switch edge.Kind {
	case graph.EdgeCallsHTTP, graph.EdgeCallsGRPC, graph.EdgeProducesTopic, graph.EdgeSubscribesTopic:
		return flow.StepBoundary
	case graph.EdgeRegistersBoundary:
		return flow.StepBoundary
	case graph.EdgeSpawns:
		return flow.StepAsync
	case graph.EdgeDefers:
		return flow.StepDefer
	case graph.EdgeWaitsOn:
		return flow.StepWait
	case graph.EdgeBranches:
		return flow.StepBranch
	case graph.EdgeCalls:
		targetNode, ok := idx.nodeByID[edge.To]
		if ok {
			kind := nodeSymbolKind(targetNode)
			name := nodeShortName(targetNode)
			if isConstructorName(name) || kind == string(symbol.KindStruct) {
				return flow.StepConstruct
			}
		}
		return flow.StepCall
	default:
		return flow.StepCall
	}
}

func stepLabel(edge graph.Edge, idx *snapshotIndex) string {
	switch edge.Kind {
	case graph.EdgeBranches:
		if edge.Evidence.Source != "" {
			return edge.Evidence.Source
		}
	case graph.EdgeSpawns, graph.EdgeDefers, graph.EdgeWaitsOn:
		if edge.Evidence.Source != "" {
			return edge.Evidence.Source
		}
	}

	target, ok := idx.nodeByID[edge.To]
	if ok {
		return nodeShortName(target)
	}
	if strings.HasPrefix(edge.To, "unresolved_") {
		return strings.TrimPrefix(edge.To, "unresolved_")
	}
	return ""
}

func edgeOrderWeight(kind graph.EdgeKind) int {
	switch kind {
	case graph.EdgeRegistersBoundary:
		return 0
	case graph.EdgeCalls, graph.EdgeCallsHTTP, graph.EdgeCallsGRPC, graph.EdgeProducesTopic, graph.EdgeSubscribesTopic, graph.EdgeReturnsHandler:
		return 1
	case graph.EdgeBranches:
		return 2
	case graph.EdgeSpawns:
		return 3
	case graph.EdgeWaitsOn:
		return 4
	case graph.EdgeDefers:
		return 5
	}
	return 10
}

func isConstructorName(name string) bool {
	return strings.HasPrefix(name, "New") || strings.HasPrefix(name, "Create") || strings.HasPrefix(name, "Init") || strings.HasPrefix(name, "Build")
}

func nodeSymbolKind(n graph.Node) string {
	if n.Properties == nil {
		return ""
	}
	return n.Properties["kind"]
}

func nodeShortName(n graph.Node) string {
	if n.Properties != nil {
		if name := n.Properties["name"]; name != "" {
			return name
		}
	}
	idx := strings.LastIndex(n.CanonicalName, ".")
	if idx >= 0 {
		return n.CanonicalName[idx+1:]
	}
	return n.CanonicalName
}

func deduplicateMarkers(markers []flow.BoundaryMarker) []flow.BoundaryMarker {
	type key struct {
		NodeID   string
		Protocol string
		Role     string
	}
	seen := map[key]bool{}
	var out []flow.BoundaryMarker
	for _, m := range markers {
		k := key{m.NodeID, m.Protocol, m.Role}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, m)
	}
	return out
}

type snapshotIndex struct {
	nodeByID map[string]graph.Node
	outgoing map[string][]graph.Edge
}

func newSnapshotIndex(snapshot graph.GraphSnapshot) *snapshotIndex {
	idx := &snapshotIndex{
		nodeByID: make(map[string]graph.Node, len(snapshot.Nodes)),
		outgoing: make(map[string][]graph.Edge),
	}
	for _, n := range snapshot.Nodes {
		idx.nodeByID[n.ID] = n
	}
	for _, e := range snapshot.Edges {
		idx.outgoing[e.From] = append(idx.outgoing[e.From], e)
	}
	return idx
}

func (idx *snapshotIndex) outgoingLegacy(nodeID string) []graph.Edge {
	var edges []graph.Edge
	for _, e := range idx.outgoing[nodeID] {
		switch e.Kind {
		case graph.EdgeCalls,
			graph.EdgeCallsHTTP,
			graph.EdgeCallsGRPC,
			graph.EdgeProducesTopic,
			graph.EdgeSubscribesTopic,
			graph.EdgeReturnsHandler,
			graph.EdgeRegistersBoundary,
			graph.EdgeSpawns,
			graph.EdgeDefers,
			graph.EdgeWaitsOn,
			graph.EdgeBranches:
			edges = append(edges, e)
		}
	}
	return edges
}

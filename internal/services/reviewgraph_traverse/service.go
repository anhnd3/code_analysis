package reviewgraph_traverse

import (
	"fmt"
	"sort"
	"strings"

	"analysis-module/internal/domain/reviewgraph"
)

type GraphData struct {
	Nodes       []reviewgraph.Node
	Edges       []reviewgraph.Edge
	NodeByID    map[string]reviewgraph.Node
	Outgoing    map[string][]reviewgraph.Edge
	Incoming    map[string][]reviewgraph.Edge
	OutgoingAsync map[string][]reviewgraph.Edge
	IncomingAsync map[string][]reviewgraph.Edge
}

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) BuildGraph(nodes []reviewgraph.Node, edges []reviewgraph.Edge) GraphData {
	graph := GraphData{
		Nodes:         append([]reviewgraph.Node(nil), nodes...),
		Edges:         append([]reviewgraph.Edge(nil), edges...),
		NodeByID:      map[string]reviewgraph.Node{},
		Outgoing:      map[string][]reviewgraph.Edge{},
		Incoming:      map[string][]reviewgraph.Edge{},
		OutgoingAsync: map[string][]reviewgraph.Edge{},
		IncomingAsync: map[string][]reviewgraph.Edge{},
	}
	for _, node := range nodes {
		graph.NodeByID[node.ID] = node
	}
	for _, edge := range edges {
		if edge.EdgeType == reviewgraph.EdgeCalls {
			graph.Outgoing[edge.SrcID] = append(graph.Outgoing[edge.SrcID], edge)
			graph.Incoming[edge.DstID] = append(graph.Incoming[edge.DstID], edge)
		}
		if reviewgraph.IsAsyncEdge(edge.EdgeType) {
			graph.OutgoingAsync[edge.SrcID] = append(graph.OutgoingAsync[edge.SrcID], edge)
			graph.IncomingAsync[edge.DstID] = append(graph.IncomingAsync[edge.DstID], edge)
		}
	}
	for key := range graph.Outgoing {
		sortEdges(graph.Outgoing[key], graph.NodeByID, true)
	}
	for key := range graph.Incoming {
		sortEdges(graph.Incoming[key], graph.NodeByID, false)
	}
	for key := range graph.OutgoingAsync {
		sortEdges(graph.OutgoingAsync[key], graph.NodeByID, true)
	}
	for key := range graph.IncomingAsync {
		sortEdges(graph.IncomingAsync[key], graph.NodeByID, false)
	}
	return graph
}

func (s Service) Traverse(graph GraphData, targetID string, mode reviewgraph.TraversalMode, includeAsync bool, forwardDepth, reverseDepth int, caps reviewgraph.TraversalCaps) (reviewgraph.TraversalResult, error) {
	if _, ok := graph.NodeByID[targetID]; !ok {
		return reviewgraph.TraversalResult{}, fmt.Errorf("target node not found: %s", targetID)
	}
	state := traversalState{
		graph:         graph,
		mode:          mode,
		caps:          caps,
		coveredNodes:  map[string]struct{}{},
		coveredEdges:  map[string]struct{}{},
		cycleByKey:    map[string]struct{}{},
		warningsSeen:  map[string]struct{}{},
	}
	_ = state.markNode(targetID)
	upstream := state.walkSync(targetID, false, reverseDepth)
	downstream := state.walkSync(targetID, true, forwardDepth)
	asyncSummaries := []reviewgraph.AsyncBridgeSummary{}
	if includeAsync {
		asyncSummaries = state.expandAsync(targetID, forwardDepth, reverseDepth)
	}

	coveredNodeIDs := mapKeys(state.coveredNodes)
	coveredEdgeIDs := mapKeys(state.coveredEdges)
	affectedFiles := state.affectedFiles()
	crossServices := state.crossServices(targetID)
	sort.Strings(affectedFiles)
	sort.Strings(crossServices)

	return reviewgraph.TraversalResult{
		TargetNodeID:        targetID,
		Mode:                mode,
		SyncUpstreamPaths:   upstream,
		SyncDownstreamPaths: downstream,
		AsyncBridges:        asyncSummaries,
		CoveredNodeIDs:      coveredNodeIDs,
		CoveredEdgeIDs:      coveredEdgeIDs,
		AffectedFiles:       affectedFiles,
		CrossServices:       crossServices,
		Ambiguities:         nil,
		Cycles:              state.cycles,
		TruncationWarnings:  state.warnings,
		Coverage: reviewgraph.CoverageStats{
			CoveredNodeCount: len(coveredNodeIDs),
			CoveredEdgeCount: len(coveredEdgeIDs),
			SharedInfraCount: state.sharedInfraCount(),
		},
	}, nil
}

type traversalState struct {
	graph        GraphData
	mode         reviewgraph.TraversalMode
	caps         reviewgraph.TraversalCaps
	coveredNodes map[string]struct{}
	coveredEdges map[string]struct{}
	cycleByKey   map[string]struct{}
	cycles       []reviewgraph.CycleSummary
	warningsSeen map[string]struct{}
	warnings     []string
}

func (s *traversalState) walkSync(startID string, forward bool, depthLimit int) []reviewgraph.PathSummary {
	paths := []reviewgraph.PathSummary{}
	expanded := map[string]struct{}{}
	var dfs func(nodeID string, path []string, depth int)
	dfs = func(nodeID string, path []string, depth int) {
		if len(paths) >= s.caps.MaxPaths {
			s.warn("max_paths reached during sync traversal")
			paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: "max_paths", Truncated: true})
			return
		}
		if s.mode == reviewgraph.TraversalBounded && depthLimit > 0 && depth >= depthLimit {
			paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: "depth_limit", Truncated: true})
			return
		}
		if reason, done := s.terminalReason(nodeID, startID, forward); done {
			paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: reason})
			return
		}
		if _, ok := expanded[nodeID]; ok && s.mode == reviewgraph.TraversalFullFlow {
			paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: "shared_path"})
			return
		}
		expanded[nodeID] = struct{}{}

		edges := s.graph.Outgoing[nodeID]
		if !forward {
			edges = s.graph.Incoming[nodeID]
		}
		if len(edges) == 0 {
			reason := "leaf"
			if !forward {
				reason = "root"
			}
			paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: reason})
			return
		}
		for _, edge := range edges {
			if !s.markEdge(edge.ID) {
				paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: "max_edges", Truncated: true})
				return
			}
			nextID := edge.DstID
			if !forward {
				nextID = edge.SrcID
			}
			if contains(path, nextID) {
				s.recordCycle(append(append([]string(nil), path...), nextID))
				paths = append(paths, reviewgraph.PathSummary{NodeIDs: append(append([]string(nil), path...), nextID), Direction: directionLabel(forward), TerminalReason: "cycle", Truncated: true})
				continue
			}
			if !s.markNode(nextID) {
				paths = append(paths, reviewgraph.PathSummary{NodeIDs: append([]string(nil), path...), Direction: directionLabel(forward), TerminalReason: "max_nodes", Truncated: true})
				return
			}
			dfs(nextID, append(append([]string(nil), path...), nextID), depth+1)
		}
	}
	dfs(startID, []string{startID}, 0)
	return paths
}

func (s *traversalState) expandAsync(targetID string, forwardDepth, reverseDepth int) []reviewgraph.AsyncBridgeSummary {
	seedNodes := mapKeys(s.coveredNodes)
	sort.Strings(seedNodes)
	bridgeIDs := []string{}
	bridgeSeen := map[string]struct{}{}
	for _, nodeID := range seedNodes {
		for _, edge := range append(append([]reviewgraph.Edge{}, s.graph.OutgoingAsync[nodeID]...), s.graph.IncomingAsync[nodeID]...) {
			bridgeID := s.bridgeNodeID(edge)
			if bridgeID == "" {
				continue
			}
			if _, ok := bridgeSeen[bridgeID]; ok {
				continue
			}
			bridgeSeen[bridgeID] = struct{}{}
			bridgeIDs = append(bridgeIDs, bridgeID)
		}
	}
	sort.Strings(bridgeIDs)
	summaries := []reviewgraph.AsyncBridgeSummary{}
	for _, bridgeID := range bridgeIDs {
		bridgeNode := s.graph.NodeByID[bridgeID]
		if !s.markNode(bridgeID) {
			break
		}
		producerEdges := []reviewgraph.Edge{}
		consumerEdges := []reviewgraph.Edge{}
		for _, edge := range s.graph.IncomingAsync[bridgeID] {
			switch edge.EdgeType {
			case reviewgraph.EdgeEmitsEvent, reviewgraph.EdgePublishesMessage, reviewgraph.EdgeEnqueuesJob, reviewgraph.EdgeSchedulesTask, reviewgraph.EdgeSpawnsAsync, reviewgraph.EdgeSendsToChannel:
				producerEdges = append(producerEdges, edge)
			}
		}
		for _, edge := range s.graph.OutgoingAsync[bridgeID] {
			switch edge.EdgeType {
			case reviewgraph.EdgeConsumesEvent, reviewgraph.EdgeSubscribesMessage, reviewgraph.EdgeDequeuesJob, reviewgraph.EdgeRunsAsync, reviewgraph.EdgeReceivesFromChannel:
				consumerEdges = append(consumerEdges, edge)
			}
		}
		sortEdges(producerEdges, s.graph.NodeByID, false)
		sortEdges(consumerEdges, s.graph.NodeByID, true)
		summary := reviewgraph.AsyncBridgeSummary{
			BridgeNodeID:      bridgeID,
			BridgeDisplayName: bridgeNode.Symbol,
			BridgeKind:        bridgeNode.Kind,
			Transport:         transportForBridge(producerEdges, consumerEdges, bridgeNode),
			TopicOrChannel:    bridgeNode.Symbol,
			ProducerCount:     len(producerEdges),
			ConsumerCount:     len(consumerEdges),
		}
		for _, edge := range limitAsyncEdges(producerEdges, s.caps.MaxAsyncFanoutPerBridge) {
			_ = s.markEdge(edge.ID)
			participant := s.graph.NodeByID[edge.SrcID]
			summary.Producers = append(summary.Producers, reviewgraph.AsyncParticipant{
				NodeID:      participant.ID,
				Service:     participant.Service,
				DisplayName: participant.Symbol,
			})
			if participant.ID != targetID {
				summary.UpstreamSyncPaths = append(summary.UpstreamSyncPaths, s.walkSync(participant.ID, false, reverseDepth)...)
			}
		}
		for _, edge := range limitAsyncEdges(consumerEdges, s.caps.MaxAsyncFanoutPerBridge) {
			_ = s.markEdge(edge.ID)
			participant := s.graph.NodeByID[edge.DstID]
			summary.Consumers = append(summary.Consumers, reviewgraph.AsyncParticipant{
				NodeID:      participant.ID,
				Service:     participant.Service,
				DisplayName: participant.Symbol,
			})
			if participant.ID != targetID {
				summary.DownstreamSyncPaths = append(summary.DownstreamSyncPaths, s.walkSync(participant.ID, true, forwardDepth)...)
			}
		}
		if len(producerEdges) > s.caps.MaxAsyncFanoutPerBridge || len(consumerEdges) > s.caps.MaxAsyncFanoutPerBridge {
			summary.FanoutTruncated = true
			s.warn("max_async_fanout_per_bridge reached during async traversal")
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func (s *traversalState) terminalReason(nodeID, startID string, forward bool) (string, bool) {
	if nodeID == startID {
		return "", false
	}
	node := s.graph.NodeByID[nodeID]
	if !forward && (node.NodeRole == reviewgraph.RoleEntrypoint || node.NodeRole == reviewgraph.RoleBoundary || node.Kind == reviewgraph.NodeService) {
		return string(nodeRoleOrDefault(node)), true
	}
	if forward && (node.NodeRole == reviewgraph.RoleBoundary || node.Kind == reviewgraph.NodeService) {
		return string(nodeRoleOrDefault(node)), true
	}
	return "", false
}

func (s *traversalState) markNode(nodeID string) bool {
	if _, ok := s.coveredNodes[nodeID]; ok {
		return true
	}
	if len(s.coveredNodes) >= s.caps.MaxNodes {
		s.warn("max_nodes reached during traversal")
		return false
	}
	s.coveredNodes[nodeID] = struct{}{}
	return true
}

func (s *traversalState) markEdge(edgeID string) bool {
	if _, ok := s.coveredEdges[edgeID]; ok {
		return true
	}
	if len(s.coveredEdges) >= s.caps.MaxEdges {
		s.warn("max_edges reached during traversal")
		return false
	}
	s.coveredEdges[edgeID] = struct{}{}
	return true
}

func (s *traversalState) recordCycle(path []string) {
	key := strings.Join(path, "->")
	if _, ok := s.cycleByKey[key]; ok {
		return
	}
	s.cycleByKey[key] = struct{}{}
	cycle := reviewgraph.CycleSummary{Path: path}
	services := map[string]struct{}{}
	asyncBoundary := false
	for _, nodeID := range path {
		node := s.graph.NodeByID[nodeID]
		if node.Service != "" {
			services[node.Service] = struct{}{}
		}
		switch node.Kind {
		case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
			asyncBoundary = true
		}
	}
	cycle.CrossService = len(services) > 1
	cycle.CrossAsyncBoundary = asyncBoundary
	s.cycles = append(s.cycles, cycle)
	if cycle.CrossService || cycle.CrossAsyncBoundary {
		s.warn("possible loop risk detected across service or async boundary")
	}
}

func (s *traversalState) warn(message string) {
	if _, ok := s.warningsSeen[message]; ok {
		return
	}
	s.warningsSeen[message] = struct{}{}
	s.warnings = append(s.warnings, message)
}

func (s *traversalState) bridgeNodeID(edge reviewgraph.Edge) string {
	src := s.graph.NodeByID[edge.SrcID]
	dst := s.graph.NodeByID[edge.DstID]
	if isBridgeKind(src.Kind) {
		return src.ID
	}
	if isBridgeKind(dst.Kind) {
		return dst.ID
	}
	return ""
}

func (s *traversalState) affectedFiles() []string {
	files := map[string]struct{}{}
	for nodeID := range s.coveredNodes {
		node := s.graph.NodeByID[nodeID]
		if node.FilePath != "" {
			files[node.FilePath] = struct{}{}
		}
	}
	return mapKeys(files)
}

func (s *traversalState) crossServices(targetID string) []string {
	targetService := s.graph.NodeByID[targetID].Service
	services := map[string]struct{}{}
	for nodeID := range s.coveredNodes {
		service := s.graph.NodeByID[nodeID].Service
		if service == "" || service == targetService {
			continue
		}
		services[service] = struct{}{}
	}
	return mapKeys(services)
}

func (s *traversalState) sharedInfraCount() int {
	count := 0
	for nodeID := range s.coveredNodes {
		if s.graph.NodeByID[nodeID].NodeRole == reviewgraph.RoleSharedInfra {
			count++
		}
	}
	return count
}

func sortEdges(edges []reviewgraph.Edge, nodeByID map[string]reviewgraph.Node, forward bool) {
	sort.Slice(edges, func(i, j int) bool {
		leftID := edges[i].DstID
		rightID := edges[j].DstID
		if !forward {
			leftID = edges[i].SrcID
			rightID = edges[j].SrcID
		}
		left := nodeByID[leftID].Symbol
		right := nodeByID[rightID].Symbol
		if left != right {
			return left < right
		}
		return edges[i].ID < edges[j].ID
	})
}

func directionLabel(forward bool) string {
	if forward {
		return "downstream"
	}
	return "upstream"
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isBridgeKind(kind reviewgraph.NodeKind) bool {
	switch kind {
	case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
		return true
	default:
		return false
	}
}

func limitAsyncEdges(edges []reviewgraph.Edge, limit int) []reviewgraph.Edge {
	if limit <= 0 || len(edges) <= limit {
		return edges
	}
	return edges[:limit]
}

func transportForBridge(producers, consumers []reviewgraph.Edge, bridge reviewgraph.Node) string {
	for _, edge := range append(append([]reviewgraph.Edge{}, producers...), consumers...) {
		if edge.Transport != "" {
			return edge.Transport
		}
	}
	switch bridge.Kind {
	case reviewgraph.NodeAsyncTask:
		return "inproc_async"
	case reviewgraph.NodeInProcChannel:
		return "inproc_channel"
	}
	return ""
}

func mapKeys[T comparable](values map[T]struct{}) []T {
	result := make([]T, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

func nodeRoleOrDefault(node reviewgraph.Node) reviewgraph.NodeRole {
	if node.NodeRole == "" {
		return reviewgraph.RoleNormal
	}
	return node.NodeRole
}

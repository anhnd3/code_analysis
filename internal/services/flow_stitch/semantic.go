package flow_stitch

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
)

type EntryMode string

const (
	EntryModeEndpointRoot          EntryMode = "endpoint_root"
	EntryModeHandlerSymbolFallback EntryMode = "handler_symbol_fallback"
)

type SemanticAudit struct {
	WorkspaceID string              `json:"workspace_id"`
	SnapshotID  string              `json:"snapshot_id"`
	Roots       []SemanticAuditRoot `json:"roots"`
}

type SemanticAuditRoot struct {
	RootNodeID            string                 `json:"root_node_id"`
	RootCanonical         string                 `json:"root_canonical"`
	EntryMode             EntryMode              `json:"entry_mode"`
	RegistersBoundaryEdge *SemanticAuditEdgeRef  `json:"registers_boundary_edge,omitempty"`
	HandlerTargetNode     *SemanticAuditNodeRef  `json:"handler_target_node,omitempty"`
	ReturnsHandlerEdge    *SemanticAuditEdgeRef  `json:"returns_handler_edge,omitempty"`
	ClosureBodyNode       *SemanticAuditNodeRef  `json:"closure_body_node,omitempty"`
	FirstBusinessCalls    []SemanticAuditEdgeRef `json:"first_business_calls"`
	SideEdges             []SemanticAuditEdgeRef `json:"side_edges"`
	Warnings              []SemanticAuditWarning `json:"warnings"`
}

type SemanticAuditEdgeRef struct {
	EdgeID          string `json:"edge_id"`
	Kind            string `json:"kind"`
	FromNodeID      string `json:"from_node_id"`
	ToNodeID        string `json:"to_node_id"`
	Label           string `json:"label,omitempty"`
	Inferred        bool   `json:"inferred,omitempty"`
	ResolutionBasis string `json:"resolution_basis,omitempty"`
}

type SemanticAuditNodeRef struct {
	NodeID        string `json:"node_id"`
	CanonicalName string `json:"canonical_name"`
	SymbolKind    string `json:"symbol_kind,omitempty"`
	ShortName     string `json:"short_name,omitempty"`
}

type SemanticAuditWarning struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type targetBucket int

const (
	targetBucketObservabilityNoise targetBucket = -2
	targetBucketWrapperNoise       targetBucket = -1
	targetBucketNeutral            targetBucket = 0
	targetBucketSetup              targetBucket = 1
	targetBucketStrongBusiness     targetBucket = 2
)

var strongBusinessTokens = map[string]bool{
	"repo": true, "repository": true, "service": true, "processor": true, "gateway": true,
	"client": true, "store": true, "session": true, "auth": true, "dao": true, "domain": true,
}

var setupTokens = map[string]bool{
	"new": true, "create": true, "build": true, "init": true, "factory": true, "config": true, "option": true,
}

var wrapperNoiseTokens = map[string]bool{
	"wrap": true, "wrapper": true, "respond": true, "response": true, "render": true,
	"write": true, "json": true, "helper": true, "util": true,
}

var observabilityNoiseTokens = map[string]bool{
	"trace": true, "tracing": true, "log": true, "logger": true, "metric": true, "metrics": true,
	"prom": true, "telemetry": true, "span": true,
}

type semanticEdgeScore struct {
	total      int
	hasOrder   bool
	orderIndex int
	confirmed  bool
	canonical  string
	nodeID     string
}

const (
	semanticBeamWidth = 3
	semanticMaxDepth  = 4
)

type semanticBeam struct {
	current string
	path    []graph.Edge
	score   int
	seen    map[string]bool
}

type semanticSearchResult struct {
	path           []graph.Edge
	blockedByNoise bool
	anchorNodeID   string
}

// walkSemanticSpine keeps deterministic entry handling for boundary registration
// and returned handler peeling. Only post-body CALLS are recovered via bounded
// beam search; side edges remain structural annotations.
func (s Service) walkSemanticSpine(root entrypoint.Root, idx *snapshotIndex) (flow.Chain, map[string]bool, bool) {
	chain := newChain(root)
	visited := map[string]bool{root.NodeID: true}
	seenEdgeIDs := map[string]bool{}
	entryMode := idx.rootEntryMode(root.NodeID)

	current := root.NodeID

	if entryMode == EntryModeEndpointRoot {
		registerEdge, ok := idx.bestEdgeByKind(root.NodeID, graph.EdgeRegistersBoundary)
		if !ok {
			return flow.Chain{}, visited, false
		}
		appendSemanticStep(&chain, registerEdge, current, idx, visited, seenEdgeIDs)
		current = registerEdge.To
	} else if !idx.hasEligibleSemanticEdge(root.NodeID) {
		return flow.Chain{}, visited, false
	}

	if returnsEdge, ok := idx.bestEdgeByKind(current, graph.EdgeReturnsHandler); ok {
		appendSemanticStep(&chain, returnsEdge, current, idx, visited, seenEdgeIDs)
		appendNodeSideSteps(&chain, current, idx, visited, seenEdgeIDs)
		current = returnsEdge.To
	}

	search := idx.searchCallBeam(current)
	if len(search.path) == 0 {
		appendNodeSideSteps(&chain, current, idx, visited, seenEdgeIDs)
	}
	fromNodeID := current
	for _, edge := range search.path {
		appendSemanticStep(&chain, edge, fromNodeID, idx, visited, seenEdgeIDs)
		appendNodeSideSteps(&chain, fromNodeID, idx, visited, seenEdgeIDs)
		fromNodeID = edge.To
	}

	if s.shouldFallbackToLegacy(root, entryMode, chain, idx) {
		return flow.Chain{}, visited, false
	}
	return chain, visited, true
}

func appendSemanticStep(chain *flow.Chain, edge graph.Edge, fromNodeID string, idx *snapshotIndex, visited map[string]bool, seenEdgeIDs map[string]bool) {
	if seenEdgeIDs[edge.ID] {
		return
	}
	chain.Steps = append(chain.Steps, newStep(fromNodeID, edge, idx))
	visited[edge.To] = true
	seenEdgeIDs[edge.ID] = true
}

func appendSemanticSideSteps(chain *flow.Chain, edges []graph.Edge, fromNodeID string, idx *snapshotIndex, visited map[string]bool, seenEdgeIDs map[string]bool) {
	for _, edge := range edges {
		appendSemanticStep(chain, edge, fromNodeID, idx, visited, seenEdgeIDs)
	}
}

func appendNodeSideSteps(chain *flow.Chain, fromNodeID string, idx *snapshotIndex, visited map[string]bool, seenEdgeIDs map[string]bool) {
	appendSemanticSideSteps(chain, idx.sideEdges(fromNodeID), fromNodeID, idx, visited, seenEdgeIDs)
}

func (s Service) shouldFallbackToLegacy(root entrypoint.Root, entryMode EntryMode, chain flow.Chain, idx *snapshotIndex) bool {
	switch {
	case entryMode == EntryModeEndpointRoot && !idx.hasEdgeKind(root.NodeID, graph.EdgeRegistersBoundary):
		return true
	case entryMode == EntryModeHandlerSymbolFallback && !idx.hasEligibleSemanticEdge(root.NodeID):
		return true
	case len(chain.Steps) == 0:
		return true
	case len(chain.Steps) != 1:
		return false
	}

	step := chain.Steps[0]
	if !idx.isUnresolvedNode(step.ToNodeID) && !idx.isNoiseNode(step.ToNodeID) {
		return false
	}
	return !idx.hasReturnsHandlerOrPromotedSyncCalls(step.ToNodeID)
}

func (s Service) BuildAudit(snapshot graph.GraphSnapshot, roots entrypoint.Result, bundle flow.Bundle) SemanticAudit {
	idx := newSnapshotIndex(snapshot)
	chainsByRoot := make(map[string]flow.Chain, len(bundle.Chains))
	for _, chain := range bundle.Chains {
		chainsByRoot[chain.RootNodeID] = chain
	}

	report := SemanticAudit{
		WorkspaceID: snapshot.WorkspaceID,
		SnapshotID:  snapshot.ID,
		Roots:       make([]SemanticAuditRoot, 0, len(roots.Roots)),
	}

	for _, root := range roots.Roots {
		report.Roots = append(report.Roots, idx.buildRootAudit(root, chainsByRoot[root.NodeID]))
	}
	return report
}

func (idx *snapshotIndex) buildRootAudit(root entrypoint.Root, chain flow.Chain) SemanticAuditRoot {
	entryMode := idx.rootEntryMode(root.NodeID)
	audit := SemanticAuditRoot{
		RootNodeID:         root.NodeID,
		RootCanonical:      root.CanonicalName,
		EntryMode:          entryMode,
		FirstBusinessCalls: []SemanticAuditEdgeRef{},
		SideEdges:          []SemanticAuditEdgeRef{},
		Warnings:           []SemanticAuditWarning{},
	}

	handlerNodeID := root.NodeID
	if entryMode == EntryModeEndpointRoot {
		if edge, ok := idx.bestEdgeByKind(root.NodeID, graph.EdgeRegistersBoundary); ok {
			ref := idx.auditEdgeRef(edge)
			audit.RegistersBoundaryEdge = &ref
			handlerNodeID = edge.To
		}
	}
	if handlerRef := idx.auditNodeRef(handlerNodeID); handlerRef != nil {
		audit.HandlerTargetNode = handlerRef
	}
	if audit.HandlerTargetNode != nil && strings.HasPrefix(audit.HandlerTargetNode.NodeID, "unresolved_") && strings.HasPrefix(root.CanonicalName, "PROXY ") {
		audit.Warnings = append(audit.Warnings, SemanticAuditWarning{
			Kind:    "gateway_proxy_external",
			Message: "gateway registration target could not be resolved to an in-repo symbol",
		})
	}

	bodyNodeID := handlerNodeID
	if edge, ok := idx.bestEdgeByKind(handlerNodeID, graph.EdgeReturnsHandler); ok {
		ref := idx.auditEdgeRef(edge)
		audit.ReturnsHandlerEdge = &ref
		bodyNodeID = edge.To
		if bodyRef := idx.auditNodeRef(bodyNodeID); bodyRef != nil {
			audit.ClosureBodyNode = bodyRef
		}
	}

	businessCalls, warnings := idx.firstBusinessCalls(handlerNodeID, bodyNodeID)
	audit.FirstBusinessCalls = businessCalls
	audit.Warnings = append(audit.Warnings, warnings...)

	for _, step := range chain.Steps {
		switch step.Kind {
		case flow.StepBranch, flow.StepAsync, flow.StepWait, flow.StepDefer:
			if edge, ok := idx.edgeByID(step.EdgeID); ok {
				audit.SideEdges = append(audit.SideEdges, idx.auditEdgeRef(edge))
			}
		}
	}
	return audit
}

func (idx *snapshotIndex) firstBusinessCalls(handlerNodeID, startNodeID string) ([]SemanticAuditEdgeRef, []SemanticAuditWarning) {
	calls := []SemanticAuditEdgeRef{}
	warnings := []SemanticAuditWarning{}
	search := idx.refinedAuditBusinessSearch(handlerNodeID, startNodeID)
	anchorNodeID := search.anchorNodeID
	if anchorNodeID == "" {
		anchorNodeID = startNodeID
	}
	if anchorNodeID == "" {
		anchorNodeID = handlerNodeID
	}
	calls = idx.collectFirstBusinessCalls(anchorNodeID, search.path, 3)
	if len(search.path) == 0 {
		if len(calls) == 0 {
			warnings = append(warnings, SemanticAuditWarning{
				Kind:    "no_business_call_after_handler",
				Message: "no promoted business call was found after the handler entry",
			})
		}
		return calls, warnings
	}

	if len(calls) == 0 {
		if search.blockedByNoise {
			warnings = append(warnings, SemanticAuditWarning{
				Kind:    "business_frontier_blocked_by_noise",
				Message: "semantic recovery reached only demoted noise/support calls after the handler entry",
			})
		} else {
			warnings = append(warnings, SemanticAuditWarning{
				Kind:    "no_business_call_after_handler",
				Message: "no promoted business call was found after the handler entry",
			})
		}
	}
	return calls, warnings
}

func (idx *snapshotIndex) collectFirstBusinessCalls(anchorNodeID string, path []graph.Edge, limit int) []SemanticAuditEdgeRef {
	if limit <= 0 {
		return nil
	}

	refs := make([]SemanticAuditEdgeRef, 0, limit)
	seen := map[string]bool{}
	appendEdge := func(edge graph.Edge) {
		if seen[edge.ID] || idx.targetBucket(edge) != targetBucketStrongBusiness {
			return
		}
		seen[edge.ID] = true
		refs = append(refs, idx.auditEdgeRef(edge))
	}

	for _, edge := range idx.anchorBusinessEdges(anchorNodeID) {
		appendEdge(edge)
		if len(refs) == limit {
			return refs
		}
	}

	for _, edge := range path {
		appendEdge(edge)
		if len(refs) == limit {
			return refs
		}
	}

	return refs
}

func (idx *snapshotIndex) anchorBusinessEdges(nodeID string) []graph.Edge {
	if nodeID == "" {
		return nil
	}
	edges := filterEdgesByKind(idx.outgoingSemantic(nodeID), graph.EdgeCalls)
	sort.SliceStable(edges, func(i, j int) bool {
		return idx.compareSemanticEdgeTie(edges[i], edges[j])
	})

	business := make([]graph.Edge, 0, len(edges))
	for _, edge := range edges {
		if idx.targetBucket(edge) == targetBucketStrongBusiness {
			business = append(business, edge)
		}
	}
	return business
}

func (idx *snapshotIndex) rootEntryMode(rootNodeID string) EntryMode {
	node, ok := idx.nodeByID[rootNodeID]
	if ok && node.Kind == graph.NodeEndpoint {
		return EntryModeEndpointRoot
	}
	return EntryModeHandlerSymbolFallback
}

func (idx *snapshotIndex) hasEligibleSemanticEdge(nodeID string) bool {
	return len(idx.outgoingSemantic(nodeID)) > 0
}

func (idx *snapshotIndex) hasEdgeKind(nodeID string, kind graph.EdgeKind) bool {
	for _, edge := range idx.outgoing[nodeID] {
		if edge.Kind == kind {
			return true
		}
	}
	return false
}

func (idx *snapshotIndex) outgoingSemantic(nodeID string) []graph.Edge {
	var edges []graph.Edge
	for _, edge := range idx.outgoing[nodeID] {
		switch edge.Kind {
		case graph.EdgeReturnsHandler, graph.EdgeCalls, graph.EdgeBranches, graph.EdgeSpawns, graph.EdgeWaitsOn, graph.EdgeDefers:
			edges = append(edges, edge)
		}
	}
	return edges
}

func (idx *snapshotIndex) outgoingByKind(nodeID string, kind graph.EdgeKind) []graph.Edge {
	var edges []graph.Edge
	for _, edge := range idx.outgoing[nodeID] {
		if edge.Kind == kind {
			edges = append(edges, edge)
		}
	}
	return edges
}

func (idx *snapshotIndex) bestEdgeByKind(nodeID string, kind graph.EdgeKind) (graph.Edge, bool) {
	return idx.bestSemanticEdge(nodeID, idx.outgoingByKind(nodeID, kind))
}

func (idx *snapshotIndex) refinedAuditBusinessSearch(handlerNodeID, bodyNodeID string) semanticSearchResult {
	anchorNodeID := bodyNodeID
	if anchorNodeID == "" {
		anchorNodeID = handlerNodeID
	}

	primary := idx.searchCallBeam(anchorNodeID)
	primary.anchorNodeID = anchorNodeID
	if idx.pathHasBusiness(primary.path) {
		return primary
	}
	if handlerNodeID == "" || handlerNodeID == anchorNodeID {
		return primary
	}

	// Handler-anchor refinement stays audit-only so it can improve the audit
	// summary without changing the semantic chain selected by walkSemanticSpine.
	fallback := idx.searchCallBeam(handlerNodeID)
	fallback.anchorNodeID = handlerNodeID
	if idx.pathHasBusiness(fallback.path) && !idx.pathHasBusiness(primary.path) {
		return fallback
	}
	if len(fallback.path) > len(primary.path) && primary.blockedByNoise {
		return fallback
	}
	return primary
}

// searchCallBeam explores only CALLS edges after deterministic entry peeling.
// Side edges are excluded from beam expansion and stay in the audit/chain as
// annotations attached to visited nodes.
func (idx *snapshotIndex) searchCallBeam(startNodeID string) semanticSearchResult {
	if startNodeID == "" {
		return semanticSearchResult{}
	}

	beams := []semanticBeam{{
		current: startNodeID,
		seen:    map[string]bool{startNodeID: true},
	}}
	var best semanticBeam
	hasBest := false

	for depth := 0; depth < semanticMaxDepth; depth++ {
		var next []semanticBeam
		for _, beam := range beams {
			callEdges := idx.sortSemanticEdges(filterEdgesByKind(idx.outgoingSemantic(beam.current), graph.EdgeCalls))
			if len(callEdges) == 0 {
				if len(beam.path) > 0 && (!hasBest || idx.compareSemanticBeams(beam, best)) {
					best = beam
					hasBest = true
				}
				continue
			}

			limit := minInt(len(callEdges), semanticBeamWidth)
			for i := 0; i < limit; i++ {
				candidate, ok := idx.extendSemanticBeam(beam, callEdges[i])
				if !ok {
					continue
				}
				if !hasBest || idx.compareSemanticBeams(candidate, best) {
					best = candidate
					hasBest = true
				}
				next = append(next, candidate)
			}
		}

		if len(next) == 0 {
			break
		}

		sort.SliceStable(next, func(i, j int) bool {
			return idx.compareSemanticBeams(next[i], next[j])
		})
		beams = idx.trimSemanticBeams(next, semanticBeamWidth)
	}

	if !hasBest {
		return semanticSearchResult{}
	}
	return semanticSearchResult{
		path:           best.path,
		blockedByNoise: idx.pathBlockedByNoise(best.path),
	}
}

func (idx *snapshotIndex) extendSemanticBeam(beam semanticBeam, edge graph.Edge) (semanticBeam, bool) {
	if beam.seen[edge.To] {
		return semanticBeam{}, false
	}
	nextSeen := make(map[string]bool, len(beam.seen)+1)
	for nodeID := range beam.seen {
		nextSeen[nodeID] = true
	}
	nextSeen[edge.To] = true

	nextPath := append(append([]graph.Edge(nil), beam.path...), edge)
	return semanticBeam{
		current: edge.To,
		path:    nextPath,
		score:   beam.score + idx.callBeamEdgeScore(edge),
		seen:    nextSeen,
	}, true
}

func (idx *snapshotIndex) trimSemanticBeams(candidates []semanticBeam, limit int) []semanticBeam {
	trimmed := make([]semanticBeam, 0, minInt(len(candidates), limit))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := idx.semanticBeamKey(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		trimmed = append(trimmed, candidate)
		if len(trimmed) == limit {
			break
		}
	}
	return trimmed
}

func (idx *snapshotIndex) semanticBeamKey(beam semanticBeam) string {
	if len(beam.path) == 0 {
		return beam.current
	}
	parts := make([]string, 0, len(beam.path)+1)
	parts = append(parts, beam.current)
	for _, edge := range beam.path {
		parts = append(parts, edge.ID)
	}
	return strings.Join(parts, "|")
}

func (idx *snapshotIndex) compareSemanticBeams(left, right semanticBeam) bool {
	switch {
	case left.score != right.score:
		return left.score > right.score
	case idx.compareSemanticBeamPath(left.path, right.path):
		return true
	case idx.compareSemanticBeamPath(right.path, left.path):
		return false
	case len(left.path) != len(right.path):
		return len(left.path) < len(right.path)
	default:
		return left.current < right.current
	}
}

func (idx *snapshotIndex) compareSemanticBeamPath(left, right []graph.Edge) bool {
	limit := minInt(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i].ID == right[i].ID {
			continue
		}
		return idx.compareSemanticEdgeTie(left[i], right[i])
	}
	return len(left) < len(right)
}

func (idx *snapshotIndex) compareSemanticEdgeTie(left, right graph.Edge) bool {
	ls := idx.semanticEdgeScore(left)
	rs := idx.semanticEdgeScore(right)

	switch {
	case ls.hasOrder && rs.hasOrder && ls.orderIndex != rs.orderIndex:
		return ls.orderIndex < rs.orderIndex
	case ls.hasOrder != rs.hasOrder:
		return ls.hasOrder
	case ls.confirmed != rs.confirmed:
		return ls.confirmed
	case ls.canonical != rs.canonical:
		return ls.canonical < rs.canonical
	default:
		return ls.nodeID < rs.nodeID
	}
}

func (idx *snapshotIndex) callBeamEdgeScore(edge graph.Edge) int {
	score := idx.semanticEdgeScore(edge).total - idx.edgeBaseWeight(edge.Kind)
	switch idx.targetBucket(edge) {
	case targetBucketStrongBusiness:
		score += 120
	case targetBucketSetup:
		score += 20
	case targetBucketWrapperNoise, targetBucketObservabilityNoise:
		score -= 120
	}
	if idx.isUnresolvedNode(edge.To) {
		score -= 60
	}
	return score
}

func (idx *snapshotIndex) pathHasBusiness(path []graph.Edge) bool {
	for _, edge := range path {
		if idx.targetBucket(edge) == targetBucketStrongBusiness {
			return true
		}
	}
	return false
}

func (idx *snapshotIndex) pathBlockedByNoise(path []graph.Edge) bool {
	for _, edge := range path {
		switch idx.targetBucket(edge) {
		case targetBucketStrongBusiness:
			return false
		case targetBucketWrapperNoise, targetBucketObservabilityNoise:
			return true
		}
	}
	return false
}

func (idx *snapshotIndex) sideEdges(nodeID string) []graph.Edge {
	return idx.sortSemanticEdges(filterSideEdges(idx.outgoingSemantic(nodeID)))
}

func filterEdgesByKind(edges []graph.Edge, kind graph.EdgeKind) []graph.Edge {
	var filtered []graph.Edge
	for _, edge := range edges {
		if edge.Kind == kind {
			filtered = append(filtered, edge)
		}
	}
	return filtered
}

func filterSideEdges(edges []graph.Edge) []graph.Edge {
	var filtered []graph.Edge
	for _, edge := range edges {
		switch edge.Kind {
		case graph.EdgeBranches, graph.EdgeSpawns, graph.EdgeWaitsOn, graph.EdgeDefers:
			filtered = append(filtered, edge)
		}
	}
	return filtered
}

func (idx *snapshotIndex) bestSemanticEdge(fromNodeID string, edges []graph.Edge) (graph.Edge, bool) {
	if len(edges) == 0 {
		return graph.Edge{}, false
	}
	sorted := idx.sortSemanticEdges(edges)
	return sorted[0], true
}

func (idx *snapshotIndex) sortSemanticEdges(edges []graph.Edge) []graph.Edge {
	sorted := append([]graph.Edge(nil), edges...)
	sort.Slice(sorted, func(i, j int) bool {
		return idx.compareSemanticEdges(sorted[i], sorted[j])
	})
	return sorted
}

func (idx *snapshotIndex) compareSemanticEdges(left, right graph.Edge) bool {
	ls := idx.semanticEdgeScore(left)
	rs := idx.semanticEdgeScore(right)

	switch {
	case ls.total != rs.total:
		return ls.total > rs.total
	case ls.hasOrder && rs.hasOrder && ls.orderIndex != rs.orderIndex:
		return ls.orderIndex < rs.orderIndex
	case ls.hasOrder != rs.hasOrder:
		return ls.hasOrder
	case ls.confirmed != rs.confirmed:
		return ls.confirmed
	case ls.canonical != rs.canonical:
		return ls.canonical < rs.canonical
	default:
		return ls.nodeID < rs.nodeID
	}
}

func (idx *snapshotIndex) semanticEdgeScore(edge graph.Edge) semanticEdgeScore {
	targetCanonical := idx.targetCanonical(edge.To)
	score := semanticEdgeScore{
		total:     idx.edgeBaseWeight(edge.Kind),
		confirmed: edge.Confidence.Tier == graph.ConfidenceConfirmed,
		canonical: targetCanonical,
		nodeID:    edge.To,
	}

	if edge.Properties != nil {
		if raw := edge.Properties["order_index"]; raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				score.hasOrder = true
				score.orderIndex = parsed
			}
		}
	}

	bucket := idx.targetBucket(edge)
	score.total += idx.bucketWeight(bucket)
	score.total += idx.lexicalAdjustment(edge)
	score.total += idx.confidenceAdjustment(edge)
	return score
}

func (idx *snapshotIndex) edgeBaseWeight(kind graph.EdgeKind) int {
	switch kind {
	case graph.EdgeRegistersBoundary:
		return 1000
	case graph.EdgeReturnsHandler:
		return 900
	case graph.EdgeCalls:
		return 500
	case graph.EdgeBranches:
		return 120
	case graph.EdgeSpawns:
		return 110
	case graph.EdgeWaitsOn:
		return 100
	case graph.EdgeDefers:
		return 90
	default:
		return 0
	}
}

func (idx *snapshotIndex) bucketWeight(bucket targetBucket) int {
	switch bucket {
	case targetBucketStrongBusiness:
		return 140
	case targetBucketSetup:
		return 40
	case targetBucketWrapperNoise:
		return -120
	case targetBucketObservabilityNoise:
		return -180
	default:
		return 0
	}
}

func (idx *snapshotIndex) lexicalAdjustment(edge graph.Edge) int {
	tokens := idx.targetTokens(edge.To)
	positive := 0
	negative := 0
	for _, token := range tokens {
		switch {
		case strongBusinessTokens[token]:
			positive++
		case wrapperNoiseTokens[token], observabilityNoiseTokens[token]:
			negative++
		}
	}
	if positive > 3 {
		positive = 3
	}
	if negative > 3 {
		negative = 3
	}
	return positive*20 - negative*20
}

func (idx *snapshotIndex) confidenceAdjustment(edge graph.Edge) int {
	switch {
	case idx.isUnresolvedNode(edge.To):
		return -80
	case edge.Confidence.Tier == graph.ConfidenceAmbiguous || edge.Confidence.Score < 0.5:
		return -40
	case edge.Confidence.Tier == graph.ConfidenceConfirmed:
		return 40
	case edge.Confidence.Tier == graph.ConfidenceInferred:
		return 10
	default:
		return 0
	}
}

func (idx *snapshotIndex) targetBucket(edge graph.Edge) targetBucket {
	tokens := idx.targetTokens(edge.To)
	if len(tokens) == 0 {
		return targetBucketNeutral
	}

	for _, token := range tokens {
		if observabilityNoiseTokens[token] {
			return targetBucketObservabilityNoise
		}
	}
	for _, token := range tokens {
		if wrapperNoiseTokens[token] {
			return targetBucketWrapperNoise
		}
	}

	for _, token := range tokens {
		if strongBusinessTokens[token] {
			return targetBucketStrongBusiness
		}
	}

	for _, token := range tokens {
		if setupTokens[token] {
			return targetBucketSetup
		}
	}

	if node, ok := idx.nodeByID[edge.To]; ok {
		if isConstructorName(nodeShortName(node)) {
			return targetBucketSetup
		}
	}

	return targetBucketNeutral
}

func (idx *snapshotIndex) targetTokens(nodeID string) []string {
	if idx.isUnresolvedNode(nodeID) {
		return tokenize(strings.TrimPrefix(nodeID, "unresolved_"))
	}

	node, ok := idx.nodeByID[nodeID]
	if !ok {
		return nil
	}

	values := []string{
		node.CanonicalName,
		node.Properties["name"],
		node.Properties["kind"],
		node.Properties["receiver"],
	}
	return tokenize(values...)
}

func tokenize(values ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 8)

	appendToken := func(raw string) {
		raw = strings.TrimSpace(strings.ToLower(raw))
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}

	for _, value := range values {
		var current []rune
		var prev rune
		flush := func() {
			if len(current) == 0 {
				return
			}
			appendToken(string(current))
			current = current[:0]
		}

		for _, r := range value {
			switch {
			case r == '/' || r == '.' || r == '_' || r == '-' || unicode.IsSpace(r):
				flush()
			case len(current) > 0 && unicode.IsLower(prev) && unicode.IsUpper(r):
				flush()
				current = append(current, unicode.ToLower(r))
			default:
				current = append(current, unicode.ToLower(r))
			}
			prev = r
		}
		flush()
	}
	return out
}

func (idx *snapshotIndex) targetCanonical(nodeID string) string {
	if node, ok := idx.nodeByID[nodeID]; ok {
		return node.CanonicalName
	}
	if idx.isUnresolvedNode(nodeID) {
		return strings.TrimPrefix(nodeID, "unresolved_")
	}
	return nodeID
}

func (idx *snapshotIndex) isPromotedSyncCall(edge graph.Edge) bool {
	return edge.Kind == graph.EdgeCalls && idx.targetBucket(edge) == targetBucketStrongBusiness
}

func (idx *snapshotIndex) hasReturnsHandlerOrPromotedSyncCalls(nodeID string) bool {
	for _, edge := range idx.outgoingSemantic(nodeID) {
		if edge.Kind == graph.EdgeReturnsHandler || idx.isPromotedSyncCall(edge) {
			return true
		}
	}
	return false
}

func (idx *snapshotIndex) isUnresolvedNode(nodeID string) bool {
	return strings.HasPrefix(nodeID, "unresolved_")
}

func (idx *snapshotIndex) isNoiseNode(nodeID string) bool {
	if idx.isUnresolvedNode(nodeID) {
		return true
	}
	edge := graph.Edge{To: nodeID}
	switch idx.targetBucket(edge) {
	case targetBucketWrapperNoise, targetBucketObservabilityNoise:
		return true
	default:
		return false
	}
}

func (idx *snapshotIndex) auditEdgeRef(edge graph.Edge) SemanticAuditEdgeRef {
	return SemanticAuditEdgeRef{
		EdgeID:          edge.ID,
		Kind:            string(edge.Kind),
		FromNodeID:      edge.From,
		ToNodeID:        edge.To,
		Label:           stepLabel(edge, idx),
		Inferred:        edge.Confidence.Tier == graph.ConfidenceInferred,
		ResolutionBasis: edge.Properties["resolution_basis"],
	}
}

func (idx *snapshotIndex) auditNodeRef(nodeID string) *SemanticAuditNodeRef {
	if nodeID == "" {
		return nil
	}
	if node, ok := idx.nodeByID[nodeID]; ok {
		ref := SemanticAuditNodeRef{
			NodeID:        node.ID,
			CanonicalName: node.CanonicalName,
			SymbolKind:    nodeSymbolKind(node),
			ShortName:     nodeShortName(node),
		}
		return &ref
	}
	if idx.isUnresolvedNode(nodeID) {
		label := strings.TrimPrefix(nodeID, "unresolved_")
		ref := SemanticAuditNodeRef{
			NodeID:        nodeID,
			CanonicalName: label,
			ShortName:     label,
		}
		return &ref
	}
	return nil
}

func (idx *snapshotIndex) edgeByID(edgeID string) (graph.Edge, bool) {
	for _, edges := range idx.outgoing {
		for _, edge := range edges {
			if edge.ID == edgeID {
				return edge, true
			}
		}
	}
	return graph.Edge{}, false
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

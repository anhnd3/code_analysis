package reviewflow_expand

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/participant_classify"
	"analysis-module/internal/services/reviewflow_policy"
	"analysis-module/pkg/ids"
)

const (
	stageBoundaryEntry      = "boundary_entry"
	stageRequestPreparation = "request_preparation"
	stageSessionContext     = "session_context"
	stageBusinessCore       = "business_core"
	stagePostProcessing     = "post_processing"
	stageResponse           = "response"
	stageDeferredAsync      = "deferred_async"
)

const (
	ClassBusinessCall       = "business_call"
	ClassValidation         = "validation"
	ClassSessionAuth        = "session_auth"
	ClassRepoCall           = "repo_call"
	ClassProcessorCall      = "processor_call"
	ClassPostProcessing     = "post_processing"
	ClassBranchBusiness     = "branch_business"
	ClassBranchValidation   = "branch_validation_error"
	ClassAsyncWorker        = "async_worker"
	ClassDeferredSideEffect = "deferred_side_effect"
	ClassSupportNoise       = "support_noise"
)

type Input struct {
	Snapshot graph.GraphSnapshot
	Root     entrypoint.Root
	Reduced  reduced.Chain
	Audit    flow_stitch.SemanticAuditRoot
	Selected reviewflow.Flow
	Policy   reviewflow_policy.Policy
}

type Metadata struct {
	Enabled           bool     `json:"enabled"`
	Family            string   `json:"family,omitempty"`
	MinDepth          int      `json:"min_depth,omitempty"`
	MaxDepth          int      `json:"max_depth,omitempty"`
	AnchorNodeIDs     []string `json:"anchor_node_ids,omitempty"`
	AnchorCount       int      `json:"anchor_count,omitempty"`
	EvidenceEdgeCount int      `json:"evidence_edge_count,omitempty"`
	AddedParticipants int      `json:"added_participants,omitempty"`
	AddedMessages     int      `json:"added_messages,omitempty"`
	AddedBlocks       int      `json:"added_blocks,omitempty"`
	SkippedDuplicates int      `json:"skipped_duplicates,omitempty"`
	DemotedNoise      int      `json:"demoted_noise,omitempty"`
	Notes             []string `json:"notes,omitempty"`
}

type Result struct {
	Flow     reviewflow.Flow `json:"flow"`
	Metadata Metadata        `json:"metadata"`
}

type Service struct {
	classifier participant_classify.Service
}

func New() Service {
	return Service{classifier: participant_classify.New()}
}

type evidenceEdge struct {
	Kind         graph.EdgeKind
	FromNodeID   string
	ToNodeID     string
	Label        string
	SourceEdgeID string
	OrderIndex   int
}

type nodeDepth struct {
	NodeID string
	Depth  int
}

type flowState struct {
	flow reviewflow.Flow

	nodeByID     map[string]graph.Node
	edgesByFrom  map[string][]graph.Edge
	classifier   participant_classify.Service
	snapshot     graph.GraphSnapshot
	participantI map[string]int
	sourceNodeTo map[string]string
	kindToIDs    map[string][]string
	stageByKind  map[string]int
	messageKeys  map[string]bool
	blockKeys    map[string]bool
	meta         *Metadata
}

func (s Service) Expand(input Input) (Result, error) {
	meta := Metadata{Enabled: true, Family: input.Policy.Family}
	if input.Selected.RootNodeID == "" {
		return Result{Flow: input.Selected, Metadata: meta}, nil
	}
	flow := ClassifyFlow(input.Selected)
	minDepth, maxDepth := expansionDepthBounds(input.Policy)
	meta.MinDepth = minDepth
	meta.MaxDepth = maxDepth
	if maxDepth <= 0 {
		meta.Notes = append(meta.Notes, "expansion disabled by policy depth")
		return Result{Flow: flow, Metadata: meta}, nil
	}

	anchors := collectAnchorNodeIDs(input)
	meta.AnchorCount = len(anchors)
	meta.AnchorNodeIDs = truncateAnchors(anchors, 24)
	if len(anchors) == 0 {
		meta.Notes = append(meta.Notes, "no expansion anchors available")
		return Result{Flow: flow, Metadata: meta}, nil
	}

	nodeByID := indexNodes(input.Snapshot.Nodes)
	edgesByFrom := indexOutgoingEdges(input.Snapshot.Edges)

	evidence := collectEvidenceEdges(input, nodeByID, edgesByFrom, minDepth, maxDepth, &meta)
	meta.EvidenceEdgeCount = len(evidence)
	if len(evidence) == 0 {
		meta.Notes = append(meta.Notes, "no eligible evidence edges for expansion")
		return Result{Flow: flow, Metadata: meta}, nil
	}

	state := newFlowState(flow, nodeByID, edgesByFrom, s.classifier, input.Snapshot, &meta)
	for _, edge := range evidence {
		state.applyEvidenceEdge(edge)
	}
	state.normalizeFlow()
	state.flow = ClassifyFlow(state.flow)
	return Result{Flow: state.flow, Metadata: meta}, nil
}

func ClassifyFlow(flow reviewflow.Flow) reviewflow.Flow {
	participantByID := map[string]reviewflow.Participant{}
	for _, participant := range flow.Participants {
		participantByID[participant.ID] = participant
	}
	stageKindByID := map[string]string{}
	for _, stage := range flow.Stages {
		stageKindByID[stage.ID] = stage.Kind
	}

	for i := range flow.Stages {
		stage := &flow.Stages[i]
		for j := range stage.Messages {
			msg := &stage.Messages[j]
			toKind := participantByID[msg.ToParticipantID].Kind
			if strings.TrimSpace(msg.Class) == "" {
				msg.Class = classifyMessage(stage.Kind, toKind, msg.Label, msg.Kind, "")
			}
		}
	}
	for i := range flow.Blocks {
		block := &flow.Blocks[i]
		stageKind := block.StageID
		if mapped := stageKindByID[block.StageID]; mapped != "" {
			stageKind = mapped
		}
		for sectionIdx := range block.Sections {
			for msgIdx := range block.Sections[sectionIdx].Messages {
				msg := &block.Sections[sectionIdx].Messages[msgIdx]
				toKind := participantByID[msg.ToParticipantID].Kind
				if strings.TrimSpace(msg.Class) == "" {
					msg.Class = classifyMessage(stageKind, toKind, msg.Label, msg.Kind, string(block.Kind))
				}
			}
		}
		if strings.TrimSpace(block.Class) == "" {
			block.Class = classifyBlock(*block)
		}
	}
	return flow
}

func collectEvidenceEdges(
	input Input,
	nodeByID map[string]graph.Node,
	edgesByFrom map[string][]graph.Edge,
	minDepth int,
	maxDepth int,
	meta *Metadata,
) []evidenceEdge {
	limit := maxEvidenceEdges(input.Policy)
	out := make([]evidenceEdge, 0, limit)
	seen := map[string]bool{}

	appendEdge := func(edge evidenceEdge) {
		if !isAllowedKind(edge.Kind) {
			return
		}
		if edge.FromNodeID == "" || edge.ToNodeID == "" {
			return
		}
		if strings.TrimSpace(edge.SourceEdgeID) == "" {
			return
		}
		if isDemotedNoise(edge, nodeByID) {
			meta.DemotedNoise++
			return
		}
		key := evidenceEdgeKey(edge)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, edge)
	}

	for _, ref := range collectAuditRefs(input.Audit) {
		kind := parseAuditKind(ref.Kind)
		if !isAllowedKind(kind) {
			continue
		}
		sourceID := strings.TrimSpace(ref.EdgeID)
		if sourceID == "" {
			sourceID = ids.Stable("audit_ref", ref.Kind, ref.FromNodeID, ref.ToNodeID, ref.Label, strconv.Itoa(ref.OrderIndex), ref.ResolutionBasis)
		}
		appendEdge(evidenceEdge{
			Kind:         kind,
			FromNodeID:   ref.FromNodeID,
			ToNodeID:     ref.ToNodeID,
			Label:        ref.Label,
			SourceEdgeID: "audit:" + sourceID,
			OrderIndex:   ref.OrderIndex,
		})
		if len(out) >= limit {
			return orderEvidence(out)
		}
	}

	for _, block := range orderedBlocks(input.Reduced.Blocks) {
		kind := graph.EdgeBranches
		switch block.Kind {
		case reduced.BlockPar:
			kind = graph.EdgeSpawns
		case reduced.BlockLoop:
			kind = graph.EdgeBranches
		case reduced.BlockAlt:
			kind = graph.EdgeBranches
		}
		for _, branch := range block.Branches {
			for _, edge := range orderedReducedEdges(branch.Edges) {
				appendEdge(evidenceEdge{
					Kind:         kind,
					FromNodeID:   edge.FromID,
					ToNodeID:     edge.ToID,
					Label:        firstNonEmpty(edge.Label, branch.Condition, block.Label),
					SourceEdgeID: "reduced:" + stableReducedEdgeRef(edge),
					OrderIndex:   edge.OrderIndex,
				})
				if len(out) >= limit {
					return orderEvidence(out)
				}
			}
		}
	}

	anchors := collectAnchorNodeIDs(input)
	for _, anchor := range anchors {
		if len(out) >= limit {
			break
		}
		if anchor == "" {
			continue
		}
		queue := []nodeDepth{{NodeID: anchor, Depth: 0}}
		visited := map[string]int{anchor: 0}
		for len(queue) > 0 && len(out) < limit {
			current := queue[0]
			queue = queue[1:]
			if current.Depth >= maxDepth {
				continue
			}
			for _, edge := range orderedGraphEdges(edgesByFrom[current.NodeID]) {
				if !isAllowedKind(edge.Kind) {
					continue
				}
				nextDepth := current.Depth + 1
				if nextDepth > maxDepth {
					continue
				}
				ev := evidenceEdge{
					Kind:         edge.Kind,
					FromNodeID:   edge.From,
					ToNodeID:     edge.To,
					Label:        edgeLabel(edge, nodeByID),
					SourceEdgeID: graphEdgeSourceID(edge),
					OrderIndex:   edgeOrderIndex(edge),
				}
				if nextDepth >= minDepth {
					appendEdge(ev)
					if len(out) >= limit {
						break
					}
				}
				if !traversesDepth(edge.Kind) {
					continue
				}
				if previous, ok := visited[edge.To]; ok && previous <= nextDepth {
					continue
				}
				visited[edge.To] = nextDepth
				queue = append(queue, nodeDepth{NodeID: edge.To, Depth: nextDepth})
			}
		}
	}

	return orderEvidence(out)
}

func newFlowState(
	flow reviewflow.Flow,
	nodeByID map[string]graph.Node,
	edgesByFrom map[string][]graph.Edge,
	classifier participant_classify.Service,
	snapshot graph.GraphSnapshot,
	meta *Metadata,
) *flowState {
	state := &flowState{
		flow:         flow,
		nodeByID:     nodeByID,
		edgesByFrom:  edgesByFrom,
		classifier:   classifier,
		snapshot:     snapshot,
		participantI: map[string]int{},
		sourceNodeTo: map[string]string{},
		kindToIDs:    map[string][]string{},
		stageByKind:  map[string]int{},
		messageKeys:  map[string]bool{},
		blockKeys:    map[string]bool{},
		meta:         meta,
	}

	for i := range state.flow.Participants {
		participant := state.flow.Participants[i]
		state.participantI[participant.ID] = i
		state.kindToIDs[participant.Kind] = append(state.kindToIDs[participant.Kind], participant.ID)
		for _, nodeID := range participant.SourceNodeIDs {
			if nodeID == "" {
				continue
			}
			state.sourceNodeTo[nodeID] = participant.ID
		}
	}

	for i := range state.flow.Stages {
		stage := state.flow.Stages[i]
		state.stageByKind[stage.Kind] = i
		for _, msg := range stage.Messages {
			state.messageKeys[messageKey(msg)] = true
		}
	}
	for _, block := range state.flow.Blocks {
		state.blockKeys[blockKey(block)] = true
		for _, section := range block.Sections {
			for _, msg := range section.Messages {
				state.messageKeys[messageKey(msg)] = true
			}
		}
	}
	return state
}

func (s *flowState) applyEvidenceEdge(edge evidenceEdge) {
	fromID := s.ensureParticipant(edge.FromNodeID)
	toID := s.ensureParticipant(edge.ToNodeID)
	if fromID == "" || toID == "" {
		return
	}
	toKind := s.participantKind(toID)
	label := expansionMessageLabel(edge)
	if strings.TrimSpace(label) == "" {
		return
	}
	class := classifyMessage("", toKind, label, messageKindForEdge(edge.Kind, ""), string(edge.Kind))
	if class == ClassSupportNoise {
		s.meta.DemotedNoise++
		return
	}
	stageKind := stageForClass(class)
	message := reviewflow.Message{
		ID:                ids.Stable("review_msg", s.flow.RootNodeID, "expand", stageKind, edge.SourceEdgeID, fromID, toID),
		FromParticipantID: fromID,
		ToParticipantID:   toID,
		Label:             label,
		Kind:              messageKindForEdge(edge.Kind, class),
		SourceEdgeIDs:     []string{edge.SourceEdgeID},
		Class:             class,
	}
	if !s.addStageMessage(stageKind, message) {
		s.meta.SkippedDuplicates++
		return
	}
	if class == ClassBranchBusiness || class == ClassBranchValidation {
		s.addBranchBlock(stageKind, message)
	}
	if class == ClassAsyncWorker && edge.Kind == graph.EdgeSpawns {
		s.addAsyncBlock(stageKind, message)
	}
}

func (s *flowState) ensureParticipant(nodeID string) string {
	if nodeID == "" {
		return ""
	}
	if participantID := s.sourceNodeTo[nodeID]; participantID != "" {
		return participantID
	}

	node := s.nodeByID[nodeID]
	if node.ID == "" {
		node = graph.Node{
			ID:            nodeID,
			Kind:          graph.NodeSymbol,
			CanonicalName: strings.TrimPrefix(nodeID, "unresolved_"),
			Properties: map[string]string{
				"name": strings.TrimPrefix(nodeID, "unresolved_"),
			},
		}
	}
	profile := s.classifier.Profile(node, s.snapshot)
	kind, label, role, external := participantIdentity(node, profile)

	if existing := s.kindToIDs[kind]; len(existing) > 0 {
		participantID := existing[0]
		s.bindParticipantSourceNode(participantID, nodeID)
		return participantID
	}

	key := kind + ":" + ids.Slug(label)
	participantID := ids.Stable("review_participant", s.flow.RootNodeID, "expand", key)
	if _, ok := s.participantI[participantID]; ok {
		s.bindParticipantSourceNode(participantID, nodeID)
		return participantID
	}

	participant := reviewflow.Participant{
		ID:            participantID,
		Kind:          kind,
		Label:         label,
		Role:          role,
		IsExternal:    external,
		SourceNodeIDs: []string{nodeID},
	}
	s.flow.Participants = append(s.flow.Participants, participant)
	s.participantI[participantID] = len(s.flow.Participants) - 1
	s.kindToIDs[kind] = append(s.kindToIDs[kind], participantID)
	s.sourceNodeTo[nodeID] = participantID
	s.meta.AddedParticipants++
	return participantID
}

func (s *flowState) bindParticipantSourceNode(participantID, nodeID string) {
	if participantID == "" || nodeID == "" {
		return
	}
	index, ok := s.participantI[participantID]
	if !ok {
		return
	}
	participant := &s.flow.Participants[index]
	for _, existing := range participant.SourceNodeIDs {
		if existing == nodeID {
			s.sourceNodeTo[nodeID] = participantID
			return
		}
	}
	participant.SourceNodeIDs = append(participant.SourceNodeIDs, nodeID)
	s.sourceNodeTo[nodeID] = participantID
}

func (s *flowState) addStageMessage(stageKind string, msg reviewflow.Message) bool {
	idx := s.ensureStage(stageKind)
	if s.messageKeys[messageKey(msg)] {
		return false
	}
	s.messageKeys[messageKey(msg)] = true

	stage := &s.flow.Stages[idx]
	stage.Messages = append(stage.Messages, msg)
	stage.ParticipantIDs = appendUnique(stage.ParticipantIDs, msg.FromParticipantID)
	stage.ParticipantIDs = appendUnique(stage.ParticipantIDs, msg.ToParticipantID)
	for _, source := range msg.SourceEdgeIDs {
		stage.SourceEdgeIDs = appendUnique(stage.SourceEdgeIDs, source)
	}
	s.meta.AddedMessages++
	return true
}

func (s *flowState) addBranchBlock(stageKind string, msg reviewflow.Message) {
	block := reviewflow.Block{
		ID:            ids.Stable("review_block", s.flow.RootNodeID, "expand", "branch", msg.SourceEdgeIDs[0], msg.Label),
		Kind:          reviewflow.BlockAlt,
		Label:         summarizeBranchLabel(msg.Label),
		Class:         classifyBranchLabel(msg.Label),
		StageID:       stageKind,
		SourceEdgeIDs: append([]string(nil), msg.SourceEdgeIDs...),
		Sections: []reviewflow.BlockSection{{
			Label:         summarizeBranchLabel(msg.Label),
			SourceEdgeIDs: append([]string(nil), msg.SourceEdgeIDs...),
			Messages:      []reviewflow.Message{msg},
		}},
	}
	key := blockKey(block)
	if s.blockKeys[key] {
		return
	}
	s.blockKeys[key] = true
	s.flow.Blocks = append(s.flow.Blocks, block)
	s.meta.AddedBlocks++
}

func (s *flowState) addAsyncBlock(stageKind string, msg reviewflow.Message) {
	block := reviewflow.Block{
		ID:            ids.Stable("review_block", s.flow.RootNodeID, "expand", "async", msg.SourceEdgeIDs[0]),
		Kind:          reviewflow.BlockPar,
		Label:         "asynchronous worker path",
		Class:         ClassAsyncWorker,
		StageID:       stageKind,
		SourceEdgeIDs: append([]string(nil), msg.SourceEdgeIDs...),
		Sections: []reviewflow.BlockSection{{
			Label:         "spawn worker",
			SourceEdgeIDs: append([]string(nil), msg.SourceEdgeIDs...),
			Messages:      []reviewflow.Message{msg},
		}},
	}
	key := blockKey(block)
	if s.blockKeys[key] {
		return
	}
	s.blockKeys[key] = true
	s.flow.Blocks = append(s.flow.Blocks, block)
	s.meta.AddedBlocks++
}

func (s *flowState) ensureStage(kind string) int {
	if idx, ok := s.stageByKind[kind]; ok {
		return idx
	}
	stage := reviewflow.Stage{
		ID:    ids.Stable("review_stage", s.flow.RootNodeID, kind),
		Kind:  kind,
		Label: stageLabel(kind),
	}
	s.flow.Stages = append(s.flow.Stages, stage)
	idx := len(s.flow.Stages) - 1
	s.stageByKind[kind] = idx
	return idx
}

func (s *flowState) participantKind(participantID string) string {
	idx, ok := s.participantI[participantID]
	if !ok {
		return ""
	}
	return s.flow.Participants[idx].Kind
}

func (s *flowState) normalizeFlow() {
	for i := range s.flow.Participants {
		s.flow.Participants[i].SourceNodeIDs = uniqueSortedStrings(s.flow.Participants[i].SourceNodeIDs)
	}
	for i := range s.flow.Stages {
		stage := &s.flow.Stages[i]
		stage.ParticipantIDs = uniqueSortedStrings(stage.ParticipantIDs)
		stage.SourceEdgeIDs = uniqueSortedStrings(stage.SourceEdgeIDs)
		sort.SliceStable(stage.Messages, func(left, right int) bool {
			return stageMessageOrder(stage.Messages[left], stage.Messages[right])
		})
	}
	sort.SliceStable(s.flow.Stages, func(left, right int) bool {
		if stageRank(s.flow.Stages[left].Kind) != stageRank(s.flow.Stages[right].Kind) {
			return stageRank(s.flow.Stages[left].Kind) < stageRank(s.flow.Stages[right].Kind)
		}
		return s.flow.Stages[left].ID < s.flow.Stages[right].ID
	})
	for i := range s.flow.Blocks {
		s.flow.Blocks[i].SourceEdgeIDs = uniqueSortedStrings(s.flow.Blocks[i].SourceEdgeIDs)
		for sectionIdx := range s.flow.Blocks[i].Sections {
			s.flow.Blocks[i].Sections[sectionIdx].SourceEdgeIDs = uniqueSortedStrings(s.flow.Blocks[i].Sections[sectionIdx].SourceEdgeIDs)
			sort.SliceStable(s.flow.Blocks[i].Sections[sectionIdx].Messages, func(left, right int) bool {
				return stageMessageOrder(s.flow.Blocks[i].Sections[sectionIdx].Messages[left], s.flow.Blocks[i].Sections[sectionIdx].Messages[right])
			})
		}
	}
	sort.SliceStable(s.flow.Blocks, func(left, right int) bool {
		if stageRank(blockStageKind(s.flow.Blocks[left])) != stageRank(blockStageKind(s.flow.Blocks[right])) {
			return stageRank(blockStageKind(s.flow.Blocks[left])) < stageRank(blockStageKind(s.flow.Blocks[right]))
		}
		if s.flow.Blocks[left].Kind != s.flow.Blocks[right].Kind {
			return s.flow.Blocks[left].Kind < s.flow.Blocks[right].Kind
		}
		return s.flow.Blocks[left].ID < s.flow.Blocks[right].ID
	})
}

func collectAnchorNodeIDs(input Input) []string {
	set := map[string]bool{}
	add := func(nodeID string) {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			return
		}
		set[nodeID] = true
	}

	add(input.Root.NodeID)
	add(input.Reduced.RootNodeID)
	if input.Audit.HandlerTargetNode != nil {
		add(input.Audit.HandlerTargetNode.NodeID)
	}
	if input.Audit.ClosureBodyNode != nil {
		add(input.Audit.ClosureBodyNode.NodeID)
	}
	for _, ref := range collectAuditRefs(input.Audit) {
		add(ref.FromNodeID)
		add(ref.ToNodeID)
	}
	for _, edge := range input.Reduced.Edges {
		add(edge.FromID)
		add(edge.ToID)
	}
	for _, block := range input.Reduced.Blocks {
		for _, branch := range block.Branches {
			for _, edge := range branch.Edges {
				add(edge.FromID)
				add(edge.ToID)
			}
		}
	}
	for _, participant := range input.Selected.Participants {
		for _, sourceNodeID := range participant.SourceNodeIDs {
			add(sourceNodeID)
		}
	}

	anchors := make([]string, 0, len(set))
	for nodeID := range set {
		anchors = append(anchors, nodeID)
	}
	sort.Strings(anchors)
	return anchors
}

func collectAuditRefs(audit flow_stitch.SemanticAuditRoot) []flow_stitch.SemanticAuditEdgeRef {
	refs := make([]flow_stitch.SemanticAuditEdgeRef, 0, len(audit.LeadingCalls)+len(audit.FirstBusinessCalls)+len(audit.BusinessHandoff)+len(audit.DrilldownCalls))
	refs = append(refs, audit.LeadingCalls...)
	refs = append(refs, audit.FirstBusinessCalls...)
	refs = append(refs, audit.BusinessHandoff...)
	refs = append(refs, audit.DrilldownCalls...)
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].OrderIndex != refs[j].OrderIndex {
			return refs[i].OrderIndex < refs[j].OrderIndex
		}
		if refs[i].FromNodeID != refs[j].FromNodeID {
			return refs[i].FromNodeID < refs[j].FromNodeID
		}
		if refs[i].ToNodeID != refs[j].ToNodeID {
			return refs[i].ToNodeID < refs[j].ToNodeID
		}
		if refs[i].Label != refs[j].Label {
			return refs[i].Label < refs[j].Label
		}
		return refs[i].EdgeID < refs[j].EdgeID
	})
	return refs
}

func indexNodes(nodes []graph.Node) map[string]graph.Node {
	indexed := make(map[string]graph.Node, len(nodes))
	for _, node := range nodes {
		indexed[node.ID] = node
	}
	return indexed
}

func indexOutgoingEdges(edges []graph.Edge) map[string][]graph.Edge {
	indexed := map[string][]graph.Edge{}
	for _, edge := range edges {
		indexed[edge.From] = append(indexed[edge.From], edge)
	}
	for from := range indexed {
		indexed[from] = orderedGraphEdges(indexed[from])
	}
	return indexed
}

func orderedGraphEdges(edges []graph.Edge) []graph.Edge {
	ordered := append([]graph.Edge(nil), edges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if edgeOrderIndex(ordered[i]) != edgeOrderIndex(ordered[j]) {
			return edgeOrderIndex(ordered[i]) < edgeOrderIndex(ordered[j])
		}
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		if ordered[i].To != ordered[j].To {
			return ordered[i].To < ordered[j].To
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

func orderedReducedEdges(edges []reduced.Edge) []reduced.Edge {
	ordered := append([]reduced.Edge(nil), edges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].OrderIndex != ordered[j].OrderIndex {
			return ordered[i].OrderIndex < ordered[j].OrderIndex
		}
		if ordered[i].FromID != ordered[j].FromID {
			return ordered[i].FromID < ordered[j].FromID
		}
		if ordered[i].ToID != ordered[j].ToID {
			return ordered[i].ToID < ordered[j].ToID
		}
		return ordered[i].Label < ordered[j].Label
	})
	return ordered
}

func orderedBlocks(blocks []reduced.Block) []reduced.Block {
	ordered := append([]reduced.Block(nil), blocks...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].OrderIndex != ordered[j].OrderIndex {
			return ordered[i].OrderIndex < ordered[j].OrderIndex
		}
		return ordered[i].Label < ordered[j].Label
	})
	return ordered
}

func evidenceEdgeKey(edge evidenceEdge) string {
	return strings.Join([]string{
		string(edge.Kind),
		edge.FromNodeID,
		edge.ToNodeID,
		edge.Label,
		edge.SourceEdgeID,
		strconv.Itoa(edge.OrderIndex),
	}, "|")
}

func orderEvidence(edges []evidenceEdge) []evidenceEdge {
	ordered := append([]evidenceEdge(nil), edges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].OrderIndex != ordered[j].OrderIndex {
			return ordered[i].OrderIndex < ordered[j].OrderIndex
		}
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		if ordered[i].FromNodeID != ordered[j].FromNodeID {
			return ordered[i].FromNodeID < ordered[j].FromNodeID
		}
		if ordered[i].ToNodeID != ordered[j].ToNodeID {
			return ordered[i].ToNodeID < ordered[j].ToNodeID
		}
		if ordered[i].Label != ordered[j].Label {
			return ordered[i].Label < ordered[j].Label
		}
		return ordered[i].SourceEdgeID < ordered[j].SourceEdgeID
	})
	return ordered
}

func maxEvidenceEdges(policy reviewflow_policy.Policy) int {
	switch policy.Family {
	case reviewflow_policy.FamilyDetectorPipeline, reviewflow_policy.FamilyScanPipeline:
		return 28
	case reviewflow_policy.FamilyBlacklistGate:
		return 16
	case reviewflow_policy.FamilySimpleQuery, reviewflow_policy.FamilyConfigLookup:
		return 8
	default:
		return 12
	}
}

func expansionDepthBounds(policy reviewflow_policy.Policy) (int, int) {
	maxDepth := policy.MaxBusinessExpansionDepth
	if maxDepth <= 0 {
		switch policy.Family {
		case reviewflow_policy.FamilyDetectorPipeline, reviewflow_policy.FamilyScanPipeline:
			maxDepth = 3
		case reviewflow_policy.FamilyBlacklistGate:
			maxDepth = 2
		case reviewflow_policy.FamilySimpleQuery, reviewflow_policy.FamilyConfigLookup:
			maxDepth = 1
		default:
			maxDepth = 1
		}
	}
	minDepth := policy.MinBusinessExpansionDepth
	switch policy.Family {
	case reviewflow_policy.FamilyDetectorPipeline, reviewflow_policy.FamilyScanPipeline, reviewflow_policy.FamilyBlacklistGate:
		if minDepth <= 0 {
			minDepth = 1
		}
	default:
		if minDepth < 0 {
			minDepth = 0
		}
	}
	if minDepth > maxDepth {
		minDepth = maxDepth
	}
	return minDepth, maxDepth
}

func isAllowedKind(kind graph.EdgeKind) bool {
	switch kind {
	case graph.EdgeCalls, graph.EdgeSpawns, graph.EdgeDefers, graph.EdgeBranches:
		return true
	default:
		return false
	}
}

func parseAuditKind(raw string) graph.EdgeKind {
	kind := graph.EdgeKind(strings.TrimSpace(raw))
	if isAllowedKind(kind) {
		return kind
	}
	if kind == "" {
		return graph.EdgeCalls
	}
	return ""
}

func traversesDepth(kind graph.EdgeKind) bool {
	switch kind {
	case graph.EdgeCalls, graph.EdgeSpawns, graph.EdgeDefers:
		return true
	default:
		return false
	}
}

func stageForClass(class string) string {
	switch class {
	case ClassValidation:
		return stageRequestPreparation
	case ClassSessionAuth:
		return stageSessionContext
	case ClassPostProcessing:
		return stagePostProcessing
	case ClassAsyncWorker, ClassDeferredSideEffect:
		return stageDeferredAsync
	default:
		return stageBusinessCore
	}
}

func classifyMessage(stageKind, toKind, label string, kind reviewflow.MessageKind, contextKind string) string {
	lower := strings.ToLower(strings.TrimSpace(label))

	switch {
	case containsAny(lower, "trace", "tracing", "logger", "logging", "metric", "metrics", "telemetry", "span"):
		return ClassSupportNoise
	case containsAny(lower, "wrapper", "helper", "utility") && !containsAny(lower, "postprocess", "post process", "detect", "predict", "blacklist", "extract", "bank"):
		return ClassSupportNoise
	}

	switch contextKind {
	case string(graph.EdgeSpawns), string(reviewflow.BlockPar):
		return ClassAsyncWorker
	case string(graph.EdgeDefers):
		return ClassDeferredSideEffect
	case string(graph.EdgeBranches), string(reviewflow.BlockAlt):
		if containsAny(lower, "invalid", "error", "fail", "deny", "reject", "unauthorized", "forbidden") {
			return ClassBranchValidation
		}
		return ClassBranchBusiness
	}

	if kind == reviewflow.MessageAsync {
		if containsAny(lower, "defer", "cleanup", "push") {
			return ClassDeferredSideEffect
		}
		return ClassAsyncWorker
	}

	switch {
	case stageKind == stageRequestPreparation || toKind == "validation" || containsAny(lower, "bind", "validate", "decode", "parse", "sanitize", "verify"):
		return ClassValidation
	case stageKind == stageSessionContext || toKind == "session_auth" || containsAny(lower, "session", "auth", "token", "claims", "context"):
		return ClassSessionAuth
	case stageKind == stagePostProcessing || containsAny(lower, "post process", "postprocess", "normalize", "classify", "sort", "merge", "transform", "deeplink", "emvco"):
		return ClassPostProcessing
	case toKind == "repo":
		return ClassRepoCall
	case toKind == "processor" || toKind == "gateway_client":
		if containsAny(lower, "post process", "postprocess", "normalize", "classify", "sort", "merge", "transform") {
			return ClassPostProcessing
		}
		return ClassProcessorCall
	default:
		return ClassBusinessCall
	}
}

func classifyBlock(block reviewflow.Block) string {
	lower := strings.ToLower(block.Label)
	if containsAny(lower, "trace", "metric", "log", "telemetry", "wrapper", "helper") {
		return ClassSupportNoise
	}
	switch block.Kind {
	case reviewflow.BlockPar:
		return ClassAsyncWorker
	case reviewflow.BlockAlt:
		if containsAny(lower, "invalid", "error", "fail", "deny", "reject", "unauthorized", "forbidden") {
			return ClassBranchValidation
		}
		for _, section := range block.Sections {
			sectionLower := strings.ToLower(section.Label)
			if containsAny(sectionLower, "invalid", "error", "fail", "deny", "reject", "unauthorized", "forbidden") {
				return ClassBranchValidation
			}
		}
		return ClassBranchBusiness
	case reviewflow.BlockSummary:
		if containsAny(lower, "post", "classify", "sort", "merge", "normalize") {
			return ClassPostProcessing
		}
		return ClassBusinessCall
	default:
		if containsAny(lower, "post", "classify", "sort", "merge", "normalize") {
			return ClassPostProcessing
		}
		return ClassBusinessCall
	}
}

func classifyBranchLabel(label string) string {
	lower := strings.ToLower(label)
	if containsAny(lower, "invalid", "error", "fail", "deny", "reject", "unauthorized", "forbidden") {
		return ClassBranchValidation
	}
	return ClassBranchBusiness
}

func summarizeBranchLabel(label string) string {
	clean := cleanLabel(label)
	if clean == "" {
		return "business decision"
	}
	if containsAny(strings.ToLower(clean), "if", "else", "case", "branch") {
		return clean
	}
	return "branch: " + clean
}

func participantIdentity(node graph.Node, profile participant_classify.Profile) (kind, label, role string, external bool) {
	switch profile.Bucket {
	case participant_classify.BucketValidation:
		return "validation", "Validation", string(profile.Role), false
	case participant_classify.BucketSessionAuth:
		return "session_auth", "Session/Auth", string(profile.Role), false
	case participant_classify.BucketRepo:
		return "repo", "Repository", string(profile.Role), false
	case participant_classify.BucketProcessor:
		return "processor", "Processor", string(profile.Role), false
	case participant_classify.BucketResponse:
		return "response", "Response", string(profile.Role), false
	case participant_classify.BucketGateway:
		return "gateway_client", "Gateway Client", string(profile.Role), profile.IsRemote
	case participant_classify.BucketRemote:
		name := strings.TrimSpace(profile.DisplayLabel)
		if name == "" {
			name = strings.TrimPrefix(node.CanonicalName, "unresolved_")
		}
		return "remote", cleanLabel(name), string(profile.Role), true
	case participant_classify.BucketAsyncSink:
		return "async_sink", "Async Worker", string(profile.Role), false
	case participant_classify.BucketHandler:
		return "handler", "Handler", string(profile.Role), false
	case participant_classify.BucketAlgorithm, participant_classify.BucketCoreEngine:
		return "processor", "Processor", string(profile.Role), false
	case participant_classify.BucketService:
		return "service", "Service", string(profile.Role), false
	default:
		if strings.Contains(strings.ToLower(node.CanonicalName), "repo") {
			return "repo", "Repository", string(profile.Role), false
		}
		if strings.Contains(strings.ToLower(node.CanonicalName), "processor") {
			return "processor", "Processor", string(profile.Role), false
		}
		return "service", "Service", string(profile.Role), false
	}
}

func messageKindForEdge(kind graph.EdgeKind, class string) reviewflow.MessageKind {
	switch kind {
	case graph.EdgeSpawns, graph.EdgeDefers:
		return reviewflow.MessageAsync
	case graph.EdgeBranches:
		if class == ClassBranchValidation {
			return reviewflow.MessageReturn
		}
		return reviewflow.MessageSync
	default:
		return reviewflow.MessageSync
	}
}

func expansionMessageLabel(edge evidenceEdge) string {
	label := cleanLabel(edge.Label)
	switch edge.Kind {
	case graph.EdgeSpawns:
		if label == "" {
			return "spawn worker"
		}
		return "spawn worker: " + label
	case graph.EdgeDefers:
		if label == "" {
			return "defer side effect"
		}
		return "defer side effect: " + label
	case graph.EdgeBranches:
		if label == "" {
			return "evaluate branch"
		}
		return "branch decision: " + label
	default:
		if label == "" {
			return "call business logic"
		}
		return label
	}
}

func blockStageKind(block reviewflow.Block) string {
	if block.StageID != "" {
		return block.StageID
	}
	switch block.Kind {
	case reviewflow.BlockPar:
		return stageDeferredAsync
	default:
		return stageBusinessCore
	}
}

func graphEdgeSourceID(edge graph.Edge) string {
	if strings.TrimSpace(edge.ID) != "" {
		return "graph:" + edge.ID
	}
	return "graph:" + ids.Stable("edge", string(edge.Kind), edge.From, edge.To, edge.Evidence.Source, strconv.Itoa(edgeOrderIndex(edge)))
}

func stableReducedEdgeRef(edge reduced.Edge) string {
	return fmt.Sprintf("%s|%s|%s|%d", edge.FromID, edge.ToID, edge.Label, edge.OrderIndex)
}

func edgeLabel(edge graph.Edge, nodeByID map[string]graph.Node) string {
	if strings.TrimSpace(edge.Evidence.Source) != "" {
		return edge.Evidence.Source
	}
	if node, ok := nodeByID[edge.To]; ok {
		if strings.TrimSpace(node.Properties["name"]) != "" {
			return node.Properties["name"]
		}
		if strings.TrimSpace(node.CanonicalName) != "" {
			return shortName(node.CanonicalName)
		}
	}
	return strings.TrimPrefix(edge.To, "unresolved_")
}

func isDemotedNoise(edge evidenceEdge, nodeByID map[string]graph.Node) bool {
	label := strings.ToLower(strings.TrimSpace(edge.Label))
	target := strings.ToLower(strings.TrimSpace(nodeByID[edge.ToNodeID].CanonicalName))
	combined := strings.TrimSpace(label + " " + target)
	if combined == "" {
		return false
	}
	if containsAny(combined, "trace", "tracing", "logger", "logging", "metric", "metrics", "telemetry", "span", "prometheus") {
		return true
	}
	if containsAny(combined, "response wrapper", "responsehelper", "writejson", "renderjson", "httpresponse", "generic helper", "utility", "util", "wrapper") &&
		!containsAny(combined, "postprocess", "post process", "detect", "predict", "blacklist", "extract", "bank", "session", "auth", "validate") {
		return true
	}
	if containsAny(combined, "helper", "common", "base") &&
		!containsAny(combined, "detect", "predict", "blacklist", "extract", "bank", "session", "auth", "validate", "postprocess", "post process") {
		return true
	}
	return false
}

func parseOrderIndex(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return parsed
}

func edgeOrderIndex(edge graph.Edge) int {
	if edge.Properties == nil {
		return 0
	}
	return parseOrderIndex(edge.Properties["order_index"])
}

func stageLabel(kind string) string {
	switch kind {
	case stageBoundaryEntry:
		return "Boundary Entry"
	case stageRequestPreparation:
		return "Request Preparation"
	case stageSessionContext:
		return "Session Context"
	case stageBusinessCore:
		return "Business Core"
	case stagePostProcessing:
		return "Post Processing"
	case stageResponse:
		return "Response"
	case stageDeferredAsync:
		return "Deferred Async"
	default:
		return cleanLabel(kind)
	}
}

func stageRank(kind string) int {
	switch kind {
	case stageBoundaryEntry:
		return 0
	case stageRequestPreparation:
		return 1
	case stageSessionContext:
		return 2
	case stageBusinessCore:
		return 3
	case stagePostProcessing:
		return 4
	case stageResponse:
		return 5
	case stageDeferredAsync:
		return 6
	default:
		return 7
	}
}

func stageMessageOrder(left, right reviewflow.Message) bool {
	if left.FromParticipantID != right.FromParticipantID {
		return left.FromParticipantID < right.FromParticipantID
	}
	if left.ToParticipantID != right.ToParticipantID {
		return left.ToParticipantID < right.ToParticipantID
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Class != right.Class {
		return left.Class < right.Class
	}
	if left.Label != right.Label {
		return left.Label < right.Label
	}
	return left.ID < right.ID
}

func messageKey(msg reviewflow.Message) string {
	return strings.Join([]string{msg.FromParticipantID, msg.ToParticipantID, string(msg.Kind), msg.Label, msg.Class}, "|")
}

func blockKey(block reviewflow.Block) string {
	return strings.Join([]string{string(block.Kind), block.Label, block.Class, block.StageID, strings.Join(block.SourceEdgeIDs, ",")}, "|")
}

func appendUnique(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	set := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = true
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func truncateAnchors(anchors []string, max int) []string {
	if len(anchors) <= max || max <= 0 {
		return append([]string(nil), anchors...)
	}
	return append([]string(nil), anchors[:max]...)
}

func containsAny(value string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func cleanLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "unresolved_")
	replacements := []string{"$inline_handler", "handler", "$closure_return", "handler"}
	replacer := strings.NewReplacer(replacements...)
	raw = replacer.Replace(raw)
	if idx := strings.LastIndex(raw, "."); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	return humanize(raw)
}

func humanize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	normalized := strings.Builder{}
	for i, r := range raw {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(raw[i-1])
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				normalized.WriteByte(' ')
			}
		}
		switch r {
		case '/', '.', '_', '-', '(', ')', '{', '}', '[', ']', ':', ';', ',', '"', '\'':
			normalized.WriteByte(' ')
		default:
			normalized.WriteRune(r)
		}
	}
	parts := strings.Fields(normalized.String())
	for i := range parts {
		if strings.EqualFold(parts[i], "qr") {
			parts[i] = "QR"
			continue
		}
		if strings.EqualFold(parts[i], "http") {
			parts[i] = "HTTP"
			continue
		}
	}
	return strings.Join(parts, " ")
}

func shortName(canonical string) string {
	if idx := strings.LastIndex(canonical, "."); idx >= 0 && idx+1 < len(canonical) {
		return canonical[idx+1:]
	}
	return canonical
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

package reviewflow_build

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/participant_classify"
	"analysis-module/pkg/ids"
)

type CandidateKind string

const (
	CandidateFaithful        CandidateKind = "faithful"
	CandidateCompactReview   CandidateKind = "compact_review"
	CandidateAsyncSummarized CandidateKind = "async_summarized"
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

// BuildResult is the deterministic reviewflow generation result for one root.
type BuildResult struct {
	Selected   reviewflow.Flow   `json:"selected"`
	Candidates []reviewflow.Flow `json:"candidates"`
	Scores     []CandidateScore  `json:"scores"`
	SelectedID string            `json:"selected_id"`
	Signature  string            `json:"signature"`
}

// ScoreBreakdown records deterministic score dimensions.
type ScoreBreakdown struct {
	BusinessClarity int `json:"business_clarity"`
	StageQuality    int `json:"stage_quality"`
	BlockQuality    int `json:"block_quality"`
	Readability     int `json:"readability"`
	NoisePenalty    int `json:"noise_penalty"`
	ArtifactPenalty int `json:"artifact_penalty"`
}

// CandidateScore records the deterministic score for one candidate.
type CandidateScore struct {
	FlowID           string         `json:"flow_id"`
	CandidateKind    string         `json:"candidate_kind"`
	Signature        string         `json:"signature"`
	Score            int            `json:"score"`
	Breakdown        ScoreBreakdown `json:"breakdown"`
	ParticipantCount int            `json:"participant_count"`
	StageCount       int            `json:"stage_count"`
	MessageCount     int            `json:"message_count"`
}

// Service builds reviewer-facing flow abstractions from reduced chains.
type Service struct {
	classifier participant_classify.Service
}

// New creates a reviewflow builder.
func New() Service {
	return Service{
		classifier: participant_classify.New(),
	}
}

type candidateSettings struct {
	kind                 CandidateKind
	compactHelpers       bool
	summarizeResponse    bool
	summarizeAsync       bool
	includeGenericReturn bool
}

type buildContext struct {
	snapshot graph.GraphSnapshot
	root     entrypoint.Root
	chain    reduced.Chain
	audit    flow_stitch.SemanticAuditRoot
	nodeByID map[string]graph.Node
}

type participantState struct {
	root           entrypoint.Root
	chain          reduced.Chain
	snapshot       graph.GraphSnapshot
	nodeByID       map[string]graph.Node
	classifier     participant_classify.Service
	settings       candidateSettings
	orderedIDs     []string
	participants   map[string]*reviewflow.Participant
	sourceNodeSets map[string]map[string]bool
}

type participantView struct {
	Participant reviewflow.Participant
	Profile     participant_classify.Profile
	Node        graph.Node
}

type stageAccumulator struct {
	kind           string
	label          string
	messages       []reviewflow.Message
	participantSet map[string]bool
	sourceEdgeSet  map[string]bool
	seen           map[string]bool
}

// Build creates a deterministic reviewflow candidate set for one root.
func (s Service) Build(snapshot graph.GraphSnapshot, root entrypoint.Root, chain reduced.Chain, audit flow_stitch.SemanticAuditRoot) (BuildResult, error) {
	if chain.RootNodeID == "" {
		return BuildResult{}, nil
	}

	ctx := buildContext{
		snapshot: snapshot,
		root:     normalizeRoot(root),
		chain:    chain,
		audit:    audit,
		nodeByID: indexGraphNodes(snapshot.Nodes),
	}

	settingsList := []candidateSettings{
		{kind: CandidateFaithful},
		{kind: CandidateCompactReview, compactHelpers: true, summarizeResponse: true, includeGenericReturn: true},
		{kind: CandidateAsyncSummarized, compactHelpers: true, summarizeResponse: true, summarizeAsync: true, includeGenericReturn: true},
	}

	candidates := make([]reviewflow.Flow, 0, len(settingsList))
	for _, settings := range settingsList {
		candidate, err := s.buildCandidate(ctx, settings)
		if err != nil {
			return BuildResult{}, err
		}
		candidates = append(candidates, candidate)
	}

	scores := scoreCandidates(candidates)
	best := selectBestScore(scores)
	selected := reviewflow.Flow{}
	for i := range candidates {
		candidates[i].Metadata.Score = findScore(scores, candidates[i].ID).Score
		if candidates[i].ID == best.FlowID {
			selected = candidates[i]
		}
	}

	return BuildResult{
		Selected:   selected,
		Candidates: candidates,
		Scores:     scores,
		SelectedID: best.FlowID,
		Signature:  best.Signature,
	}, nil
}

func (s Service) buildCandidate(ctx buildContext, settings candidateSettings) (reviewflow.Flow, error) {
	participants := newParticipantState(ctx.root, ctx.chain, ctx.snapshot, ctx.nodeByID, s.classifier, settings)
	stageOrder := []string{
		stageBoundaryEntry,
		stageRequestPreparation,
		stageSessionContext,
		stageBusinessCore,
		stagePostProcessing,
		stageResponse,
		stageDeferredAsync,
	}
	stages := map[string]*stageAccumulator{}
	for _, kind := range stageOrder {
		stages[kind] = &stageAccumulator{
			kind:           kind,
			label:          stageLabel(kind),
			participantSet: map[string]bool{},
			sourceEdgeSet:  map[string]bool{},
			seen:           map[string]bool{},
		}
	}

	boundary := participants.ensureBoundary()
	handler, hasHandler := s.resolveHandlerParticipant(ctx, participants)
	if hasHandler {
		msg := reviewflow.Message{
			ID:                ids.Stable("review_msg", ctx.root.NodeID, "boundary_entry", boundary.Participant.ID, handler.Participant.ID, string(settings.kind)),
			FromParticipantID: boundary.Participant.ID,
			ToParticipantID:   handler.Participant.ID,
			Label:             dispatchLabel(ctx.root),
			Kind:              reviewflow.MessageSync,
			SourceEdgeIDs:     collectEntrySourceEdgeIDs(ctx.audit),
		}
		stages[stageBoundaryEntry].add(msg)
	}

	for _, edge := range orderedReducedEdges(ctx.chain.Edges) {
		if hasHandler && edge.FromID == ctx.chain.RootNodeID {
			to := participants.ensure(edge.ToID, "")
			if to.Participant.ID == handler.Participant.ID {
				continue
			}
		}
		from := participants.ensure(edge.FromID, "")
		to := participants.ensure(edge.ToID, "")
		msg, ok := s.edgeMessage(ctx.root, edge, from, to, settings)
		if !ok {
			continue
		}
		stageKind := stageForMessage(msg.Label, from, to)
		stages[stageKind].add(msg)
	}

	for _, auditEdge := range orderedAuditEdges(ctx.audit.FirstBusinessCalls) {
		from := handler
		if !hasHandler {
			from = boundary
		}
		to := participants.ensure(auditEdge.ToNodeID, auditEdge.Label)
		msg := reviewflow.Message{
			ID:                ids.Stable("review_msg", ctx.root.NodeID, "audit", auditEdge.EdgeID, string(settings.kind)),
			FromParticipantID: from.Participant.ID,
			ToParticipantID:   to.Participant.ID,
			Label:             summarizeBusinessLabel(auditEdge.Label, to.Participant.Label, settings),
			Kind:              messageKindFromProfiles(from, to, true),
			SourceEdgeIDs:     []string{auditEdge.EdgeID},
		}
		stages[stageBusinessCore].add(msg)
	}

	blocks := s.buildBlocks(ctx, participants, settings, hasHandler, handler.Participant.ID)
	for _, block := range blocks {
		if block.StageID == "" {
			continue
		}
		if block.Kind == reviewflow.BlockSummary {
			stages[block.StageID].add(reviewflow.Message{
				ID:                ids.Stable("review_msg", ctx.root.NodeID, block.ID, "summary"),
				FromParticipantID: boundary.Participant.ID,
				ToParticipantID:   boundary.Participant.ID,
				Label:             block.Label,
				Kind:              reviewflow.MessageSync,
				SourceEdgeIDs:     append([]string(nil), block.SourceEdgeIDs...),
			})
		}
	}

	if settings.includeGenericReturn && ctx.root.RootType == entrypoint.RootHTTP && hasHandler {
		responseFrom := handler.Participant.ID
		if response, ok := participants.firstByKind(string(participant_classify.BucketResponse)); ok {
			responseFrom = response.ID
		}
		stages[stageResponse].add(reviewflow.Message{
			ID:                ids.Stable("review_msg", ctx.root.NodeID, "response", responseFrom, boundary.Participant.ID, string(settings.kind)),
			FromParticipantID: responseFrom,
			ToParticipantID:   boundary.Participant.ID,
			Label:             responseLabel(ctx.root),
			Kind:              reviewflow.MessageReturn,
		})
	}

	notes := buildReviewNotes(ctx, participants)
	stageList := make([]reviewflow.Stage, 0, len(stageOrder))
	for _, kind := range stageOrder {
		stage := stages[kind].build(ctx.root.NodeID)
		if len(stage.Messages) == 0 {
			continue
		}
		stageList = append(stageList, stage)
	}

	participantList := participants.list()
	flow := reviewflow.Flow{
		RootNodeID:     ctx.root.NodeID,
		CanonicalName:  ctx.root.CanonicalName,
		Participants:   participantList,
		Stages:         stageList,
		Blocks:         blocks,
		Notes:          notes,
		SourceRootType: string(ctx.root.RootType),
		SourceEvidence: ctx.root.Evidence,
		Metadata: reviewflow.Metadata{
			CandidateKind: string(settings.kind),
			RootFramework: ctx.root.Framework,
		},
	}
	signature := buildSignature(flow)
	flow.ID = ids.Stable("reviewflow", ctx.root.NodeID, string(settings.kind), signature)
	flow.Metadata.Signature = signature
	return flow, nil
}

func newParticipantState(root entrypoint.Root, chain reduced.Chain, snapshot graph.GraphSnapshot, nodeByID map[string]graph.Node, classifier participant_classify.Service, settings candidateSettings) *participantState {
	return &participantState{
		root:           root,
		chain:          chain,
		snapshot:       snapshot,
		nodeByID:       nodeByID,
		classifier:     classifier,
		settings:       settings,
		participants:   map[string]*reviewflow.Participant{},
		sourceNodeSets: map[string]map[string]bool{},
	}
}

func (p *participantState) ensureBoundary() participantView {
	label := p.root.CanonicalName
	if p.root.Framework == "grpc-gateway" {
		label = "Gateway Proxy"
	}
	part := p.ensureByIdentity("boundary", label, string(participant_classify.BucketBoundary), string(reduced.RoleBoundary), false, p.root.NodeID)
	return participantView{
		Participant: part,
		Profile: participant_classify.Profile{
			Role:         reduced.RoleBoundary,
			Bucket:       participant_classify.BucketBoundary,
			ShortName:    p.root.CanonicalName,
			DisplayLabel: label,
		},
		Node: p.nodeFor(p.root.NodeID, p.root.CanonicalName),
	}
}

func (p *participantState) ensure(nodeID, fallbackCanonical string) participantView {
	if nodeID == p.root.NodeID || nodeID == p.chain.RootNodeID {
		return p.ensureBoundary()
	}

	node := p.nodeFor(nodeID, fallbackCanonical)
	profile := p.classifier.Profile(node, p.snapshot)
	if existing, ok := p.nodeByID[node.ID]; ok {
		profile = p.classifier.Profile(existing, p.snapshot)
		node = existing
	}
	kind, label, key, role, external := p.identityFor(node, profile)
	part := p.ensureByIdentity(key, label, kind, role, external, node.ID)
	return participantView{
		Participant: part,
		Profile:     profile,
		Node:        node,
	}
}

func (p *participantState) ensureByIdentity(key, label, kind, role string, external bool, sourceNodeID string) reviewflow.Participant {
	id := ids.Stable("review_participant", p.root.NodeID, key)
	if existing, ok := p.participants[id]; ok {
		if p.sourceNodeSets[id] == nil {
			p.sourceNodeSets[id] = map[string]bool{}
		}
		if sourceNodeID != "" {
			p.sourceNodeSets[id][sourceNodeID] = true
		}
		existing.SourceNodeIDs = sortedKeys(p.sourceNodeSets[id])
		return *existing
	}

	part := &reviewflow.Participant{
		ID:         id,
		Kind:       kind,
		Label:      label,
		Role:       role,
		IsExternal: external,
	}
	p.participants[id] = part
	p.sourceNodeSets[id] = map[string]bool{}
	if sourceNodeID != "" {
		p.sourceNodeSets[id][sourceNodeID] = true
	}
	part.SourceNodeIDs = sortedKeys(p.sourceNodeSets[id])
	p.orderedIDs = append(p.orderedIDs, id)
	return *part
}

func (p *participantState) identityFor(node graph.Node, profile participant_classify.Profile) (kind, label, key, role string, external bool) {
	role = string(profile.Role)
	cluster := clusterToken(profile)
	switch {
	case p.root.Framework == "grpc-gateway" && (strings.Contains(strings.ToLower(node.CanonicalName), "handlerfromendpoint") || strings.HasPrefix(strings.ToLower(node.CanonicalName), "register")):
		return string(participant_classify.BucketGateway), "Gateway Client", "gateway_client", string(profile.Role), profile.IsRemote
	case profile.Bucket == participant_classify.BucketResponse:
		return string(profile.Bucket), "Response", "response", role, false
	case profile.Bucket == participant_classify.BucketValidation:
		return string(profile.Bucket), "Validation", "validation", role, false
	case profile.Bucket == participant_classify.BucketSessionAuth:
		return string(profile.Bucket), "Session/Auth", "session_auth", role, false
	case profile.Bucket == participant_classify.BucketHandler:
		return string(profile.Bucket), "Handler", "handler", role, false
	case profile.Bucket == participant_classify.BucketRepo:
		label = "Repository"
		key = "repo"
		if cluster != "" {
			label = titleToken(cluster) + " Repo"
			key = "repo:" + cluster
		}
		return string(profile.Bucket), label, key, role, false
	case profile.Bucket == participant_classify.BucketService:
		label = "Service"
		key = "service"
		if cluster != "" {
			label = titleToken(cluster) + " Service"
			key = "service:" + cluster
		}
		return string(profile.Bucket), label, key, role, false
	case profile.Bucket == participant_classify.BucketProcessor:
		label = "Processor"
		key = "processor"
		if cluster != "" {
			label = titleToken(cluster) + " Processor"
			key = "processor:" + cluster
		}
		return string(profile.Bucket), label, key, role, false
	case profile.Bucket == participant_classify.BucketAlgorithm || profile.Bucket == participant_classify.BucketCoreEngine:
		label = "Business Logic"
		key = "algorithm"
		if cluster != "" {
			label = titleToken(cluster) + " Logic"
			key = "algorithm:" + cluster
		}
		return string(profile.Bucket), label, key, role, false
	case profile.Bucket == participant_classify.BucketGateway:
		label = "Gateway Client"
		key = "gateway_client"
		if cluster != "" {
			label = titleToken(cluster) + " Client"
			key = "gateway_client:" + cluster
		}
		return string(profile.Bucket), label, key, role, profile.IsRemote
	case profile.Bucket == participant_classify.BucketAsyncSink:
		return string(profile.Bucket), "Async Worker", "async_sink", role, false
	case profile.Bucket == participant_classify.BucketRemote:
		label = profile.DisplayLabel
		if label == "" {
			label = "Remote"
		}
		return string(profile.Bucket), label, "remote:" + ids.Slug(label), role, true
	case profile.Bucket == participant_classify.BucketHelper && p.settings.compactHelpers:
		return string(participant_classify.BucketHandler), "Handler", "handler", string(reduced.RoleHandler), false
	default:
		label = profile.DisplayLabel
		if label == "" {
			label = humanizeFallback(node.CanonicalName)
		}
		return string(profile.Bucket), label, string(profile.Bucket) + ":" + ids.Slug(label), role, false
	}
}

func (p *participantState) nodeFor(nodeID, fallbackCanonical string) graph.Node {
	if node, ok := p.nodeByID[nodeID]; ok {
		return node
	}
	canonical := fallbackCanonical
	if canonical == "" && strings.HasPrefix(nodeID, "unresolved_") {
		canonical = strings.TrimPrefix(nodeID, "unresolved_")
	}
	return graph.Node{
		ID:            nodeID,
		CanonicalName: canonical,
		Properties: map[string]string{
			"name": shortName(canonical),
		},
	}
}

func (p *participantState) list() []reviewflow.Participant {
	out := make([]reviewflow.Participant, 0, len(p.orderedIDs))
	for _, id := range p.orderedIDs {
		part := *p.participants[id]
		part.SourceNodeIDs = sortedKeys(p.sourceNodeSets[id])
		out = append(out, part)
	}
	return out
}

func (p *participantState) firstByKind(kind string) (reviewflow.Participant, bool) {
	for _, id := range p.orderedIDs {
		part := p.participants[id]
		if part.Kind == kind {
			return *part, true
		}
	}
	return reviewflow.Participant{}, false
}

func (s Service) resolveHandlerParticipant(ctx buildContext, participants *participantState) (participantView, bool) {
	if ctx.audit.HandlerTargetNode != nil {
		return participants.ensure(ctx.audit.HandlerTargetNode.NodeID, ctx.audit.HandlerTargetNode.CanonicalName), true
	}
	if ctx.audit.ClosureBodyNode != nil {
		return participants.ensure(ctx.audit.ClosureBodyNode.NodeID, ctx.audit.ClosureBodyNode.CanonicalName), true
	}
	edges := orderedReducedEdges(ctx.chain.Edges)
	for _, edge := range edges {
		if edge.FromID == ctx.chain.RootNodeID {
			return participants.ensure(edge.ToID, ""), true
		}
	}
	return participantView{}, false
}

func (s Service) edgeMessage(root entrypoint.Root, edge reduced.Edge, from, to participantView, settings candidateSettings) (reviewflow.Message, bool) {
	if settings.compactHelpers && from.Participant.ID == to.Participant.ID {
		return reviewflow.Message{}, false
	}

	label := abstractEdgeLabel(root, edge, from, to, settings)
	if label == "" {
		return reviewflow.Message{}, false
	}
	return reviewflow.Message{
		ID:                ids.Stable("review_msg", root.NodeID, edgeRef(edge), string(settings.kind)),
		FromParticipantID: from.Participant.ID,
		ToParticipantID:   to.Participant.ID,
		Label:             label,
		Kind:              messageKindFromEdge(edge, to),
		SourceEdgeIDs:     []string{edgeRef(edge)},
	}, true
}

func (s Service) buildBlocks(ctx buildContext, participants *participantState, settings candidateSettings, hasHandler bool, handlerID string) []reviewflow.Block {
	blocks := make([]reviewflow.Block, 0, len(ctx.chain.Blocks))
	for _, block := range orderedReducedBlocks(ctx.chain.Blocks) {
		rfBlock := reviewflow.Block{
			ID:            ids.Stable("review_block", ctx.root.NodeID, fmt.Sprintf("%d", block.OrderIndex), string(settings.kind)),
			Kind:          mapBlockKind(block.Kind, settings),
			Label:         summarizeBlockLabel(block, settings),
			StageID:       blockStage(block),
			SourceEdgeIDs: blockEdgeRefs(block),
		}

		for idx, branch := range block.Branches {
			section := reviewflow.BlockSection{
				Label:         summarizeSectionLabel(branch.Condition),
				SourceEdgeIDs: branchEdgeRefs(branch),
			}
			for _, edge := range branch.Edges {
				from := participants.ensure(edge.FromID, "")
				to := participants.ensure(edge.ToID, "")
				msg, ok := s.edgeMessage(ctx.root, edge, from, to, settings)
				if !ok {
					continue
				}
				if settings.summarizeAsync && block.Kind == reduced.BlockPar {
					msg.Label = "spawn worker"
				}
				section.Messages = append(section.Messages, msg)
			}

			if settings.summarizeAsync && block.Kind == reduced.BlockPar && len(section.Messages) == 0 && hasHandler {
				asyncParticipant := participants.ensureByIdentity("async_sink", "Async Worker", string(participant_classify.BucketAsyncSink), string(reduced.RoleAsync), false, "")
				section.Messages = append(section.Messages, reviewflow.Message{
					ID:                ids.Stable("review_msg", ctx.root.NodeID, rfBlock.ID, fmt.Sprintf("branch_%d", idx)),
					FromParticipantID: handlerID,
					ToParticipantID:   asyncParticipant.ID,
					Label:             "spawn worker",
					Kind:              reviewflow.MessageAsync,
				})
			}

			if len(section.Messages) > 0 {
				rfBlock.Sections = append(rfBlock.Sections, section)
			}
		}
		if len(rfBlock.Sections) == 0 && rfBlock.Kind != reviewflow.BlockSummary {
			continue
		}
		blocks = append(blocks, rfBlock)
	}
	return blocks
}

func buildReviewNotes(ctx buildContext, participants *participantState) []reviewflow.Note {
	var notes []reviewflow.Note
	for _, warning := range ctx.audit.Warnings {
		text := warning.Message
		if text == "" {
			continue
		}
		over := ""
		if boundary, ok := participants.firstByKind(string(participant_classify.BucketBoundary)); ok {
			over = boundary.ID
		}
		notes = append(notes, reviewflow.Note{
			ID:                ids.Stable("review_note", ctx.root.NodeID, warning.Kind, text),
			OverParticipantID: over,
			Text:              text,
			Kind:              warning.Kind,
		})
	}
	for _, note := range ctx.chain.Notes {
		if note.Kind != "subset" && !strings.Contains(strings.ToLower(note.Text), "external") {
			continue
		}
		participant, ok := participants.firstByKind(string(participant_classify.BucketBoundary))
		if nodeParticipant, exists := participants.participants[ids.Stable("review_participant", ctx.root.NodeID, "boundary")]; exists {
			participant = *nodeParticipant
			ok = true
		}
		if !ok {
			continue
		}
		notes = append(notes, reviewflow.Note{
			ID:                ids.Stable("review_note", ctx.root.NodeID, note.AtNodeID, note.Text),
			OverParticipantID: participant.ID,
			Text:              note.Text,
			Kind:              note.Kind,
			SourceNodeIDs:     []string{note.AtNodeID},
		})
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].ID < notes[j].ID })
	return notes
}

func (s *stageAccumulator) add(msg reviewflow.Message) {
	key := msg.FromParticipantID + "|" + msg.ToParticipantID + "|" + string(msg.Kind) + "|" + msg.Label
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	s.messages = append(s.messages, msg)
	s.participantSet[msg.FromParticipantID] = true
	s.participantSet[msg.ToParticipantID] = true
	for _, ref := range msg.SourceEdgeIDs {
		if ref != "" {
			s.sourceEdgeSet[ref] = true
		}
	}
}

func (s *stageAccumulator) build(rootNodeID string) reviewflow.Stage {
	participantIDs := sortedKeys(s.participantSet)
	sourceEdgeIDs := sortedKeys(s.sourceEdgeSet)
	return reviewflow.Stage{
		ID:             ids.Stable("review_stage", rootNodeID, s.kind),
		Kind:           s.kind,
		Label:          s.label,
		ParticipantIDs: participantIDs,
		Messages:       append([]reviewflow.Message(nil), s.messages...),
		SourceEdgeIDs:  sourceEdgeIDs,
	}
}

func stageForMessage(label string, from, to participantView) string {
	lower := strings.ToLower(label)
	switch {
	case from.Participant.Kind == string(participant_classify.BucketBoundary) && to.Participant.Kind == string(participant_classify.BucketHandler):
		return stageBoundaryEntry
	case to.Participant.Kind == string(participant_classify.BucketValidation) || containsReviewTokens(lower, "bind", "validate", "decode", "parse"):
		return stageRequestPreparation
	case to.Participant.Kind == string(participant_classify.BucketSessionAuth) || containsReviewTokens(lower, "session", "auth", "token", "context"):
		return stageSessionContext
	case to.Participant.Kind == string(participant_classify.BucketResponse):
		return stageResponse
	case to.Participant.Kind == string(participant_classify.BucketAsyncSink):
		return stageDeferredAsync
	case containsReviewTokens(lower, "sort", "merge", "classify", "select", "transform", "normalize"):
		return stagePostProcessing
	default:
		return stageBusinessCore
	}
}

func messageKindFromEdge(edge reduced.Edge, to participantView) reviewflow.MessageKind {
	if to.Participant.Kind == string(participant_classify.BucketAsyncSink) || edge.CrossRepo {
		return reviewflow.MessageAsync
	}
	return reviewflow.MessageSync
}

func messageKindFromProfiles(from, to participantView, inferredBusiness bool) reviewflow.MessageKind {
	if to.Participant.Kind == string(participant_classify.BucketRemote) || to.Participant.Kind == string(participant_classify.BucketGateway) {
		return reviewflow.MessageAsync
	}
	if inferredBusiness && from.Participant.Kind == string(participant_classify.BucketHandler) && to.Participant.Kind == string(participant_classify.BucketResponse) {
		return reviewflow.MessageReturn
	}
	return reviewflow.MessageSync
}

func abstractEdgeLabel(root entrypoint.Root, edge reduced.Edge, from, to participantView, settings candidateSettings) string {
	switch {
	case from.Participant.Kind == string(participant_classify.BucketBoundary) && to.Participant.Kind == string(participant_classify.BucketHandler):
		return dispatchLabel(root)
	case to.Participant.Kind == string(participant_classify.BucketValidation):
		return "validate request"
	case to.Participant.Kind == string(participant_classify.BucketSessionAuth):
		return "load session context"
	case to.Participant.Kind == string(participant_classify.BucketResponse):
		if settings.summarizeResponse {
			return "prepare response"
		}
		return "write response"
	case to.Participant.Kind == string(participant_classify.BucketRepo):
		return summarizeBusinessLabel(edge.Label, "query repository", settings)
	case to.Participant.Kind == string(participant_classify.BucketService):
		return summarizeBusinessLabel(edge.Label, "run service", settings)
	case to.Participant.Kind == string(participant_classify.BucketProcessor):
		return summarizeBusinessLabel(edge.Label, "process request", settings)
	case to.Participant.Kind == string(participant_classify.BucketGateway):
		return summarizeBusinessLabel(edge.Label, "call gateway client", settings)
	case to.Participant.Kind == string(participant_classify.BucketRemote):
		return summarizeBusinessLabel(edge.Label, "call remote service", settings)
	case to.Participant.Kind == string(participant_classify.BucketAlgorithm) || to.Participant.Kind == string(participant_classify.BucketCoreEngine):
		return summarizeBusinessLabel(edge.Label, "apply business logic", settings)
	case to.Participant.Kind == string(participant_classify.BucketAsyncSink):
		if settings.summarizeAsync {
			return "spawn worker"
		}
	}

	label := cleanReviewLabel(edge.Label)
	if label == "" {
		label = cleanReviewLabel(to.Participant.Label)
	}
	if settings.compactHelpers && isNoisyReviewLabel(label) {
		return ""
	}
	if label == "" {
		return "call"
	}
	return label
}

func summarizeBusinessLabel(raw, fallback string, settings candidateSettings) string {
	label := cleanReviewLabel(raw)
	if settings.kind == CandidateFaithful && label != "" {
		return label
	}
	if label == "" || isGenericLabel(label) || isNoisyReviewLabel(label) {
		return fallback
	}
	return label
}

func orderedReducedEdges(edges []reduced.Edge) []reduced.Edge {
	out := append([]reduced.Edge(nil), edges...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].OrderIndex != out[j].OrderIndex {
			return out[i].OrderIndex < out[j].OrderIndex
		}
		if out[i].FromID != out[j].FromID {
			return out[i].FromID < out[j].FromID
		}
		if out[i].ToID != out[j].ToID {
			return out[i].ToID < out[j].ToID
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func orderedReducedBlocks(blocks []reduced.Block) []reduced.Block {
	out := append([]reduced.Block(nil), blocks...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].OrderIndex != out[j].OrderIndex {
			return out[i].OrderIndex < out[j].OrderIndex
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func orderedAuditEdges(edges []flow_stitch.SemanticAuditEdgeRef) []flow_stitch.SemanticAuditEdgeRef {
	out := append([]flow_stitch.SemanticAuditEdgeRef(nil), edges...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].EdgeID != out[j].EdgeID {
			return out[i].EdgeID < out[j].EdgeID
		}
		if out[i].ToNodeID != out[j].ToNodeID {
			return out[i].ToNodeID < out[j].ToNodeID
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func normalizeRoot(root entrypoint.Root) entrypoint.Root {
	if root.Method == "" || root.Path == "" {
		parts := strings.SplitN(root.CanonicalName, " ", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[1], "/") && isUpperASCII(parts[0]) {
			if root.Method == "" {
				root.Method = parts[0]
			}
			if root.Path == "" {
				root.Path = parts[1]
			}
		}
	}
	return root
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
		return humanizeFallback(kind)
	}
}

func dispatchLabel(root entrypoint.Root) string {
	if root.Framework == "grpc-gateway" {
		return "dispatch proxy request"
	}
	return "dispatch request"
}

func responseLabel(root entrypoint.Root) string {
	if root.Framework == "grpc-gateway" {
		return "proxy response"
	}
	return "return response"
}

func clusterToken(profile participant_classify.Profile) string {
	candidates := append([]string(nil), profile.NameTokens...)
	candidates = append(candidates, profile.PackageTokens...)
	if profile.ReceiverToken != "" {
		candidates = append(candidates, profile.ReceiverToken)
	}
	stopwords := map[string]bool{
		"get": true, "set": true, "handle": true, "handler": true, "service": true, "repo": true, "repository": true,
		"client": true, "processor": true, "process": true, "request": true, "response": true, "write": true,
		"json": true, "main": true, "http": true, "grpc": true, "camera": false, "detect": false,
	}
	for _, token := range candidates {
		if token == "" || stopwords[token] {
			continue
		}
		return token
	}
	return ""
}

func cleanReviewLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "unresolved_")
	raw = strings.ReplaceAll(raw, "$inline_handler_0", "handler")
	raw = strings.ReplaceAll(raw, "$closure_return_0", "handler")
	raw = strings.ReplaceAll(raw, "$inline_handler", "handler")
	raw = strings.ReplaceAll(raw, "$closure_return", "handler")
	if idx := strings.LastIndex(raw, "."); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	return humanizeFallback(raw)
}

func humanizeFallback(raw string) string {
	return participant_classifyHumanize(raw)
}

func participant_classifyHumanize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var normalized strings.Builder
	for i, r := range raw {
		if i > 0 && unicode.IsUpper(r) {
			prev := rune(raw[i-1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				normalized.WriteByte(' ')
			}
		}
		switch r {
		case '/', '.', '_', '-', ' ':
			normalized.WriteByte(' ')
		default:
			normalized.WriteRune(r)
		}
	}
	parts := strings.Fields(normalized.String())
	if len(parts) == 0 {
		return raw
	}
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

func containsReviewTokens(value string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func isNoisyReviewLabel(label string) bool {
	lower := strings.ToLower(label)
	return containsReviewTokens(lower, "$closure", "$inline", "goroutine", "helper", "wrapper")
}

func isGenericLabel(label string) bool {
	lower := strings.ToLower(label)
	return lower == "call" || lower == "handler" || lower == "service" || lower == "processor"
}

func mapBlockKind(kind reduced.BlockKind, settings candidateSettings) reviewflow.BlockKind {
	switch kind {
	case reduced.BlockAlt:
		return reviewflow.BlockAlt
	case reduced.BlockLoop:
		return reviewflow.BlockLoop
	case reduced.BlockPar:
		if settings.summarizeAsync {
			return reviewflow.BlockPar
		}
		return reviewflow.BlockPar
	default:
		return reviewflow.BlockSummary
	}
}

func summarizeBlockLabel(block reduced.Block, settings candidateSettings) string {
	switch block.Kind {
	case reduced.BlockPar:
		if settings.summarizeAsync {
			return "spawn worker"
		}
		if block.Label != "" {
			return cleanReviewLabel(block.Label)
		}
		return "parallel work"
	case reduced.BlockLoop:
		if block.Label != "" {
			return cleanReviewLabel(block.Label)
		}
		return "for each item"
	case reduced.BlockAlt:
		if block.Label != "" && block.Label != "branch" {
			return cleanReviewLabel(block.Label)
		}
		return "conditional path"
	default:
		return cleanReviewLabel(block.Label)
	}
}

func summarizeSectionLabel(label string) string {
	label = cleanReviewLabel(label)
	if label == "" {
		return "else"
	}
	return label
}

func blockStage(block reduced.Block) string {
	switch block.Kind {
	case reduced.BlockPar:
		return stageDeferredAsync
	case reduced.BlockLoop:
		return stageBusinessCore
	case reduced.BlockAlt:
		return stageBusinessCore
	default:
		return stagePostProcessing
	}
}

func collectEntrySourceEdgeIDs(audit flow_stitch.SemanticAuditRoot) []string {
	var refs []string
	if audit.RegistersBoundaryEdge != nil && audit.RegistersBoundaryEdge.EdgeID != "" {
		refs = append(refs, audit.RegistersBoundaryEdge.EdgeID)
	}
	if audit.ReturnsHandlerEdge != nil && audit.ReturnsHandlerEdge.EdgeID != "" {
		refs = append(refs, audit.ReturnsHandlerEdge.EdgeID)
	}
	sort.Strings(refs)
	return refs
}

func edgeRef(edge reduced.Edge) string {
	return fmt.Sprintf("%s|%s|%s|%d", edge.FromID, edge.ToID, edge.Label, edge.OrderIndex)
}

func blockEdgeRefs(block reduced.Block) []string {
	set := map[string]bool{}
	for _, branch := range block.Branches {
		for _, ref := range branchEdgeRefs(branch) {
			set[ref] = true
		}
	}
	return sortedKeys(set)
}

func branchEdgeRefs(branch reduced.Branch) []string {
	set := map[string]bool{}
	for _, edge := range branch.Edges {
		set[edgeRef(edge)] = true
	}
	return sortedKeys(set)
}

func buildSignature(flow reviewflow.Flow) string {
	var parts []string
	parts = append(parts, flow.RootNodeID, flow.CanonicalName, flow.SourceRootType, flow.Metadata.CandidateKind, flow.Metadata.RootFramework)
	for _, participant := range flow.Participants {
		parts = append(parts, "p:"+participant.Kind+":"+participant.Label)
	}
	for _, stage := range flow.Stages {
		parts = append(parts, "s:"+stage.Kind)
		for _, message := range stage.Messages {
			parts = append(parts, "m:"+message.FromParticipantID+":"+message.ToParticipantID+":"+string(message.Kind)+":"+message.Label)
		}
	}
	for _, block := range flow.Blocks {
		parts = append(parts, "b:"+string(block.Kind)+":"+block.Label)
	}
	for _, note := range flow.Notes {
		parts = append(parts, "n:"+note.Kind+":"+note.Text)
	}
	return strings.Join(parts, "|")
}

func indexGraphNodes(nodes []graph.Node) map[string]graph.Node {
	index := make(map[string]graph.Node, len(nodes))
	for _, node := range nodes {
		index[node.ID] = node
	}
	return index
}

func sortedKeys[K comparable](set map[K]bool) []K {
	keys := make([]K, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	return keys
}

func titleToken(token string) string {
	if token == "" {
		return ""
	}
	if strings.EqualFold(token, "qr") {
		return "QR"
	}
	return strings.ToUpper(token[:1]) + token[1:]
}

func shortName(canonical string) string {
	if idx := strings.LastIndex(canonical, "."); idx >= 0 && idx+1 < len(canonical) {
		return canonical[idx+1:]
	}
	return canonical
}

func isUpperASCII(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

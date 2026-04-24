package reviewflow_bootstrap

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/pkg/ids"
)

var ErrInsufficientEvidence = errors.New("bootstrap lifecycle insufficient")

const (
	stageProcessEntry      = "process_entry"
	stageConfiguration     = "configuration"
	stageDependencyInit    = "dependency_initialization"
	stageRouteRegistration = "route_registration"
	stageServerStart       = "server_start"
	stageServeLoop         = "serve_loop"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (s Service) Build(snapshot graph.GraphSnapshot, root entrypoint.Root) (reviewflow.Flow, error) {
	if root.NodeID == "" {
		return reviewflow.Flow{}, ErrInsufficientEvidence
	}
	nodeByID := make(map[string]graph.Node, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodeByID[node.ID] = node
	}
	rootNode, ok := nodeByID[root.NodeID]
	if !ok {
		return reviewflow.Flow{}, ErrInsufficientEvidence
	}

	processParticipant := reviewflow.Participant{
		ID:    ids.Stable("review_participant", root.NodeID, "bootstrap_process"),
		Kind:  "client",
		Label: "Process / CLI",
		Role:  "process",
	}
	mainParticipant := reviewflow.Participant{
		ID:            ids.Stable("review_participant", root.NodeID, "bootstrap_main"),
		Kind:          "bootstrap_main",
		Label:         humanizeCanonical(root.CanonicalName),
		Role:          "main",
		SourceNodeIDs: []string{root.NodeID},
	}
	if strings.TrimSpace(mainParticipant.Label) == "" {
		mainParticipant.Label = shortCanonical(rootNode.CanonicalName)
	}

	stages := make([]reviewflow.Stage, 0, 6)
	stageEdges := collectBootstrapEdges(snapshot, root.NodeID, 6, 256)
	evidenceCount := 0

	stages = append(stages, reviewflow.Stage{
		ID:             ids.Stable("review_stage", root.NodeID, stageProcessEntry),
		Kind:           stageProcessEntry,
		Label:          "Process Entry",
		ParticipantIDs: []string{processParticipant.ID, mainParticipant.ID},
		Messages: []reviewflow.Message{{
			ID:                ids.Stable("review_msg", root.NodeID, stageProcessEntry, "start_process"),
			FromParticipantID: processParticipant.ID,
			ToParticipantID:   mainParticipant.ID,
			Label:             "lifecycle summary: start process",
			Kind:              reviewflow.MessageSync,
		}},
	})

	type stageDef struct {
		stageKind  string
		stageLabel string
		tokens     []string
		fallback   string
	}
	defs := []stageDef{
		{
			stageKind:  stageConfiguration,
			stageLabel: "Configuration",
			tokens:     []string{"config", "viper", "env", "yaml", "toml", "load", "readconfig"},
		},
		{
			stageKind:  stageDependencyInit,
			stageLabel: "Dependency Initialization",
			tokens:     []string{"new", "init", "client", "service", "repo", "session", "kafka", "redis", "jaeger", "trace", "producer", "execute"},
		},
		{
			stageKind:  stageRouteRegistration,
			stageLabel: "Route Registration",
			tokens:     []string{"router", "route", "group", "get", "post", "handler", "withrouter", "use", "middleware"},
		},
		{
			stageKind:  stageServerStart,
			stageLabel: "Server Start",
			tokens:     []string{"run", "listen", "serve", "starthttp", "startserver"},
			fallback:   "lifecycle summary: start HTTP server",
		},
	}

	participantsByID := map[string]reviewflow.Participant{
		processParticipant.ID: processParticipant,
		mainParticipant.ID:    mainParticipant,
	}

	for _, def := range defs {
		msg, participant, matched := buildEvidenceMessage(root, mainParticipant, stageEdges, def.tokens, nodeByID)
		if matched {
			evidenceCount++
			participantsByID[participant.ID] = participant
			stages = append(stages, reviewflow.Stage{
				ID:             ids.Stable("review_stage", root.NodeID, def.stageKind),
				Kind:           def.stageKind,
				Label:          def.stageLabel,
				ParticipantIDs: []string{mainParticipant.ID, participant.ID},
				Messages:       []reviewflow.Message{msg},
				SourceEdgeIDs:  append([]string(nil), msg.SourceEdgeIDs...),
			})
			continue
		}
		if strings.TrimSpace(def.fallback) != "" {
			stages = append(stages, reviewflow.Stage{
				ID:             ids.Stable("review_stage", root.NodeID, def.stageKind),
				Kind:           def.stageKind,
				Label:          def.stageLabel,
				ParticipantIDs: []string{mainParticipant.ID},
				Messages: []reviewflow.Message{{
					ID:                ids.Stable("review_msg", root.NodeID, def.stageKind, "fallback"),
					FromParticipantID: mainParticipant.ID,
					ToParticipantID:   mainParticipant.ID,
					Label:             def.fallback,
					Kind:              reviewflow.MessageSync,
				}},
			})
		}
	}

	if evidenceCount == 0 {
		if msg, ok := rootEvidenceMessage(root, rootNode, mainParticipant); ok {
			evidenceCount++
			stages = append(stages, reviewflow.Stage{
				ID:             ids.Stable("review_stage", root.NodeID, stageConfiguration, "root_evidence"),
				Kind:           stageConfiguration,
				Label:          "Configuration",
				ParticipantIDs: []string{mainParticipant.ID},
				Messages:       []reviewflow.Message{msg},
			})
		} else {
			return reviewflow.Flow{}, ErrInsufficientEvidence
		}
	}

	stages = append(stages, reviewflow.Stage{
		ID:             ids.Stable("review_stage", root.NodeID, stageServeLoop),
		Kind:           stageServeLoop,
		Label:          "Serve Loop",
		ParticipantIDs: []string{mainParticipant.ID},
		Messages: []reviewflow.Message{{
			ID:                ids.Stable("review_msg", root.NodeID, stageServeLoop, "serve_requests"),
			FromParticipantID: mainParticipant.ID,
			ToParticipantID:   mainParticipant.ID,
			Label:             "lifecycle summary: serve requests",
			Kind:              reviewflow.MessageSync,
		}},
	})

	participants := make([]reviewflow.Participant, 0, len(participantsByID))
	for _, participant := range participantsByID {
		participants = append(participants, participant)
	}
	sort.SliceStable(participants, func(i, j int) bool {
		return participants[i].ID < participants[j].ID
	})

	flow := reviewflow.Flow{
		ID:             ids.Stable("reviewflow", root.NodeID, "bootstrap_lifecycle"),
		RootNodeID:     root.NodeID,
		CanonicalName:  root.CanonicalName,
		Participants:   participants,
		Stages:         stages,
		SourceRootType: string(root.RootType),
		SourceEvidence: root.Evidence,
		Metadata: reviewflow.Metadata{
			CandidateKind: "bootstrap_lifecycle",
			Signature:     ids.Stable("reviewflow_sig", root.NodeID, fmt.Sprintf("%d", len(stages)), fmt.Sprintf("%d", evidenceCount)),
			RootFramework: root.Framework,
		},
	}
	return flow, nil
}

func collectBootstrapEdges(snapshot graph.GraphSnapshot, rootNodeID string, maxDepth, maxEdges int) []graph.Edge {
	type queueItem struct {
		nodeID string
		depth  int
	}
	outgoing := map[string][]graph.Edge{}
	for _, edge := range snapshot.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge)
	}
	for nodeID := range outgoing {
		sort.SliceStable(outgoing[nodeID], func(i, j int) bool {
			if outgoing[nodeID][i].Kind != outgoing[nodeID][j].Kind {
				return outgoing[nodeID][i].Kind < outgoing[nodeID][j].Kind
			}
			return outgoing[nodeID][i].To < outgoing[nodeID][j].To
		})
	}

	queue := []queueItem{{nodeID: rootNodeID, depth: 0}}
	seenNode := map[string]bool{rootNodeID: true}
	seenEdge := map[string]bool{}
	edges := make([]graph.Edge, 0, 64)

	for len(queue) > 0 && len(edges) < maxEdges {
		item := queue[0]
		queue = queue[1:]
		if item.depth >= maxDepth {
			continue
		}
		for _, edge := range outgoing[item.nodeID] {
			if !isBootstrapEdgeKind(edge.Kind) {
				continue
			}
			if edge.ID != "" {
				if seenEdge[edge.ID] {
					continue
				}
				seenEdge[edge.ID] = true
			}
			edges = append(edges, edge)
			if !seenNode[edge.To] {
				seenNode[edge.To] = true
				queue = append(queue, queueItem{nodeID: edge.To, depth: item.depth + 1})
			}
		}
	}
	return edges
}

func isBootstrapEdgeKind(kind graph.EdgeKind) bool {
	switch kind {
	case graph.EdgeCalls, graph.EdgeEntrypointTo, graph.EdgeRegistersBoundary:
		return true
	default:
		return false
	}
}

func buildEvidenceMessage(root entrypoint.Root, mainParticipant reviewflow.Participant, edges []graph.Edge, tokens []string, nodeByID map[string]graph.Node) (reviewflow.Message, reviewflow.Participant, bool) {
	for _, edge := range edges {
		target := nodeByID[edge.To]
		needle := strings.ToLower(edge.Evidence.Details + " " + edge.Evidence.Source + " " + target.CanonicalName + " " + target.Properties["name"])
		if !containsToken(needle, tokens) {
			continue
		}
		targetLabel := humanizeCanonical(target.CanonicalName)
		if strings.TrimSpace(targetLabel) == "" {
			targetLabel = shortCanonical(target.CanonicalName)
		}
		participant := reviewflow.Participant{
			ID:            ids.Stable("review_participant", root.NodeID, "bootstrap_target", edge.To),
			Kind:          "bootstrap_component",
			Label:         targetLabel,
			Role:          "component",
			SourceNodeIDs: []string{target.ID},
		}
		label := shortCanonical(target.CanonicalName)
		if label == "" {
			label = targetLabel
		}
		msg := reviewflow.Message{
			ID:                ids.Stable("review_msg", root.NodeID, "bootstrap", edge.ID),
			FromParticipantID: mainParticipant.ID,
			ToParticipantID:   participant.ID,
			Label:             "call " + label,
			Kind:              reviewflow.MessageSync,
		}
		if edge.ID != "" {
			msg.SourceEdgeIDs = []string{edge.ID}
		}
		return msg, participant, true
	}
	return reviewflow.Message{}, reviewflow.Participant{}, false
}

func containsToken(value string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func shortCanonical(canonical string) string {
	canonical = strings.TrimSpace(canonical)
	if canonical == "" {
		return ""
	}
	parts := strings.FieldsFunc(canonical, func(r rune) bool {
		switch r {
		case '/', '\\', '.', ':':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return canonical
	}
	return parts[len(parts)-1]
}

func humanizeCanonical(canonical string) string {
	short := shortCanonical(canonical)
	short = strings.ReplaceAll(short, "_", " ")
	short = strings.TrimSpace(short)
	if short == "" {
		return ""
	}
	runes := []rune(strings.ToLower(short))
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func rootEvidenceMessage(root entrypoint.Root, rootNode graph.Node, mainParticipant reviewflow.Participant) (reviewflow.Message, bool) {
	rootEvidence := strings.TrimSpace(root.Evidence)
	if rootEvidence != "" {
		return reviewflow.Message{
			ID:                ids.Stable("review_msg", root.NodeID, stageConfiguration, "root_evidence"),
			FromParticipantID: mainParticipant.ID,
			ToParticipantID:   mainParticipant.ID,
			Label:             "lifecycle evidence: " + rootEvidence,
			Kind:              reviewflow.MessageSync,
		}, true
	}
	source := firstNonEmpty(
		rootNode.Properties["file_path"],
		rootNode.Properties["package"],
		rootNode.Properties["name"],
		rootNode.Properties["signature"],
	)
	if strings.TrimSpace(source) == "" {
		return reviewflow.Message{}, false
	}
	rootName := strings.TrimSpace(rootNode.CanonicalName)
	if rootName == "" {
		rootName = strings.TrimSpace(root.CanonicalName)
	}
	if rootName == "" {
		return reviewflow.Message{}, false
	}
	return reviewflow.Message{
		ID:                ids.Stable("review_msg", root.NodeID, stageConfiguration, "root_symbol"),
		FromParticipantID: mainParticipant.ID,
		ToParticipantID:   mainParticipant.ID,
		Label:             "lifecycle evidence: entrypoint " + shortCanonical(rootName),
		Kind:              reviewflow.MessageSync,
	}, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

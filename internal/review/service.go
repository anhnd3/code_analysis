package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"analysis-module/internal/facts"
	"analysis-module/internal/llm"
	factquery "analysis-module/internal/query"
	factsqlite "analysis-module/internal/store/sqlite"
	"analysis-module/pkg/ids"
)

type Request struct {
	WorkspaceID string
	SnapshotID  string
	Symbol      string
	MaxDepth    int
	MaxSteps    int
	OutDir      string
}

type Result struct {
	Flow facts.ReviewFlow `json:"flow"`
}

type Service struct {
	artifactRoot string
	query        factquery.Service
	client       llm.Client
}

func New(artifactRoot string, query factquery.Service, client llm.Client) Service {
	return Service{
		artifactRoot: artifactRoot,
		query:        query,
		client:       client,
	}
}

func (s Service) Run(req Request) (Result, error) {
	if req.MaxDepth <= 0 {
		req.MaxDepth = 3
	}
	if req.MaxSteps <= 0 {
		req.MaxSteps = 80
	}

	rootPacket, err := s.query.InspectFunction(factquery.InspectRequest{
		WorkspaceID: req.WorkspaceID,
		SnapshotID:  req.SnapshotID,
		Symbol:      req.Symbol,
	})
	if err != nil {
		return Result{}, err
	}
	flow := facts.ReviewFlow{
		ID:                ids.Stable("review", req.WorkspaceID, req.SnapshotID, rootPacket.RootSymbol.ID, time.Now().UTC().Format(time.RFC3339Nano)),
		WorkspaceID:       req.WorkspaceID,
		SnapshotID:        req.SnapshotID,
		RootSymbolID:      rootPacket.RootSymbol.ID,
		RootCanonicalName: rootPacket.RootSymbol.CanonicalName,
		CreatedAt:         time.Now().UTC(),
	}
	reviewDir := req.OutDir
	if reviewDir == "" {
		reviewDir = filepath.Join(s.artifactRoot, "workspaces", req.WorkspaceID, "snapshots", req.SnapshotID, "review")
	}

	type queueItem struct {
		SymbolID string
		Depth    int
	}
	queue := []queueItem{{SymbolID: rootPacket.RootSymbol.ID, Depth: 0}}
	visited := map[string]bool{}
	steps := 0

	for len(queue) > 0 && steps < req.MaxSteps {
		item := queue[0]
		queue = queue[1:]
		if visited[item.SymbolID] {
			continue
		}
		visited[item.SymbolID] = true

		symbolFact, err := s.query.SymbolByID(req.WorkspaceID, req.SnapshotID, item.SymbolID)
		if err != nil {
			flow.UncertaintyNotes = append(flow.UncertaintyNotes, fmt.Sprintf("failed to load symbol %s: %v", item.SymbolID, err))
			continue
		}
		packet, err := s.query.InspectFunction(factquery.InspectRequest{
			WorkspaceID: req.WorkspaceID,
			SnapshotID:  req.SnapshotID,
			Symbol:      symbolFact.ID,
		})
		if err != nil {
			flow.UncertaintyNotes = append(flow.UncertaintyNotes, fmt.Sprintf("failed to inspect symbol %s: %v", symbolFact.CanonicalName, err))
			continue
		}
		llmResp, err := s.client.Review(llm.ReviewRequest{
			RootSymbol: symbolFact,
			Packet:     packet,
			Depth:      item.Depth,
		})
		if err != nil {
			flow.UncertaintyNotes = append(flow.UncertaintyNotes, fmt.Sprintf("llm review failed for %s: %v", symbolFact.CanonicalName, err))
			if artifactErr := writeLLMErrorArtifact(reviewDir, req, symbolFact, item.Depth, err); artifactErr != nil {
				flow.UncertaintyNotes = append(flow.UncertaintyNotes, fmt.Sprintf("failed to write llm error artifact: %v", artifactErr))
			}
			llmResp = ambiguousResponseForPacket(packet, fmt.Sprintf("llm review unavailable: %v", err))
		}

		candidateByTarget := map[string]facts.CallCandidate{}
		for _, candidate := range packet.OutgoingCandidates {
			if candidate.ID != "" {
				candidateByTarget[candidate.ID] = candidate
			}
			if candidate.TargetSymbolID != "" {
				candidateByTarget[candidate.TargetSymbolID] = candidate
			}
			if candidate.TargetCanonicalName != "" {
				candidateByTarget[candidate.TargetCanonicalName] = candidate
			}
		}

		for _, decision := range llmResp.Decisions {
			candidate, matched := candidateForDecision(decision, candidateByTarget)
			target := candidate.TargetSymbolID
			targetCanonical := candidate.TargetCanonicalName
			status := decision.Status
			rationale := decision.Rationale

			if matched && targetCanonical == "" {
				targetCanonical = decision.TargetCanonicalName
			}
			if matched && targetCanonical == "" {
				targetCanonical = candidate.TargetCanonicalName
			}
			if matched && target == "" {
				target = decision.TargetSymbolID
			}
			if !matched {
				status = facts.StepAmbiguous
				if rationale == "" {
					rationale = "decision target did not map to an evidence-backed candidate"
				}
			}
			if status == facts.StepAccepted && candidate.TargetSymbolID == "" {
				status = facts.StepAmbiguous
				if rationale == "" {
					rationale = "accepted decision did not resolve to an evidence-backed target symbol"
				}
			}
			if status == facts.StepAccepted && target == "" {
				status = facts.StepAmbiguous
				if rationale == "" {
					rationale = "accepted candidate was not resolved to a stable target symbol"
				}
			}
			if target == "" && decision.TargetSymbolID != "" {
				target = decision.TargetSymbolID
			}
			if targetCanonical == "" {
				targetCanonical = decision.TargetCanonicalName
			}
			if targetCanonical == "" {
				targetCanonical = candidate.TargetCanonicalName
			}

			step := facts.ReviewStep{
				ID:                ids.Stable("review", "step", symbolFact.ID, target, targetCanonical, strconv.Itoa(steps)),
				FromSymbolID:      symbolFact.ID,
				FromCanonicalName: symbolFact.CanonicalName,
				ToSymbolID:        target,
				ToCanonicalName:   targetCanonical,
				Status:            status,
				Rationale:         rationale,
				Evidence: []facts.EvidenceRef{
					{
						SymbolID:  symbolFact.ID,
						FilePath:  packet.RootFile.RelativePath,
						StartLine: symbolFact.StartLine,
						EndLine:   symbolFact.EndLine,
						Source:    candidate.EvidenceSource,
					},
				},
			}
			flow.Steps = append(flow.Steps, step)
			switch step.Status {
			case facts.StepAccepted:
				flow.Accepted = append(flow.Accepted, step)
				if target != "" && item.Depth < req.MaxDepth {
					queue = append(queue, queueItem{SymbolID: target, Depth: item.Depth + 1})
				}
			case facts.StepRejected:
				flow.Rejected = append(flow.Rejected, step)
			default:
				flow.Ambiguous = append(flow.Ambiguous, step)
			}
			steps++
			if steps >= req.MaxSteps {
				break
			}
		}
		for _, note := range llmResp.Notes {
			flow.UncertaintyNotes = append(flow.UncertaintyNotes, note)
		}
	}

	store, err := factsqlite.New(factsqlite.PathFor(s.artifactRoot, req.WorkspaceID, req.SnapshotID))
	if err == nil {
		_ = store.SaveReviewFlow(flow)
		_ = store.Close()
	}
	return Result{Flow: flow}, nil
}

func candidateForDecision(decision llm.HopDecision, candidates map[string]facts.CallCandidate) (facts.CallCandidate, bool) {
	if decision.TargetSymbolID != "" {
		if candidate, ok := candidates[decision.TargetSymbolID]; ok {
			return candidate, true
		}
	}
	if decision.TargetCanonicalName != "" {
		if candidate, ok := candidates[decision.TargetCanonicalName]; ok {
			return candidate, true
		}
	}
	return facts.CallCandidate{}, false
}

func ambiguousResponseForPacket(packet facts.ContextPacket, note string) llm.ReviewResponse {
	decisions := make([]llm.HopDecision, 0, len(packet.OutgoingCandidates))
	for _, candidate := range packet.OutgoingCandidates {
		targetCanonical := candidate.TargetCanonicalName
		if targetCanonical == "" {
			targetCanonical = candidate.ID
		}
		decisions = append(decisions, llm.HopDecision{
			TargetSymbolID:      candidate.TargetSymbolID,
			TargetCanonicalName: targetCanonical,
			Status:              facts.StepAmbiguous,
			Rationale:           note,
		})
	}
	return llm.ReviewResponse{
		Decisions: decisions,
		Notes:     []string{note},
	}
}

func writeLLMErrorArtifact(reviewDir string, req Request, symbolFact facts.SymbolFact, depth int, err error) error {
	if reviewDir == "" {
		return nil
	}
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"workspace_id": req.WorkspaceID,
		"snapshot_id":  req.SnapshotID,
		"symbol_id":    symbolFact.ID,
		"symbol":       symbolFact.CanonicalName,
		"depth":        depth,
		"error":        err.Error(),
		"created_at":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		return marshalErr
	}
	return os.WriteFile(filepath.Join(reviewDir, "llm_error.json"), data, 0o644)
}

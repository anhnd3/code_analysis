package review

import (
	"fmt"
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
			llmResp, _ = llm.NoopClient{}.Review(llm.ReviewRequest{
				RootSymbol: symbolFact,
				Packet:     packet,
				Depth:      item.Depth,
			})
		}

		candidateByTarget := map[string]facts.CallCandidate{}
		for _, candidate := range packet.OutgoingCandidates {
			if candidate.TargetSymbolID != "" {
				candidateByTarget[candidate.TargetSymbolID] = candidate
			}
			if candidate.TargetCanonicalName != "" {
				candidateByTarget[candidate.TargetCanonicalName] = candidate
			}
		}

		for _, decision := range llmResp.Decisions {
			target := decision.TargetSymbolID
			targetCanonical := decision.TargetCanonicalName
			if target == "" && targetCanonical != "" {
				if resolved, resolveErr := s.query.ResolveSymbol(req.WorkspaceID, req.SnapshotID, targetCanonical); resolveErr == nil {
					target = resolved.ID
					targetCanonical = resolved.CanonicalName
				}
			}
			candidate := candidateByTarget[target]
			if candidate.ID == "" && targetCanonical != "" {
				candidate = candidateByTarget[targetCanonical]
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
				Status:            decision.Status,
				Rationale:         decision.Rationale,
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

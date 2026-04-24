package reviewflow_build

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/reviewflow_policy"
)

func scoreCandidates(candidates []reviewflow.Flow) []CandidateScore {
	return scoreCandidatesWithPolicy(candidates, nil)
}

func scoreCandidatesWithPolicy(candidates []reviewflow.Flow, policy *reviewflow_policy.Policy) []CandidateScore {
	scores := make([]CandidateScore, 0, len(candidates))
	for _, candidate := range candidates {
		breakdown := scoreBreakdown(candidate)
		total := breakdown.BusinessClarity + breakdown.StageQuality + breakdown.BlockQuality + breakdown.Readability - breakdown.NoisePenalty - breakdown.ArtifactPenalty
		scores = append(scores, CandidateScore{
			FlowID:           candidate.ID,
			CandidateKind:    candidate.Metadata.CandidateKind,
			Signature:        candidate.Metadata.Signature,
			Score:            total,
			Breakdown:        breakdown,
			ParticipantCount: len(candidate.Participants),
			StageCount:       len(candidate.Stages),
			MessageCount:     countMessages(candidate),
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		if scores[i].Breakdown.ArtifactPenalty != scores[j].Breakdown.ArtifactPenalty {
			return scores[i].Breakdown.ArtifactPenalty < scores[j].Breakdown.ArtifactPenalty
		}
		if scores[i].StageCount != scores[j].StageCount {
			return scores[i].StageCount > scores[j].StageCount
		}
		preferredKinds := preferredCandidateKinds(policy)
		if candidatePriority(scores[i].CandidateKind, preferredKinds) != candidatePriority(scores[j].CandidateKind, preferredKinds) {
			return candidatePriority(scores[i].CandidateKind, preferredKinds) < candidatePriority(scores[j].CandidateKind, preferredKinds)
		}
		return scores[i].Signature < scores[j].Signature
	})
	return scores
}

func selectBestScore(scores []CandidateScore) CandidateScore {
	if len(scores) == 0 {
		return CandidateScore{}
	}
	return scores[0]
}

func findScore(scores []CandidateScore, flowID string) CandidateScore {
	for _, score := range scores {
		if score.FlowID == flowID {
			return score
		}
	}
	return CandidateScore{}
}

func scoreBreakdown(flow reviewflow.Flow) ScoreBreakdown {
	breakdown := ScoreBreakdown{}
	messageCount := countMessages(flow)
	if hasStage(flow, stageBusinessCore) {
		breakdown.BusinessClarity += 4
	}
	if hasBusinessParticipants(flow) {
		breakdown.BusinessClarity += 3
	}
	if hasStage(flow, stageResponse) {
		breakdown.StageQuality += 2
	}
	if len(flow.Stages) >= 3 {
		breakdown.StageQuality += 2
	}
	if len(flow.Blocks) > 0 {
		breakdown.BlockQuality += 2
	}
	if len(flow.Participants) >= 2 && len(flow.Participants) <= 6 {
		breakdown.Readability += 3
	}
	if messageCount > 0 && messageCount <= 8 {
		breakdown.Readability += 2
	}

	for _, participant := range flow.Participants {
		lower := strings.ToLower(participant.Label)
		if strings.Contains(lower, "$closure") || strings.Contains(lower, "$inline") {
			breakdown.ArtifactPenalty += 5
		}
		if participant.Kind == "helper" {
			breakdown.NoisePenalty++
		}
	}
	for _, stage := range flow.Stages {
		for _, message := range stage.Messages {
			lower := strings.ToLower(message.Label)
			if strings.Contains(lower, "$closure") || strings.Contains(lower, "$inline") || strings.Contains(lower, "goroutine") {
				breakdown.ArtifactPenalty += 5
			}
			if strings.Contains(lower, "helper") || strings.Contains(lower, "wrapper") {
				breakdown.NoisePenalty++
			}
		}
	}
	if len(flow.Participants) > 7 {
		breakdown.NoisePenalty += len(flow.Participants) - 7
	}
	if messageCount > 10 {
		breakdown.NoisePenalty += messageCount - 10
	}
	return breakdown
}

func countMessages(flow reviewflow.Flow) int {
	count := 0
	for _, stage := range flow.Stages {
		count += len(stage.Messages)
	}
	for _, block := range flow.Blocks {
		for _, section := range block.Sections {
			count += len(section.Messages)
		}
	}
	return count
}

func hasStage(flow reviewflow.Flow, kind string) bool {
	for _, stage := range flow.Stages {
		if stage.Kind == kind {
			return true
		}
	}
	return false
}

func hasBusinessParticipants(flow reviewflow.Flow) bool {
	for _, participant := range flow.Participants {
		switch participant.Kind {
		case "repo", "service", "processor", "gateway_client", "remote", "algorithm", "core_engine":
			return true
		}
	}
	return false
}

func preferredCandidateKinds(policy *reviewflow_policy.Policy) []string {
	if policy == nil {
		return nil
	}
	return append([]string(nil), policy.PreferredCandidateKinds...)
}

func candidatePriority(kind string, preferred []string) int {
	for idx, pref := range preferred {
		if kind == pref {
			return idx
		}
	}
	base := len(preferred)
	switch kind {
	case string(CandidateCompactReview):
		return base
	case string(CandidateAsyncSummarized):
		return base + 1
	case string(CandidateFaithful):
		return base + 2
	default:
		return base + 3
	}
}

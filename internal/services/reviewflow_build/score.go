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
		breakdown := scoreBreakdown(candidate, policy)
		total := breakdown.BusinessClarity + breakdown.StageQuality + breakdown.BlockQuality + breakdown.Readability - breakdown.NoisePenalty - breakdown.ArtifactPenalty + breakdown.PolicyAdjustment
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

func scoreBreakdown(flow reviewflow.Flow, policy *reviewflow_policy.Policy) ScoreBreakdown {
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
	breakdown.PolicyAdjustment = policyScoreAdjustment(flow, policy)
	return breakdown
}

func policyScoreAdjustment(flow reviewflow.Flow, policy *reviewflow_policy.Policy) int {
	if policy == nil {
		return 0
	}

	candidateKind := flow.Metadata.CandidateKind
	participantCount := len(flow.Participants)
	blockCount := len(flow.Blocks)
	branchPresent := hasBranchSignals(flow)
	asyncPresent := hasAsyncSignals(flow)
	postProcessingPresent := hasStage(flow, stagePostProcessing)
	responsePresent := hasStage(flow, stageResponse)
	decisionPresent := hasDecisionSignals(flow)

	adjustment := 0
	switch policy.Family {
	case reviewflow_policy.FamilySimpleQuery, reviewflow_policy.FamilyConfigLookup:
		if candidateKind == string(CandidateCompactReview) {
			adjustment += 5
		}
		if participantCount <= 6 {
			adjustment += 2
		} else {
			adjustment -= participantCount - 6
		}
		if blockCount == 0 {
			adjustment += 2
		} else {
			adjustment -= blockCount
		}
		if responsePresent {
			adjustment += 2
		}
		if asyncPresent {
			adjustment -= 2
		}
		if branchPresent {
			adjustment -= 2
		}
	case reviewflow_policy.FamilyDetectorPipeline, reviewflow_policy.FamilyScanPipeline:
		if candidateKind == string(CandidateFaithful) {
			adjustment += 2
		}
		if policy.PreserveBranchBlocks {
			if branchPresent {
				adjustment += 4
			} else {
				adjustment -= 4
			}
		}
		if policy.PreserveAsyncBlocks {
			if asyncPresent {
				adjustment += 4
			} else {
				adjustment -= 4
			}
		}
		if policy.PreservePostProcessing {
			if postProcessingPresent {
				adjustment += 3
			} else {
				adjustment -= 3
			}
		}
		if participantCount >= 4 {
			adjustment += 1
		}
	case reviewflow_policy.FamilyBlacklistGate:
		if candidateKind == string(CandidateCompactReview) {
			adjustment += 3
		}
		if decisionPresent {
			adjustment += 3
		}
		if policy.PreserveBranchBlocks {
			if branchPresent {
				adjustment += 2
			} else {
				adjustment -= 2
			}
		}
		if policy.PreservePostProcessing {
			if postProcessingPresent {
				adjustment += 1
			} else {
				adjustment -= 1
			}
		}
	case reviewflow_policy.FamilyBootstrapStartup:
		return 0
	}

	adjustment += preferredKindBonus(candidateKind, policy.PreferredCandidateKinds)
	return adjustment
}

func preferredKindBonus(kind string, preferred []string) int {
	for idx, pref := range preferred {
		if pref != kind {
			continue
		}
		if idx == 0 {
			return 2
		}
		return 1
	}
	return 0
}

func hasBranchSignals(flow reviewflow.Flow) bool {
	for _, block := range flow.Blocks {
		if block.Kind == reviewflow.BlockAlt || block.Kind == reviewflow.BlockLoop || block.Kind == reviewflow.BlockPar {
			return true
		}
	}
	return false
}

func hasAsyncSignals(flow reviewflow.Flow) bool {
	if hasStage(flow, stageDeferredAsync) {
		return true
	}
	for _, stage := range flow.Stages {
		for _, message := range stage.Messages {
			if message.Kind == reviewflow.MessageAsync {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		if block.Kind == reviewflow.BlockPar {
			return true
		}
		for _, section := range block.Sections {
			for _, message := range section.Messages {
				if message.Kind == reviewflow.MessageAsync {
					return true
				}
			}
		}
	}
	return false
}

func hasDecisionSignals(flow reviewflow.Flow) bool {
	decisionTokens := []string{"allow", "deny", "block", "blacklist", "whitelist", "reject"}
	for _, stage := range flow.Stages {
		if containsAnyToken(stage.Label, decisionTokens...) {
			return true
		}
		for _, message := range stage.Messages {
			if containsAnyToken(message.Label, decisionTokens...) {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		if containsAnyToken(block.Label, decisionTokens...) {
			return true
		}
		for _, section := range block.Sections {
			if containsAnyToken(section.Label, decisionTokens...) {
				return true
			}
			for _, message := range section.Messages {
				if containsAnyToken(message.Label, decisionTokens...) {
					return true
				}
			}
		}
	}
	return false
}

func containsAnyToken(value string, tokens ...string) bool {
	lower := strings.ToLower(value)
	for _, token := range tokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
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

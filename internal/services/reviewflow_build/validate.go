package reviewflow_build

import (
	"errors"
	"fmt"
	"strings"

	"analysis-module/internal/domain/reviewflow"
)

var (
	ErrBuildEmpty                  = errors.New("reviewflow build result is empty")
	ErrNoSelectedCandidate         = errors.New("reviewflow selected candidate is empty")
	ErrSelectedIDMissingCandidates = errors.New("reviewflow selected id is missing from candidates")
	ErrSelectedSignatureEmpty      = errors.New("reviewflow selected signature is empty")
	ErrCandidateCountOutOfRange    = errors.New("reviewflow candidate count is out of range")
	ErrSelectedFlowEmpty           = errors.New("reviewflow selected candidate has no participants or stages")
	ErrSelectedOutputLeak          = errors.New("reviewflow selected output leaks raw artifacts")
)

// ValidateSelectedBuildResult checks the selected review candidate invariants required by strict mode.
func ValidateSelectedBuildResult(build BuildResult) error {
	var problems []error

	if len(build.Candidates) == 0 {
		problems = append(problems, ErrBuildEmpty)
	}
	if len(build.Candidates) < 1 || len(build.Candidates) > 3 {
		problems = append(problems, ErrCandidateCountOutOfRange)
	}
	if build.SelectedID == "" {
		problems = append(problems, ErrNoSelectedCandidate)
	}
	if build.Signature == "" {
		problems = append(problems, ErrSelectedSignatureEmpty)
	}

	selected, ok := selectedCandidateByID(build.Candidates, build.SelectedID)
	if build.Selected.RootNodeID != "" {
		selected = build.Selected
	}
	if build.SelectedID != "" && !ok {
		problems = append(problems, ErrSelectedIDMissingCandidates)
	}
	if len(selected.Participants) == 0 && len(selected.Stages) == 0 {
		problems = append(problems, ErrSelectedFlowEmpty)
	}
	if selectedFlowHasVisibleLeak(selected) {
		problems = append(problems, ErrSelectedOutputLeak)
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.Join(problems...)
}

func selectedCandidateByID(candidates []reviewflow.Flow, selectedID string) (reviewflow.Flow, bool) {
	if selectedID == "" {
		return reviewflow.Flow{}, false
	}
	for _, candidate := range candidates {
		if candidate.ID == selectedID {
			return candidate, true
		}
	}
	return reviewflow.Flow{}, false
}

func selectedFlowHasVisibleLeak(flow reviewflow.Flow) bool {
	for _, participant := range flow.Participants {
		if visibleReviewArtifactLeak(participant.Label) {
			return true
		}
	}
	for _, stage := range flow.Stages {
		if visibleReviewArtifactLeak(stage.Label) {
			return true
		}
		for _, message := range stage.Messages {
			if visibleReviewArtifactLeak(message.Label) {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		if visibleReviewArtifactLeak(block.Label) {
			return true
		}
		for _, section := range block.Sections {
			if visibleReviewArtifactLeak(section.Label) {
				return true
			}
			for _, message := range section.Messages {
				if visibleReviewArtifactLeak(message.Label) {
					return true
				}
			}
		}
	}
	for _, note := range flow.Notes {
		if visibleReviewArtifactLeak(note.Text) {
			return true
		}
	}
	return false
}

func visibleReviewArtifactLeak(value string) bool {
	lower := strings.ToLower(value)
	if containsReviewTokens(lower, "$closure", "$inline") {
		return true
	}
	return containsRawGoroutineBodyText(lower) || strings.ContainsAny(value, "{}")
}

func containsRawGoroutineBodyText(value string) bool {
	return containsReviewTokens(
		value,
		"go func",
		"func()",
		"func (",
		"wg.",
		"defer ",
		"<-",
		"select {",
		"chan ",
	)
}

func describeValidationError(err error) string {
	if err == nil {
		return ""
	}
	var parts []string
	for _, problem := range []struct {
		err  error
		text string
	}{
		{ErrBuildEmpty, "build empty"},
		{ErrNoSelectedCandidate, "no selected candidate"},
		{ErrSelectedIDMissingCandidates, "selected id missing from candidates"},
		{ErrSelectedSignatureEmpty, "selected signature empty"},
		{ErrCandidateCountOutOfRange, "candidate count out of range"},
		{ErrSelectedFlowEmpty, "selected flow empty"},
		{ErrSelectedOutputLeak, "selected output leaks raw artifacts"},
	} {
		if errors.Is(err, problem.err) {
			parts = append(parts, problem.text)
		}
	}
	if len(parts) == 0 {
		return err.Error()
	}
	return fmt.Sprintf("review validation failed: %s", strings.Join(parts, ", "))
}

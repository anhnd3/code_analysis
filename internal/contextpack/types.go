package contextpack

import (
	"fmt"
	"strings"

	"analysis-module/internal/harness"
)

type ContextPacket struct {
	TaskID       string              `json:"task_id"`
	Role         harness.RoleName    `json:"role"`
	Goal         string              `json:"goal"`
	Inputs       []ContextInput      `json:"inputs,omitempty"`
	SourceSlices []SourceSlice       `json:"source_slices,omitempty"`
	Facts        map[string]any      `json:"facts,omitempty"`
	Constraints  []string            `json:"constraints,omitempty"`
	OutputSchema string              `json:"output_schema"`
	TokenBudget  harness.TokenBudget `json:"token_budget"`
}

type ContextInput struct {
	Type        string              `json:"type"`
	Ref         harness.ArtifactRef `json:"ref"`
	Description string              `json:"description,omitempty"`
}

type SourceSlice struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content,omitempty"`
}

// ValidateContextPacket performs deterministic contract validation on a context packet.
func ValidateContextPacket(packet ContextPacket) harness.ValidationReport {
	issues := make([]harness.ValidationIssue, 0)

	if strings.TrimSpace(packet.TaskID) == "" {
		issues = append(issues, harness.ValidationIssue{
			Code:     "context_task_id_required",
			Message:  "context task_id is required",
			Severity: "error",
			Source:   "context.task_id",
		})
	}

	if strings.TrimSpace(string(packet.Role)) == "" {
		issues = append(issues, harness.ValidationIssue{
			Code:     "context_role_required",
			Message:  "context role is required",
			Severity: "error",
			Source:   "context.role",
		})
	}

	if strings.TrimSpace(packet.Goal) == "" {
		issues = append(issues, harness.ValidationIssue{
			Code:     "context_goal_required",
			Message:  "context goal is required",
			Severity: "error",
			Source:   "context.goal",
		})
	}

	if strings.TrimSpace(packet.OutputSchema) == "" {
		issues = append(issues, harness.ValidationIssue{
			Code:     "context_output_schema_required",
			Message:  "context output_schema is required",
			Severity: "error",
			Source:   "context.output_schema",
		})
	}

	for i, input := range packet.Inputs {
		if strings.TrimSpace(input.Type) == "" {
			issues = append(issues, harness.ValidationIssue{
				Code:     "context_input_type_required",
				Message:  "context input type is required",
				Severity: "error",
				Source:   fmt.Sprintf("context.inputs[%d].type", i),
			})
		}

		if strings.TrimSpace(input.Ref.Type) == "" {
			issues = append(issues, harness.ValidationIssue{
				Code:     "context_input_ref_type_required",
				Message:  "context input ref type is required",
				Severity: "error",
				Source:   fmt.Sprintf("context.inputs[%d].ref.type", i),
			})
		}

		if strings.TrimSpace(input.Ref.Path) == "" {
			issues = append(issues, harness.ValidationIssue{
				Code:     "context_input_ref_path_required",
				Message:  "context input ref path is required",
				Severity: "error",
				Source:   fmt.Sprintf("context.inputs[%d].ref.path", i),
			})
		}
	}

	for i, slice := range packet.SourceSlices {
		if strings.TrimSpace(slice.FilePath) == "" {
			issues = append(issues, harness.ValidationIssue{
				Code:     "source_slice_file_path_required",
				Message:  "source slice file_path is required",
				Severity: "error",
				Source:   fmt.Sprintf("context.source_slices[%d].file_path", i),
			})
		}

		if slice.StartLine <= 0 {
			issues = append(issues, harness.ValidationIssue{
				Code:     "source_slice_start_line_invalid",
				Message:  "source slice start_line must be greater than 0",
				Severity: "error",
				Source:   fmt.Sprintf("context.source_slices[%d].start_line", i),
			})
		}

		if slice.EndLine < slice.StartLine {
			issues = append(issues, harness.ValidationIssue{
				Code:     "source_slice_end_line_invalid",
				Message:  "source slice end_line must be greater than or equal to start_line",
				Severity: "error",
				Source:   fmt.Sprintf("context.source_slices[%d].end_line", i),
			})
		}
	}

	return harness.ValidationReport{
		Accepted: len(issues) == 0,
		Issues:   issues,
	}
}

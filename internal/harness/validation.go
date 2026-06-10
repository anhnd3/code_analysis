package harness

import (
	"fmt"
	"regexp"
)

const (
	CODE_TASK_ID_REQUIRED         = "task_id_required"
	CODE_TASK_ID_INVALID          = "task_id_invalid"
	CODE_TASK_ROLE_REQUIRED       = "task_role_required"
	CODE_TASK_OUTPUT_DIR_REQUIRED = "task_output_dir_required"
	CODE_ARTIFACT_TYPE_REQUIRED   = "artifact_type_required"
	CODE_ARTIFACT_PATH_REQUIRED   = "artifact_path_required"
)

const (
	SEVERITY_ERROR = "error"
)

func ValidateTask(task SubAgentTask) ValidationReport {
	report := ValidationReport{}
	issues := make([]ValidationIssue, 0)

	idRegex := regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]*$")

	// 1. Check ID presence
	if task.ID == "" {
		issues = append(issues, ValidationIssue{
			Code:     CODE_TASK_ID_REQUIRED,
			Message:  "task ID is required",
			Severity: SEVERITY_ERROR,
			Source:   "task.id",
		})
	}

	// 2. Check ID format
	if task.ID != "" && !idRegex.MatchString(task.ID) {
		issues = append(issues, ValidationIssue{
			Code:     CODE_TASK_ID_INVALID,
			Message:  "task ID must start with an alphanumeric character",
			Severity: SEVERITY_ERROR,
			Source:   "task.id",
		})
	}

	// 3. Check Role
	if task.Role == "" {
		issues = append(issues, ValidationIssue{
			Code:     CODE_TASK_ROLE_REQUIRED,
			Message:  "task role is required",
			Severity: SEVERITY_ERROR,
			Source:   "task.role",
		})
	}

	// 4. Check OutputDir
	if task.OutputDir == "" {
		issues = append(issues, ValidationIssue{
			Code:     CODE_TASK_OUTPUT_DIR_REQUIRED,
			Message:  "task output directory is required",
			Severity: SEVERITY_ERROR,
			Source:   "task.output_dir",
		})
	}

	// 5. Check InputArtifacts
	for i, artifact := range task.InputArtifacts {
		if artifact.Type == "" {
			issues = append(issues, ValidationIssue{
				Code:     CODE_ARTIFACT_TYPE_REQUIRED,
				Message:  "input artifact type is required",
				Severity: SEVERITY_ERROR,
				Source:   fmt.Sprintf("task.input_artifacts[%d].type", i),
			})
		} else if artifact.Path == "" {
			issues = append(issues, ValidationIssue{
				Code:     CODE_ARTIFACT_PATH_REQUIRED,
				Message:  "input artifact path is required",
				Severity: SEVERITY_ERROR,
				Source:   fmt.Sprintf("task.input_artifacts[%d].path", i),
			})
		}
	}

	report.Accepted = len(issues) == 0
	return ValidationReport{Accepted: report.Accepted, Issues: issues}
}

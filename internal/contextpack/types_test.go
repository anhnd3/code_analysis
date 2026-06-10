package contextpack

import (
	"encoding/json"
	"testing"

	"analysis-module/internal/harness"
)

func TestContextPacketJSONShape(t *testing.T) {
	packet := ContextPacket{
		TaskID: "task-123",
		Role:   harness.RoleName("reviewer"),
		Goal:   "review code",
		Inputs: []ContextInput{
			{
				Type: "artifact",
				Ref: harness.ArtifactRef{
					Type: "spec",
					Path: "/tmp/spec.md",
				},
				Description: "spec file",
			},
		},
		SourceSlices: []SourceSlice{
			{
				FilePath:  "internal/contextpack/types.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "sample",
			},
		},
		Facts: map[string]any{
			"repo": "analysis-module",
		},
		Constraints:  []string{"no-network"},
		OutputSchema: `{"type":"object"}`,
		TokenBudget: harness.TokenBudget{
			MaxInputTokens:  1200,
			MaxOutputTokens: 600,
		},
	}

	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal context packet: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal context packet: %v", err)
	}

	for _, key := range []string{
		"task_id",
		"role",
		"goal",
		"inputs",
		"source_slices",
		"facts",
		"constraints",
		"output_schema",
		"token_budget",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing JSON key %q", key)
		}
	}

	tokenBudget, ok := payload["token_budget"].(map[string]any)
	if !ok {
		t.Fatalf("token_budget has unexpected type %T", payload["token_budget"])
	}

	for _, key := range []string{"max_input_tokens", "max_output_tokens"} {
		if _, ok := tokenBudget[key]; !ok {
			t.Fatalf("missing token_budget JSON key %q", key)
		}
	}

	slices, ok := payload["source_slices"].([]any)
	if !ok || len(slices) != 1 {
		t.Fatalf("source_slices has unexpected shape: %#v", payload["source_slices"])
	}

	firstSlice, ok := slices[0].(map[string]any)
	if !ok {
		t.Fatalf("source_slices[0] has unexpected type %T", slices[0])
	}

	for _, key := range []string{"file_path", "start_line", "end_line"} {
		if _, ok := firstSlice[key]; !ok {
			t.Fatalf("missing source slice JSON key %q", key)
		}
	}
}

func TestValidateContextPacket(t *testing.T) {
	tests := []struct {
		name           string
		packet         ContextPacket
		expectedAccept bool
		expectedIssues []harness.ValidationIssue
	}{
		{
			name: "valid context packet accepted",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				Inputs: []ContextInput{
					{Type: "artifact", Ref: harness.ArtifactRef{Type: "spec", Path: "/tmp/spec.md"}},
				},
				SourceSlices: []SourceSlice{
					{FilePath: "main.go", StartLine: 1, EndLine: 5},
				},
			},
			expectedAccept: true,
		},
		{
			name: "missing task ID rejected",
			packet: ContextPacket{
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_task_id_required",
				Severity: "error",
				Source:   "context.task_id",
			}},
		},
		{
			name: "missing role rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_role_required",
				Severity: "error",
				Source:   "context.role",
			}},
		},
		{
			name: "missing goal rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         " \n\t ",
				OutputSchema: `{"type":"object"}`,
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_goal_required",
				Severity: "error",
				Source:   "context.goal",
			}},
		},
		{
			name: "missing output schema rejected",
			packet: ContextPacket{
				TaskID: "task-123",
				Role:   harness.RoleName("reviewer"),
				Goal:   "review code",
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_output_schema_required",
				Severity: "error",
				Source:   "context.output_schema",
			}},
		},
		{
			name: "input missing type rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				Inputs: []ContextInput{{
					Type: "",
					Ref:  harness.ArtifactRef{Type: "spec", Path: "/tmp/spec.md"},
				}},
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_input_type_required",
				Severity: "error",
				Source:   "context.inputs[0].type",
			}},
		},
		{
			name: "input ref missing type rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				Inputs: []ContextInput{{
					Type: "artifact",
					Ref:  harness.ArtifactRef{Path: "/tmp/spec.md"},
				}},
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_input_ref_type_required",
				Severity: "error",
				Source:   "context.inputs[0].ref.type",
			}},
		},
		{
			name: "input ref missing path rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				Inputs: []ContextInput{{
					Type: "artifact",
					Ref:  harness.ArtifactRef{Type: "spec"},
				}},
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "context_input_ref_path_required",
				Severity: "error",
				Source:   "context.inputs[0].ref.path",
			}},
		},
		{
			name: "source slice missing file path rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				SourceSlices: []SourceSlice{{
					StartLine: 1,
					EndLine:   5,
				}},
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "source_slice_file_path_required",
				Severity: "error",
				Source:   "context.source_slices[0].file_path",
			}},
		},
		{
			name: "source slice start line 0 rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				SourceSlices: []SourceSlice{{
					FilePath:  "main.go",
					StartLine: 0,
					EndLine:   5,
				}},
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "source_slice_start_line_invalid",
				Severity: "error",
				Source:   "context.source_slices[0].start_line",
			}},
		},
		{
			name: "source slice end line before start line rejected",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				SourceSlices: []SourceSlice{{
					FilePath:  "main.go",
					StartLine: 10,
					EndLine:   9,
				}},
			},
			expectedIssues: []harness.ValidationIssue{{
				Code:     "source_slice_end_line_invalid",
				Severity: "error",
				Source:   "context.source_slices[0].end_line",
			}},
		},
		{
			name: "independent input checks can report multiple issues",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				Inputs:       []ContextInput{{}},
			},
			expectedIssues: []harness.ValidationIssue{
				{Code: "context_input_type_required", Severity: "error", Source: "context.inputs[0].type"},
				{Code: "context_input_ref_type_required", Severity: "error", Source: "context.inputs[0].ref.type"},
				{Code: "context_input_ref_path_required", Severity: "error", Source: "context.inputs[0].ref.path"},
			},
		},
		{
			name: "independent source slice checks can report multiple issues",
			packet: ContextPacket{
				TaskID:       "task-123",
				Role:         harness.RoleName("reviewer"),
				Goal:         "review code",
				OutputSchema: `{"type":"object"}`,
				SourceSlices: []SourceSlice{{
					StartLine: 0,
					EndLine:   -1,
				}},
			},
			expectedIssues: []harness.ValidationIssue{
				{Code: "source_slice_file_path_required", Severity: "error", Source: "context.source_slices[0].file_path"},
				{Code: "source_slice_start_line_invalid", Severity: "error", Source: "context.source_slices[0].start_line"},
				{Code: "source_slice_end_line_invalid", Severity: "error", Source: "context.source_slices[0].end_line"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ValidateContextPacket(tt.packet)
			if report.Accepted != tt.expectedAccept {
				t.Fatalf("Accepted = %v, want %v", report.Accepted, tt.expectedAccept)
			}

			if len(report.Warnings) != 0 {
				t.Fatalf("expected no warnings, got %#v", report.Warnings)
			}

			if len(report.Issues) != len(tt.expectedIssues) {
				t.Fatalf("issue count = %d, want %d; issues=%#v", len(report.Issues), len(tt.expectedIssues), report.Issues)
			}

			for i, expected := range tt.expectedIssues {
				actual := report.Issues[i]
				if actual.Code != expected.Code {
					t.Fatalf("issue[%d].Code = %q, want %q", i, actual.Code, expected.Code)
				}
				if actual.Source != expected.Source {
					t.Fatalf("issue[%d].Source = %q, want %q", i, actual.Source, expected.Source)
				}
				if actual.Severity != expected.Severity {
					t.Fatalf("issue[%d].Severity = %q, want %q", i, actual.Severity, expected.Severity)
				}
				if actual.Message == "" {
					t.Fatalf("issue[%d].Message must be non-empty", i)
				}
			}
		})
	}
}

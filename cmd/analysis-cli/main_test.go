package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"analysis-module/internal/export"
	"analysis-module/internal/facts"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- splitCSV tests (keep existing coverage style) ----------

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"", nil},
		{",,,", []string{}},
		{"a,,b", []string{"a", "b"}},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := splitCSV(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// ---------- readReviewFlow behavior tests ----------

func TestReadReviewFlowValidJSON(t *testing.T) {
	flow := facts.ReviewFlow{
		ID:                "test-flow-001",
		WorkspaceID:       "ws-abc",
		SnapshotID:        "snap-123",
		RootSymbolID:      "sym-root",
		RootCanonicalName: "service.Handle",
		CreatedAt:         time.Now().UTC(),
		Steps: []facts.ReviewStep{
			{
				ID:                "step-1",
				FromCanonicalName: "service.Handle",
				ToCanonicalName:   "repo.Fetch",
				Status:            facts.StepAccepted,
			},
		},
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "flow.json")
	data, err := json.Marshal(flow)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	got, err := readReviewFlow(path)
	require.NoError(t, err)
	assert.Equal(t, flow.ID, got.ID)
	assert.Equal(t, flow.RootCanonicalName, got.RootCanonicalName)
	assert.Len(t, got.Steps, len(flow.Steps))
}

func TestReadReviewFlowInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{ not json"), 0o644))

	_, err := readReviewFlow(path)
	assert.Error(t, err)
}

func TestReadReviewFlowMissingFile(t *testing.T) {
	path := filepath.Join(os.TempDir(), "does-not-exist-12345-flow.json")
	_, err := readReviewFlow(path)
	assert.Error(t, err)
}

// ---------- export helper behavior tests ----------

// Helper: create a valid ReviewFlow file in tmp and return path.
func writeTestFlowFile(t *testing.T, dir string, id string) string {
	t.Helper()
	flow := facts.ReviewFlow{
		ID:                id,
		WorkspaceID:       "ws-test",
		SnapshotID:        "snap-1",
		RootSymbolID:      "root-sym",
		RootCanonicalName: "svc.StartHandler",
		CreatedAt:         time.Now().UTC(),
		Steps: []facts.ReviewStep{
			{
				ID:                "s1",
				FromCanonicalName: "svc.StartHandler",
				ToCanonicalName:   "repo.LoadConfig",
				Status:            facts.StepAccepted,
				Rationale:         "direct call",
			},
		},
		Accepted: []facts.ReviewStep{
			{
				ID:                "s1",
				FromCanonicalName: "svc.StartHandler",
				ToCanonicalName:   "repo.LoadConfig",
				Status:            facts.StepAccepted,
			},
		},
		UncertaintyNotes: []string{"minor ambiguity in routing"},
	}

	path := filepath.Join(dir, "flow.json")
	data, err := json.Marshal(flow)
	require.NoError(t, err, "marshal test flow")
	require.NoError(t, os.WriteFile(path, data, 0o644), "write test flow file")
	return path
}

// Test export-md behavior via real helper used by CLI.
func TestExportMarkdownBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	// Use the same Read+Render pipeline as runExportMarkdown.
	service := export.NewMarkdownService()

	path := writeTestFlowFile(t, tmpDir, "md-test-flow")

	flow, err := readReviewFlow(path)
	require.NoError(t, err)

	body := service.Render(flow)

	outPath := filepath.Join(tmpDir, "flow.md")
	err = os.WriteFile(outPath, []byte(body), 0o644)
	require.NoError(t, err)

	// Assertions: file exists and non-empty; contains expected markers.
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(data)))
	assert.Contains(t, string(data), "# Flow Review")
	assert.Contains(t, string(data), "svc.StartHandler")
}

// Test export-mermaid behavior via real helper.
func TestExportMermaidBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	service := export.NewMermaidService()

	path := writeTestFlowFile(t, tmpDir, "mmd-test-flow")

	flow, err := readReviewFlow(path)
	require.NoError(t, err)

	diagram := service.Render(flow)

	outPath := filepath.Join(tmpDir, "flow.mmd")
	err = os.WriteFile(outPath, []byte(diagram), 0o644)
	require.NoError(t, err)

	// Assertions: file exists and non-empty; looks like mermaid.
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(data)))
	assert.Contains(t, string(data), "sequenceDiagram")
}

// Test export-graphjson behavior via real helper.
func TestExportGraphJSONBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	service := export.NewGraphJSONService()

	path := writeTestFlowFile(t, tmpDir, "gjson-test-flow")

	flow, err := readReviewFlow(path)
	require.NoError(t, err)

	gjData, err := service.Render(flow)
	require.NoError(t, err)

	outPath := filepath.Join(tmpDir, "graph.json")
	err = os.WriteFile(outPath, gjData, 0o644)
	require.NoError(t, err)

	// Read back and validate JSON structure.
	back, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(back)))

	var result map[string]interface{}
	err = json.Unmarshal(back, &result)
	require.NoError(t, err)
	assert.NotNil(t, result["nodes"])
	assert.NotNil(t, result["edges"])
}

// ---------- printUsage safety test ----------

func TestPrintUsageDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		printUsage()
	})
}

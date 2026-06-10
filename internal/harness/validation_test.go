package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateTask(t *testing.T) {
	tests := []struct {
		task           SubAgentTask
		expectAccepted bool
	}{
		{task: SubAgentTask{ID: "t1-test", Role: "tester-role", OutputDir: "/tmp/out/t1"}, expectAccepted: true},
		{task: SubAgentTask{Role: "r1", OutputDir: "/tmp/d1"}, expectAccepted: false},
		{task: SubAgentTask{ID: "a/b/c", Role: "r", OutputDir: "/tmp/d"}, expectAccepted: false},
		{task: SubAgentTask{ID: "a b c", Role: "r", OutputDir: "/tmp/d"}, expectAccepted: false},
		{task: SubAgentTask{ID: "id1"}, expectAccepted: false},
		{task: SubAgentTask{ID: "id2", Role: "r"}, expectAccepted: false},
		{task: SubAgentTask{
			ID:             "t3",
			Role:           "role3",
			OutputDir:      "/tmp/d3",
			InputArtifacts: []ArtifactRef{{Path: "p1"}},
		}, expectAccepted: false},
		{task: SubAgentTask{
			ID:             "t4",
			Role:           "role4",
			OutputDir:      "/tmp/d4",
			InputArtifacts: []ArtifactRef{{Type: "a"}},
		}, expectAccepted: false},
	}

	for i, tt := range tests {
		t.Run(t.Name(), func(t *testing.T) {
			report := ValidateTask(tt.task)

			if report.Accepted != tt.expectAccepted || (report.Accepted && len(report.Issues) > 0) {
				t.Errorf("test case %d: validation failed", i+1)
				return
			}
		})
	}
}

func TestArtifactRefJSONShape(t *testing.T) {
	a := ArtifactRef{Type: "input", Path: "/path/to/file"}

	out, err := json.MarshalIndent(a, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	for _, k := range [2]string{"type", "path"} {
		if !strings.Contains(s, k) {
			t.Fatalf("missing key %q in ArtifactRef JSON", k)
		}
	}
}

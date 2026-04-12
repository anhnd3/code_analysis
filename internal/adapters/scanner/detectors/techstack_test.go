package detectors

import (
	"testing"

	"analysis-module/internal/tests/fixtures"
)

func TestTechStackDetectorDetectsGoAndTests(t *testing.T) {
	detector := NewTechStackDetector()
	profile, err := detector.Detect(fixtures.WorkspacePath(t, "single_go_service"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(profile.Languages) == 0 {
		t.Fatal("expected at least one language")
	}
	if len(profile.TestFrameworks) == 0 {
		t.Fatal("expected test framework hint")
	}
}

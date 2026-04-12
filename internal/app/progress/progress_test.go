package progress

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlainReporterThrottlesNonInteractiveSpam(t *testing.T) {
	var buffer bytes.Buffer
	reporter := &StderrReporter{
		writer: &buffer,
		mode:   ModePlain,
	}

	reporter.StartStage("extract", 1000)
	for i := 0; i < 10; i++ {
		reporter.Status("file=example.ts")
		reporter.Advance(1)
	}
	reporter.FinishStage("done")

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) > 3 {
		t.Fatalf("expected plain reporter to stay throttled, got %d lines: %q", len(lines), buffer.String())
	}
	if !strings.Contains(buffer.String(), "stage=extract") {
		t.Fatalf("expected stage output, got %q", buffer.String())
	}
}

package indexer

import (
	scannerport "analysis-module/internal/ports/scanner"
)

// WorkspaceScanner wraps the port interface for workspace scanning.
type WorkspaceScanner interface {
	Scan(req scannerport.ScanWorkspaceRequest) (scannerport.ScanWorkspaceResult, error)
}

// Reporter defines the interface for reporting progress during long-running stages.
type Reporter interface {
	StartStage(name string, total int)
	Advance(delta int)
	Status(message string)
	FinishStage(message string)
}

// noopReporter discards all output.
type noopReporter struct{}

func (noopReporter) StartStage(string, int) {}
func (noopReporter) Advance(int)            {}
func (noopReporter) Status(string)          {}
func (noopReporter) FinishStage(string)     {}

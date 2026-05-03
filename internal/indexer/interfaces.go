package indexer

// WorkspaceScanner is used to scan a workspace for repositories.
type WorkspaceScanner interface {
	Scan(req ScanWorkspaceRequest) (ScanWorkspaceResult, error)
}

// Reporter defines the interface for reporting progress during long-running stages.
type Reporter interface {
	StartStage(name string, total int)
	Advance(delta int)
	Status(message string)
	FinishStage(message string)
}

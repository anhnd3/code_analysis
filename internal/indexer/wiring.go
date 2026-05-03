package indexer

import ()

// WorkflowOptions holds configuration for building default scan and index workflows.
type WorkflowOptions struct {
	ArtifactRoot   string
	ArtifactStore  ArtifactStore
	SnapshotManage SnapshotService
	Reporter       Reporter
}

// DefaultWorkflows pairs a ScanWorkflow with an IndexWorkflow.
type DefaultWorkflows struct {
	Scan  ScanWorkflow
	Index IndexWorkflow
}

// NewDefaultWorkflows wires together the default scan and index workflows using
// detector, scanner, inventory builder, boundary registry, framework detectors, and symbol extractors.
func NewDefaultWorkflows(opts WorkflowOptions) (DefaultWorkflows, error) {
	if opts.Reporter == nil {
		opts.Reporter = noopReporter{}
	}

	repoRootDetector := NewRepoRootDetector(opts.Reporter)
	techStackDetector := NewTechStackDetector()
	serviceDetector := NewServiceDetector()

	scanner := NewWorkspaceScannerService(repoRootDetector, techStackDetector, serviceDetector, opts.Reporter)

	inventoryBuilder := NewInventoryBuilder()

	scanWorkflow := NewScanWorkflow(scanner, inventoryBuilder, opts.ArtifactStore, opts.SnapshotManage)

	boundaryRegistry := NewRegistry()
	boundaryRegistry.Register(NewGinDetector())
	boundaryRegistry.Register(NewNetHTTPDetector())
	boundaryRegistry.Register(NewGRPCGatewayDetector())

	boundaryDetectSvc := NewBoundaryDetectorService(boundaryRegistry)

	symbolExtract := NewSymbolExtractorService(
		opts.Reporter,
		NewGoExtractor(),
		NewPythonExtractor(),
		NewJavaScriptExtractor(),
	)

	indexWorkflow := NewIndexWorkflow(scanWorkflow, symbolExtract, boundaryDetectSvc, opts.SnapshotManage, opts.ArtifactStore, opts.ArtifactRoot)

	return DefaultWorkflows{Scan: scanWorkflow, Index: indexWorkflow}, nil
}

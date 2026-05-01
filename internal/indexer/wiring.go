package indexer

import (
	artifactstoreport "analysis-module/internal/ports/artifactstore"

	boundaryframeworks "analysis-module/internal/indexer/boundary/frameworks"
	boundaryreg "analysis-module/internal/indexer/boundary/go"
	"analysis-module/internal/indexer/detector"
	goextractor "analysis-module/internal/indexer/extractor/go"
	jsextractor "analysis-module/internal/indexer/extractor/javascript"
	pythonextractor "analysis-module/internal/indexer/extractor/python"
	"analysis-module/internal/services/snapshot_manage"
)

// WorkflowOptions holds configuration for building default scan and index workflows.
type WorkflowOptions struct {
	ArtifactRoot   string
	ArtifactStore  artifactstoreport.Store
	SnapshotManage snapshot_manage.Service
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

	repoRootDetector := detector.NewRepoRootDetector(opts.Reporter)
	techStackDetector := detector.NewTechStackDetector()
	serviceDetector := detector.NewServiceDetector()

	scanner := NewWorkspaceScannerService(repoRootDetector, techStackDetector, serviceDetector, opts.Reporter)

	inventoryBuilder := NewInventoryBuilder()

	scanWorkflow := NewScanWorkflow(scanner, inventoryBuilder, opts.ArtifactStore, opts.SnapshotManage)

	boundaryRegistry := boundaryreg.NewRegistry()
	boundaryRegistry.Register(boundaryframeworks.NewGinDetector())
	boundaryRegistry.Register(boundaryframeworks.NewNetHTTPDetector())
	boundaryRegistry.Register(boundaryframeworks.NewGRPCGatewayDetector())

	boundaryDetectSvc := NewBoundaryDetectorService(boundaryRegistry)

	symbolExtract := NewSymbolExtractorService(
		opts.Reporter,
		goextractor.New(),
		pythonextractor.New(),
		jsextractor.New(),
	)

	indexWorkflow := NewIndexWorkflow(scanWorkflow, symbolExtract, boundaryDetectSvc, opts.SnapshotManage, opts.ArtifactStore, opts.ArtifactRoot)

	return DefaultWorkflows{Scan: scanWorkflow, Index: indexWorkflow}, nil
}

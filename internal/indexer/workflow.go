package indexer

import (
	"analysis-module/internal/facts"
)

// IndexRequest describes inputs for a full index operation.
type IndexRequest struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

// IndexResult holds the complete output of an indexing run including facts and artifacts.
type IndexResult struct {
	WorkspaceID        string            `json:"workspace_id"`
	SnapshotID         string            `json:"snapshot_id"`
	SQLitePath         string            `json:"sqlite_path"`
	JSONL              facts.JSONLResult `json:"jsonl"`
	RepositoryCount    int               `json:"repository_count"`
	FileCount          int               `json:"file_count"`
	SymbolCount        int               `json:"symbol_count"`
	CallCandidateCount int               `json:"call_candidate_count"`
	ArtifactRefs       []ArtifactRef     `json:"artifact_refs"`
}

// IndexWorkflow orchestrates scanning, symbol extraction, and boundary detection.
type IndexWorkflow struct {
	scanWorkflow   ScanWorkflow
	symbolExtract  SymbolExtractorService
	boundaryDetect BoundaryDetectorService
	snapshotManage SnapshotNamer
	artifactStore  ArtifactStore
	artifactRoot   string
}

func NewIndexWorkflow(
	scanWorkflow ScanWorkflow,
	symbolExtract SymbolExtractorService,
	boundaryDetect BoundaryDetectorService,
	snapshotManage SnapshotNamer,
	artifactStore ArtifactStore,
	artifactRoot string,
) IndexWorkflow {
	return IndexWorkflow{
		scanWorkflow:   scanWorkflow,
		symbolExtract:  symbolExtract,
		boundaryDetect: boundaryDetect,
		snapshotManage: snapshotManage,
		artifactStore:  artifactStore,
		artifactRoot:   artifactRoot,
	}
}

// Run executes the full index workflow.
func (w IndexWorkflow) Run(req IndexRequest) (IndexResult, error) {
	analyzeResult, err := w.scanWorkflow.Run(ScanRequest{
		WorkspacePath:  req.WorkspacePath,
		IgnorePatterns: req.IgnorePatterns,
		TargetHints:    req.TargetHints,
	})
	if err != nil {
		return IndexResult{}, err
	}

	extraction, err := w.symbolExtract.Build(analyzeResult.Inventory)
	if err != nil {
		return IndexResult{}, err
	}

	var allSymbols []Symbol
	for _, repoExt := range extraction.Repositories {
		for _, fileExt := range repoExt.Files {
			allSymbols = append(allSymbols, fileExt.Symbols...)
		}
	}

	boundaryResult, err := w.boundaryDetect.DetectAllDetailed(analyzeResult.Inventory, allSymbols)
	if err != nil {
		boundaryResult.Diagnostics = append(boundaryResult.Diagnostics, Diagnostic{
			Category: "boundary_detection_failed",
			Message:  err.Error(),
		})
	}

	workspaceID := string(analyzeResult.WorkspaceManifest.ID)
	snapshotID := w.snapshotManage.NewSnapshotID(workspaceID)

	index := BuildIndexFromExtraction(
		analyzeResult.Inventory,
		extraction,
		boundaryResult,
		workspaceID,
		snapshotID,
	)

	sqlitePath := facts.SQLitePathFor(w.artifactRoot, workspaceID, snapshotID)
	sqliteStore, err := facts.NewSQLiteStore(sqlitePath)
	if err != nil {
		return IndexResult{}, err
	}
	if err := sqliteStore.SaveIndex(index); err != nil {
		_ = sqliteStore.Close()
		return IndexResult{}, err
	}
	_ = sqliteStore.Close()

	jsonlStore := facts.NewJSONLStore(w.artifactRoot)
	jsonlResult, err := jsonlStore.SaveIndex(index)
	if err != nil {
		return IndexResult{}, err
	}

	refs := append([]ArtifactRef{}, analyzeResult.ArtifactRefs...)
	if ref, saveErr := w.artifactStore.SaveJSON(workspaceID, snapshotID, "facts_index.json", TypeFactsIndex, index); saveErr == nil {
		refs = append(refs, ref)
	}

	return IndexResult{
		WorkspaceID:        workspaceID,
		SnapshotID:         snapshotID,
		SQLitePath:         sqlitePath,
		JSONL:              jsonlResult,
		RepositoryCount:    index.RepositoryCount,
		FileCount:          index.FileCount,
		SymbolCount:        index.SymbolCount,
		CallCandidateCount: index.CallCandidateCount,
		ArtifactRefs:       refs,
	}, nil
}

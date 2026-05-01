package indexer

import (
	"time"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/facts"
	factsqlite "analysis-module/internal/facts/sqlite"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	"analysis-module/internal/services/snapshot_manage"
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
	ArtifactRefs       []artifact.Ref    `json:"artifact_refs"`
}

// IndexWorkflow orchestrates scanning, symbol extraction, and boundary detection.
type IndexWorkflow struct {
	scanWorkflow   ScanWorkflow
	symbolExtract  SymbolExtractorService
	boundaryDetect BoundaryDetectorService
	snapshotManage snapshot_manage.Service
	artifactStore  artifactstoreport.Store
	artifactRoot   string
}

func NewIndexWorkflow(
	scanWorkflow ScanWorkflow,
	symbolExtract SymbolExtractorService,
	boundaryDetect BoundaryDetectorService,
	snapshotManage snapshot_manage.Service,
	artifactStore artifactstoreport.Store,
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

	var allSymbols []symbol.Symbol
	for _, repoExt := range extraction.Repositories {
		for _, fileExt := range repoExt.Files {
			allSymbols = append(allSymbols, fileExt.Symbols...)
		}
	}

	detectedRoots, err := w.boundaryDetect.DetectAll(analyzeResult.Inventory, allSymbols)
	if err != nil {
		detectedRoots = nil
	}

	workspaceID := string(analyzeResult.WorkspaceManifest.ID)
	snapshotID := w.snapshotManage.NewSnapshotID(workspaceID)

	index := facts.BuildIndex(facts.BuildInput{
		WorkspaceID: workspaceID,
		SnapshotID:  snapshotID,
		Inventory:   analyzeResult.Inventory,
		Extraction:  extraction,
		Boundaries:  detectedRoots,
		GeneratedAt: time.Now().UTC(),
	})

	sqlitePath := factsqlite.PathFor(w.artifactRoot, workspaceID, snapshotID)
	sqliteStore, err := factsqlite.New(sqlitePath)
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

	refs := append([]artifact.Ref{}, analyzeResult.ArtifactRefs...)
	if ref, saveErr := w.artifactStore.SaveJSON(workspaceID, snapshotID, "facts_index.json", artifact.TypeFactsIndex, index); saveErr == nil {
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

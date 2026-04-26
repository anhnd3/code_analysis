package facts_index

import (
	"time"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/facts"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	"analysis-module/internal/services/boundary_detect"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/services/symbol_index"
	factjsonl "analysis-module/internal/store/jsonl"
	factsqlite "analysis-module/internal/store/sqlite"
	"analysis-module/internal/workflows/analyze_workspace"
)

type Request struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

type Result struct {
	WorkspaceID        string           `json:"workspace_id"`
	SnapshotID         string           `json:"snapshot_id"`
	SQLitePath         string           `json:"sqlite_path"`
	JSONL              factjsonl.Result `json:"jsonl"`
	RepositoryCount    int              `json:"repository_count"`
	FileCount          int              `json:"file_count"`
	SymbolCount        int              `json:"symbol_count"`
	CallCandidateCount int              `json:"call_candidate_count"`
	ArtifactRefs       []artifact.Ref   `json:"artifact_refs"`
}

type Workflow struct {
	analyze        analyze_workspace.Workflow
	symbolIndex    symbol_index.Service
	boundaryDetect boundary_detect.Service
	snapshotManage snapshot_manage.Service
	artifactStore  artifactstoreport.Store
	artifactRoot   string
}

func New(
	analyze analyze_workspace.Workflow,
	symbolIndex symbol_index.Service,
	boundaryDetect boundary_detect.Service,
	snapshotManage snapshot_manage.Service,
	artifactStore artifactstoreport.Store,
	artifactRoot string,
) Workflow {
	return Workflow{
		analyze:        analyze,
		symbolIndex:    symbolIndex,
		boundaryDetect: boundaryDetect,
		snapshotManage: snapshotManage,
		artifactStore:  artifactStore,
		artifactRoot:   artifactRoot,
	}
}

func (w Workflow) Run(req Request) (Result, error) {
	analyzeResult, err := w.analyze.Run(analyze_workspace.Request{
		WorkspacePath:  req.WorkspacePath,
		IgnorePatterns: req.IgnorePatterns,
		TargetHints:    req.TargetHints,
	})
	if err != nil {
		return Result{}, err
	}
	extraction, err := w.symbolIndex.Build(analyzeResult.Inventory)
	if err != nil {
		return Result{}, err
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
		return Result{}, err
	}
	if err := sqliteStore.SaveIndex(index); err != nil {
		_ = sqliteStore.Close()
		return Result{}, err
	}
	_ = sqliteStore.Close()

	jsonlStore := factjsonl.New(w.artifactRoot)
	jsonlResult, err := jsonlStore.SaveIndex(index)
	if err != nil {
		return Result{}, err
	}

	refs := append([]artifact.Ref{}, analyzeResult.ArtifactRefs...)
	if ref, saveErr := w.artifactStore.SaveJSON(workspaceID, snapshotID, "facts_index.json", artifact.TypeFactsIndex, index); saveErr == nil {
		refs = append(refs, ref)
	}

	return Result{
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

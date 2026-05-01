package facts

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// JSONLStore persists facts to JSON/JSONL files under the artifact root.
type JSONLStore struct {
	artifactRoot string
}

// JSONLResult holds output file paths for a saved index snapshot.
type JSONLResult struct {
	IndexPath          string `json:"index_path"`
	RepositoriesPath   string `json:"repositories_path"`
	ServicesPath       string `json:"services_path"`
	FilesPath          string `json:"files_path"`
	SymbolsPath        string `json:"symbols_path"`
	ImportsPath        string `json:"imports_path"`
	ExportsPath        string `json:"exports_path"`
	CallCandidatesPath string `json:"call_candidates_path"`
	ExecutionHintsPath string `json:"execution_hints_path"`
	DiagnosticsPath    string `json:"diagnostics_path"`
	TestsPath          string `json:"tests_path"`
	BoundariesPath     string `json:"boundaries_path"`
}

// NewJSONLStore creates a JSONL store for the given artifact root.
func NewJSONLStore(artifactRoot string) JSONLStore {
	return JSONLStore{artifactRoot: filepath.Clean(artifactRoot)}
}

// SaveIndex writes the index to JSON/JSONL files and returns output paths.
func (s JSONLStore) SaveIndex(index Index) (JSONLResult, error) {
	baseDir := filepath.Join(s.artifactRoot, "workspaces", index.WorkspaceID, "snapshots", index.SnapshotID, "facts")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return JSONLResult{}, err
	}

	result := JSONLResult{
		IndexPath:          filepath.Join(baseDir, "facts_index.json"),
		RepositoriesPath:   filepath.Join(baseDir, "facts_repositories.jsonl"),
		ServicesPath:       filepath.Join(baseDir, "facts_services.jsonl"),
		FilesPath:          filepath.Join(baseDir, "facts_files.jsonl"),
		SymbolsPath:        filepath.Join(baseDir, "facts_symbols.jsonl"),
		ImportsPath:        filepath.Join(baseDir, "facts_imports.jsonl"),
		ExportsPath:        filepath.Join(baseDir, "facts_exports.jsonl"),
		CallCandidatesPath: filepath.Join(baseDir, "facts_call_candidates.jsonl"),
		ExecutionHintsPath: filepath.Join(baseDir, "facts_execution_hints.jsonl"),
		DiagnosticsPath:    filepath.Join(baseDir, "facts_diagnostics.jsonl"),
		TestsPath:          filepath.Join(baseDir, "facts_tests.jsonl"),
		BoundariesPath:     filepath.Join(baseDir, "facts_boundaries.jsonl"),
	}

	if err := writeJSON(result.IndexPath, index); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.RepositoriesPath, index.Repositories); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.ServicesPath, index.Services); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.FilesPath, index.Files); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.SymbolsPath, index.Symbols); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.ImportsPath, index.Imports); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.ExportsPath, index.Exports); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.CallCandidatesPath, index.CallCandidates); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.ExecutionHintsPath, index.ExecutionHints); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.DiagnosticsPath, index.Diagnostics); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.TestsPath, index.Tests); err != nil {
		return JSONLResult{}, err
	}
	if err := writeJSONL(result.BoundariesPath, index.Boundaries); err != nil {
		return JSONLResult{}, err
	}
	return result, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeJSONL[T any](path string, values []T) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	encoder := json.NewEncoder(writer)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return err
		}
	}
	return nil
}

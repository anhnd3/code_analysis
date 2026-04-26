package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"analysis-module/internal/facts"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SaveIndex(index facts.Index) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := s.deleteSnapshot(tx, index.WorkspaceID, index.SnapshotID); err != nil {
		return err
	}

	if _, err := tx.Exec(`
insert into index_runs(workspace_id, snapshot_id, generated_at, repository_count, file_count, symbol_count, call_candidate_count, issue_counts_json)
values(?, ?, ?, ?, ?, ?, ?, ?)
`,
		index.WorkspaceID,
		index.SnapshotID,
		index.GeneratedAt.UTC().Format(time.RFC3339Nano),
		index.RepositoryCount,
		index.FileCount,
		index.SymbolCount,
		index.CallCandidateCount,
		mustJSON(index.IssueCounts),
	); err != nil {
		return err
	}

	repoStmt, err := tx.Prepare(`
insert into repositories(workspace_id, snapshot_id, id, name, root_path, role, languages_json, build_files_json, frameworks_json)
values(?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer repoStmt.Close()
	for _, repo := range index.Repositories {
		if _, err := repoStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			repo.ID,
			repo.Name,
			repo.RootPath,
			repo.Role,
			mustJSON(repo.Languages),
			mustJSON(repo.BuildFiles),
			mustJSON(repo.Frameworks),
		); err != nil {
			return err
		}
	}

	svcStmt, err := tx.Prepare(`
insert into services(workspace_id, snapshot_id, id, repository_id, name, root_path, entrypoints_json, boundary_hints_json)
values(?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer svcStmt.Close()
	for _, svc := range index.Services {
		if _, err := svcStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			svc.ID,
			svc.RepositoryID,
			svc.Name,
			svc.RootPath,
			mustJSON(svc.Entrypoints),
			mustJSON(svc.BoundaryHints),
		); err != nil {
			return err
		}
	}

	fileStmt, err := tx.Prepare(`
insert into files(workspace_id, snapshot_id, id, repository_id, repository_root, relative_path, absolute_path, language, package_name)
values(?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer fileStmt.Close()
	for _, file := range index.Files {
		if _, err := fileStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			file.ID,
			file.RepositoryID,
			file.RepositoryRoot,
			file.RelativePath,
			file.AbsolutePath,
			file.Language,
			file.PackageName,
		); err != nil {
			return err
		}
	}

	symbolStmt, err := tx.Prepare(`
insert into symbols(workspace_id, snapshot_id, id, repository_id, file_id, file_path, canonical_name, name, receiver, kind, signature, start_line, start_col, end_line, end_col, start_byte, end_byte)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer symbolStmt.Close()
	for _, sym := range index.Symbols {
		if _, err := symbolStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			sym.ID,
			sym.RepositoryID,
			sym.FileID,
			sym.FilePath,
			sym.CanonicalName,
			sym.Name,
			sym.Receiver,
			sym.Kind,
			sym.Signature,
			sym.StartLine,
			sym.StartCol,
			sym.EndLine,
			sym.EndCol,
			sym.StartByte,
			sym.EndByte,
		); err != nil {
			return err
		}
	}

	importStmt, err := tx.Prepare(`
insert into imports(workspace_id, snapshot_id, id, file_id, file_path, source, alias, imported_name, export_name, resolved_path, is_default, is_namespace, is_local)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer importStmt.Close()
	for _, imp := range index.Imports {
		if _, err := importStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			imp.ID,
			imp.FileID,
			imp.FilePath,
			imp.Source,
			imp.Alias,
			imp.ImportedName,
			imp.ExportName,
			imp.ResolvedPath,
			boolToInt(imp.IsDefault),
			boolToInt(imp.IsNamespace),
			boolToInt(imp.IsLocal),
		); err != nil {
			return err
		}
	}

	exportStmt, err := tx.Prepare(`
insert into exports(workspace_id, snapshot_id, id, file_id, file_path, name, canonical_name, is_default)
values(?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer exportStmt.Close()
	for _, exp := range index.Exports {
		if _, err := exportStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			exp.ID,
			exp.FileID,
			exp.FilePath,
			exp.Name,
			exp.CanonicalName,
			boolToInt(exp.IsDefault),
		); err != nil {
			return err
		}
	}

	callStmt, err := tx.Prepare(`
insert into call_candidates(workspace_id, snapshot_id, id, source_symbol_id, target_symbol_id, target_canonical_name, target_file_path, target_export_name, relationship, evidence_type, evidence_source, extraction_method, confidence_score, order_index)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer callStmt.Close()
	for _, candidate := range index.CallCandidates {
		if _, err := callStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			candidate.ID,
			candidate.SourceSymbolID,
			candidate.TargetSymbolID,
			candidate.TargetCanonicalName,
			candidate.TargetFilePath,
			candidate.TargetExportName,
			candidate.Relationship,
			candidate.EvidenceType,
			candidate.EvidenceSource,
			candidate.ExtractionMethod,
			candidate.ConfidenceScore,
			candidate.OrderIndex,
		); err != nil {
			return err
		}
	}

	hintStmt, err := tx.Prepare(`
insert into execution_hints(workspace_id, snapshot_id, id, file_path, source_symbol_id, target_symbol_id, target_symbol, kind, evidence, order_index)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer hintStmt.Close()
	for _, hint := range index.ExecutionHints {
		if _, err := hintStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			hint.ID,
			hint.FilePath,
			hint.SourceSymbolID,
			hint.TargetSymbolID,
			hint.TargetSymbol,
			hint.Kind,
			hint.Evidence,
			hint.OrderIndex,
		); err != nil {
			return err
		}
	}

	diagStmt, err := tx.Prepare(`
insert into diagnostics(workspace_id, snapshot_id, id, file_path, symbol_id, category, message, evidence)
values(?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer diagStmt.Close()
	for _, diag := range index.Diagnostics {
		if _, err := diagStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			diag.ID,
			diag.FilePath,
			diag.SymbolID,
			diag.Category,
			diag.Message,
			diag.Evidence,
		); err != nil {
			return err
		}
	}

	testStmt, err := tx.Prepare(`
insert into tests(workspace_id, snapshot_id, id, symbol_id, file_id, file_path, canonical_name, name, start_line, end_line)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer testStmt.Close()
	for _, testFact := range index.Tests {
		if _, err := testStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			testFact.ID,
			testFact.SymbolID,
			testFact.FileID,
			testFact.FilePath,
			testFact.CanonicalName,
			testFact.Name,
			testFact.StartLine,
			testFact.EndLine,
		); err != nil {
			return err
		}
	}

	boundaryStmt, err := tx.Prepare(`
insert into boundaries(workspace_id, snapshot_id, id, repository_id, kind, framework, method, path, canonical_name, source_file, source_expr, handler_target, source_start_byte, source_end_byte)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer boundaryStmt.Close()
	for _, boundary := range index.Boundaries {
		if _, err := boundaryStmt.Exec(
			index.WorkspaceID,
			index.SnapshotID,
			boundary.ID,
			boundary.RepositoryID,
			boundary.Kind,
			boundary.Framework,
			boundary.Method,
			boundary.Path,
			boundary.CanonicalName,
			boundary.SourceFile,
			boundary.SourceExpr,
			boundary.HandlerTarget,
			boundary.SourceStartByte,
			boundary.SourceEndByte,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ResolveSymbol(workspaceID, snapshotID, selector string) (facts.SymbolFact, error) {
	row := s.db.QueryRow(`
select id, repository_id, file_id, file_path, canonical_name, name, receiver, kind, signature, start_line, start_col, end_line, end_col, start_byte, end_byte
from symbols
where workspace_id = ? and snapshot_id = ? and (id = ? or canonical_name = ?)
limit 1
`, workspaceID, snapshotID, selector, selector)
	return scanSymbol(row)
}

func (s *Store) GetSymbolByID(workspaceID, snapshotID, symbolID string) (facts.SymbolFact, error) {
	row := s.db.QueryRow(`
select id, repository_id, file_id, file_path, canonical_name, name, receiver, kind, signature, start_line, start_col, end_line, end_col, start_byte, end_byte
from symbols
where workspace_id = ? and snapshot_id = ? and id = ?
limit 1
`, workspaceID, snapshotID, symbolID)
	return scanSymbol(row)
}

func (s *Store) GetFileByID(workspaceID, snapshotID, fileID string) (facts.FileFact, error) {
	row := s.db.QueryRow(`
select id, repository_id, repository_root, relative_path, absolute_path, language, package_name
from files
where workspace_id = ? and snapshot_id = ? and id = ?
limit 1
`, workspaceID, snapshotID, fileID)
	var file facts.FileFact
	if err := row.Scan(&file.ID, &file.RepositoryID, &file.RepositoryRoot, &file.RelativePath, &file.AbsolutePath, &file.Language, &file.PackageName); err != nil {
		return facts.FileFact{}, err
	}
	return file, nil
}

func (s *Store) ListImportsByFile(workspaceID, snapshotID, fileID string) ([]facts.ImportFact, error) {
	rows, err := s.db.Query(`
select id, file_id, file_path, source, alias, imported_name, export_name, resolved_path, is_default, is_namespace, is_local
from imports
where workspace_id = ? and snapshot_id = ? and file_id = ?
order by source, alias
`, workspaceID, snapshotID, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []facts.ImportFact
	for rows.Next() {
		var item facts.ImportFact
		var isDefault, isNamespace, isLocal int
		if err := rows.Scan(
			&item.ID,
			&item.FileID,
			&item.FilePath,
			&item.Source,
			&item.Alias,
			&item.ImportedName,
			&item.ExportName,
			&item.ResolvedPath,
			&isDefault,
			&isNamespace,
			&isLocal,
		); err != nil {
			return nil, err
		}
		item.IsDefault = intToBool(isDefault)
		item.IsNamespace = intToBool(isNamespace)
		item.IsLocal = intToBool(isLocal)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListOutgoingCallCandidates(workspaceID, snapshotID, sourceSymbolID string) ([]facts.CallCandidate, error) {
	rows, err := s.db.Query(`
select id, source_symbol_id, target_symbol_id, target_canonical_name, target_file_path, target_export_name, relationship, evidence_type, evidence_source, extraction_method, confidence_score, order_index
from call_candidates
where workspace_id = ? and snapshot_id = ? and source_symbol_id = ?
order by order_index, id
`, workspaceID, snapshotID, sourceSymbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCallCandidates(rows)
}

func (s *Store) ListIncomingCallCandidates(workspaceID, snapshotID, targetSymbolID string) ([]facts.CallCandidate, error) {
	rows, err := s.db.Query(`
select id, source_symbol_id, target_symbol_id, target_canonical_name, target_file_path, target_export_name, relationship, evidence_type, evidence_source, extraction_method, confidence_score, order_index
from call_candidates
where workspace_id = ? and snapshot_id = ? and target_symbol_id = ?
order by order_index, id
`, workspaceID, snapshotID, targetSymbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCallCandidates(rows)
}

func (s *Store) ListTestsByFile(workspaceID, snapshotID, fileID string) ([]facts.TestFact, error) {
	rows, err := s.db.Query(`
select id, symbol_id, file_id, file_path, canonical_name, name, start_line, end_line
from tests
where workspace_id = ? and snapshot_id = ? and file_id = ?
order by start_line, name
`, workspaceID, snapshotID, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []facts.TestFact
	for rows.Next() {
		var item facts.TestFact
		if err := rows.Scan(
			&item.ID,
			&item.SymbolID,
			&item.FileID,
			&item.FilePath,
			&item.CanonicalName,
			&item.Name,
			&item.StartLine,
			&item.EndLine,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveReviewFlow(flow facts.ReviewFlow) error {
	_, err := s.db.Exec(`
insert into review_flows(id, workspace_id, snapshot_id, root_symbol_id, root_canonical_name, created_at, steps_json, accepted_json, ambiguous_json, rejected_json, uncertainty_json)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
on conflict(id) do update set
workspace_id = excluded.workspace_id,
snapshot_id = excluded.snapshot_id,
root_symbol_id = excluded.root_symbol_id,
root_canonical_name = excluded.root_canonical_name,
created_at = excluded.created_at,
steps_json = excluded.steps_json,
accepted_json = excluded.accepted_json,
ambiguous_json = excluded.ambiguous_json,
rejected_json = excluded.rejected_json,
uncertainty_json = excluded.uncertainty_json
`,
		flow.ID,
		flow.WorkspaceID,
		flow.SnapshotID,
		flow.RootSymbolID,
		flow.RootCanonicalName,
		flow.CreatedAt.UTC().Format(time.RFC3339Nano),
		mustJSON(flow.Steps),
		mustJSON(flow.Accepted),
		mustJSON(flow.Ambiguous),
		mustJSON(flow.Rejected),
		mustJSON(flow.UncertaintyNotes),
	)
	return err
}

func (s *Store) GetReviewFlow(flowID string) (facts.ReviewFlow, error) {
	row := s.db.QueryRow(`
select id, workspace_id, snapshot_id, root_symbol_id, root_canonical_name, created_at, steps_json, accepted_json, ambiguous_json, rejected_json, uncertainty_json
from review_flows
where id = ?
limit 1
`, flowID)
	var flow facts.ReviewFlow
	var createdAt string
	var stepsJSON, acceptedJSON, ambiguousJSON, rejectedJSON, uncertaintyJSON string
	if err := row.Scan(
		&flow.ID,
		&flow.WorkspaceID,
		&flow.SnapshotID,
		&flow.RootSymbolID,
		&flow.RootCanonicalName,
		&createdAt,
		&stepsJSON,
		&acceptedJSON,
		&ambiguousJSON,
		&rejectedJSON,
		&uncertaintyJSON,
	); err != nil {
		return facts.ReviewFlow{}, err
	}
	flow.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	_ = json.Unmarshal([]byte(stepsJSON), &flow.Steps)
	_ = json.Unmarshal([]byte(acceptedJSON), &flow.Accepted)
	_ = json.Unmarshal([]byte(ambiguousJSON), &flow.Ambiguous)
	_ = json.Unmarshal([]byte(rejectedJSON), &flow.Rejected)
	_ = json.Unmarshal([]byte(uncertaintyJSON), &flow.UncertaintyNotes)
	return flow, nil
}

func (s *Store) deleteSnapshot(tx *sql.Tx, workspaceID, snapshotID string) error {
	tables := []string{
		"review_flows",
		"boundaries",
		"tests",
		"diagnostics",
		"execution_hints",
		"call_candidates",
		"exports",
		"imports",
		"symbols",
		"files",
		"services",
		"repositories",
		"index_runs",
	}
	for _, table := range tables {
		if _, err := tx.Exec("delete from "+table+" where workspace_id = ? and snapshot_id = ?", workspaceID, snapshotID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrate() error {
	statements := []string{
		`create table if not exists index_runs(
workspace_id text not null,
snapshot_id text not null,
generated_at text not null,
repository_count integer not null,
file_count integer not null,
symbol_count integer not null,
call_candidate_count integer not null,
issue_counts_json text not null,
primary key(workspace_id, snapshot_id)
)`,
		`create table if not exists repositories(
workspace_id text not null,
snapshot_id text not null,
id text not null,
name text not null,
root_path text not null,
role text not null,
languages_json text not null,
build_files_json text not null,
frameworks_json text not null,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists services(
workspace_id text not null,
snapshot_id text not null,
id text not null,
repository_id text not null,
name text not null,
root_path text not null,
entrypoints_json text not null,
boundary_hints_json text not null,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists files(
workspace_id text not null,
snapshot_id text not null,
id text not null,
repository_id text not null,
repository_root text not null,
relative_path text not null,
absolute_path text not null,
language text not null,
package_name text,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists symbols(
workspace_id text not null,
snapshot_id text not null,
id text not null,
repository_id text not null,
file_id text not null,
file_path text not null,
canonical_name text not null,
name text not null,
receiver text,
kind text not null,
signature text,
start_line integer not null,
start_col integer not null,
end_line integer not null,
end_col integer not null,
start_byte integer not null,
end_byte integer not null,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists imports(
workspace_id text not null,
snapshot_id text not null,
id text not null,
file_id text not null,
file_path text not null,
source text not null,
alias text,
imported_name text,
export_name text,
resolved_path text,
is_default integer not null,
is_namespace integer not null,
is_local integer not null,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists exports(
workspace_id text not null,
snapshot_id text not null,
id text not null,
file_id text not null,
file_path text not null,
name text not null,
canonical_name text,
is_default integer not null,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists call_candidates(
workspace_id text not null,
snapshot_id text not null,
id text not null,
source_symbol_id text not null,
target_symbol_id text,
target_canonical_name text,
target_file_path text,
target_export_name text,
relationship text not null,
evidence_type text not null,
evidence_source text not null,
extraction_method text not null,
confidence_score real not null,
order_index integer not null default 0,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists execution_hints(
workspace_id text not null,
snapshot_id text not null,
id text not null,
file_path text not null,
source_symbol_id text not null,
target_symbol_id text,
target_symbol text,
kind text not null,
evidence text,
order_index integer not null default 0,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists diagnostics(
workspace_id text not null,
snapshot_id text not null,
id text not null,
file_path text not null,
symbol_id text,
category text not null,
message text not null,
evidence text,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists tests(
workspace_id text not null,
snapshot_id text not null,
id text not null,
symbol_id text not null,
file_id text not null,
file_path text not null,
canonical_name text not null,
name text not null,
start_line integer not null,
end_line integer not null,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists boundaries(
workspace_id text not null,
snapshot_id text not null,
id text not null,
repository_id text not null,
kind text not null,
framework text,
method text,
path text,
canonical_name text not null,
source_file text not null,
source_expr text,
handler_target text,
source_start_byte integer,
source_end_byte integer,
primary key(workspace_id, snapshot_id, id)
)`,
		`create table if not exists review_flows(
id text primary key,
workspace_id text not null,
snapshot_id text not null,
root_symbol_id text not null,
root_canonical_name text not null,
created_at text not null,
steps_json text not null,
accepted_json text not null,
ambiguous_json text not null,
rejected_json text not null,
uncertainty_json text not null
)`,
		`create index if not exists idx_symbols_selector on symbols(workspace_id, snapshot_id, canonical_name)`,
		`create index if not exists idx_calls_source on call_candidates(workspace_id, snapshot_id, source_symbol_id)`,
		`create index if not exists idx_calls_target on call_candidates(workspace_id, snapshot_id, target_symbol_id)`,
		`create index if not exists idx_imports_file on imports(workspace_id, snapshot_id, file_id)`,
		`create index if not exists idx_tests_file on tests(workspace_id, snapshot_id, file_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func scanCallCandidates(rows *sql.Rows) ([]facts.CallCandidate, error) {
	var out []facts.CallCandidate
	for rows.Next() {
		var item facts.CallCandidate
		if err := rows.Scan(
			&item.ID,
			&item.SourceSymbolID,
			&item.TargetSymbolID,
			&item.TargetCanonicalName,
			&item.TargetFilePath,
			&item.TargetExportName,
			&item.Relationship,
			&item.EvidenceType,
			&item.EvidenceSource,
			&item.ExtractionMethod,
			&item.ConfidenceScore,
			&item.OrderIndex,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSymbol(row scanner) (facts.SymbolFact, error) {
	var symbolFact facts.SymbolFact
	if err := row.Scan(
		&symbolFact.ID,
		&symbolFact.RepositoryID,
		&symbolFact.FileID,
		&symbolFact.FilePath,
		&symbolFact.CanonicalName,
		&symbolFact.Name,
		&symbolFact.Receiver,
		&symbolFact.Kind,
		&symbolFact.Signature,
		&symbolFact.StartLine,
		&symbolFact.StartCol,
		&symbolFact.EndLine,
		&symbolFact.EndCol,
		&symbolFact.StartByte,
		&symbolFact.EndByte,
	); err != nil {
		return facts.SymbolFact{}, err
	}
	return symbolFact, nil
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int) bool {
	return v != 0
}

var ErrNotFound = errors.New("not found")

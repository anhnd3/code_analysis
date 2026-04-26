package facts

import "time"

type Index struct {
	WorkspaceID        string              `json:"workspace_id"`
	SnapshotID         string              `json:"snapshot_id"`
	GeneratedAt        time.Time           `json:"generated_at"`
	Repositories       []RepositoryFact    `json:"repositories"`
	Services           []ServiceFact       `json:"services"`
	Files              []FileFact          `json:"files"`
	Symbols            []SymbolFact        `json:"symbols"`
	Imports            []ImportFact        `json:"imports"`
	Exports            []ExportFact        `json:"exports"`
	CallCandidates     []CallCandidate     `json:"call_candidates"`
	ExecutionHints     []ExecutionHintFact `json:"execution_hints"`
	Diagnostics        []DiagnosticFact    `json:"diagnostics"`
	Tests              []TestFact          `json:"tests"`
	Boundaries         []BoundaryFact      `json:"boundaries"`
	IssueCounts        IssueCountsFact     `json:"issue_counts"`
	RepositoryCount    int                 `json:"repository_count"`
	FileCount          int                 `json:"file_count"`
	SymbolCount        int                 `json:"symbol_count"`
	CallCandidateCount int                 `json:"call_candidate_count"`
}

type RepositoryFact struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	RootPath   string   `json:"root_path"`
	Role       string   `json:"role"`
	Languages  []string `json:"languages,omitempty"`
	BuildFiles []string `json:"build_files,omitempty"`
	Frameworks []string `json:"frameworks,omitempty"`
}

type ServiceFact struct {
	ID            string   `json:"id"`
	RepositoryID  string   `json:"repository_id"`
	Name          string   `json:"name"`
	RootPath      string   `json:"root_path"`
	Entrypoints   []string `json:"entrypoints,omitempty"`
	BoundaryHints []string `json:"boundary_hints,omitempty"`
}

type FileFact struct {
	ID             string `json:"id"`
	RepositoryID   string `json:"repository_id"`
	RepositoryRoot string `json:"repository_root"`
	RelativePath   string `json:"relative_path"`
	AbsolutePath   string `json:"absolute_path"`
	Language       string `json:"language"`
	PackageName    string `json:"package_name,omitempty"`
}

type SymbolFact struct {
	ID            string `json:"id"`
	RepositoryID  string `json:"repository_id"`
	FileID        string `json:"file_id"`
	FilePath      string `json:"file_path"`
	CanonicalName string `json:"canonical_name"`
	Name          string `json:"name"`
	Receiver      string `json:"receiver,omitempty"`
	Kind          string `json:"kind"`
	Signature     string `json:"signature,omitempty"`
	StartLine     uint32 `json:"start_line"`
	StartCol      uint32 `json:"start_col"`
	EndLine       uint32 `json:"end_line"`
	EndCol        uint32 `json:"end_col"`
	StartByte     uint32 `json:"start_byte"`
	EndByte       uint32 `json:"end_byte"`
}

type ImportFact struct {
	ID           string `json:"id"`
	FileID       string `json:"file_id"`
	FilePath     string `json:"file_path"`
	Source       string `json:"source"`
	Alias        string `json:"alias,omitempty"`
	ImportedName string `json:"imported_name,omitempty"`
	ExportName   string `json:"export_name,omitempty"`
	ResolvedPath string `json:"resolved_path,omitempty"`
	IsDefault    bool   `json:"is_default,omitempty"`
	IsNamespace  bool   `json:"is_namespace,omitempty"`
	IsLocal      bool   `json:"is_local,omitempty"`
}

type ExportFact struct {
	ID            string `json:"id"`
	FileID        string `json:"file_id"`
	FilePath      string `json:"file_path"`
	Name          string `json:"name"`
	CanonicalName string `json:"canonical_name,omitempty"`
	IsDefault     bool   `json:"is_default,omitempty"`
}

type CallCandidate struct {
	ID                  string  `json:"id"`
	SourceSymbolID      string  `json:"source_symbol_id"`
	TargetSymbolID      string  `json:"target_symbol_id,omitempty"`
	TargetCanonicalName string  `json:"target_canonical_name,omitempty"`
	TargetFilePath      string  `json:"target_file_path,omitempty"`
	TargetExportName    string  `json:"target_export_name,omitempty"`
	Relationship        string  `json:"relationship"`
	EvidenceType        string  `json:"evidence_type"`
	EvidenceSource      string  `json:"evidence_source"`
	ExtractionMethod    string  `json:"extraction_method"`
	ConfidenceScore     float64 `json:"confidence_score"`
	OrderIndex          int     `json:"order_index,omitempty"`
}

type ExecutionHintFact struct {
	ID             string `json:"id"`
	FilePath       string `json:"file_path"`
	SourceSymbolID string `json:"source_symbol_id"`
	TargetSymbolID string `json:"target_symbol_id,omitempty"`
	TargetSymbol   string `json:"target_symbol,omitempty"`
	Kind           string `json:"kind"`
	Evidence       string `json:"evidence,omitempty"`
	OrderIndex     int    `json:"order_index"`
}

type DiagnosticFact struct {
	ID       string `json:"id"`
	FilePath string `json:"file_path"`
	SymbolID string `json:"symbol_id,omitempty"`
	Category string `json:"category"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

type TestFact struct {
	ID            string `json:"id"`
	SymbolID      string `json:"symbol_id"`
	FileID        string `json:"file_id"`
	FilePath      string `json:"file_path"`
	CanonicalName string `json:"canonical_name"`
	Name          string `json:"name"`
	StartLine     uint32 `json:"start_line"`
	EndLine       uint32 `json:"end_line"`
}

type BoundaryFact struct {
	ID              string `json:"id"`
	RepositoryID    string `json:"repository_id"`
	Kind            string `json:"kind"`
	Framework       string `json:"framework,omitempty"`
	Method          string `json:"method,omitempty"`
	Path            string `json:"path,omitempty"`
	CanonicalName   string `json:"canonical_name"`
	SourceFile      string `json:"source_file"`
	SourceExpr      string `json:"source_expr,omitempty"`
	HandlerTarget   string `json:"handler_target,omitempty"`
	SourceStartByte uint32 `json:"source_start_byte,omitempty"`
	SourceEndByte   uint32 `json:"source_end_byte,omitempty"`
}

type IssueCountsFact struct {
	UnresolvedImports             int `json:"unresolved_imports"`
	AmbiguousRelations            int `json:"ambiguous_relations"`
	UnsupportedConstructs         int `json:"unsupported_constructs"`
	SkippedIgnoredFiles           int `json:"skipped_ignored_files"`
	DeferredBoundaryStitching     int `json:"deferred_boundary_stitching"`
	ServiceAttributionAmbiguities int `json:"service_attribution_ambiguities"`
}

type ContextPacket struct {
	WorkspaceID        string          `json:"workspace_id"`
	SnapshotID         string          `json:"snapshot_id"`
	RootSymbol         SymbolFact      `json:"root_symbol"`
	RootFile           FileFact        `json:"root_file"`
	FunctionSource     string          `json:"function_source,omitempty"`
	SurroundingContext string          `json:"surrounding_context,omitempty"`
	Imports            []ImportFact    `json:"imports,omitempty"`
	OutgoingCandidates []CallCandidate `json:"outgoing_candidates,omitempty"`
	IncomingCandidates []CallCandidate `json:"incoming_candidates,omitempty"`
	NearbyTests        []TestFact      `json:"nearby_tests,omitempty"`
}

type StepStatus string

const (
	StepAccepted  StepStatus = "accepted"
	StepAmbiguous StepStatus = "ambiguous"
	StepRejected  StepStatus = "rejected"
)

type EvidenceRef struct {
	SymbolID  string `json:"symbol_id,omitempty"`
	FilePath  string `json:"file_path,omitempty"`
	StartLine uint32 `json:"start_line,omitempty"`
	EndLine   uint32 `json:"end_line,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Source    string `json:"source,omitempty"`
}

type ReviewStep struct {
	ID                string        `json:"id"`
	FromSymbolID      string        `json:"from_symbol_id"`
	FromCanonicalName string        `json:"from_canonical_name"`
	ToSymbolID        string        `json:"to_symbol_id,omitempty"`
	ToCanonicalName   string        `json:"to_canonical_name,omitempty"`
	Status            StepStatus    `json:"status"`
	Rationale         string        `json:"rationale,omitempty"`
	Evidence          []EvidenceRef `json:"evidence,omitempty"`
}

type ReviewFlow struct {
	ID                string       `json:"id"`
	WorkspaceID       string       `json:"workspace_id"`
	SnapshotID        string       `json:"snapshot_id"`
	RootSymbolID      string       `json:"root_symbol_id"`
	RootCanonicalName string       `json:"root_canonical_name"`
	CreatedAt         time.Time    `json:"created_at"`
	Steps             []ReviewStep `json:"steps"`
	Accepted          []ReviewStep `json:"accepted,omitempty"`
	Ambiguous         []ReviewStep `json:"ambiguous,omitempty"`
	Rejected          []ReviewStep `json:"rejected,omitempty"`
	UncertaintyNotes  []string     `json:"uncertainty_notes,omitempty"`
}

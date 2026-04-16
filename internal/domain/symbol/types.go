package symbol

import "analysis-module/internal/domain/executionhint"

type ID string
type Kind string

const (
	KindFunction     Kind = "function"
	KindMethod       Kind = "method"
	KindClass        Kind = "class"
	KindStruct       Kind = "struct"
	KindInterface    Kind = "interface"
	KindPackage      Kind = "package"
	KindRouteHandler Kind = "route_handler"
	KindGRPCHandler  Kind = "grpc_handler"
	KindProducer     Kind = "producer"
	KindConsumer     Kind = "consumer"
	KindTestFunction Kind = "test_function"
)

type CodeLocation struct {
	FilePath  string `json:"file_path"`
	StartLine uint32 `json:"start_line"`
	StartCol  uint32 `json:"start_col"`
	EndLine   uint32 `json:"end_line"`
	EndCol    uint32 `json:"end_col"`
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
}

type FileRef struct {
	RepositoryID   string `json:"repository_id"`
	RepositoryRoot string `json:"repository_root"`
	AbsolutePath   string `json:"absolute_path"`
	RelativePath   string `json:"relative_path"`
	Language       string `json:"language"`
}

type ImportBinding struct {
	Source       string `json:"source"`
	Alias        string `json:"alias,omitempty"`
	ImportedName string `json:"imported_name,omitempty"`
	ExportName   string `json:"export_name,omitempty"`
	ResolvedPath string `json:"resolved_path,omitempty"`
	IsDefault    bool   `json:"is_default,omitempty"`
	IsNamespace  bool   `json:"is_namespace,omitempty"`
	IsLocal      bool   `json:"is_local,omitempty"`
}

type ExportBinding struct {
	Name          string `json:"name"`
	CanonicalName string `json:"canonical_name,omitempty"`
	IsDefault     bool   `json:"is_default,omitempty"`
}

type Diagnostic struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	FilePath string `json:"file_path,omitempty"`
	SymbolID ID     `json:"symbol_id,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type Symbol struct {
	ID            ID           `json:"id"`
	RepositoryID  string       `json:"repository_id"`
	ServiceID     string       `json:"service_id,omitempty"`
	FilePath      string       `json:"file_path"`
	PackageName   string       `json:"package_name"`
	Name          string       `json:"name"`
	Receiver      string       `json:"receiver,omitempty"`
	CanonicalName string       `json:"canonical_name"`
	Kind          Kind         `json:"kind"`
	Signature     string            `json:"signature"`
	Location      CodeLocation      `json:"location"`
	Properties    map[string]string `json:"properties,omitempty"`
}

type RelationCandidate struct {
	SourceSymbolID      ID      `json:"source_symbol_id"`
	TargetCanonicalName string  `json:"target_canonical_name"`
	TargetFilePath      string  `json:"target_file_path,omitempty"`
	TargetExportName    string  `json:"target_export_name,omitempty"`
	Relationship        string  `json:"relationship"`
	EvidenceType        string  `json:"evidence_type"`
	EvidenceSource      string  `json:"evidence_source"`
	ExtractionMethod    string  `json:"extraction_method"`
	ConfidenceScore     float64 `json:"confidence_score"`
}

type FileExtractionResult struct {
	FilePath       string              `json:"file_path"`
	Language       string              `json:"language,omitempty"`
	PackageName    string              `json:"package_name"`
	Imports        []string            `json:"imports"`
	ImportBindings []ImportBinding     `json:"import_bindings,omitempty"`
	Exports        []ExportBinding     `json:"exports,omitempty"`
	Symbols        []Symbol            `json:"symbols"`
	Relations      []RelationCandidate    `json:"relations"`
	Hints          []executionhint.Hint   `json:"hints,omitempty"`
	Diagnostics    []Diagnostic           `json:"diagnostics,omitempty"`
	Warnings       []string               `json:"warnings"`
}

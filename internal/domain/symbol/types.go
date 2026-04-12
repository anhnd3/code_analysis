package symbol

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
	RepositoryID string `json:"repository_id"`
	AbsolutePath string `json:"absolute_path"`
	RelativePath string `json:"relative_path"`
	Language     string `json:"language"`
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
	Signature     string       `json:"signature"`
	Location      CodeLocation `json:"location"`
}

type RelationCandidate struct {
	SourceSymbolID      ID      `json:"source_symbol_id"`
	TargetCanonicalName string  `json:"target_canonical_name"`
	Relationship        string  `json:"relationship"`
	EvidenceType        string  `json:"evidence_type"`
	EvidenceSource      string  `json:"evidence_source"`
	ExtractionMethod    string  `json:"extraction_method"`
	ConfidenceScore     float64 `json:"confidence_score"`
}

type FileExtractionResult struct {
	FilePath    string              `json:"file_path"`
	Language    string              `json:"language,omitempty"`
	PackageName string              `json:"package_name"`
	Imports     []string            `json:"imports"`
	Symbols     []Symbol            `json:"symbols"`
	Relations   []RelationCandidate `json:"relations"`
	Warnings    []string            `json:"warnings"`
}

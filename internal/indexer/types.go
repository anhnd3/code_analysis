package indexer

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── Artifact types ──────────────────────────────────────────────

type ArtifactType string

const (
	TypeWorkspaceManifest     ArtifactType = "workspace_manifest"
	TypeRepositoryManifests   ArtifactType = "repository_manifests"
	TypeServiceManifests      ArtifactType = "service_manifests"
	TypeScanWarnings          ArtifactType = "scan_warnings"
	TypeGraphNodes            ArtifactType = "graph_nodes"
	TypeGraphEdges            ArtifactType = "graph_edges"
	TypeQualityReport         ArtifactType = "quality_report"
	TypeBuildSnapshotResult   ArtifactType = "build_snapshot_result"
	TypeReviewBundle          ArtifactType = "review_bundle"
	TypeBlastRadius           ArtifactType = "blast_radius"
	TypeImpactedTests         ArtifactType = "impacted_tests"
	TypeFlowBundle            ArtifactType = "flow_bundle"
	TypeBoundaryBundle        ArtifactType = "boundary_bundle"
	TypeRootExports           ArtifactType = "root_exports"
	TypeReducedChain          ArtifactType = "reduced_chain"
	TypeReviewFlow            ArtifactType = "review_flow"
	TypeReviewFlowBuild       ArtifactType = "review_flow_build"
	TypeSequenceModel         ArtifactType = "sequence_model"
	TypeMermaidDiagram        ArtifactType = "mermaid_diagram"
	TypeServiceCoverageReport ArtifactType = "service_coverage_report"
	TypeServiceReviewPack     ArtifactType = "service_review_pack"
	TypeSelectedFlows         ArtifactType = "selected_flows"
	TypeFactsIndex            ArtifactType = "facts_index"
	TypeReviewMarkdown        ArtifactType = "review_markdown"
)

type ArtifactRef struct {
	Type        ArtifactType `json:"type"`
	WorkspaceID string       `json:"workspace_id"`
	SnapshotID  string       `json:"snapshot_id,omitempty"`
	Path        string       `json:"path"`
}

// ─── ArtifactStore interface ──────────────────────────────────────

type ArtifactStore interface {
	SaveJSON(workspaceID, snapshotID, fileName string, artifactType ArtifactType, payload any) (ArtifactRef, error)
	SaveText(workspaceID, snapshotID, fileName string, artifactType ArtifactType, body string) (ArtifactRef, error)
}

// ─── SnapshotNamer interface ──────────────────────────────────────

type SnapshotNamer interface {
	NewWorkspaceID(root string) string
	NewSnapshotID(workspaceID string) string
}

// ─── IDs helpers ──────────────────────────────────────────────────

func Stable(prefix string, parts ...string) string {
	return prefix + "_" + stableDigest(parts...)
}

func StableSuffix(parts ...string) string {
	return stableDigest(parts...)
}

func Slug(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if builder.Len() == 0 || lastUnderscore {
				continue
			}
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	slug := strings.Trim(builder.String(), "_")
	if slug == "" {
		return "workspace"
	}
	return slug
}

func stableDigest(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// ─── TargetRef types ──────────────────────────────────────────────

type TargetKind string

const (
	TargetUnknown       TargetKind = "unknown"
	TargetPackageMethod TargetKind = "package_method"
	TargetBareFunction  TargetKind = "bare_function"
)

type TargetRefKind string

const (
	TargetRefKindExactSymbolID     TargetRefKind = "exact_symbol_id"
	TargetRefKindExactCanonical    TargetRefKind = "exact_canonical"
	TargetRefKindPackageMethodHint TargetRefKind = "package_method_hint"
	TargetRefKindUnknown           TargetRefKind = "unknown"
)

type PackageMethodRef struct {
	PackageToken string `json:"package_token"`
	MethodName   string `json:"method_name"`
}

func NormalizePackageToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "/") {
		return raw
	}
	parts := strings.Split(raw, ".")
	last := parts[len(parts)-1]
	if last != "" && !strings.Contains(last, " ") {
		return last
	}
	return raw
}

func ParsePackageMethodRef(raw string) (PackageMethodRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "/") {
		return PackageMethodRef{}, false
	}
	if strings.Count(raw, ".") != 1 {
		return PackageMethodRef{}, false
	}
	idx := strings.LastIndex(raw, ".")
	if idx <= 0 || idx >= len(raw)-1 {
		return PackageMethodRef{}, false
	}
	ref := PackageMethodRef{
		PackageToken: NormalizePackageToken(raw[:idx]),
		MethodName:   strings.TrimSpace(raw[idx+1:]),
	}
	if ref.PackageToken == "" || ref.MethodName == "" {
		return PackageMethodRef{}, false
	}
	return ref, true
}

func BuildPackageMethodHint(packageToken, methodName string) string {
	packageToken = NormalizePackageToken(packageToken)
	methodName = strings.TrimSpace(methodName)
	if packageToken == "" || methodName == "" {
		return ""
	}
	return packageToken + "." + methodName
}

// ─── ExecutionHint types ──────────────────────────────────────────

type HintKind string

const (
	HintBranch        HintKind = "branch"
	HintSpawn         HintKind = "spawn"
	HintDefer         HintKind = "defer"
	HintWait          HintKind = "wait"
	HintReturnHandler HintKind = "return_handler"
)

type Hint struct {
	Kind           HintKind `json:"kind"`
	Symbol         string   `json:"symbol"`
	SourceSymbolID string   `json:"source_symbol_id,omitempty"`
	TargetSymbolID string   `json:"target_symbol_id,omitempty"`
	TargetSymbol   string   `json:"target_symbol,omitempty"`
	Evidence       string   `json:"evidence,omitempty"`
	Details        string   `json:"details,omitempty"`
	OrderIndex     int      `json:"order_index,omitempty"`
}

// ─── Symbol types ──────────────────────────────────────────────────

type SymbolID string
type SymbolKind string

const (
	KindFunction     SymbolKind = "function"
	KindMethod       SymbolKind = "method"
	KindClass        SymbolKind = "class"
	KindStruct       SymbolKind = "struct"
	KindInterface    SymbolKind = "interface"
	KindPackage      SymbolKind = "package"
	KindRouteHandler SymbolKind = "route_handler"
	KindGRPCHandler  SymbolKind = "grpc_handler"
	KindProducer     SymbolKind = "producer"
	KindConsumer     SymbolKind = "consumer"
	KindTestFunction SymbolKind = "test_function"
)

// Aliases kept for backward compatibility. These must match the Kind* values above.
const (
	SymbolKindFunction     SymbolKind = KindFunction
	SymbolKindMethod       SymbolKind = KindMethod
	SymbolKindRouteHandler SymbolKind = KindRouteHandler
	SymbolKindGRPCHandler  SymbolKind = KindGRPCHandler
	SymbolKindTestFunction SymbolKind = KindTestFunction
	SymbolKindStruct       SymbolKind = KindStruct
	SymbolKindInterface    SymbolKind = KindInterface
	SymbolKindClass        SymbolKind = KindClass
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
	Category string   `json:"category"`
	Message  string   `json:"message"`
	FilePath string   `json:"file_path,omitempty"`
	SymbolID SymbolID `json:"symbol_id,omitempty"`
	Evidence string   `json:"evidence,omitempty"`
}

type Symbol struct {
	ID            SymbolID          `json:"id"`
	RepositoryID  string            `json:"repository_id"`
	ServiceID     string            `json:"service_id,omitempty"`
	FilePath      string            `json:"file_path"`
	PackageName   string            `json:"package_name"`
	Name          string            `json:"name"`
	Receiver      string            `json:"receiver,omitempty"`
	CanonicalName string            `json:"canonical_name"`
	Kind          SymbolKind        `json:"kind"`
	Signature     string            `json:"signature"`
	Location      CodeLocation      `json:"location"`
	Properties    map[string]string `json:"properties,omitempty"`
}

type RelationCandidate struct {
	SourceSymbolID      SymbolID   `json:"source_symbol_id"`
	TargetCanonicalName string     `json:"target_canonical_name"`
	TargetFilePath      string     `json:"target_file_path,omitempty"`
	TargetExportName    string     `json:"target_export_name,omitempty"`
	TargetKind          TargetKind `json:"-"`
	Relationship        string     `json:"relationship"`
	EvidenceType        string     `json:"evidence_type"`
	EvidenceSource      string     `json:"evidence_source"`
	ExtractionMethod    string     `json:"extraction_method"`
	ConfidenceScore     float64    `json:"confidence_score"`
	OrderIndex          int        `json:"order_index,omitempty"`
}

type FileExtractionResult struct {
	FilePath       string              `json:"file_path"`
	Language       string              `json:"language,omitempty"`
	PackageName    string              `json:"package_name"`
	Imports        []string            `json:"imports"`
	ImportBindings []ImportBinding     `json:"import_bindings,omitempty"`
	Exports        []ExportBinding     `json:"exports,omitempty"`
	Symbols        []Symbol            `json:"symbols"`
	Relations      []RelationCandidate `json:"relations"`
	Hints          []Hint              `json:"hints,omitempty"`
	Diagnostics    []Diagnostic        `json:"diagnostics,omitempty"`
	Warnings       []string            `json:"warnings"`
}

// RepositoryExtraction groups file-level extraction results per repository.
type RepositoryExtraction struct {
	Repository Manifest               `json:"repository"`
	Files      []FileExtractionResult `json:"files"`
}

// ExtractionResult is the aggregate result of symbol extraction across all repositories.
type ExtractionResult struct {
	Repositories []RepositoryExtraction `json:"repositories"`
}

// ─── SymbolExtractor interface ──────────────────────────────────────

type SymbolExtractor interface {
	Supports(lang string) bool
	ExtractFile(file FileRef) (FileExtractionResult, error)
}

type LocalRelationExtractor interface {
	ExtractRelations(file FileRef, symbols []Symbol) ([]RelationCandidate, error)
}

type SemanticHarness interface {
	Name() string
}

// ─── Repository types ──────────────────────────────────────────────

type RepoID string
type RepoRole string
type Language string

const (
	RoleService   RepoRole = "service"
	RoleSharedLib RepoRole = "shared_lib"
	RoleInfra     RepoRole = "infra"
	RoleDocs      RepoRole = "docs"
	RoleUnknown   RepoRole = "unknown"

	LanguageGo     Language = "go"
	LanguagePython Language = "python"
	LanguageJS     Language = "javascript"
	LanguageTS     Language = "typescript"
	LanguageJava   Language = "java"
	LanguageYAML   Language = "yaml"
	LanguageJSON   Language = "json"
)

type BoundaryHint struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	Details string `json:"details"`
}

type TechStackProfile struct {
	Languages      []Language `json:"languages"`
	BuildFiles     []string   `json:"build_files"`
	TestFrameworks []string   `json:"test_frameworks"`
	FrameworkHints []string   `json:"framework_hints"`
}

type Manifest struct {
	ID                RepoID            `json:"id"`
	Name              string            `json:"name"`
	RootPath          string            `json:"root_path"`
	Role              RepoRole          `json:"role"`
	IgnoreSignature   string            `json:"ignore_signature,omitempty"`
	TechStack         TechStackProfile  `json:"tech_stack"`
	GoFiles           []string          `json:"go_files"`
	PythonFiles       []string          `json:"python_files,omitempty"`
	JavaScriptFiles   []string          `json:"javascript_files,omitempty"`
	TypeScriptFiles   []string          `json:"typescript_files,omitempty"`
	ConfigFiles       []string          `json:"config_files"`
	IssueCounts       IssueCounts       `json:"issue_counts,omitempty"`
	BoundaryHints     []BoundaryHint    `json:"boundary_hints"`
	CandidateServices []ServiceManifest `json:"candidate_services"`
}

type ExtractionPlan struct {
	RepositoryID RepoID   `json:"repository_id"`
	Language     Language `json:"language"`
	Files        []string `json:"files"`
}

type Inventory struct {
	WorkspaceID     string           `json:"workspace_id"`
	IgnoreSignature string           `json:"ignore_signature"`
	IssueCounts     IssueCounts      `json:"issue_counts,omitempty"`
	Repositories    []Manifest       `json:"repositories"`
	Plans           []ExtractionPlan `json:"plans"`
}

// ─── Service types ──────────────────────────────────────────────

type ServiceID string
type BoundaryType string

const (
	BoundaryHTTP  BoundaryType = "http"
	BoundaryGRPC  BoundaryType = "grpc"
	BoundaryKafka BoundaryType = "kafka"
)

type ServiceManifest struct {
	ID           ServiceID      `json:"id"`
	Name         string         `json:"name"`
	RepositoryID string         `json:"repository_id"`
	RootPath     string         `json:"root_path"`
	Entrypoints  []string       `json:"entrypoints"`
	Boundaries   []BoundaryType `json:"boundaries"`
}

// ─── Boundary types ──────────────────────────────────────────────

type LinkStatus string

const (
	StatusConfirmed        LinkStatus = "confirmed"
	StatusCompatibleSubset LinkStatus = "compatible_subset"
	StatusCandidate        LinkStatus = "candidate"
	StatusMismatch         LinkStatus = "mismatch"
	StatusExternalOnly     LinkStatus = "external_only"
)

type Protocol string

const (
	ProtocolGRPC  Protocol = "grpc"
	ProtocolREST  Protocol = "rest"
	ProtocolKafka Protocol = "kafka"
)

type BoundaryIdentity struct {
	Protocol    Protocol `json:"protocol"`
	ServiceName string   `json:"service_name"`
	Endpoint    string   `json:"endpoint"`
	Detail      string   `json:"detail,omitempty"`
}

type ContractField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  int    `json:"tag,omitempty"`
}

type BoundaryContract struct {
	Package        string          `json:"package,omitempty"`
	ServiceName    string          `json:"service_name"`
	RPCName        string          `json:"rpc_name,omitempty"`
	RequestType    string          `json:"request_type,omitempty"`
	ResponseType   string          `json:"response_type,omitempty"`
	RequestFields  []ContractField `json:"request_fields,omitempty"`
	ResponseFields []ContractField `json:"response_fields,omitempty"`
	Method         string          `json:"method,omitempty"`
	Path           string          `json:"path,omitempty"`
	TopicName      string          `json:"topic_name,omitempty"`
	EventType      string          `json:"event_type,omitempty"`
}

type BoundaryLink struct {
	OutboundNodeID string     `json:"outbound_node_id"`
	InboundNodeID  string     `json:"inbound_node_id"`
	Protocol       Protocol   `json:"protocol"`
	Status         LinkStatus `json:"status"`
	OutboundRepoID string     `json:"outbound_repo_id"`
	InboundRepoID  string     `json:"inbound_repo_id"`
	Evidence       string     `json:"evidence,omitempty"`
}

type BoundaryBundle struct {
	Links []BoundaryLink `json:"links"`
}

// ─── BoundaryRoot types ──────────────────────────────────────────────

type RootKind string

const (
	RootKindHTTP        RootKind = "http"
	RootKindGRPC        RootKind = "grpc"
	RootKindHTTPGateway RootKind = "http_gateway"
	RootKindCLI         RootKind = "cli"
	RootKindWorker      RootKind = "worker"
	RootKindConsumer    RootKind = "consumer"
)

type BoundaryRoot struct {
	ID                string     `json:"id"`
	Kind              RootKind   `json:"kind"`
	Framework         string     `json:"framework"`
	Method            string     `json:"method"`
	Path              string     `json:"path"`
	CanonicalName     string     `json:"canonical_name"`
	HandlerTarget     string     `json:"handler_target"`
	HandlerTargetKind TargetKind `json:"-"`
	RepositoryID      string     `json:"repository_id"`
	SourceFile        string     `json:"source_file"`
	SourceStartByte   uint32     `json:"source_start_byte"`
	SourceEndByte     uint32     `json:"source_end_source"`
	SourceExpr        string     `json:"source_expr"`
	Confidence        string     `json:"confidence"`
}

func StableBoundaryID(root BoundaryRoot) string {
	return Stable(
		"boundary",
		root.RepositoryID,
		root.SourceFile,
		strconv.FormatUint(uint64(root.SourceStartByte), 10),
		strconv.FormatUint(uint64(root.SourceEndByte), 10),
		root.Framework,
		root.Method,
		root.Path,
	)
}

// ─── Analysis types ──────────────────────────────────────────────

type SnapshotRef struct {
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id"`
}

type SnapshotFingerprint struct {
	WorkspaceID      string   `json:"workspace_id"`
	IgnoreSignature  string   `json:"ignore_signature"`
	RepositoryIDs    []string `json:"repository_ids"`
	TrackedFileCount int      `json:"tracked_file_count"`
}

type IssueCounts struct {
	UnresolvedImports             int `json:"unresolved_imports"`
	AmbiguousRelations            int `json:"ambiguous_relations"`
	UnsupportedConstructs         int `json:"unsupported_constructs"`
	SkippedIgnoredFiles           int `json:"skipped_ignored_files"`
	DeferredBoundaryStitching     int `json:"deferred_boundary_stitching"`
	ServiceAttributionAmbiguities int `json:"service_attribution_ambiguities"`
}

func (c *IssueCounts) Add(other IssueCounts) {
	c.UnresolvedImports += other.UnresolvedImports
	c.AmbiguousRelations += other.AmbiguousRelations
	c.UnsupportedConstructs += other.UnsupportedConstructs
	c.SkippedIgnoredFiles += other.SkippedIgnoredFiles
	c.DeferredBoundaryStitching += other.DeferredBoundaryStitching
	c.ServiceAttributionAmbiguities += other.ServiceAttributionAmbiguities
}

// ─── IgnorePolicy ──────────────────────────────────────────────

var defaultIgnoredDirs = map[string]struct{}{
	".git":          {},
	"artifacts":     {},
	".pytest_cache": {},
	"__pycache__":   {},
	".venv":         {},
	"node_modules":  {},
	"testdata":      {},
	"vendor":        {},
}

type IgnorePolicy struct {
	Patterns  []string `json:"patterns"`
	Signature string   `json:"signature"`
}

func NewIgnorePolicy(patterns []string) IgnorePolicy {
	normalized := make([]string, 0, len(patterns))
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		pattern = normalizePattern(pattern)
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		normalized = append(normalized, pattern)
	}
	sort.Strings(normalized)
	return IgnorePolicy{
		Patterns:  normalized,
		Signature: Stable("ignore", strings.Join(normalized, "|")),
	}
}

func (p IgnorePolicy) ShouldIgnore(root, path string, isDir bool) bool {
	rel := relativeToRoot(root, path)
	if rel == "." || rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if _, ok := defaultIgnoredDirs[part]; ok {
			return true
		}
	}
	for _, pattern := range p.Patterns {
		if pattern == "" {
			continue
		}
		if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
			return true
		}
		if strings.Contains(rel, pattern) {
			return true
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		if isDir {
			base := parts[len(parts)-1]
			if base == pattern {
				return true
			}
		}
	}
	return false
}

func normalizePattern(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = filepath.ToSlash(raw)
	raw = strings.TrimPrefix(raw, "./")
	raw = strings.Trim(raw, "/")
	return raw
}

func relativeToRoot(root, path string) string {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// ─── Scanner types ──────────────────────────────────────

type ScanWorkspaceRequest struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

type ScanWorkspaceResult struct {
	WorkspacePath string     `json:"workspace_path"`
	Repositories  []Manifest `json:"repositories"`
	Warnings      []string   `json:"warnings"`
}

// ─── Workspace types ──────────────────────────────────────

type WorkspaceManifest struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	RootPath        string    `json:"root_path"`
	Repositories    []string  `json:"repositories"`
	CreatedAt       time.Time `json:"created_at"`
	IgnoreSignature string    `json:"ignore_signature,omitempty"`
	RepositoryIDs   []string  `json:"repository_ids,omitempty"`
	Languages       []string  `json:"languages,omitempty"`
	Warnings        []string  `json:"warnings,omitempty"`
	ScannedAt       time.Time `json:"scanned_at,omitempty"`
}

// ─── RepoMap types ──────────────────────────────────────

type RepoMapMode string

type RankedItem struct {
	Kind  string `json:"kind"`
	Score int    `json:"score"`
	Path  string `json:"path"`
}

type RepoMap struct {
	Mode  RepoMapMode  `json:"mode"`
	Items []RankedItem `json:"items"`
}

// ─── Entrypoint types ──────────────────────────────────────

type RootType string

type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

type Entrypoint struct {
	NodeID        string          `json:"node_id"`
	CanonicalName string          `json:"canonical_name"`
	RootType      RootType        `json:"root_type"`
	Confidence    ConfidenceLevel `json:"confidence"`
	RepositoryID  string          `json:"repository_id"`
	ServiceID     string          `json:"service_id,omitempty"`
	Framework     string          `json:"framework,omitempty"`
	Method        string          `json:"method,omitempty"`
	Path          string          `json:"path,omitempty"`
	Evidence      string          `json:"evidence,omitempty"`
}

type EntrypointResult struct {
	Roots []Entrypoint `json:"roots"`
}

// ─── Quality types ──────────────────────────────────────

type CoverageMetric struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Total int    `json:"total"`
}

type GapReport struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Count    int    `json:"count"`
}

type AnalysisQualityReport struct {
	SnapshotID  string              `json:"snapshot_id"`
	Metrics     []CoverageMetric    `json:"metrics"`
	IssueCounts IssueCounts         `json:"issue_counts"`
	Gaps        []GapReport         `json:"gaps"`
	FlowMetrics *FlowQualityMetrics `json:"flow_metrics,omitempty"`
}

type FlowQualityMetrics struct {
	ResolvedEntrypoints     int `json:"resolved_entrypoints"`
	StitchedEdges           int `json:"stitched_edges"`
	BoundaryMarkers         int `json:"boundary_markers"`
	ConfirmedLinks          int `json:"confirmed_links"`
	SubsetCompatibleLinks   int `json:"subset_compatible_links"`
	CandidateLinks          int `json:"candidate_links"`
	MismatchLinks           int `json:"mismatch_links"`
	ExternalOnlyLinks       int `json:"external_only_links"`
	ReducedChainsGenerated  int `json:"reduced_chains_generated"`
	MermaidExportsGenerated int `json:"mermaid_exports_generated"`
}

// ─── RepoMap Builder interface ──────────────────────

type RepoMapBuilder interface {
	Build(target string, snapshotID string) (RepoMap, error)
}

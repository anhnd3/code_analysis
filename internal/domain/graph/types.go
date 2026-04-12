package graph

import (
	"time"

	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/symbol"
)

type NodeKind string
type EdgeKind string
type ConfidenceTier string

const (
	NodeWorkspace  NodeKind = "WORKSPACE"
	NodeRepository NodeKind = "REPOSITORY"
	NodeService    NodeKind = "SERVICE"
	NodePackage    NodeKind = "PACKAGE"
	NodeFile       NodeKind = "FILE"
	NodeSymbol     NodeKind = "SYMBOL"
	NodeEndpoint   NodeKind = "ENDPOINT"
	NodeTopic      NodeKind = "TOPIC"
	NodeTest       NodeKind = "TEST"
	NodeConfig     NodeKind = "CONFIG"
	NodeTableRef   NodeKind = "TABLE_REF"

	EdgeContains         EdgeKind = "CONTAINS"
	EdgeDefines          EdgeKind = "DEFINES"
	EdgeImports          EdgeKind = "IMPORTS"
	EdgeCalls            EdgeKind = "CALLS"
	EdgeBelongsToService EdgeKind = "BELONGS_TO_SERVICE"
	EdgeTestedBy         EdgeKind = "TESTED_BY"
	EdgeReadsConfig      EdgeKind = "READS_CONFIG"
	EdgeCallsHTTP        EdgeKind = "CALLS_HTTP"
	EdgeCallsGRPC        EdgeKind = "CALLS_GRPC"
	EdgeProducesTopic    EdgeKind = "PRODUCES_TOPIC"
	EdgeSubscribesTopic  EdgeKind = "SUBSCRIBES_TOPIC"
	EdgeEntrypointTo     EdgeKind = "ENTRYPOINT_TO"

	ConfidenceConfirmed ConfidenceTier = "confirmed"
	ConfidenceInferred  ConfidenceTier = "inferred"
	ConfidenceAmbiguous ConfidenceTier = "ambiguous"
)

type Evidence struct {
	Type             string `json:"type"`
	Source           string `json:"source"`
	ExtractionMethod string `json:"extraction_method"`
	Details          string `json:"details"`
}

type Confidence struct {
	Tier  ConfidenceTier `json:"tier"`
	Score float64        `json:"score"`
}

type Node struct {
	ID            string               `json:"id"`
	Kind          NodeKind             `json:"kind"`
	CanonicalName string               `json:"canonical_name"`
	Language      string               `json:"language,omitempty"`
	RepositoryID  string               `json:"repository_id,omitempty"`
	FilePath      string               `json:"file_path,omitempty"`
	Location      *symbol.CodeLocation `json:"location,omitempty"`
	Properties    map[string]string    `json:"properties,omitempty"`
	SnapshotID    string               `json:"snapshot_id"`
}

type Edge struct {
	ID         string            `json:"id"`
	Kind       EdgeKind          `json:"kind"`
	From       string            `json:"from"`
	To         string            `json:"to"`
	Evidence   Evidence          `json:"evidence"`
	Confidence Confidence        `json:"confidence"`
	Properties map[string]string `json:"properties,omitempty"`
	SnapshotID string            `json:"snapshot_id"`
}

type SnapshotMetadata struct {
	IgnoreSignature string               `json:"ignore_signature,omitempty"`
	RepositoryCount int                  `json:"repository_count"`
	FileCount       int                  `json:"file_count"`
	SymbolCount     int                  `json:"symbol_count"`
	EdgeCount       int                  `json:"edge_count"`
	IssueCounts     analysis.IssueCounts `json:"issue_counts,omitempty"`
}

type GraphSnapshot struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	CreatedAt   time.Time        `json:"created_at"`
	Nodes       []Node           `json:"nodes"`
	Edges       []Edge           `json:"edges"`
	Metadata    SnapshotMetadata `json:"metadata"`
}

type Path struct {
	NodeIDs []string `json:"node_ids"`
}

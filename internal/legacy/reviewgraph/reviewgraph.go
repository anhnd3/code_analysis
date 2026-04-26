package reviewgraph

import (
	domainreviewgraph "analysis-module/internal/domain/reviewgraph"
	reviewgraphexportsvc "analysis-module/internal/services/reviewgraph_export"
	reviewgraphimportsvc "analysis-module/internal/services/reviewgraph_import"
	reviewgraphpathssvc "analysis-module/internal/services/reviewgraph_paths"
	reviewgraphselectsvc "analysis-module/internal/services/reviewgraph_select"
	reviewgraphexportwf "analysis-module/internal/workflows/review_graph_export"
	reviewgraphimportwf "analysis-module/internal/workflows/review_graph_import"
	reviewgraphselectwf "analysis-module/internal/workflows/review_graph_list_startpoints"
)

// Package reviewgraph is the compatibility boundary for the legacy deterministic review-graph stack.

type NodeKind = domainreviewgraph.NodeKind
type NodeRole = domainreviewgraph.NodeRole
type EdgeType = domainreviewgraph.EdgeType
type FlowKind = domainreviewgraph.FlowKind
type ArtifactType = domainreviewgraph.ArtifactType
type TraversalMode = domainreviewgraph.TraversalMode

type Node = domainreviewgraph.Node
type Edge = domainreviewgraph.Edge
type Artifact = domainreviewgraph.Artifact
type ResolvedTarget = domainreviewgraph.ResolvedTarget
type CycleSummary = domainreviewgraph.CycleSummary
type TraversalCaps = domainreviewgraph.TraversalCaps
type PathSummary = domainreviewgraph.PathSummary
type AsyncParticipant = domainreviewgraph.AsyncParticipant
type AsyncBridgeSummary = domainreviewgraph.AsyncBridgeSummary
type CoverageStats = domainreviewgraph.CoverageStats
type TraversalResult = domainreviewgraph.TraversalResult
type ImportDiagnostic = domainreviewgraph.ImportDiagnostic
type ImportCounts = domainreviewgraph.ImportCounts
type ImportManifest = domainreviewgraph.ImportManifest

const (
	NodeFunction      = domainreviewgraph.NodeFunction
	NodeMethod        = domainreviewgraph.NodeMethod
	NodeClass         = domainreviewgraph.NodeClass
	NodeType          = domainreviewgraph.NodeType
	NodeModule        = domainreviewgraph.NodeModule
	NodeFile          = domainreviewgraph.NodeFile
	NodeService       = domainreviewgraph.NodeService
	NodeWorkflow      = domainreviewgraph.NodeWorkflow
	NodeContainer     = domainreviewgraph.NodeContainer
	NodeHTTPEndpoint  = domainreviewgraph.NodeHTTPEndpoint
	NodeGRPCMethod    = domainreviewgraph.NodeGRPCMethod
	NodeEventTopic    = domainreviewgraph.NodeEventTopic
	NodePubSubChannel = domainreviewgraph.NodePubSubChannel
	NodeQueue         = domainreviewgraph.NodeQueue
	NodeSchedulerJob  = domainreviewgraph.NodeSchedulerJob
	NodeAsyncTask     = domainreviewgraph.NodeAsyncTask
	NodeInProcChannel = domainreviewgraph.NodeInProcChannel

	RoleNormal        = domainreviewgraph.RoleNormal
	RoleEntrypoint    = domainreviewgraph.RoleEntrypoint
	RoleBoundary      = domainreviewgraph.RoleBoundary
	RolePublicAPI     = domainreviewgraph.RolePublicAPI
	RoleAsyncProducer = domainreviewgraph.RoleAsyncProducer
	RoleAsyncConsumer = domainreviewgraph.RoleAsyncConsumer
	RoleScheduler     = domainreviewgraph.RoleScheduler
	RoleSharedInfra   = domainreviewgraph.RoleSharedInfra

	EdgeDefines             = domainreviewgraph.EdgeDefines
	EdgeImports             = domainreviewgraph.EdgeImports
	EdgeCalls               = domainreviewgraph.EdgeCalls
	EdgeBelongsToService    = domainreviewgraph.EdgeBelongsToService
	EdgeEmitsEvent          = domainreviewgraph.EdgeEmitsEvent
	EdgeConsumesEvent       = domainreviewgraph.EdgeConsumesEvent
	EdgePublishesMessage    = domainreviewgraph.EdgePublishesMessage
	EdgeSubscribesMessage   = domainreviewgraph.EdgeSubscribesMessage
	EdgeEnqueuesJob         = domainreviewgraph.EdgeEnqueuesJob
	EdgeDequeuesJob         = domainreviewgraph.EdgeDequeuesJob
	EdgeSchedulesTask       = domainreviewgraph.EdgeSchedulesTask
	EdgeTriggersHTTP        = domainreviewgraph.EdgeTriggersHTTP
	EdgeSpawnsAsync         = domainreviewgraph.EdgeSpawnsAsync
	EdgeRunsAsync           = domainreviewgraph.EdgeRunsAsync
	EdgeSendsToChannel      = domainreviewgraph.EdgeSendsToChannel
	EdgeReceivesFromChannel = domainreviewgraph.EdgeReceivesFromChannel
	EdgeContains            = domainreviewgraph.EdgeContains
	EdgeBelongsToContainer  = domainreviewgraph.EdgeBelongsToContainer
	EdgeEntrypointFor       = domainreviewgraph.EdgeEntrypointFor
	EdgeOrchestrates        = domainreviewgraph.EdgeOrchestrates
	EdgeUsesSharedHelper    = domainreviewgraph.EdgeUsesSharedHelper
	EdgeCollapsesHelper     = domainreviewgraph.EdgeCollapsesHelper

	FlowSync  = domainreviewgraph.FlowSync
	FlowAsync = domainreviewgraph.FlowAsync

	ArtifactImportManifest       = domainreviewgraph.ArtifactImportManifest
	ArtifactResolvedTargets      = domainreviewgraph.ArtifactResolvedTargets
	ArtifactReviewFlow           = domainreviewgraph.ArtifactReviewFlow
	ArtifactReviewIndex          = domainreviewgraph.ArtifactReviewIndex
	ArtifactReviewThreadIndex    = domainreviewgraph.ArtifactReviewThreadIndex
	ArtifactReviewThreadOverview = domainreviewgraph.ArtifactReviewThreadOverview
	ArtifactReviewThreadFocus    = domainreviewgraph.ArtifactReviewThreadFocus
	ArtifactResidualSummary      = domainreviewgraph.ArtifactResidualSummary
	ArtifactDiagnostics          = domainreviewgraph.ArtifactDiagnostics
	ArtifactRunManifest          = domainreviewgraph.ArtifactRunManifest
	ArtifactLayeringPlan         = domainreviewgraph.ArtifactLayeringPlan
	ArtifactLayeredTargets       = domainreviewgraph.ArtifactLayeredTargets
	ArtifactLayeringCompare      = domainreviewgraph.ArtifactLayeringCompare

	TraversalFullFlow = domainreviewgraph.TraversalFullFlow
	TraversalBounded  = domainreviewgraph.TraversalBounded

	ImporterVersion       = domainreviewgraph.ImporterVersion
	AsyncHeuristicVersion = domainreviewgraph.AsyncHeuristicVersion
	AsyncV2Version        = domainreviewgraph.AsyncV2Version
)

type PathsService = reviewgraphpathssvc.Service
type ResolvedPaths = reviewgraphpathssvc.ResolvedPaths

type ImportWorkflow = reviewgraphimportwf.Workflow
type SelectWorkflow = reviewgraphselectwf.Workflow
type ExportWorkflow = reviewgraphexportwf.Workflow

type ImportRequest = reviewgraphimportwf.Request
type ImportResult = reviewgraphimportwf.Result
type SelectRequest = reviewgraphselectwf.Request
type SelectResult = reviewgraphselectwf.Result
type ExportRequest = reviewgraphexportwf.Request
type ExportResult = reviewgraphexportwf.Result

func NewPathsService(artifactRoot string) PathsService {
	return reviewgraphpathssvc.New(artifactRoot)
}

func NewImportWorkflow(service reviewgraphimportsvc.Service) ImportWorkflow {
	return reviewgraphimportwf.New(service)
}

func NewSelectWorkflow(service reviewgraphselectsvc.Service) SelectWorkflow {
	return reviewgraphselectwf.New(service)
}

func NewExportWorkflow(service reviewgraphexportsvc.Service) ExportWorkflow {
	return reviewgraphexportwf.New(service)
}

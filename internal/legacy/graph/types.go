package graph

import domaingraph "analysis-module/internal/domain/graph"

// Package graph is the compatibility boundary for the legacy deterministic graph model.

type NodeKind = domaingraph.NodeKind
type EdgeKind = domaingraph.EdgeKind
type ConfidenceTier = domaingraph.ConfidenceTier

type Evidence = domaingraph.Evidence
type Confidence = domaingraph.Confidence
type Node = domaingraph.Node
type Edge = domaingraph.Edge
type SnapshotMetadata = domaingraph.SnapshotMetadata
type GraphSnapshot = domaingraph.GraphSnapshot
type Path = domaingraph.Path

const (
	NodeWorkspace  = domaingraph.NodeWorkspace
	NodeRepository = domaingraph.NodeRepository
	NodeService    = domaingraph.NodeService
	NodePackage    = domaingraph.NodePackage
	NodeFile       = domaingraph.NodeFile
	NodeSymbol     = domaingraph.NodeSymbol
	NodeEndpoint   = domaingraph.NodeEndpoint
	NodeTopic      = domaingraph.NodeTopic
	NodeTest       = domaingraph.NodeTest
	NodeConfig     = domaingraph.NodeConfig
	NodeTableRef   = domaingraph.NodeTableRef

	EdgeContains          = domaingraph.EdgeContains
	EdgeDefines           = domaingraph.EdgeDefines
	EdgeImports           = domaingraph.EdgeImports
	EdgeCalls             = domaingraph.EdgeCalls
	EdgeRegistersBoundary = domaingraph.EdgeRegistersBoundary
	EdgeReturnsHandler    = domaingraph.EdgeReturnsHandler
	EdgeSpawns            = domaingraph.EdgeSpawns
	EdgeDefers            = domaingraph.EdgeDefers
	EdgeWaitsOn           = domaingraph.EdgeWaitsOn
	EdgeBelongsToService  = domaingraph.EdgeBelongsToService
	EdgeTestedBy          = domaingraph.EdgeTestedBy
	EdgeReadsConfig       = domaingraph.EdgeReadsConfig
	EdgeCallsHTTP         = domaingraph.EdgeCallsHTTP
	EdgeCallsGRPC         = domaingraph.EdgeCallsGRPC
	EdgeProducesTopic     = domaingraph.EdgeProducesTopic
	EdgeSubscribesTopic   = domaingraph.EdgeSubscribesTopic
	EdgeEntrypointTo      = domaingraph.EdgeEntrypointTo
	EdgeBranches          = domaingraph.EdgeBranches

	ConfidenceConfirmed = domaingraph.ConfidenceConfirmed
	ConfidenceInferred  = domaingraph.ConfidenceInferred
	ConfidenceAmbiguous = domaingraph.ConfidenceAmbiguous
)

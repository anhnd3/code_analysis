package reviewflow

import (
	domainreviewflow "analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/reviewflow_build"
	"analysis-module/internal/services/reviewflow_expand"
	"analysis-module/internal/services/reviewflow_policy"
)

// Package reviewflow is the compatibility boundary for the legacy deterministic reviewflow stack.

type MessageKind = domainreviewflow.MessageKind
type BlockKind = domainreviewflow.BlockKind
type Flow = domainreviewflow.Flow
type Participant = domainreviewflow.Participant
type Stage = domainreviewflow.Stage
type Message = domainreviewflow.Message
type BlockSection = domainreviewflow.BlockSection
type Block = domainreviewflow.Block
type Note = domainreviewflow.Note
type Metadata = domainreviewflow.Metadata

const (
	MessageSync   = domainreviewflow.MessageSync
	MessageAsync  = domainreviewflow.MessageAsync
	MessageReturn = domainreviewflow.MessageReturn

	BlockAlt     = domainreviewflow.BlockAlt
	BlockLoop    = domainreviewflow.BlockLoop
	BlockPar     = domainreviewflow.BlockPar
	BlockSummary = domainreviewflow.BlockSummary
)

type FlowStitchService = flow_stitch.Service
type ChainReduceService = chain_reduce.Service
type ReviewFlowBuildService = reviewflow_build.Service
type ReviewFlowExpandService = reviewflow_expand.Service
type ReviewFlowPolicyService = reviewflow_policy.Service

type ReviewFlowBuildOptions = reviewflow_build.BuildOptions
type ReviewFlowBuildResult = reviewflow_build.BuildResult
type ReviewFlowExpandInput = reviewflow_expand.Input
type ReviewFlowExpandResult = reviewflow_expand.Result
type ReviewFlowPolicy = reviewflow_policy.Policy
type ReviewFlowPolicyResolveInput = reviewflow_policy.ResolveInput
type ReviewFlowPolicyResolveResult = reviewflow_policy.ResolveResult
type ReviewFlowBuildCandidateKind = reviewflow_build.CandidateKind

func NewFlowStitchService() FlowStitchService {
	return flow_stitch.New()
}

func NewChainReduceService() ChainReduceService {
	return chain_reduce.New()
}

func NewReviewFlowBuildService() ReviewFlowBuildService {
	return reviewflow_build.New()
}

func NewReviewFlowExpandService() ReviewFlowExpandService {
	return reviewflow_expand.New()
}

func NewReviewFlowPolicyService() ReviewFlowPolicyService {
	return reviewflow_policy.New()
}

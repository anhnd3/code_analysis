package reviewflow_policy

import (
	"strings"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/reviewpack"
)

const (
	FamilyBootstrapStartup = "bootstrap_startup"
	FamilySimpleQuery      = "simple_query"
	FamilyConfigLookup     = "config_lookup"
	FamilyDetectorPipeline = "detector_pipeline"
	FamilyScanPipeline     = "scan_pipeline"
	FamilyBlacklistGate    = "blacklist_gate"
	FamilyGatewayProxy     = "gateway_proxy"
	FamilyDefault          = "default"
)

type Policy struct {
	Family                    string   `json:"family"`
	PreferredCandidateKinds   []string `json:"preferred_candidate_kinds,omitempty"`
	MaxVisibleParticipants    int      `json:"max_visible_participants,omitempty"`
	MinBusinessExpansionDepth int      `json:"min_business_expansion_depth,omitempty"`
	MaxBusinessExpansionDepth int      `json:"max_business_expansion_depth,omitempty"`
	PreserveBranchBlocks      bool     `json:"preserve_branch_blocks,omitempty"`
	PreserveAsyncBlocks       bool     `json:"preserve_async_blocks,omitempty"`
	PreservePostProcessing    bool     `json:"preserve_post_processing,omitempty"`
	AddHTTPEntryParticipants  bool     `json:"add_http_entry_participants,omitempty"`
	AddBootstrapLifecycle     bool     `json:"add_bootstrap_lifecycle,omitempty"`
}

type ResolveInput struct {
	Root           entrypoint.Root
	ExpectedFamily string
	ServiceName    string
	WorkspacePath  string
}

type ResolveResult struct {
	Policy Policy
	Source reviewpack.PolicySource
}

type Service struct{}

func New() Service {
	return Service{}
}

func (s Service) Resolve(input ResolveInput) ResolveResult {
	_ = input.ServiceName
	_ = input.WorkspacePath

	family := normalizeFamily(input.ExpectedFamily)
	if family != "" {
		return ResolveResult{
			Policy: policyForFamily(family),
			Source: reviewpack.PolicySourceManifest,
		}
	}

	derived := deriveFamilyFromRoot(input.Root)
	if derived != FamilyDefault {
		return ResolveResult{
			Policy: policyForFamily(derived),
			Source: reviewpack.PolicySourceRouteMetadata,
		}
	}
	return ResolveResult{
		Policy: policyForFamily(FamilyDefault),
		Source: reviewpack.PolicySourceDefault,
	}
}

func normalizeFamily(family string) string {
	switch strings.TrimSpace(strings.ToLower(family)) {
	case FamilyBootstrapStartup:
		return FamilyBootstrapStartup
	case FamilySimpleQuery:
		return FamilySimpleQuery
	case FamilyConfigLookup:
		return FamilyConfigLookup
	case FamilyDetectorPipeline:
		return FamilyDetectorPipeline
	case FamilyScanPipeline:
		return FamilyScanPipeline
	case FamilyBlacklistGate:
		return FamilyBlacklistGate
	case FamilyGatewayProxy:
		return FamilyGatewayProxy
	case FamilyDefault:
		return FamilyDefault
	default:
		return ""
	}
}

func deriveFamilyFromRoot(root entrypoint.Root) string {
	if root.RootType == entrypoint.RootBootstrap {
		return FamilyBootstrapStartup
	}
	if root.Framework == "grpc-gateway" {
		return FamilyGatewayProxy
	}
	path := strings.ToLower(root.Path)
	if strings.Contains(path, "detect-qr") {
		return FamilyDetectorPipeline
	}
	if strings.Contains(path, "blacklist") {
		return FamilyBlacklistGate
	}
	if strings.Contains(path, "predict") || strings.Contains(path, "scan360") || strings.Contains(path, "extract-bank-info") {
		return FamilyScanPipeline
	}
	if strings.Contains(path, "config") {
		return FamilyConfigLookup
	}
	if strings.EqualFold(root.Method, "GET") {
		return FamilySimpleQuery
	}
	return FamilyDefault
}

func policyForFamily(family string) Policy {
	switch family {
	case FamilyBootstrapStartup:
		return Policy{
			Family:                    FamilyBootstrapStartup,
			PreferredCandidateKinds:   []string{"compact_review", "faithful", "async_summarized"},
			MaxVisibleParticipants:    8,
			MinBusinessExpansionDepth: 0,
			MaxBusinessExpansionDepth: 0,
			PreserveBranchBlocks:      false,
			PreserveAsyncBlocks:       false,
			PreservePostProcessing:    false,
			AddHTTPEntryParticipants:  false,
			AddBootstrapLifecycle:     true,
		}
	case FamilyDetectorPipeline:
		return Policy{
			Family:                    FamilyDetectorPipeline,
			PreferredCandidateKinds:   []string{"faithful", "compact_review", "async_summarized"},
			MaxVisibleParticipants:    10,
			MinBusinessExpansionDepth: 1,
			MaxBusinessExpansionDepth: 3,
			PreserveBranchBlocks:      true,
			PreserveAsyncBlocks:       true,
			PreservePostProcessing:    true,
			AddHTTPEntryParticipants:  true,
		}
	case FamilyScanPipeline:
		return Policy{
			Family:                    FamilyScanPipeline,
			PreferredCandidateKinds:   []string{"compact_review", "faithful", "async_summarized"},
			MaxVisibleParticipants:    10,
			MinBusinessExpansionDepth: 1,
			MaxBusinessExpansionDepth: 3,
			PreserveBranchBlocks:      true,
			PreserveAsyncBlocks:       true,
			PreservePostProcessing:    true,
			AddHTTPEntryParticipants:  true,
		}
	case FamilyBlacklistGate:
		return Policy{
			Family:                    FamilyBlacklistGate,
			PreferredCandidateKinds:   []string{"compact_review", "faithful", "async_summarized"},
			MaxVisibleParticipants:    8,
			MinBusinessExpansionDepth: 1,
			MaxBusinessExpansionDepth: 2,
			PreserveBranchBlocks:      true,
			PreserveAsyncBlocks:       false,
			PreservePostProcessing:    false,
			AddHTTPEntryParticipants:  true,
		}
	case FamilyConfigLookup:
		return Policy{
			Family:                    FamilyConfigLookup,
			PreferredCandidateKinds:   []string{"compact_review", "faithful", "async_summarized"},
			MaxVisibleParticipants:    6,
			MinBusinessExpansionDepth: 0,
			MaxBusinessExpansionDepth: 1,
			PreserveBranchBlocks:      false,
			PreserveAsyncBlocks:       false,
			PreservePostProcessing:    false,
			AddHTTPEntryParticipants:  true,
		}
	case FamilySimpleQuery:
		return Policy{
			Family:                    FamilySimpleQuery,
			PreferredCandidateKinds:   []string{"compact_review", "faithful", "async_summarized"},
			MaxVisibleParticipants:    6,
			MinBusinessExpansionDepth: 0,
			MaxBusinessExpansionDepth: 1,
			PreserveBranchBlocks:      false,
			PreserveAsyncBlocks:       false,
			PreservePostProcessing:    false,
			AddHTTPEntryParticipants:  true,
		}
	case FamilyGatewayProxy:
		return Policy{
			Family:                    FamilyGatewayProxy,
			PreferredCandidateKinds:   []string{"compact_review", "faithful", "async_summarized"},
			MaxVisibleParticipants:    7,
			MinBusinessExpansionDepth: 0,
			MaxBusinessExpansionDepth: 1,
			PreserveBranchBlocks:      false,
			PreserveAsyncBlocks:       false,
			PreservePostProcessing:    false,
			AddHTTPEntryParticipants:  true,
		}
	default:
		return Policy{
			Family:                    FamilyDefault,
			PreferredCandidateKinds:   []string{"compact_review", "async_summarized", "faithful"},
			MaxVisibleParticipants:    7,
			MinBusinessExpansionDepth: 0,
			MaxBusinessExpansionDepth: 1,
			PreserveBranchBlocks:      false,
			PreserveAsyncBlocks:       false,
			PreservePostProcessing:    false,
			AddHTTPEntryParticipants:  true,
		}
	}
}

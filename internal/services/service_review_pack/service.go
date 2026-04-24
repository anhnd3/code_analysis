package service_review_pack

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/reviewpack"
)

type Service struct{}

func New() Service {
	return Service{}
}

type BuildInput struct {
	ServiceName        string
	ExpectedRoots      []reviewpack.ExpectedRoot
	ResolvedRoots      []entrypoint.Root
	DetectedBoundaries []boundaryroot.Root
	Outcomes           map[string]RenderOutcome
}

type RenderOutcome struct {
	ExpectedRootID    string
	RootNodeID        string
	CanonicalName     string
	Status            reviewpack.CoverageStatus
	Reason            reviewpack.DeterministicReason
	FailureStage      reviewpack.FailureStage
	RenderSource      reviewpack.RenderSource
	ArtifactSlug      string
	Family            string
	PolicySource      reviewpack.PolicySource
	MermaidPath       string
	ReviewFlowPath    string
	SequenceModelPath string
}

func (s Service) LoadExpectedRoots(path string) ([]reviewpack.ExpectedRoot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var roots []reviewpack.ExpectedRoot
	if err := json.Unmarshal(data, &roots); err != nil {
		return nil, err
	}
	for i := range roots {
		roots[i].Method = strings.ToUpper(strings.TrimSpace(roots[i].Method))
		roots[i].Path = strings.TrimSpace(roots[i].Path)
		roots[i].RootType = strings.TrimSpace(roots[i].RootType)
		roots[i].Family = strings.TrimSpace(roots[i].Family)
	}
	return roots, nil
}

func (s Service) Build(input BuildInput) (reviewpack.ServiceReviewPack, error) {
	coverage := make([]reviewpack.CoverageItem, 0, len(input.ExpectedRoots))
	selected := make([]reviewpack.SelectedFlow, 0, len(input.ExpectedRoots))

	for _, expected := range input.ExpectedRoots {
		item := reviewpack.CoverageItem{
			ExpectedRootID:   expected.ID,
			Method:           expected.Method,
			Path:             expected.Path,
			RootType:         expected.RootType,
			Family:           expected.Family,
			Required:         expected.Required,
			RequiredBlocking: expected.Required,
		}

		resolvedRoot, hasResolved := matchResolved(expected, input.ResolvedRoots)
		if !hasResolved {
			if hasDetectedBoundary(expected, input.DetectedBoundaries) {
				item.Status = reviewpack.CoverageMissing
				item.Reason = reviewpack.ReasonRootNotResolved
				item.FailureStage = reviewpack.FailureStageResolution
			} else {
				item.Status = reviewpack.CoverageMissing
				item.Reason = reviewpack.ReasonRootNotDetected
				item.FailureStage = reviewpack.FailureStageDetection
			}
			if !expected.Required {
				item.RequiredBlocking = false
				if item.Reason == "" {
					item.Reason = reviewpack.ReasonOptionalRootNotRendered
				}
			}
			coverage = append(coverage, item)
			continue
		}

		item.RootNodeID = resolvedRoot.NodeID

		outcome, ok := input.Outcomes[expected.ID]
		if !ok {
			item.Status = reviewpack.CoverageSkipped
			item.Reason = reviewpack.ReasonReviewRenderFailed
			item.FailureStage = reviewpack.FailureStageRendering
			if !expected.Required {
				item.RequiredBlocking = false
			}
			coverage = append(coverage, item)
			continue
		}

		item.Status = outcome.Status
		item.Reason = outcome.Reason
		item.FailureStage = outcome.FailureStage
		item.RenderSource = outcome.RenderSource
		item.ArtifactSlug = outcome.ArtifactSlug
		if item.Status == reviewpack.CoverageRendered {
			item.Reason = ""
			item.FailureStage = ""
		} else if item.FailureStage == "" {
			item.FailureStage = reviewpack.FailureStageRendering
		}
		if !expected.Required {
			item.RequiredBlocking = false
			if item.Status != reviewpack.CoverageRendered && item.Reason == "" {
				item.Reason = reviewpack.ReasonOptionalRootNotRendered
			}
		}

		coverage = append(coverage, item)
		if item.Status == reviewpack.CoverageRendered {
			selected = append(selected, reviewpack.SelectedFlow{
				ExpectedRootID:    expected.ID,
				RootNodeID:        outcome.RootNodeID,
				CanonicalName:     outcome.CanonicalName,
				Family:            firstNonEmpty(outcome.Family, expected.Family),
				PolicySource:      policySourceDefault(outcome.PolicySource, expected.Family),
				RenderSource:      outcome.RenderSource,
				ArtifactSlug:      outcome.ArtifactSlug,
				MermaidPath:       outcome.MermaidPath,
				ReviewFlowPath:    outcome.ReviewFlowPath,
				SequenceModelPath: outcome.SequenceModelPath,
			})
		}
	}

	sort.SliceStable(selected, func(i, j int) bool {
		return selected[i].ExpectedRootID < selected[j].ExpectedRootID
	})

	return reviewpack.ServiceReviewPack{
		ServiceName:   input.ServiceName,
		ExpectedRoots: append([]reviewpack.ExpectedRoot(nil), input.ExpectedRoots...),
		Coverage:      coverage,
		SelectedFlows: selected,
	}, nil
}

func Markdown(pack reviewpack.ServiceReviewPack) string {
	var b strings.Builder
	b.WriteString(CoverageMarkdown(pack))
	fmt.Fprintf(&b, "\n# Selected Flows\n\n")
	if len(pack.SelectedFlows) == 0 {
		fmt.Fprintf(&b, "No rendered selected flows.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "| expected_root_id | canonical_name | policy_source | render_source | mermaid_path |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- |\n")
	for _, selected := range pack.SelectedFlows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			selected.ExpectedRootID,
			selected.CanonicalName,
			selected.PolicySource,
			selected.RenderSource,
			selected.MermaidPath,
		)
	}
	return b.String()
}

func CoverageMarkdown(pack reviewpack.ServiceReviewPack) string {
	var b strings.Builder
	total := len(pack.Coverage)
	rendered := 0
	requiredBlocking := 0
	blockingFailed := 0
	for _, item := range pack.Coverage {
		if item.Status == reviewpack.CoverageRendered {
			rendered++
		}
		if item.RequiredBlocking {
			requiredBlocking++
			if item.Status != reviewpack.CoverageRendered {
				blockingFailed++
			}
		}
	}

	fmt.Fprintf(&b, "# Service Coverage Report\n\n")
	fmt.Fprintf(&b, "- Service: `%s`\n", pack.ServiceName)
	fmt.Fprintf(&b, "- Expected roots: `%d`\n", total)
	fmt.Fprintf(&b, "- Rendered: `%d`\n", rendered)
	fmt.Fprintf(&b, "- Blocking required roots: `%d`\n", requiredBlocking)
	fmt.Fprintf(&b, "- Blocking required roots not rendered: `%d`\n\n", blockingFailed)
	fmt.Fprintf(&b, "| expected_root_id | status | required_blocking | reason | failure_stage | render_source |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- |\n")
	for _, item := range pack.Coverage {
		fmt.Fprintf(&b, "| %s | %s | %t | %s | %s | %s |\n",
			item.ExpectedRootID,
			item.Status,
			item.RequiredBlocking,
			item.Reason,
			item.FailureStage,
			item.RenderSource,
		)
	}
	return b.String()
}

func matchResolved(expected reviewpack.ExpectedRoot, resolved []entrypoint.Root) (entrypoint.Root, bool) {
	rootType := strings.TrimSpace(expected.RootType)
	method := strings.ToUpper(strings.TrimSpace(expected.Method))
	path := strings.TrimSpace(expected.Path)

	if rootType == string(entrypoint.RootBootstrap) {
		bootstrap := make([]entrypoint.Root, 0, len(resolved))
		for _, root := range resolved {
			if string(root.RootType) == rootType {
				bootstrap = append(bootstrap, root)
			}
		}
		if len(bootstrap) == 0 {
			return entrypoint.Root{}, false
		}
		sort.SliceStable(bootstrap, func(i, j int) bool {
			return bootstrap[i].CanonicalName < bootstrap[j].CanonicalName
		})
		return bootstrap[0], true
	}

	for _, root := range resolved {
		if string(root.RootType) != rootType {
			continue
		}
		if strings.EqualFold(root.Method, method) && root.Path == path {
			return root, true
		}
	}
	return entrypoint.Root{}, false
}

func hasDetectedBoundary(expected reviewpack.ExpectedRoot, detected []boundaryroot.Root) bool {
	wantKind := strings.TrimSpace(expected.RootType)
	wantMethod := strings.ToUpper(strings.TrimSpace(expected.Method))
	wantPath := strings.TrimSpace(expected.Path)

	for _, root := range detected {
		if string(root.Kind) != wantKind {
			continue
		}
		if wantKind == string(entrypoint.RootBootstrap) {
			return true
		}
		if strings.EqualFold(root.Method, wantMethod) && root.Path == wantPath {
			return true
		}
	}
	return false
}

func firstNonEmpty(parts ...string) string {
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			return part
		}
	}
	return ""
}

func policySourceDefault(source reviewpack.PolicySource, family string) reviewpack.PolicySource {
	if source != "" {
		return source
	}
	if strings.TrimSpace(family) != "" {
		return reviewpack.PolicySourceManifest
	}
	return reviewpack.PolicySourceDefault
}

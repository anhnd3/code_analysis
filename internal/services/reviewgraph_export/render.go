package reviewgraph_export

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/reviewgraph"
)

func renderFlowMarkdown(index int, target reviewgraph.ResolvedTarget, node reviewgraph.Node, result reviewgraph.TraversalResult, diagnostics []reviewgraph.ImportDiagnostic) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Impact Review\n")
	fmt.Fprintf(builder, "Target: `%s`\n", target.DisplayName)
	fmt.Fprintf(builder, "Reason: `%s`\n", target.Reason)
	fmt.Fprintf(builder, "Flow Scope: `%s`\n\n", result.Mode)

	builder.WriteString("## 1. Synchronous Flow\n")
	writePaths(builder, "Upstream", result.SyncUpstreamPaths)
	writePaths(builder, "Downstream", result.SyncDownstreamPaths)
	builder.WriteString("\n## 2. Asynchronous Flow\n")
	if len(result.AsyncBridges) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, bridge := range result.AsyncBridges {
			fmt.Fprintf(builder, "- `%s` (`%s`) producers=%d consumers=%d\n", bridge.BridgeDisplayName, bridge.BridgeKind, bridge.ProducerCount, bridge.ConsumerCount)
			if len(bridge.Producers) > 0 {
				builder.WriteString("  Producers:\n")
				for _, producer := range groupParticipants(bridge.Producers) {
					fmt.Fprintf(builder, "  - %s\n", producer)
				}
			}
			if len(bridge.Consumers) > 0 {
				builder.WriteString("  Consumers:\n")
				for _, consumer := range groupParticipants(bridge.Consumers) {
					fmt.Fprintf(builder, "  - %s\n", consumer)
				}
			}
		}
	}

	builder.WriteString("\n## 3. Affected Files\n")
	if len(result.AffectedFiles) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, file := range result.AffectedFiles {
			fmt.Fprintf(builder, "- `%s`\n", file)
		}
	}

	builder.WriteString("\n## 4. Cross-Service Risk\n")
	if len(result.CrossServices) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, service := range result.CrossServices {
			fmt.Fprintf(builder, "- `%s`\n", service)
		}
	}

	builder.WriteString("\n## 5. Ambiguities\n")
	if len(diagnostics) == 0 && len(result.Ambiguities) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, ambiguity := range result.Ambiguities {
			fmt.Fprintf(builder, "- %s\n", ambiguity)
		}
		for _, diagnostic := range diagnostics {
			if diagnostic.Line > 0 {
				fmt.Fprintf(builder, "- `%s:%d` %s\n", diagnostic.FilePath, diagnostic.Line, diagnostic.Message)
			} else {
				fmt.Fprintf(builder, "- `%s` %s\n", diagnostic.FilePath, diagnostic.Message)
			}
		}
	}

	builder.WriteString("\n## 6. Flow Coverage\n")
	fmt.Fprintf(builder, "- nodes in slice: %d\n", result.Coverage.CoveredNodeCount)
	fmt.Fprintf(builder, "- edges in slice: %d\n", result.Coverage.CoveredEdgeCount)
	fmt.Fprintf(builder, "- shared infra touched: %d\n", result.Coverage.SharedInfraCount)
	if len(result.Cycles) > 0 {
		builder.WriteString("- cycles:\n")
		for _, cycle := range result.Cycles {
			fmt.Fprintf(builder, "  - `%s`\n", strings.Join(cycle.Path, " -> "))
		}
	}
	if len(result.TruncationWarnings) > 0 {
		builder.WriteString("- truncation warnings:\n")
		for _, warning := range result.TruncationWarnings {
			fmt.Fprintf(builder, "  - %s\n", warning)
		}
	}
	_ = index
	_ = node
	return builder.String()
}

func renderIndexMarkdown(snapshotID string, nodes []reviewgraph.Node, edges []reviewgraph.Edge, flowPaths []string, targetCount int, coveredNodes, coveredEdges map[string]struct{}, residualGroups []residualGroup, manifest reviewgraph.ImportManifest, report quality.AnalysisQualityReport) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Review Index\n")
	fmt.Fprintf(builder, "- snapshot: `%s`\n", snapshotID)
	fmt.Fprintf(builder, "- total nodes: %d\n", len(nodes))
	fmt.Fprintf(builder, "- total edges: %d\n", len(edges))
	fmt.Fprintf(builder, "- starting points: %d\n", targetCount)
	fmt.Fprintf(builder, "- nodes covered by at least one flow: %d\n", len(coveredNodes))
	fmt.Fprintf(builder, "- edges covered by at least one flow: %d\n", len(coveredEdges))
	fmt.Fprintf(builder, "- shared infra nodes: %d\n", countSharedInfra(nodes))
	fmt.Fprintf(builder, "- residual groups: %d\n", len(residualGroups))
	fmt.Fprintf(builder, "- ambiguous/diagnostic entries: %d\n\n", len(manifest.Diagnostics)+len(report.Gaps))

	builder.WriteString("## Flow Files\n")
	for _, path := range flowPaths {
		fmt.Fprintf(builder, "- `%s`\n", filepath.Base(path))
	}
	return builder.String()
}

func renderResidualMarkdown(groups []residualGroup) string {
	builder := &strings.Builder{}
	builder.WriteString("# Orphans And Residuals\n")
	if len(groups) == 0 {
		builder.WriteString("- none\n")
		return builder.String()
	}
	for _, group := range groups {
		fmt.Fprintf(builder, "\n## %s\n", group.Header)
		fmt.Fprintf(builder, "- count: %d\n", group.Count)
		if len(group.SampleNodes) == 0 {
			continue
		}
		builder.WriteString("- sample nodes:\n")
		for _, node := range group.SampleNodes {
			fmt.Fprintf(builder, "  - `%s` (%s)\n", node.Symbol, node.FilePath)
		}
	}
	return builder.String()
}

func renderDiagnosticsMarkdown(manifest reviewgraph.ImportManifest, report quality.AnalysisQualityReport, coveredNodeCount, coveredEdgeCount int) string {
	builder := &strings.Builder{}
	builder.WriteString("# Diagnostics\n")
	builder.WriteString("## Quality Gaps\n")
	if len(report.Gaps) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, gap := range report.Gaps {
			fmt.Fprintf(builder, "- `%s`: %s (%d)\n", gap.Category, gap.Message, gap.Count)
		}
	}

	builder.WriteString("\n## Import Diagnostics\n")
	if len(manifest.Diagnostics) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, diagnostic := range manifest.Diagnostics {
			if diagnostic.Line > 0 {
				fmt.Fprintf(builder, "- `%s:%d` %s\n", diagnostic.FilePath, diagnostic.Line, diagnostic.Message)
			} else {
				fmt.Fprintf(builder, "- `%s` %s\n", diagnostic.FilePath, diagnostic.Message)
			}
		}
	}

	builder.WriteString("\n## Dropped Content\n")
	fmt.Fprintf(builder, "- dropped test nodes: %d\n", manifest.Counts.DroppedTestNodeCount)
	fmt.Fprintf(builder, "- dropped test edges: %d\n", manifest.Counts.DroppedTestEdgeCount)
	fmt.Fprintf(builder, "- dropped ignored nodes: %d\n", manifest.Counts.DroppedIgnoredNodes)
	fmt.Fprintf(builder, "- dropped ignored edges: %d\n", manifest.Counts.DroppedIgnoredEdges)
	fmt.Fprintf(builder, "- dropped generated nodes: %d\n", manifest.Counts.DroppedGeneratedNodes)
	fmt.Fprintf(builder, "- dropped generated edges: %d\n", manifest.Counts.DroppedGeneratedEdges)
	fmt.Fprintf(builder, "- weak async matches: %d\n", manifest.Counts.WeakAsyncMatches)
	fmt.Fprintf(builder, "- contextual boundary edges: %d\n", manifest.Counts.ContextualBoundaryEdges)

	builder.WriteString("\n## Coverage Summary\n")
	fmt.Fprintf(builder, "- covered nodes: %d\n", coveredNodeCount)
	fmt.Fprintf(builder, "- covered edges: %d\n", coveredEdgeCount)
	return builder.String()
}

func writePaths(builder *strings.Builder, label string, paths []reviewgraph.PathSummary) {
	fmt.Fprintf(builder, "### %s\n", label)
	if len(paths) == 0 {
		builder.WriteString("- none\n")
		return
	}
	for _, path := range paths {
		fmt.Fprintf(builder, "- `%s` (%s)\n", strings.Join(path.NodeIDs, " -> "), path.TerminalReason)
	}
}

func groupParticipants(participants []reviewgraph.AsyncParticipant) []string {
	grouped := map[string][]string{}
	for _, participant := range participants {
		service := participant.Service
		if service == "" {
			service = "unknown_service"
		}
		grouped[service] = append(grouped[service], participant.DisplayName)
	}
	services := make([]string, 0, len(grouped))
	for service := range grouped {
		services = append(services, service)
	}
	sort.Strings(services)
	result := make([]string, 0, len(services))
	for _, service := range services {
		names := grouped[service]
		sort.Strings(names)
		if len(names) > 3 {
			names = append(names[:3], fmt.Sprintf("... +%d more", len(grouped[service])-3))
		}
		result = append(result, fmt.Sprintf("%s: %s", service, strings.Join(names, ", ")))
	}
	return result
}

func countSharedInfra(nodes []reviewgraph.Node) int {
	count := 0
	for _, node := range nodes {
		if node.NodeRole == reviewgraph.RoleSharedInfra {
			count++
		}
	}
	return count
}

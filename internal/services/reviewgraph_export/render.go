package reviewgraph_export

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/reviewgraph"
)

type treeStep struct {
	Key   string
	Label string
}

type groupedTreeNode struct {
	Key      string
	Label    string
	Children map[string]*groupedTreeNode
	Markers  map[string]struct{}
}

type renderContext struct {
	fileLabel  string
	classLabel string
}

func renderFlowMarkdown(index int, target reviewgraph.ResolvedTarget, node reviewgraph.Node, result reviewgraph.TraversalResult, diagnostics []reviewgraph.ImportDiagnostic, nodeByID map[string]reviewgraph.Node, renderMode string) string {
	if renderMode == "raw" {
		return renderFlowMarkdownRaw(index, target, node, result, diagnostics)
	}
	return renderFlowMarkdownGrouped(index, target, node, result, diagnostics, nodeByID)
}

func renderFlowMarkdownGrouped(index int, target reviewgraph.ResolvedTarget, node reviewgraph.Node, result reviewgraph.TraversalResult, diagnostics []reviewgraph.ImportDiagnostic, nodeByID map[string]reviewgraph.Node) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Impact Review\n")
	fmt.Fprintf(builder, "Target: `%s`\n", target.DisplayName)
	fmt.Fprintf(builder, "Reason: `%s`\n", target.Reason)
	fmt.Fprintf(builder, "Flow Scope: `%s`\n\n", result.Mode)

	builder.WriteString("## 1. Synchronous Flow\n")
	writeGroupedPaths(builder, "Upstream", result.SyncUpstreamPaths, nodeByID)
	writeGroupedPaths(builder, "Downstream", result.SyncDownstreamPaths, nodeByID)

	builder.WriteString("\n## 2. Asynchronous Flow\n")
	writeGroupedAsync(builder, result.AsyncBridges, nodeByID)

	builder.WriteString("\n## 3. Affected Files\n")
	writeAffectedFiles(builder, result.AffectedFiles)

	builder.WriteString("\n## 4. Cross-Service Risk\n")
	writeCrossServices(builder, result.CrossServices)

	builder.WriteString("\n## 5. Ambiguities\n")
	writeAmbiguities(builder, result.Ambiguities, diagnostics)

	builder.WriteString("\n## 6. Flow Coverage\n")
	writeCoverage(builder, result)

	_ = index
	_ = node
	return builder.String()
}

func renderFlowMarkdownRaw(index int, target reviewgraph.ResolvedTarget, node reviewgraph.Node, result reviewgraph.TraversalResult, diagnostics []reviewgraph.ImportDiagnostic) string {
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
	writeAffectedFiles(builder, result.AffectedFiles)

	builder.WriteString("\n## 4. Cross-Service Risk\n")
	writeCrossServices(builder, result.CrossServices)

	builder.WriteString("\n## 5. Ambiguities\n")
	writeAmbiguities(builder, result.Ambiguities, diagnostics)

	builder.WriteString("\n## 6. Flow Coverage\n")
	writeCoverage(builder, result)

	_ = index
	_ = node
	return builder.String()
}

func writeGroupedPaths(builder *strings.Builder, label string, paths []reviewgraph.PathSummary, nodeByID map[string]reviewgraph.Node) {
	fmt.Fprintf(builder, "### %s\n", label)
	if len(paths) == 0 {
		builder.WriteString("- none\n")
		return
	}
	root := buildGroupedTree(paths, nodeByID)
	writeGroupedTree(builder, root, 0)
}

func writeGroupedAsync(builder *strings.Builder, bridges []reviewgraph.AsyncBridgeSummary, nodeByID map[string]reviewgraph.Node) {
	if len(bridges) == 0 {
		builder.WriteString("- none\n")
		return
	}
	for _, bridge := range bridges {
		transport := bridge.Transport
		if transport == "" {
			transport = "unknown"
		}
		fmt.Fprintf(builder, "- `%s` (`%s`, transport=`%s`) producers=%d consumers=%d\n", bridge.BridgeDisplayName, bridge.BridgeKind, transport, bridge.ProducerCount, bridge.ConsumerCount)
		if len(bridge.Producers) > 0 {
			builder.WriteString("  Producers:\n")
			root := buildParticipantTree(bridge.Producers, nodeByID)
			writeGroupedTree(builder, root, 1)
			if len(bridge.UpstreamSyncPaths) > 0 {
				builder.WriteString("  Producer Sync Context:\n")
				root := buildGroupedTree(bridge.UpstreamSyncPaths, nodeByID)
				writeGroupedTree(builder, root, 1)
			}
		}
		if len(bridge.Consumers) > 0 {
			builder.WriteString("  Consumers:\n")
			root := buildParticipantTree(bridge.Consumers, nodeByID)
			writeGroupedTree(builder, root, 1)
			if len(bridge.DownstreamSyncPaths) > 0 {
				builder.WriteString("  Consumer Sync Context:\n")
				root := buildGroupedTree(bridge.DownstreamSyncPaths, nodeByID)
				writeGroupedTree(builder, root, 1)
			}
		}
		if bridge.FanoutTruncated {
			builder.WriteString("  - `fanout truncated`\n")
		}
	}
}

func writeAffectedFiles(builder *strings.Builder, files []string) {
	if len(files) == 0 {
		builder.WriteString("- none\n")
		return
	}
	for _, file := range files {
		fmt.Fprintf(builder, "- `%s`\n", file)
	}
}

func writeCrossServices(builder *strings.Builder, services []string) {
	if len(services) == 0 {
		builder.WriteString("- none\n")
		return
	}
	for _, service := range services {
		fmt.Fprintf(builder, "- `%s`\n", service)
	}
}

func writeAmbiguities(builder *strings.Builder, ambiguities []string, diagnostics []reviewgraph.ImportDiagnostic) {
	if len(diagnostics) == 0 && len(ambiguities) == 0 {
		builder.WriteString("- none\n")
		return
	}
	for _, ambiguity := range ambiguities {
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

func writeCoverage(builder *strings.Builder, result reviewgraph.TraversalResult) {
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
}

func buildGroupedTree(paths []reviewgraph.PathSummary, nodeByID map[string]reviewgraph.Node) *groupedTreeNode {
	root := &groupedTreeNode{Children: map[string]*groupedTreeNode{}, Markers: map[string]struct{}{}}
	for _, path := range paths {
		insertPath(root, path, nodeByID)
	}
	return root
}

func buildParticipantTree(participants []reviewgraph.AsyncParticipant, nodeByID map[string]reviewgraph.Node) *groupedTreeNode {
	root := &groupedTreeNode{Children: map[string]*groupedTreeNode{}, Markers: map[string]struct{}{}}
	for _, participant := range participants {
		insertParticipant(root, participant, nodeByID)
	}
	return root
}

func insertPath(root *groupedTreeNode, path reviewgraph.PathSummary, nodeByID map[string]reviewgraph.Node) {
	current := root
	ctx := renderContext{}
	for _, nodeID := range path.NodeIDs {
		node, ok := nodeByID[nodeID]
		steps, next := stepsForNode(nodeID, node, ok, ctx, "")
		ctx = next
		for _, step := range steps {
			current = ensureChild(current, step.Key, step.Label)
		}
	}
	addPathMarkers(current, path.TerminalReason, path.Truncated)
}

func insertParticipant(root *groupedTreeNode, participant reviewgraph.AsyncParticipant, nodeByID map[string]reviewgraph.Node) {
	current := root
	node, ok := nodeByID[participant.NodeID]
	steps, _ := stepsForNode(participant.NodeID, node, ok, renderContext{}, participant.DisplayName)
	for _, step := range steps {
		current = ensureChild(current, step.Key, step.Label)
	}
}

func ensureChild(parent *groupedTreeNode, key, label string) *groupedTreeNode {
	if parent.Children == nil {
		parent.Children = map[string]*groupedTreeNode{}
	}
	if child, ok := parent.Children[key]; ok {
		return child
	}
	child := &groupedTreeNode{
		Key:      key,
		Label:    label,
		Children: map[string]*groupedTreeNode{},
		Markers:  map[string]struct{}{},
	}
	parent.Children[key] = child
	return child
}

func addPathMarkers(node *groupedTreeNode, terminalReason string, truncated bool) {
	if node == nil {
		return
	}
	if strings.TrimSpace(terminalReason) != "" {
		node.Markers[terminalReason] = struct{}{}
	}
	if truncated {
		node.Markers["truncated"] = struct{}{}
	}
}

func stepsForNode(nodeID string, node reviewgraph.Node, found bool, ctx renderContext, fallbackLabel string) ([]treeStep, renderContext) {
	if !found {
		label := firstNonEmpty(fallbackLabel, nodeID)
		return []treeStep{{Key: "external:" + nodeID, Label: label}}, renderContext{}
	}
	if isStandaloneNode(node) {
		return []treeStep{{Key: "external:" + nodeID, Label: standaloneNodeLabel(node, fallbackLabel)}}, renderContext{}
	}
	if node.Kind == reviewgraph.NodeFile {
		label := firstNonEmpty(filepath.ToSlash(node.Symbol), filepath.ToSlash(node.FilePath), node.ID)
		return []treeStep{{Key: "file:" + label, Label: label}}, renderContext{fileLabel: label}
	}

	fileLabel := filepath.ToSlash(firstNonEmpty(node.FilePath, node.Symbol))
	steps := []treeStep{}
	if fileLabel != "" && fileLabel != ctx.fileLabel {
		steps = append(steps, treeStep{Key: "file:" + fileLabel, Label: fileLabel})
		ctx.fileLabel = fileLabel
		ctx.classLabel = ""
	}
	classLabel := ownerLabel(node)
	if classLabel != "" {
		if classLabel != ctx.classLabel {
			steps = append(steps, treeStep{Key: "class:" + fileLabel + ":" + classLabel, Label: classLabel})
			ctx.classLabel = classLabel
		}
	} else {
		ctx.classLabel = ""
	}
	steps = append(steps, treeStep{Key: "node:" + nodeID, Label: callableNodeLabel(node, fallbackLabel)})
	return steps, ctx
}

func isStandaloneNode(node reviewgraph.Node) bool {
	switch node.Kind {
	case reviewgraph.NodeService, reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
		return true
	default:
		return node.FilePath == ""
	}
}

func standaloneNodeLabel(node reviewgraph.Node, fallbackLabel string) string {
	base := firstNonEmpty(node.Symbol, fallbackLabel, node.ID)
	switch node.Kind {
	case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
		return fmt.Sprintf("%s [%s]", base, node.Kind)
	default:
		return base
	}
}

func callableNodeLabel(node reviewgraph.Node, fallbackLabel string) string {
	label := firstNonEmpty(node.Symbol, fallbackLabel, node.ID)
	switch node.Kind {
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
		parts := strings.Split(label, ".")
		if len(parts) > 0 {
			label = parts[len(parts)-1]
		}
	}
	return label
}

func ownerLabel(node reviewgraph.Node) string {
	if node.Kind != reviewgraph.NodeMethod {
		return ""
	}
	parts := strings.Split(node.Symbol, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func writeGroupedTree(builder *strings.Builder, root *groupedTreeNode, depth int) {
	children := sortedChildren(root)
	for _, child := range children {
		indent := strings.Repeat("  ", depth)
		markers := markerSuffix(child.Markers)
		fmt.Fprintf(builder, "%s- `%s`%s\n", indent, child.Label, markers)
		writeGroupedTree(builder, child, depth+1)
	}
}

func markerSuffix(markers map[string]struct{}) string {
	if len(markers) == 0 {
		return ""
	}
	values := make([]string, 0, len(markers))
	for marker := range markers {
		values = append(values, marker)
	}
	sort.Strings(values)
	return fmt.Sprintf(" (%s)", strings.Join(values, ", "))
}

func sortedChildren(root *groupedTreeNode) []*groupedTreeNode {
	children := make([]*groupedTreeNode, 0, len(root.Children))
	for _, child := range root.Children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Label != children[j].Label {
			return children[i].Label < children[j].Label
		}
		return children[i].Key < children[j].Key
	})
	return children
}

func renderIndexMarkdown(snapshotID, reviewDir, currentPath, threadsIndexPath string, nodes []reviewgraph.Node, edges []reviewgraph.Edge, flowPaths []string, targetCount int, coveredNodes, coveredEdges map[string]struct{}, residualGroups []residualGroup, manifest reviewgraph.ImportManifest, report quality.AnalysisQualityReport) string {
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

	if strings.TrimSpace(threadsIndexPath) != "" {
		fmt.Fprintf(builder, "## Companion Thread Views\n")
		fmt.Fprintf(builder, "- [threads/00_index.md](%s)\n\n", relativeLink(currentPath, threadsIndexPath))
	}

	builder.WriteString("## Flow Files\n")
	for _, path := range flowPaths {
		fmt.Fprintf(builder, "- [%s](%s)\n", filepath.Base(path), relativeLink(currentPath, path))
	}
	_ = reviewDir
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

package reviewgraph_export

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/services/reviewgraph_traverse"
)

type overallTraversalSource struct {
	Target     reviewgraph.ResolvedTarget
	TargetNode reviewgraph.Node
	Result     reviewgraph.TraversalResult
}

type overallPathStep struct {
	Key       string
	Label     string
	Connector string
}

type overallFlowTreeNode struct {
	Key       string
	Label     string
	Connector string
	Children  map[string]*overallFlowTreeNode
	Markers   map[string]struct{}
}

func renderOverallSyncMarkdown(
	sources []overallTraversalSource,
	graph reviewgraph_traverse.GraphData,
	affectedFiles, crossServices []string,
	diagnostics []reviewgraph.ImportDiagnostic,
	summary reviewgraph.TraversalResult,
) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Synchronous System Flow\n")
	fmt.Fprintf(builder, "- selected roots: %d\n", len(sources))
	fmt.Fprintf(builder, "- stitched journeys: %d\n\n", len(sources))

	builder.WriteString("## Start Flow\n")
	if len(sources) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, group := range groupOverallSourcesByService(sources) {
			fmt.Fprintf(builder, "### %s\n", group.service)
			for _, source := range group.sources {
				builder.WriteString("```text\n")
				writeOverallTextTree(builder, buildOverallSyncTree(source, graph))
				builder.WriteString("```\n\n")
			}
		}
	}

	writeOverallDeepDive(builder, sources, affectedFiles, crossServices, diagnostics, summary)
	return builder.String()
}

func renderOverallAsyncMarkdown(
	sources []overallTraversalSource,
	graph reviewgraph_traverse.GraphData,
	affectedFiles, crossServices []string,
	diagnostics []reviewgraph.ImportDiagnostic,
	summary reviewgraph.TraversalResult,
) string {
	builder := &strings.Builder{}
	bridges := mergeAsyncBridges(sources)

	fmt.Fprintf(builder, "# Asynchronous System Flow\n")
	fmt.Fprintf(builder, "- selected roots: %d\n", len(sources))
	fmt.Fprintf(builder, "- async continuations: %d\n\n", len(bridges))

	builder.WriteString("## Start Flow\n")
	if len(bridges) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, bridge := range bridges {
			fmt.Fprintf(builder, "### %s\n", overallAsyncBridgeLabel(bridge, graph.NodeByID))
			builder.WriteString("```text\n")
			lines := append([]string{}, buildAsyncProducerLines(bridge, graph)...)
			lines = append(lines, buildAsyncConsumerLines(bridge, graph)...)
			lines = dedupeStringSlice(lines)
			if len(lines) == 0 {
				lines = append(lines, overallAsyncBridgeLabel(bridge, graph.NodeByID))
			}
			for _, line := range lines {
				fmt.Fprintf(builder, "%s\n", line)
			}
			builder.WriteString("```\n\n")
		}
	}

	writeOverallDeepDive(builder, sources, affectedFiles, crossServices, diagnostics, summary)
	return builder.String()
}

func renderOverallIndexMarkdown(
	snapshotID, currentPath string,
	nodes []reviewgraph.Node,
	edges []reviewgraph.Edge,
	flowPaths []string,
	targetCount int,
	coveredNodes, coveredEdges map[string]struct{},
	residualGroups []residualGroup,
	manifest reviewgraph.ImportManifest,
	report quality.AnalysisQualityReport,
) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Review Index\n")
	fmt.Fprintf(builder, "- snapshot: `%s`\n", snapshotID)
	fmt.Fprintf(builder, "- total nodes: %d\n", len(nodes))
	fmt.Fprintf(builder, "- total edges: %d\n", len(edges))
	fmt.Fprintf(builder, "- starting points: %d\n", targetCount)
	fmt.Fprintf(builder, "- nodes covered by default workflows: %d\n", len(coveredNodes))
	fmt.Fprintf(builder, "- edges covered by default workflows: %d\n", len(coveredEdges))
	fmt.Fprintf(builder, "- residual groups: %d\n", len(residualGroups))
	fmt.Fprintf(builder, "- import diagnostics: %d\n", len(manifest.Diagnostics))
	fmt.Fprintf(builder, "- quality gaps: %d\n\n", len(report.Gaps))

	builder.WriteString("## System Docs\n")
	if len(flowPaths) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, path := range flowPaths {
			fmt.Fprintf(builder, "- [%s](%s)\n", filepath.Base(path), relativeLink(currentPath, path))
		}
	}

	builder.WriteString("\n## Quality Gaps\n")
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
	fmt.Fprintf(builder, "- contextual boundary edges: %d\n", manifest.Counts.ContextualBoundaryEdges)

	builder.WriteString("\n## Residual Coverage\n")
	if len(residualGroups) == 0 {
		builder.WriteString("- none\n")
	} else {
		limit := len(residualGroups)
		if limit > 10 {
			limit = 10
		}
		for _, group := range residualGroups[:limit] {
			fmt.Fprintf(builder, "- `%s` (%d)\n", group.Header, group.Count)
		}
		if len(residualGroups) > limit {
			fmt.Fprintf(builder, "- ... +%d more groups\n", len(residualGroups)-limit)
		}
	}

	return builder.String()
}

func writeOverallDeepDive(
	builder *strings.Builder,
	sources []overallTraversalSource,
	affectedFiles, crossServices []string,
	diagnostics []reviewgraph.ImportDiagnostic,
	summary reviewgraph.TraversalResult,
) {
	builder.WriteString("## Deep Dive\n")
	builder.WriteString("### Selected Roots\n")
	if len(sources) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, source := range sortOverallSources(append([]overallTraversalSource(nil), sources...)) {
			service := overallSourceService(source)
			fmt.Fprintf(builder, "- `%s` (%s, reason=`%s`)\n", source.Target.DisplayName, service, source.Target.Reason)
		}
	}

	builder.WriteString("\n### Affected Files\n")
	writeAffectedFiles(builder, affectedFiles)

	builder.WriteString("\n### Cross-Service Reach\n")
	writeCrossServices(builder, crossServices)

	builder.WriteString("\n### Ambiguities\n")
	writeAmbiguities(builder, summary.Ambiguities, diagnostics)

	builder.WriteString("\n### Coverage\n")
	writeCoverage(builder, summary)
}

func buildOverallSyncTree(source overallTraversalSource, graph reviewgraph_traverse.GraphData) *overallFlowTreeNode {
	root := newOverallFlowTreeNode("", "", "")
	if len(source.Result.SyncDownstreamPaths) == 0 {
		insertOverallPath(root, []overallPathStep{{
			Key:   source.Target.TargetNodeID,
			Label: overallNodeDisplay(source.TargetNode, true, source.Target.DisplayName),
		}}, "", false)
		return root
	}
	for _, path := range source.Result.SyncDownstreamPaths {
		steps := buildForwardPathSteps(path.NodeIDs, graph)
		if len(steps) == 0 {
			continue
		}
		insertOverallPath(root, steps, path.TerminalReason, path.Truncated)
	}
	return root
}

func buildForwardPathSteps(nodeIDs []string, graph reviewgraph_traverse.GraphData) []overallPathStep {
	steps := make([]overallPathStep, 0, len(nodeIDs))
	for index, nodeID := range nodeIDs {
		node, found := graph.NodeByID[nodeID]
		step := overallPathStep{
			Key:   nodeID,
			Label: overallNodeDisplay(node, found, nodeID),
		}
		if index > 0 {
			step.Key = syncConnectorForStep(graph, nodeIDs[index-1], nodeID) + "|" + nodeID
			step.Connector = syncConnectorForStep(graph, nodeIDs[index-1], nodeID)
		}
		steps = append(steps, step)
	}
	return steps
}

func insertOverallPath(root *overallFlowTreeNode, steps []overallPathStep, terminalReason string, truncated bool) {
	current := root
	for _, step := range steps {
		current = ensureOverallFlowChild(current, step)
	}
	addOverallPathMarkers(current, terminalReason, truncated)
}

func ensureOverallFlowChild(parent *overallFlowTreeNode, step overallPathStep) *overallFlowTreeNode {
	if parent.Children == nil {
		parent.Children = map[string]*overallFlowTreeNode{}
	}
	if child, ok := parent.Children[step.Key]; ok {
		return child
	}
	child := newOverallFlowTreeNode(step.Key, step.Label, step.Connector)
	parent.Children[step.Key] = child
	return child
}

func newOverallFlowTreeNode(key, label, connector string) *overallFlowTreeNode {
	return &overallFlowTreeNode{
		Key:       key,
		Label:     label,
		Connector: connector,
		Children:  map[string]*overallFlowTreeNode{},
		Markers:   map[string]struct{}{},
	}
}

func addOverallPathMarkers(node *overallFlowTreeNode, terminalReason string, truncated bool) {
	if node == nil {
		return
	}
	if marker := overallMarkerLabel(terminalReason); marker != "" {
		node.Markers[marker] = struct{}{}
	}
	if truncated {
		node.Markers["truncated"] = struct{}{}
	}
}

func overallMarkerLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case "", "leaf", "root", "normal":
		return ""
	case "depth_limit":
		return "depth limit"
	case "max_paths":
		return "max paths"
	case "max_nodes":
		return "max nodes"
	case "max_edges":
		return "max edges"
	case "shared_path":
		return "shared path"
	default:
		return strings.ReplaceAll(reason, "_", " ")
	}
}

func writeOverallTextTree(builder *strings.Builder, root *overallFlowTreeNode) {
	children := sortedOverallFlowChildren(root)
	if len(children) == 0 {
		builder.WriteString("(none)\n")
		return
	}
	for _, child := range children {
		writeOverallFlowBranch(builder, child, "", false)
	}
}

func writeOverallFlowBranch(builder *strings.Builder, start *overallFlowTreeNode, baseIndent string, continuation bool) {
	segment, tail := overallLinearSegment(start)
	
	lineBuf := &strings.Builder{}
	for index, node := range segment {
		if index == 0 {
			if continuation {
				fmt.Fprintf(lineBuf, "%s %s", node.Connector, node.Label)
			} else {
				lineBuf.WriteString(node.Label)
			}
			continue
		}
		fmt.Fprintf(lineBuf, " %s %s", node.Connector, node.Label)
	}
	
	segmentText := lineBuf.String()
	builder.WriteString(baseIndent)
	builder.WriteString(segmentText)
	builder.WriteString(markerSuffix(tail.Markers))
	builder.WriteString("\n")

	// Calculate the indent for children.
	// We want children to align with the start of the last node in the segment.
	// That means baseIndent + strings.Repeat(" ", len(segmentText) - len(lastNode.Connector) - len(lastNode.Label) - 2)
	// Wait, if we want them to align with " connector label", we pad the length of everything BEFORE the last connector.
	childIndent := baseIndent
	if len(segment) > 1 {
		lastNode := segment[len(segment)-1]
		// length before the last node:
		// total length - len(lastNode.Connector) - len(lastNode.Label) - 2 (for spaces)
		lenBeforeLast := len(segmentText) - len(lastNode.Connector) - len(lastNode.Label) - 2
		if lenBeforeLast > 0 {
			childIndent += strings.Repeat(" ", lenBeforeLast+1)
		} else {
		    childIndent += strings.Repeat(" ", len(segmentText))
		}
	} else {
		if continuation {
			// If it's a single node continuation, child aligns with the connector
			lenBeforeLast := len(segmentText) - len(start.Connector) - len(start.Label) - 1
			if lenBeforeLast > 0 {
				childIndent += strings.Repeat(" ", lenBeforeLast)
			} else {
				childIndent += strings.Repeat(" ", len(start.Label))
			}
		} else {
			childIndent += strings.Repeat(" ", len(start.Label)+1)
		}
	}

	for _, child := range sortedOverallFlowChildren(tail) {
		writeOverallFlowBranch(builder, child, childIndent, true)
	}
}

func overallLinearSegment(start *overallFlowTreeNode) ([]*overallFlowTreeNode, *overallFlowTreeNode) {
	segment := []*overallFlowTreeNode{start}
	current := start
	for len(current.Markers) == 0 {
		children := sortedOverallFlowChildren(current)
		if len(children) != 1 {
			break
		}
		current = children[0]
		segment = append(segment, current)
	}
	return segment, current
}

func sortedOverallFlowChildren(node *overallFlowTreeNode) []*overallFlowTreeNode {
	children := make([]*overallFlowTreeNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Label != children[j].Label {
			return children[i].Label < children[j].Label
		}
		if children[i].Connector != children[j].Connector {
			return children[i].Connector < children[j].Connector
		}
		return children[i].Key < children[j].Key
	})
	return children
}

func buildAsyncProducerLines(bridge reviewgraph.AsyncBridgeSummary, graph reviewgraph_traverse.GraphData) []string {
	bridgeLabel := overallAsyncBridgeLabel(bridge, graph.NodeByID)
	connector := asyncConnectorLabel(bridge.Transport, bridge.TopicOrChannel)
	lines := []string{}
	if len(bridge.UpstreamSyncPaths) > 0 {
		for _, path := range bridge.UpstreamSyncPaths {
			reversed := reverseNodeIDs(path.NodeIDs)
			line := formatPathStepsLine(buildForwardPathSteps(reversed, graph))
			if strings.TrimSpace(line) == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", line, connector, bridgeLabel))
		}
		return lines
	}
	for _, producer := range sortAsyncParticipants(bridge.Producers, graph.NodeByID) {
		lines = append(lines, fmt.Sprintf("%s %s %s", overallParticipantLabel(producer, graph.NodeByID), connector, bridgeLabel))
	}
	return lines
}

func buildAsyncConsumerLines(bridge reviewgraph.AsyncBridgeSummary, graph reviewgraph_traverse.GraphData) []string {
	bridgeLabel := overallAsyncBridgeLabel(bridge, graph.NodeByID)
	connector := asyncConnectorLabel(bridge.Transport, bridge.TopicOrChannel)
	lines := []string{}
	if len(bridge.DownstreamSyncPaths) > 0 {
		for _, path := range bridge.DownstreamSyncPaths {
			line := formatPathStepsLine(buildForwardPathSteps(path.NodeIDs, graph))
			if strings.TrimSpace(line) == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", bridgeLabel, connector, line))
		}
		return lines
	}
	for _, consumer := range sortAsyncParticipants(bridge.Consumers, graph.NodeByID) {
		lines = append(lines, fmt.Sprintf("%s %s %s", bridgeLabel, connector, overallParticipantLabel(consumer, graph.NodeByID)))
	}
	return lines
}

func formatPathStepsLine(steps []overallPathStep) string {
	if len(steps) == 0 {
		return ""
	}
	builder := &strings.Builder{}
	builder.WriteString(steps[0].Label)
	for index := 1; index < len(steps); index++ {
		fmt.Fprintf(builder, " %s %s", steps[index].Connector, steps[index].Label)
	}
	return builder.String()
}

func overallParticipantLabel(participant reviewgraph.AsyncParticipant, nodeByID map[string]reviewgraph.Node) string {
	node, found := nodeByID[participant.NodeID]
	return overallNodeDisplay(node, found, participant.DisplayName)
}

func overallAsyncBridgeLabel(bridge reviewgraph.AsyncBridgeSummary, nodeByID map[string]reviewgraph.Node) string {
	node, found := nodeByID[bridge.BridgeNodeID]
	if found {
		return overallNodeDisplay(node, true, bridge.BridgeDisplayName)
	}
	label := firstNonEmpty(bridge.TopicOrChannel, bridge.BridgeDisplayName, bridge.BridgeNodeID)
	if bridge.BridgeKind != "" {
		return fmt.Sprintf("%s [%s]", label, bridge.BridgeKind)
	}
	return label
}

func mergeAsyncBridges(sources []overallTraversalSource) []reviewgraph.AsyncBridgeSummary {
	merged := map[string]*reviewgraph.AsyncBridgeSummary{}
	for _, source := range sources {
		for _, bridge := range source.Result.AsyncBridges {
			existing, ok := merged[bridge.BridgeNodeID]
			if !ok {
				copyBridge := bridge
				copyBridge.Producers = append([]reviewgraph.AsyncParticipant(nil), bridge.Producers...)
				copyBridge.Consumers = append([]reviewgraph.AsyncParticipant(nil), bridge.Consumers...)
				copyBridge.UpstreamSyncPaths = append([]reviewgraph.PathSummary(nil), bridge.UpstreamSyncPaths...)
				copyBridge.DownstreamSyncPaths = append([]reviewgraph.PathSummary(nil), bridge.DownstreamSyncPaths...)
				merged[bridge.BridgeNodeID] = &copyBridge
				continue
			}
			existing.Producers = mergeAsyncParticipants(existing.Producers, bridge.Producers)
			existing.Consumers = mergeAsyncParticipants(existing.Consumers, bridge.Consumers)
			existing.UpstreamSyncPaths = mergePathSummaries(existing.UpstreamSyncPaths, bridge.UpstreamSyncPaths)
			existing.DownstreamSyncPaths = mergePathSummaries(existing.DownstreamSyncPaths, bridge.DownstreamSyncPaths)
			existing.FanoutTruncated = existing.FanoutTruncated || bridge.FanoutTruncated
			existing.Transport = firstNonEmpty(existing.Transport, bridge.Transport)
			existing.TopicOrChannel = firstNonEmpty(existing.TopicOrChannel, bridge.TopicOrChannel)
			existing.ProducerCount = len(existing.Producers)
			existing.ConsumerCount = len(existing.Consumers)
		}
	}
	result := make([]reviewgraph.AsyncBridgeSummary, 0, len(merged))
	for _, bridge := range merged {
		result = append(result, *bridge)
	}
	sort.Slice(result, func(i, j int) bool {
		left := firstNonEmpty(result[i].BridgeDisplayName, result[i].TopicOrChannel, result[i].BridgeNodeID)
		right := firstNonEmpty(result[j].BridgeDisplayName, result[j].TopicOrChannel, result[j].BridgeNodeID)
		if left != right {
			return left < right
		}
		return result[i].BridgeNodeID < result[j].BridgeNodeID
	})
	return result
}

func mergeAsyncParticipants(left, right []reviewgraph.AsyncParticipant) []reviewgraph.AsyncParticipant {
	seen := map[string]reviewgraph.AsyncParticipant{}
	for _, participant := range append(append([]reviewgraph.AsyncParticipant{}, left...), right...) {
		key := participant.NodeID
		if strings.TrimSpace(key) == "" {
			key = participant.DisplayName
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = participant
	}
	result := make([]reviewgraph.AsyncParticipant, 0, len(seen))
	for _, participant := range seen {
		result = append(result, participant)
	}
	return result
}

func mergePathSummaries(left, right []reviewgraph.PathSummary) []reviewgraph.PathSummary {
	seen := map[string]reviewgraph.PathSummary{}
	for _, path := range append(append([]reviewgraph.PathSummary{}, left...), right...) {
		key := strings.Join(path.NodeIDs, "->") + "|" + path.Direction + "|" + path.TerminalReason
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = path
	}
	result := make([]reviewgraph.PathSummary, 0, len(seen))
	for _, path := range seen {
		result = append(result, path)
	}
	sort.Slice(result, func(i, j int) bool {
		leftKey := strings.Join(result[i].NodeIDs, "->")
		rightKey := strings.Join(result[j].NodeIDs, "->")
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return result[i].TerminalReason < result[j].TerminalReason
	})
	return result
}

func sortAsyncParticipants(participants []reviewgraph.AsyncParticipant, nodeByID map[string]reviewgraph.Node) []reviewgraph.AsyncParticipant {
	sorted := append([]reviewgraph.AsyncParticipant(nil), participants...)
	sort.Slice(sorted, func(i, j int) bool {
		left := overallParticipantLabel(sorted[i], nodeByID)
		right := overallParticipantLabel(sorted[j], nodeByID)
		if left != right {
			return left < right
		}
		return sorted[i].NodeID < sorted[j].NodeID
	})
	return sorted
}

type overallSourceGroup struct {
	service string
	sources []overallTraversalSource
}

func groupOverallSourcesByService(sources []overallTraversalSource) []overallSourceGroup {
	grouped := map[string][]overallTraversalSource{}
	for _, source := range sortOverallSources(append([]overallTraversalSource(nil), sources...)) {
		service := overallSourceService(source)
		grouped[service] = append(grouped[service], source)
	}
	services := make([]string, 0, len(grouped))
	for service := range grouped {
		services = append(services, service)
	}
	sort.Strings(services)
	result := make([]overallSourceGroup, 0, len(services))
	for _, service := range services {
		result = append(result, overallSourceGroup{
			service: service,
			sources: grouped[service],
		})
	}
	return result
}

func sortOverallSources(sources []overallTraversalSource) []overallTraversalSource {
	sort.Slice(sources, func(i, j int) bool {
		leftService := overallSourceService(sources[i])
		rightService := overallSourceService(sources[j])
		if leftService != rightService {
			return leftService < rightService
		}
		if sources[i].Target.DisplayName != sources[j].Target.DisplayName {
			return sources[i].Target.DisplayName < sources[j].Target.DisplayName
		}
		return sources[i].Target.TargetNodeID < sources[j].Target.TargetNodeID
	})
	return sources
}

func overallSourceService(source overallTraversalSource) string {
	return firstNonEmpty(source.TargetNode.Service, source.TargetNode.Repo, "unknown_service")
}

func overallNodeDisplay(node reviewgraph.Node, found bool, fallback string) string {
	if !found {
		return firstNonEmpty(fallback, "(unknown)")
	}
	label := firstNonEmpty(node.Symbol, fallback, node.ID)
	switch node.Kind {
	case reviewgraph.NodeFunction, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
		label = overallCallableLabel(label, false)
	case reviewgraph.NodeMethod:
		label = overallCallableLabel(label, true)
	case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
		label = fmt.Sprintf("%s [%s]", label, node.Kind)
	case reviewgraph.NodeService:
		return firstNonEmpty(node.Service, label)
	}
	if service := strings.TrimSpace(node.Service); service != "" {
		return service + ": " + label
	}
	return label
}

func overallCallableLabel(symbol string, includeOwner bool) string {
	parts := strings.FieldsFunc(strings.TrimSpace(symbol), func(r rune) bool {
		return r == '.' || r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return symbol
	}
	if includeOwner && len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return parts[len(parts)-1]
}

func syncConnectorForStep(graph reviewgraph_traverse.GraphData, srcID, dstID string) string {
	srcNode := graph.NodeByID[srcID]
	dstNode := graph.NodeByID[dstID]
	for _, edge := range graph.Outgoing[srcID] {
		if edge.DstID != dstID {
			continue
		}
		if label := syncTransportLabel(edge); label != "" {
			return fmt.Sprintf("-- %s -->", label)
		}
		if srcNode.Service != "" && dstNode.Service != "" && srcNode.Service != dstNode.Service {
			return "-- handoff -->"
		}
		return "->"
	}
	if srcNode.Service != "" && dstNode.Service != "" && srcNode.Service != dstNode.Service {
		return "-- handoff -->"
	}
	return "->"
}

func syncTransportLabel(edge reviewgraph.Edge) string {
	if label := normalizeTransportLabel(edge.Transport); label != "" {
		return label
	}
	if strings.TrimSpace(edge.MetadataJSON) == "" {
		return ""
	}
	meta := map[string]any{}
	if err := json.Unmarshal([]byte(edge.MetadataJSON), &meta); err != nil {
		return ""
	}
	legacyKind, _ := meta["legacy_kind"].(string)
	switch legacyKind {
	case "CALLS_GRPC":
		return "gRPC"
	case "CALLS_HTTP", "TRIGGERS_HTTP":
		return "HTTP"
	case "ENTRYPOINT_TO":
		return "entrypoint"
	default:
		return ""
	}
}

func normalizeTransportLabel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "grpc", "grpcs":
		return "gRPC"
	case "http", "https":
		return "HTTP"
	default:
		return strings.TrimSpace(raw)
	}
}

func asyncConnectorLabel(transport, topic string) string {
	transport = normalizeTransportLabel(transport)
	topic = strings.TrimSpace(topic)
	switch {
	case transport != "" && topic != "":
		return fmt.Sprintf("-- %s:%s -->", transport, topic)
	case topic != "":
		return fmt.Sprintf("-- %s -->", topic)
	case transport != "":
		return fmt.Sprintf("-- %s -->", transport)
	default:
		return "-- async -->"
	}
}

func reverseNodeIDs(nodeIDs []string) []string {
	result := append([]string(nil), nodeIDs...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func dedupeStringSlice(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

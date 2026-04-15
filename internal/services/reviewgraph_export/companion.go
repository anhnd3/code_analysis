package reviewgraph_export

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/domain/reviewgraph"
)

type threadArtifactEntry struct {
	Target       reviewgraph.ResolvedTarget
	FlowPath     string
	OverviewPath string
	FocusFiles   []threadFocusFile
}

type threadFocusFile struct {
	Path     string
	Label    string
	Kind     string
	BucketID string
}

type companionExportResult struct {
	OverviewPath string
	FocusFiles   []threadFocusFile
}

type bucketKind string

const (
	bucketKindFile     bucketKind = "file"
	bucketKindClass    bucketKind = "class"
	bucketKindModule   bucketKind = "module"
	bucketKindBoundary bucketKind = "boundary"
)

type mergedBucket struct {
	ID        string
	Kind      bucketKind
	Label     string
	Focusable bool
	Markers   map[string]struct{}
}

type mergedEdge struct {
	SrcID  string
	DstID  string
	Count  int
}

type mergedGraph struct {
	Nodes map[string]*mergedBucket
	Edges map[string]*mergedEdge
}

func exportThreadCompanionFiles(threadDir, reviewDir, flowPath string, index int, target reviewgraph.ResolvedTarget, targetNode reviewgraph.Node, result reviewgraph.TraversalResult, nodeByID map[string]reviewgraph.Node, companionView string) (companionExportResult, error) {
	if err := os.MkdirAll(threadDir, 0o755); err != nil {
		return companionExportResult{}, err
	}

	anchorBucket := bucketForNode(target.TargetNodeID, targetNode, true, target.DisplayName)
	syncGraph := buildSyncMergedGraph(result, nodeByID, "")
	asyncGraph := buildAsyncMergedGraph(result, nodeByID, "")
	focusBuckets := collectFocusBuckets(syncGraph, asyncGraph)

	focusFiles := []threadFocusFile{}
	if companionView == "all" && len(focusBuckets) > 0 {
		focusDir := filepath.Join(threadDir, "focus")
		if err := os.MkdirAll(focusDir, 0o755); err != nil {
			return companionExportResult{}, err
		}
		for focusIndex, focusBucket := range focusBuckets {
			focusSync := buildSyncMergedGraph(result, nodeByID, focusBucket.ID)
			focusAsync := buildAsyncMergedGraph(result, nodeByID, focusBucket.ID)
			if len(focusSync.Nodes) == 0 && len(focusAsync.Nodes) == 0 {
				focusSync = newMergedGraph()
				focusSync.addBucket(focusBucket)
			}
			fileName := fmt.Sprintf("%02d_%s_%s.md", focusIndex+1, focusBucket.Kind, reviewgraph.FlowSlug(focusBucket.Label))
			focusPath := filepath.Join(focusDir, fileName)
			focusContent := renderThreadFocusMarkdown(target, flowPath, focusPath, focusBucket, focusSync, focusAsync)
			if err := os.WriteFile(focusPath, []byte(focusContent), 0o644); err != nil {
				return companionExportResult{}, err
			}
			focusFiles = append(focusFiles, threadFocusFile{
				Path:     focusPath,
				Label:    focusBucket.Label,
				Kind:     string(focusBucket.Kind),
				BucketID: focusBucket.ID,
			})
		}
	}

	overviewPath := filepath.Join(threadDir, "00_overview.md")
	overviewContent := renderThreadOverviewMarkdown(index, target, flowPath, overviewPath, anchorBucket.ID, syncGraph, asyncGraph, focusFiles)
	if err := os.WriteFile(overviewPath, []byte(overviewContent), 0o644); err != nil {
		return companionExportResult{}, err
	}

	_ = reviewDir
	return companionExportResult{
		OverviewPath: overviewPath,
		FocusFiles:   focusFiles,
	}, nil
}

func newMergedGraph() *mergedGraph {
	return &mergedGraph{
		Nodes: map[string]*mergedBucket{},
		Edges: map[string]*mergedEdge{},
	}
}

func (g *mergedGraph) addBucket(bucket mergedBucket) {
	if existing, ok := g.Nodes[bucket.ID]; ok {
		if existing.Markers == nil {
			existing.Markers = map[string]struct{}{}
		}
		for marker := range bucket.Markers {
			existing.Markers[marker] = struct{}{}
		}
		return
	}
	copyBucket := bucket
	if copyBucket.Markers == nil {
		copyBucket.Markers = map[string]struct{}{}
	}
	g.Nodes[bucket.ID] = &copyBucket
}

func (g *mergedGraph) addSequence(sequence []mergedBucket) {
	if len(sequence) == 0 {
		return
	}
	for _, bucket := range sequence {
		g.addBucket(bucket)
	}
	for index := 0; index < len(sequence)-1; index++ {
		srcID := sequence[index].ID
		dstID := sequence[index+1].ID
		if srcID == dstID {
			continue
		}
		key := srcID + "->" + dstID
		if edge, ok := g.Edges[key]; ok {
			edge.Count++
			continue
		}
		g.Edges[key] = &mergedEdge{SrcID: srcID, DstID: dstID, Count: 1}
	}
}

func (g *mergedGraph) addMarker(bucketID, marker string) {
	if strings.TrimSpace(bucketID) == "" || strings.TrimSpace(marker) == "" {
		return
	}
	bucket, ok := g.Nodes[bucketID]
	if !ok {
		return
	}
	if bucket.Markers == nil {
		bucket.Markers = map[string]struct{}{}
	}
	bucket.Markers[marker] = struct{}{}
}

func buildSyncMergedGraph(result reviewgraph.TraversalResult, nodeByID map[string]reviewgraph.Node, focusBucketID string) *mergedGraph {
	graph := newMergedGraph()
	candidates := []struct {
		paths   []reviewgraph.PathSummary
		reverse bool
	}{
		{paths: result.SyncUpstreamPaths, reverse: true},
		{paths: result.SyncDownstreamPaths, reverse: false},
	}
	for _, bridge := range result.AsyncBridges {
		candidates = append(candidates,
			struct {
				paths   []reviewgraph.PathSummary
				reverse bool
			}{paths: bridge.UpstreamSyncPaths, reverse: true},
			struct {
				paths   []reviewgraph.PathSummary
				reverse bool
			}{paths: bridge.DownstreamSyncPaths, reverse: false},
		)
	}
	for _, candidate := range candidates {
		for _, path := range candidate.paths {
			sequence, terminalBucketID := bucketSequenceForPath(path, nodeByID, candidate.reverse)
			if len(sequence) == 0 {
				continue
			}
			if focusBucketID != "" && !sequenceContainsBucket(sequence, focusBucketID) {
				continue
			}
			graph.addSequence(sequence)
			if path.TerminalReason != "" {
				graph.addMarker(terminalBucketID, path.TerminalReason)
			}
			if path.Truncated {
				graph.addMarker(terminalBucketID, "truncated")
			}
		}
	}
	return graph
}

func buildAsyncMergedGraph(result reviewgraph.TraversalResult, nodeByID map[string]reviewgraph.Node, focusBucketID string) *mergedGraph {
	graph := newMergedGraph()
	for _, bridge := range result.AsyncBridges {
		if focusBucketID != "" && !asyncBridgeTouchesFocus(bridge, nodeByID, focusBucketID) {
			continue
		}
		bridgeNode, found := nodeByID[bridge.BridgeNodeID]
		bridgeBucket := bucketForNode(bridge.BridgeNodeID, bridgeNode, found, bridge.BridgeDisplayName)
		graph.addBucket(bridgeBucket)
		if bridge.FanoutTruncated {
			graph.addMarker(bridgeBucket.ID, "fanout_truncated")
		}
		for _, producer := range bridge.Producers {
			node, ok := nodeByID[producer.NodeID]
			producerBucket := bucketForNode(producer.NodeID, node, ok, producer.DisplayName)
			graph.addSequence([]mergedBucket{producerBucket, bridgeBucket})
		}
		for _, consumer := range bridge.Consumers {
			node, ok := nodeByID[consumer.NodeID]
			consumerBucket := bucketForNode(consumer.NodeID, node, ok, consumer.DisplayName)
			graph.addSequence([]mergedBucket{bridgeBucket, consumerBucket})
		}
	}
	return graph
}

func bucketSequenceForPath(path reviewgraph.PathSummary, nodeByID map[string]reviewgraph.Node, reverse bool) ([]mergedBucket, string) {
	if len(path.NodeIDs) == 0 {
		return nil, ""
	}
	order := append([]string(nil), path.NodeIDs...)
	if reverse {
		for left, right := 0, len(order)-1; left < right; left, right = left+1, right-1 {
			order[left], order[right] = order[right], order[left]
		}
	}
	sequence := []mergedBucket{}
	for _, nodeID := range order {
		node, ok := nodeByID[nodeID]
		bucket := bucketForNode(nodeID, node, ok, nodeID)
		if len(sequence) > 0 && sequence[len(sequence)-1].ID == bucket.ID {
			continue
		}
		sequence = append(sequence, bucket)
	}
	terminalNodeID := path.NodeIDs[len(path.NodeIDs)-1]
	terminalNode, ok := nodeByID[terminalNodeID]
	terminalBucket := bucketForNode(terminalNodeID, terminalNode, ok, terminalNodeID)
	return sequence, terminalBucket.ID
}

func bucketForNode(nodeID string, node reviewgraph.Node, found bool, fallback string) mergedBucket {
	if !found {
		return mergedBucket{
			ID:        "boundary:" + nodeID,
			Kind:      bucketKindBoundary,
			Label:     firstNonEmpty(fallback, nodeID),
			Focusable: false,
			Markers:   map[string]struct{}{},
		}
	}
	if isStandaloneNode(node) {
		return mergedBucket{
			ID:        "boundary:" + node.ID,
			Kind:      bucketKindBoundary,
			Label:     standaloneNodeLabel(node, fallback),
			Focusable: false,
			Markers:   map[string]struct{}{},
		}
	}
	switch node.Kind {
	case reviewgraph.NodeModule:
		label := firstNonEmpty(node.Symbol, filepath.ToSlash(node.FilePath), node.ID)
		if node.FilePath != "" {
			label = fmt.Sprintf("%s::%s", filepath.ToSlash(node.FilePath), symbolLeaf(label))
		}
		return mergedBucket{
			ID:        "module:" + firstNonEmpty(filepath.ToSlash(node.FilePath), label, node.ID),
			Kind:      bucketKindModule,
			Label:     label,
			Focusable: true,
			Markers:   map[string]struct{}{},
		}
	case reviewgraph.NodeClass, reviewgraph.NodeType:
		fileLabel := filepath.ToSlash(firstNonEmpty(node.FilePath, node.Symbol))
		typeLabel := symbolLeaf(node.Symbol)
		label := firstNonEmpty(typeLabel, node.Symbol, node.ID)
		if fileLabel != "" {
			label = fmt.Sprintf("%s::%s", fileLabel, label)
		}
		return mergedBucket{
			ID:        "class:" + fileLabel + ":" + firstNonEmpty(typeLabel, node.Symbol, node.ID),
			Kind:      bucketKindClass,
			Label:     label,
			Focusable: true,
			Markers:   map[string]struct{}{},
		}
	}
	if node.Kind == reviewgraph.NodeMethod {
		owner := ownerLabel(node)
		if owner != "" {
			fileLabel := filepath.ToSlash(firstNonEmpty(node.FilePath, node.Symbol))
			return mergedBucket{
				ID:        "class:" + fileLabel + ":" + owner,
				Kind:      bucketKindClass,
				Label:     fmt.Sprintf("%s::%s", fileLabel, owner),
				Focusable: true,
				Markers:   map[string]struct{}{},
			}
		}
	}
	fileLabel := filepath.ToSlash(firstNonEmpty(node.FilePath, node.Symbol, node.ID))
	return mergedBucket{
		ID:        "file:" + fileLabel,
		Kind:      bucketKindFile,
		Label:     fileLabel,
		Focusable: true,
		Markers:   map[string]struct{}{},
	}
}

func collectFocusBuckets(graphs ...*mergedGraph) []mergedBucket {
	byID := map[string]mergedBucket{}
	for _, graph := range graphs {
		if graph == nil {
			continue
		}
		for _, bucket := range graph.Nodes {
			if !bucket.Focusable {
				continue
			}
			byID[bucket.ID] = *bucket
		}
	}
	result := make([]mergedBucket, 0, len(byID))
	for _, bucket := range byID {
		result = append(result, bucket)
	}
	sort.Slice(result, func(i, j int) bool {
		left := bucketKindPriority(result[i].Kind)
		right := bucketKindPriority(result[j].Kind)
		if left != right {
			return left < right
		}
		if result[i].Label != result[j].Label {
			return result[i].Label < result[j].Label
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func bucketKindPriority(kind bucketKind) int {
	switch kind {
	case bucketKindFile:
		return 0
	case bucketKindClass:
		return 1
	case bucketKindModule:
		return 2
	default:
		return 3
	}
}

func sequenceContainsBucket(sequence []mergedBucket, bucketID string) bool {
	for _, bucket := range sequence {
		if bucket.ID == bucketID {
			return true
		}
	}
	return false
}

func asyncBridgeTouchesFocus(bridge reviewgraph.AsyncBridgeSummary, nodeByID map[string]reviewgraph.Node, focusBucketID string) bool {
	for _, producer := range bridge.Producers {
		node, ok := nodeByID[producer.NodeID]
		if bucketForNode(producer.NodeID, node, ok, producer.DisplayName).ID == focusBucketID {
			return true
		}
	}
	for _, consumer := range bridge.Consumers {
		node, ok := nodeByID[consumer.NodeID]
		if bucketForNode(consumer.NodeID, node, ok, consumer.DisplayName).ID == focusBucketID {
			return true
		}
	}
	for _, candidate := range []struct {
		paths   []reviewgraph.PathSummary
		reverse bool
	}{
		{paths: bridge.UpstreamSyncPaths, reverse: true},
		{paths: bridge.DownstreamSyncPaths, reverse: false},
	} {
		for _, path := range candidate.paths {
			sequence, _ := bucketSequenceForPath(path, nodeByID, candidate.reverse)
			if sequenceContainsBucket(sequence, focusBucketID) {
				return true
			}
		}
	}
	return false
}

func renderThreadOverviewMarkdown(index int, target reviewgraph.ResolvedTarget, flowPath, currentPath, anchorBucketID string, syncGraph, asyncGraph *mergedGraph, focusFiles []threadFocusFile) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Thread Overview\n")
	fmt.Fprintf(builder, "Target: `%s`\n", target.DisplayName)
	fmt.Fprintf(builder, "Reason: `%s`\n", target.Reason)
	fmt.Fprintf(builder, "Base Flow: [%s](%s)\n\n", filepath.Base(flowPath), relativeLink(currentPath, flowPath))

	builder.WriteString("## 1. Sync Thread\n")
	builder.WriteString(renderMermaidGraph(syncGraph, anchorBucketID, "anchor"))

	builder.WriteString("\n## 2. Async Thread\n")
	builder.WriteString(renderMermaidGraph(asyncGraph, anchorBucketID, "anchor"))

	if len(focusFiles) > 0 {
		builder.WriteString("\n## 3. Focus Views\n")
		for _, focus := range focusFiles {
			fmt.Fprintf(builder, "- `%s` [%s](%s)\n", focus.Label, filepath.Base(focus.Path), relativeLink(currentPath, focus.Path))
		}
	}

	_ = index
	return builder.String()
}

func renderThreadFocusMarkdown(target reviewgraph.ResolvedTarget, flowPath, currentPath string, focusBucket mergedBucket, syncGraph, asyncGraph *mergedGraph) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Thread Focus\n")
	fmt.Fprintf(builder, "Target: `%s`\n", target.DisplayName)
	fmt.Fprintf(builder, "Focus: `%s`\n", focusBucket.Label)
	fmt.Fprintf(builder, "Bucket Kind: `%s`\n", focusBucket.Kind)
	fmt.Fprintf(builder, "Base Flow: [%s](%s)\n\n", filepath.Base(flowPath), relativeLink(currentPath, flowPath))

	builder.WriteString("## 1. Sync Thread\n")
	builder.WriteString(renderMermaidGraph(syncGraph, focusBucket.ID, "focus"))

	builder.WriteString("\n## 2. Async Thread\n")
	builder.WriteString(renderMermaidGraph(asyncGraph, focusBucket.ID, "focus"))
	return builder.String()
}

func renderOverallWorkflowMarkdown(title, graphLabel string, targets []reviewgraph.ResolvedTarget, graph *mergedGraph, affectedFiles, crossServices []string, diagnostics []reviewgraph.ImportDiagnostic, summary reviewgraph.TraversalResult) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# %s\n", title)
	fmt.Fprintf(builder, "- selected roots: %d\n\n", len(targets))

	if len(targets) > 0 {
		builder.WriteString("## Selected Roots\n")
		for _, target := range targets {
			fmt.Fprintf(builder, "- `%s` (%s)\n", target.DisplayName, target.Reason)
		}
		builder.WriteString("\n")
	}

	fmt.Fprintf(builder, "## 1. %s Workflow\n", graphLabel)
	builder.WriteString(renderMermaidGraph(graph, "", ""))

	builder.WriteString("\n## 2. Affected Files\n")
	writeAffectedFiles(builder, affectedFiles)

	builder.WriteString("\n## 3. Cross-Service Risk\n")
	writeCrossServices(builder, crossServices)

	builder.WriteString("\n## 4. Ambiguities\n")
	writeAmbiguities(builder, summary.Ambiguities, diagnostics)

	builder.WriteString("\n## 5. Flow Coverage\n")
	writeCoverage(builder, summary)
	return builder.String()
}

func renderThreadIndexMarkdown(snapshotID, reviewDir, threadsDir string, entries []threadArtifactEntry) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# Thread Companion Views\n")
	fmt.Fprintf(builder, "- snapshot: `%s`\n", snapshotID)
	fmt.Fprintf(builder, "- targets with companion views: %d\n\n", len(entries))
	builder.WriteString("## Targets\n")
	if len(entries) == 0 {
		builder.WriteString("- none\n")
		return builder.String()
	}
	for _, entry := range entries {
		overviewLink := relativeLink(filepath.Join(threadsDir, "00_index.md"), entry.OverviewPath)
		flowLink := relativeLink(filepath.Join(threadsDir, "00_index.md"), entry.FlowPath)
		fmt.Fprintf(builder, "- `%s` [overview](%s) [base flow](%s)\n", entry.Target.DisplayName, overviewLink, flowLink)
		for _, focus := range entry.FocusFiles {
			fmt.Fprintf(builder, "  - `%s` [focus](%s)\n", focus.Label, relativeLink(filepath.Join(threadsDir, "00_index.md"), focus.Path))
		}
	}
	_ = reviewDir
	return builder.String()
}

func renderMermaidGraph(graph *mergedGraph, highlightBucketID, highlightClass string) string {
	if graph == nil || len(graph.Nodes) == 0 {
		return "- none\n"
	}
	builder := &strings.Builder{}
	builder.WriteString("```mermaid\n")
	builder.WriteString("flowchart TD\n")

	nodeIDs := sortedMergedNodeIDs(graph)
	aliases := map[string]string{}
	for index, nodeID := range nodeIDs {
		aliases[nodeID] = fmt.Sprintf("N%d", index+1)
	}
	for _, nodeID := range nodeIDs {
		node := graph.Nodes[nodeID]
		fmt.Fprintf(builder, "  %s[\"%s\"]\n", aliases[nodeID], escapeMermaidLabel(mermaidBucketLabel(node)))
	}

	for _, edge := range sortedMergedEdges(graph) {
		label := ""
		if edge.Count > 1 {
			label = fmt.Sprintf("|x%d|", edge.Count)
		}
		fmt.Fprintf(builder, "  %s -->%s %s\n", aliases[edge.SrcID], label, aliases[edge.DstID])
	}

	if highlightBucketID != "" {
		switch highlightClass {
		case "focus":
			builder.WriteString("  classDef focus fill:#d9f2ff,stroke:#0b6fa4,stroke-width:3px;\n")
		default:
			builder.WriteString("  classDef anchor fill:#fff3b0,stroke:#b58900,stroke-width:3px;\n")
		}
		if _, ok := aliases[highlightBucketID]; ok {
			fmt.Fprintf(builder, "  class %s %s;\n", aliases[highlightBucketID], highlightClassOrDefault(highlightClass))
		}
	}

	builder.WriteString("```\n")
	return builder.String()
}

func highlightClassOrDefault(value string) string {
	if value == "focus" {
		return value
	}
	return "anchor"
}

func sortedMergedNodeIDs(graph *mergedGraph) []string {
	result := make([]string, 0, len(graph.Nodes))
	for nodeID := range graph.Nodes {
		result = append(result, nodeID)
	}
	sort.Slice(result, func(i, j int) bool {
		left := graph.Nodes[result[i]]
		right := graph.Nodes[result[j]]
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.ID < right.ID
	})
	return result
}

func sortedMergedEdges(graph *mergedGraph) []*mergedEdge {
	result := make([]*mergedEdge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool {
		leftSrc := graph.Nodes[result[i].SrcID].Label
		rightSrc := graph.Nodes[result[j].SrcID].Label
		if leftSrc != rightSrc {
			return leftSrc < rightSrc
		}
		leftDst := graph.Nodes[result[i].DstID].Label
		rightDst := graph.Nodes[result[j].DstID].Label
		if leftDst != rightDst {
			return leftDst < rightDst
		}
		return result[i].SrcID+result[i].DstID < result[j].SrcID+result[j].DstID
	})
	return result
}

func mermaidBucketLabel(bucket *mergedBucket) string {
	label := bucket.Label
	markers := sortedMarkerList(bucket.Markers)
	if len(markers) > 0 {
		label += "<br/>[" + strings.Join(markers, ", ") + "]"
	}
	return label
}

func sortedMarkerList(markers map[string]struct{}) []string {
	if len(markers) == 0 {
		return nil
	}
	result := make([]string, 0, len(markers))
	for marker := range markers {
		result = append(result, marker)
	}
	sort.Strings(result)
	return result
}

func escapeMermaidLabel(label string) string {
	label = strings.ReplaceAll(label, "\\", "\\\\")
	label = strings.ReplaceAll(label, "\"", "&quot;")
	return label
}

func relativeLink(fromPath, toPath string) string {
	rel, err := filepath.Rel(filepath.Dir(fromPath), toPath)
	if err != nil {
		return filepath.ToSlash(toPath)
	}
	return filepath.ToSlash(rel)
}

func symbolLeaf(symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return ""
	}
	parts := strings.FieldsFunc(symbol, func(r rune) bool {
		return r == '.' || r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return symbol
	}
	return parts[len(parts)-1]
}

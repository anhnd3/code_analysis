package reviewgraph_import

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewgraph"
	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
)

var (
	configValuePattern = regexp.MustCompile(`(?i)^\s*(topic|channel|queue|cron|schedule)\s*:\s*["']?([^"'#]+)["']?`)
	producerPattern    = regexp.MustCompile(`(?i)\b(publish|emit|produce|send)\w*\s*\(\s*["']([^"']+)["']`)
	consumerPattern    = regexp.MustCompile(`(?i)\b(subscribe|consume|listen|on)\w*\s*\(\s*["']([^"']+)["']`)
	enqueuePattern     = regexp.MustCompile(`(?i)\b(enqueue|push)\w*\s*\(\s*["']([^"']+)["']`)
	dequeuePattern     = regexp.MustCompile(`(?i)\b(dequeue|pull|pop)\w*\s*\(\s*["']([^"']+)["']`)
	schedulerPattern   = regexp.MustCompile(`(?i)\b(cron|schedule|every|register)\w*\s*\(\s*["']([^"']+)["']`)
)

func (b *builder) addAsyncHeuristics() error {
	for _, repo := range b.repositories {
		files := collectRepositoryFiles(repo)
		for _, relPath := range files {
			if relPath == "" {
				continue
			}
			if matched, _ := b.ignoreRules.Match(relPath); matched {
				continue
			}
			absPath := filepath.Join(repo.RootPath, filepath.FromSlash(relPath))
			data, err := os.ReadFile(absPath)
			if err != nil {
				continue
			}
			serviceNames := b.inferServicesFromPath(string(repo.ID), relPath)
			serviceName := ""
			if len(serviceNames) > 0 {
				serviceName = serviceNames[0]
			}
			transport := detectTransport(relPath, string(data))
			if isConfigFile(relPath) {
				b.scanConfigFile(string(repo.ID), serviceName, relPath, transport, string(data))
			}
			if isSourceFile(relPath) {
				b.scanSourceFile(string(repo.ID), serviceName, relPath, transport, string(data))
			}
		}
	}
	return nil
}

func collectRepositoryFiles(repo repository.Manifest) []string {
	all := []string{}
	all = append(all, repo.ConfigFiles...)
	all = append(all, repo.GoFiles...)
	all = append(all, repo.PythonFiles...)
	all = append(all, repo.JavaScriptFiles...)
	all = append(all, repo.TypeScriptFiles...)
	seen := map[string]struct{}{}
	result := make([]string, 0, len(all))
	for _, file := range all {
		file = filepath.ToSlash(file)
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

func (b *builder) scanConfigFile(repoID, serviceName, relPath, transport, contents string) {
	matchedAny := false
	lines := strings.Split(contents, "\n")
	for index, rawLine := range lines {
		match := configValuePattern.FindStringSubmatch(rawLine)
		if len(match) != 3 {
			continue
		}
		matchedAny = true
		kindName := strings.ToLower(match[1])
		value := strings.TrimSpace(match[2])
		if value == "" {
			continue
		}
		node, ok := bridgeNodeForMatch(repoID, serviceName, relPath, transport, kindName, value, 0, "config")
		if !ok {
			continue
		}
		node.SnapshotID = b.snapshotID
		node.MetadataJSON = reviewsqlite.EncodeJSON(map[string]any{
			"source":     "config",
			"confidence": 0.7,
			"line":       index + 1,
		})
		b.nodesByID[node.ID] = node
	}
	if !matchedAny && strings.Contains(strings.ToLower(contents), "kafka") {
		b.counts.WeakAsyncMatches++
		b.diagnostics = append(b.diagnostics, reviewgraph.ImportDiagnostic{
			Category: "weak_async_match",
			Message:  "transport hint found in config without a concrete topic, channel, queue, or schedule",
			FilePath: relPath,
		})
	}
}

func (b *builder) scanSourceFile(repoID, serviceName, relPath, transport, contents string) {
	lines := strings.Split(contents, "\n")
	for index, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if candidate, ok := parseAsyncLine(repoID, serviceName, relPath, transport, line); ok {
			symbolNode, found := b.findContainingSymbol(relPath, index+1)
			if !found {
				b.counts.WeakAsyncMatches++
				b.diagnostics = append(b.diagnostics, reviewgraph.ImportDiagnostic{
					Category: "weak_async_match",
					Message:  "async-looking construct had a literal target but could not be attached to a function or method",
					FilePath: relPath,
					Line:     index + 1,
					Evidence: line,
				})
				continue
			}
			candidate.SnapshotID = b.snapshotID
			b.nodesByID[candidate.ID] = candidate
			edge := reviewgraph.Edge{
				ID:             reviewgraph.EdgeIDWithEvidence(symbolNode.ID, candidateEdgeType(candidate.Kind, line), candidate.ID, relPath, line),
				SnapshotID:     b.snapshotID,
				FlowKind:       reviewgraph.FlowAsync,
				Confidence:     0.95,
				EvidenceFile:   relPath,
				EvidenceLine:   index + 1,
				EvidenceText:   line,
				Transport:      detectTransport(relPath, line),
				TopicOrChannel: candidate.Symbol,
				MetadataJSON:   reviewsqlite.EncodeJSON(map[string]any{"source": "async_heuristic", "version": reviewgraph.AsyncHeuristicVersion}),
			}
			if isConsumerCandidate(line) && candidate.Kind != reviewgraph.NodeSchedulerJob {
				edge.SrcID = candidate.ID
				edge.DstID = symbolNode.ID
			} else {
				edge.SrcID = symbolNode.ID
				edge.DstID = candidate.ID
			}
			edge.EdgeType = candidateEdgeType(candidate.Kind, line)
			b.addEdge(edge)
			b.promoteNodeRole(symbolNode.ID, roleForAsyncEdge(edge.EdgeType))
			continue
		}
		if containsAsyncHint(line) {
			b.counts.WeakAsyncMatches++
			b.diagnostics = append(b.diagnostics, reviewgraph.ImportDiagnostic{
				Category: "weak_async_match",
				Message:  "async-looking construct could not be converted into a concrete bridge edge",
				FilePath: relPath,
				Line:     index + 1,
				Evidence: line,
			})
		}
	}
}

func parseAsyncLine(repoID, serviceName, relPath, transport, line string) (reviewgraph.Node, bool) {
	for _, candidate := range []struct {
		pattern *regexp.Regexp
		kindKey string
	}{
		{pattern: producerPattern, kindKey: "producer"},
		{pattern: consumerPattern, kindKey: "consumer"},
		{pattern: enqueuePattern, kindKey: "queue_producer"},
		{pattern: dequeuePattern, kindKey: "queue_consumer"},
		{pattern: schedulerPattern, kindKey: "scheduler"},
	} {
		match := candidate.pattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		value := strings.TrimSpace(match[2])
		if value == "" {
			continue
		}
		node, ok := bridgeNodeForMatch(repoID, serviceName, relPath, transport, candidate.kindKey, value, 0, "source")
		if ok {
			return node, true
		}
	}
	return reviewgraph.Node{}, false
}

func bridgeNodeForMatch(repoID, serviceName, relPath, transport, kindKey, value string, line int, source string) (reviewgraph.Node, bool) {
	transport = firstNonEmpty(transport, detectTransport(relPath, kindKey+" "+value))
	language := languageForPath(relPath)
	switch kindKey {
	case "topic", "producer", "consumer":
		if transport == "kafka" {
			return reviewgraph.Node{
				ID:           reviewgraph.EventTopicNodeID(transport, value),
				SnapshotID:   "",
				Repo:         repoID,
				Service:      serviceName,
				Language:     language,
				Kind:         reviewgraph.NodeEventTopic,
				Symbol:       value,
				FilePath:     relPath,
				NodeRole:     reviewgraph.RoleBoundary,
				MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": source, "transport": transport, "line": line}),
			}, true
		}
		return reviewgraph.Node{
			ID:           reviewgraph.PubSubChannelNodeID(firstNonEmpty(transport, "pubsub"), value),
			SnapshotID:   "",
			Repo:         repoID,
			Service:      serviceName,
			Language:     language,
			Kind:         reviewgraph.NodePubSubChannel,
			Symbol:       value,
			FilePath:     relPath,
			NodeRole:     reviewgraph.RoleBoundary,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": source, "transport": transport, "line": line}),
		}, true
	case "channel":
		return reviewgraph.Node{
			ID:           reviewgraph.PubSubChannelNodeID(firstNonEmpty(transport, "pubsub"), value),
			SnapshotID:   "",
			Repo:         repoID,
			Service:      serviceName,
			Language:     language,
			Kind:         reviewgraph.NodePubSubChannel,
			Symbol:       value,
			FilePath:     relPath,
			NodeRole:     reviewgraph.RoleBoundary,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": source, "transport": transport, "line": line}),
		}, true
	case "queue", "queue_producer", "queue_consumer":
		return reviewgraph.Node{
			ID:           reviewgraph.QueueNodeID(firstNonEmpty(transport, "queue"), value),
			SnapshotID:   "",
			Repo:         repoID,
			Service:      serviceName,
			Language:     language,
			Kind:         reviewgraph.NodeQueue,
			Symbol:       value,
			FilePath:     relPath,
			NodeRole:     reviewgraph.RoleBoundary,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": source, "transport": transport, "line": line}),
		}, true
	case "cron", "schedule", "scheduler":
		return reviewgraph.Node{
			ID:           reviewgraph.SchedulerJobNodeID(firstNonEmpty(serviceName, repoID), value),
			SnapshotID:   "",
			Repo:         repoID,
			Service:      serviceName,
			Language:     language,
			Kind:         reviewgraph.NodeSchedulerJob,
			Symbol:       value,
			FilePath:     relPath,
			NodeRole:     reviewgraph.RoleScheduler,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": source, "transport": firstNonEmpty(transport, "scheduler"), "line": line}),
		}, true
	default:
		return reviewgraph.Node{}, false
	}
}

func (b *builder) findContainingSymbol(filePath string, line int) (reviewgraph.Node, bool) {
	best := reviewgraph.Node{}
	found := false
	bestSpan := 0
	for _, node := range b.nodesByID {
		if filepath.ToSlash(node.FilePath) != filepath.ToSlash(filePath) {
			continue
		}
		switch node.Kind {
		case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
		default:
			continue
		}
		if node.StartLine == 0 || node.EndLine == 0 {
			continue
		}
		if line < node.StartLine || line > node.EndLine {
			continue
		}
		span := node.EndLine - node.StartLine
		if !found || span < bestSpan {
			best = node
			bestSpan = span
			found = true
		}
	}
	return best, found
}

func detectTransport(relPath, contents string) string {
	lower := strings.ToLower(relPath + "\n" + contents)
	switch {
	case strings.Contains(lower, "kafka"):
		return "kafka"
	case strings.Contains(lower, "redis"):
		return "redis"
	case strings.Contains(lower, "pubsub") || strings.Contains(lower, "pub/sub"):
		return "pubsub"
	case strings.Contains(lower, "queue") || strings.Contains(lower, "job"):
		return "queue"
	case strings.Contains(lower, "cron") || strings.Contains(lower, "schedule"):
		return "scheduler"
	default:
		return ""
	}
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	default:
		return "unknown"
	}
}

func isConfigFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json", ".toml":
		return true
	default:
		return false
	}
}

func isSourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}

func containsAsyncHint(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "kafka") || strings.Contains(lower, "redis") || strings.Contains(lower, "pubsub") || strings.Contains(lower, "queue") || strings.Contains(lower, "cron") || strings.Contains(lower, "schedule")
}

func isConsumerCandidate(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "subscribe") || strings.Contains(lower, "consume") || strings.Contains(lower, "listen") || strings.Contains(lower, " on(") || strings.Contains(lower, ".on(") || strings.Contains(lower, "dequeue") || strings.Contains(lower, "pull(") || strings.Contains(lower, "pop(")
}

func candidateEdgeType(kind reviewgraph.NodeKind, evidence string) reviewgraph.EdgeType {
	lower := strings.ToLower(evidence)
	switch kind {
	case reviewgraph.NodeEventTopic:
		if isConsumerCandidate(lower) {
			return reviewgraph.EdgeConsumesEvent
		}
		return reviewgraph.EdgeEmitsEvent
	case reviewgraph.NodeQueue:
		if isConsumerCandidate(lower) {
			return reviewgraph.EdgeDequeuesJob
		}
		return reviewgraph.EdgeEnqueuesJob
	case reviewgraph.NodeSchedulerJob:
		return reviewgraph.EdgeSchedulesTask
	default:
		if isConsumerCandidate(lower) {
			return reviewgraph.EdgeSubscribesMessage
		}
		return reviewgraph.EdgePublishesMessage
	}
}

func roleForAsyncEdge(edgeType reviewgraph.EdgeType) reviewgraph.NodeRole {
	switch edgeType {
	case reviewgraph.EdgeConsumesEvent, reviewgraph.EdgeSubscribesMessage, reviewgraph.EdgeDequeuesJob:
		return reviewgraph.RoleAsyncConsumer
	case reviewgraph.EdgeSchedulesTask:
		return reviewgraph.RoleScheduler
	default:
		return reviewgraph.RoleAsyncProducer
	}
}

func (b *builder) promoteNodeRole(nodeID string, role reviewgraph.NodeRole) {
	node, ok := b.nodesByID[nodeID]
	if !ok {
		return
	}
	current := []reviewgraph.NodeRole{node.NodeRole, role}
	node.NodeRole = roleFromSet(current)
	b.nodesByID[nodeID] = node
}

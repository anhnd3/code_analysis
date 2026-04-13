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
	configValuePattern      = regexp.MustCompile(`(?i)^\s*(topic|channel|queue|cron|schedule|subject|stream|exchange)\s*:\s*["']?([^"'#]+)["']?`)
	producerPattern         = regexp.MustCompile(`(?i)\b(publish|emit|produce|send)\w*\s*\(\s*["']([^"']+)["']`)
	consumerPattern         = regexp.MustCompile(`(?i)\b(subscribe|consume|listen|on)\w*\s*\(\s*["']([^"']+)["']`)
	enqueuePattern          = regexp.MustCompile(`(?i)\b(enqueue|push)\w*\s*\(\s*["']([^"']+)["']`)
	dequeuePattern          = regexp.MustCompile(`(?i)\b(dequeue|pull|pop)\w*\s*\(\s*["']([^"']+)["']`)
	schedulerPattern        = regexp.MustCompile(`(?i)\b(cron|schedule|every|register)\w*\s*\(\s*["']([^"']+)["']`)
	topicConfigPattern      = regexp.MustCompile(`(?i)\b(topic|channel|queue|subject|stream)\s*[:=]\s*["']([^"']+)["']`)
	rabbitPublishPattern    = regexp.MustCompile(`(?i)\bpublish\w*\s*\(\s*["'][^"']*["']\s*,\s*["']([^"']+)["']`)
	goRoutinePattern        = regexp.MustCompile(`\bgo\s+([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
	goChannelSendPattern    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*<-\s*.+`)
	goChannelRecvPattern    = regexp.MustCompile(`<-\s*([A-Za-z_][A-Za-z0-9_]*)`)
	pythonTaskPattern       = regexp.MustCompile(`\b(?:asyncio\.create_task|asyncio\.ensure_future|[A-Za-z_][A-Za-z0-9_\.]*\.create_task)\s*\(\s*([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
	pythonThreadPattern     = regexp.MustCompile(`\b(?:threading\.)?Thread\s*\([^)]*target\s*=\s*([A-Za-z_][A-Za-z0-9_\.]*)`)
	pythonExecutorPattern   = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_\.]*\.submit\s*\(\s*([A-Za-z_][A-Za-z0-9_\.]*)`)
	pythonQueuePutPattern   = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\.]*)\.(?:put|put_nowait)\s*\(`)
	pythonQueueGetPattern   = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\.]*)\.(?:get|get_nowait)\s*\(`)
	jsWorkerPattern         = regexp.MustCompile("(?i)\\bnew\\s+(?:[A-Za-z_][A-Za-z0-9_\\.]*\\.)?Worker\\s*\\(\\s*[\"'`]([^\"'`]+)[\"'`]")
	jsWorkerThreadPattern   = regexp.MustCompile("(?i)\\bworker_threads\\.Worker\\s*\\(\\s*[\"'`]([^\"'`]+)[\"'`]")
	jsMessageSendPattern    = regexp.MustCompile(`(?i)\b(?:self|parentPort|[A-Za-z_][A-Za-z0-9_]*)\.postMessage\s*\(`)
	jsMessageReceivePattern = regexp.MustCompile(`(?i)(?:\.onmessage\b|\.addEventListener\s*\(\s*["']message["']|\.on\s*\(\s*["']message["'])`)
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
		node, ok := bridgeNodeForMatch(repoID, serviceName, relPath, transport, kindName, value, index+1, "config")
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
	if !matchedAny && containsAsyncHint(contents) {
		b.appendWeakAsyncDiagnostic(relPath, 0, "", "transport hint found in config without a concrete async target")
	}
}

func (b *builder) scanSourceFile(repoID, serviceName, relPath, transport, contents string) {
	lines := strings.Split(contents, "\n")
	for index, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lineNumber := index + 1
		matched := false
		if b.tryAddExternalAsyncLine(repoID, serviceName, relPath, transport, line, lineNumber) {
			matched = true
		}
		if b.tryAddNativeAsyncLine(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
		if !matched && containsAsyncHint(line) {
			b.appendWeakAsyncDiagnostic(relPath, lineNumber, line, "async-looking construct could not be converted into a concrete bridge edge")
		}
	}
}

func (b *builder) tryAddExternalAsyncLine(repoID, serviceName, relPath, transport, line string, lineNumber int) bool {
	candidate, ok := parseAsyncLine(repoID, serviceName, relPath, transport, line)
	if !ok {
		return false
	}
	symbolNode, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		b.appendWeakAsyncDiagnostic(relPath, lineNumber, line, "async-looking construct had a literal target but could not be attached to a function or method")
		return true
	}
	candidate.SnapshotID = b.snapshotID
	b.nodesByID[candidate.ID] = candidate
	edgeType := candidateEdgeType(candidate.Kind, line)
	edge := reviewgraph.Edge{
		ID:             reviewgraph.EdgeIDWithEvidence(symbolNode.ID, edgeType, candidate.ID, relPath, line),
		SnapshotID:     b.snapshotID,
		FlowKind:       reviewgraph.FlowAsync,
		Confidence:     0.95,
		EvidenceFile:   relPath,
		EvidenceLine:   lineNumber,
		EvidenceText:   line,
		Transport:      detectTransport(relPath, line),
		TopicOrChannel: candidate.Symbol,
		MetadataJSON:   reviewsqlite.EncodeJSON(map[string]any{"source": "async_v2", "version": reviewgraph.AsyncV2Version}),
	}
	if isConsumerCandidate(line) && candidate.Kind != reviewgraph.NodeSchedulerJob {
		edge.SrcID = candidate.ID
		edge.DstID = symbolNode.ID
	} else {
		edge.SrcID = symbolNode.ID
		edge.DstID = candidate.ID
	}
	edge.EdgeType = edgeType
	b.addEdge(edge)
	b.promoteNodeRole(symbolNode.ID, roleForAsyncEdge(edge.EdgeType))
	return true
}

func (b *builder) tryAddNativeAsyncLine(repoID, serviceName, relPath, line string, lineNumber int) bool {
	matched := false
	switch languageForPath(relPath) {
	case "go":
		if b.tryAddGoAsyncTask(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
		if b.tryAddGoChannelBridge(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
	case "python":
		if b.tryAddPythonAsyncTask(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
		if b.tryAddPythonQueueBridge(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
	case "javascript", "typescript":
		if b.tryAddJSWorkerBridge(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
		if b.tryAddJSMessageBridge(repoID, serviceName, relPath, line, lineNumber) {
			matched = true
		}
	}
	return matched
}

func (b *builder) tryAddGoAsyncTask(repoID, serviceName, relPath, line string, lineNumber int) bool {
	match := goRoutinePattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return false
	}
	owner, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		b.appendWeakAsyncDiagnostic(relPath, lineNumber, line, "goroutine launch could not be attached to a function or method")
		return true
	}
	target, ok := b.resolveRunnableTarget(repoID, relPath, match[1])
	if !ok {
		b.appendWeakAsyncDiagnostic(relPath, lineNumber, line, "goroutine target could not be statically resolved")
		return true
	}
	b.attachAsyncTask(repoID, serviceName, relPath, line, lineNumber, owner, target)
	return true
}

func (b *builder) tryAddGoChannelBridge(repoID, serviceName, relPath, line string, lineNumber int) bool {
	owner, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		return false
	}
	matched := false
	if sendMatch := goChannelSendPattern.FindStringSubmatch(line); len(sendMatch) == 2 && !strings.Contains(line, "go ") {
		b.attachInProcChannel(repoID, serviceName, relPath, line, lineNumber, owner, normalizeChannelName(sendMatch[1]), true)
		matched = true
	}
	if recvMatch := goChannelRecvPattern.FindStringSubmatch(line); len(recvMatch) == 2 {
		b.attachInProcChannel(repoID, serviceName, relPath, line, lineNumber, owner, normalizeChannelName(recvMatch[1]), false)
		matched = true
	}
	return matched
}

func (b *builder) tryAddPythonAsyncTask(repoID, serviceName, relPath, line string, lineNumber int) bool {
	owner, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		return false
	}
	targets := []string{}
	for _, pattern := range []*regexp.Regexp{pythonTaskPattern, pythonThreadPattern, pythonExecutorPattern} {
		if match := pattern.FindStringSubmatch(line); len(match) == 2 {
			targets = appendIfMissing(targets, match[1])
		}
	}
	if strings.Contains(line, "asyncio.gather") {
		for _, candidate := range extractCallTargets(line) {
			targets = appendIfMissing(targets, candidate)
		}
	}
	if len(targets) == 0 {
		return false
	}
	matched := false
	for _, rawTarget := range targets {
		target, ok := b.resolveRunnableTarget(repoID, relPath, rawTarget)
		if !ok {
			b.appendWeakAsyncDiagnostic(relPath, lineNumber, line, "python async task target could not be statically resolved")
			matched = true
			continue
		}
		b.attachAsyncTask(repoID, serviceName, relPath, line, lineNumber, owner, target)
		matched = true
	}
	return matched
}

func (b *builder) tryAddPythonQueueBridge(repoID, serviceName, relPath, line string, lineNumber int) bool {
	owner, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		return false
	}
	matched := false
	if putMatch := pythonQueuePutPattern.FindStringSubmatch(line); len(putMatch) == 2 {
		b.attachInProcChannel(repoID, serviceName, relPath, line, lineNumber, owner, normalizeChannelName(putMatch[1]), true)
		matched = true
	}
	if getMatch := pythonQueueGetPattern.FindStringSubmatch(line); len(getMatch) == 2 {
		b.attachInProcChannel(repoID, serviceName, relPath, line, lineNumber, owner, normalizeChannelName(getMatch[1]), false)
		matched = true
	}
	return matched
}

func (b *builder) tryAddJSWorkerBridge(repoID, serviceName, relPath, line string, lineNumber int) bool {
	owner, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		return false
	}
	targetPath := ""
	if match := jsWorkerPattern.FindStringSubmatch(line); len(match) == 2 {
		targetPath = match[1]
	}
	if targetPath == "" {
		if match := jsWorkerThreadPattern.FindStringSubmatch(line); len(match) == 2 {
			targetPath = match[1]
		}
	}
	if targetPath == "" {
		return false
	}
	target, ok := b.resolveWorkerTarget(repoID, relPath, targetPath)
	if !ok {
		b.appendWeakAsyncDiagnostic(relPath, lineNumber, line, "worker target could not be statically resolved")
		return true
	}
	b.attachAsyncTask(repoID, serviceName, relPath, line, lineNumber, owner, target)
	return true
}

func (b *builder) tryAddJSMessageBridge(repoID, serviceName, relPath, line string, lineNumber int) bool {
	owner, found := b.findContainingSymbol(relPath, lineNumber)
	if !found {
		return false
	}
	matched := false
	if jsMessageSendPattern.MatchString(line) {
		b.attachInProcChannel(repoID, serviceName, relPath, line, lineNumber, owner, "message", true)
		matched = true
	}
	if jsMessageReceivePattern.MatchString(line) {
		b.attachInProcChannel(repoID, serviceName, relPath, line, lineNumber, owner, "message", false)
		matched = true
	}
	return matched
}

func (b *builder) attachAsyncTask(repoID, serviceName, relPath, line string, lineNumber int, owner, target reviewgraph.Node) {
	bridge := reviewgraph.Node{
		ID:           reviewgraph.AsyncTaskNodeID(owner.ID, target.Symbol),
		SnapshotID:   b.snapshotID,
		Repo:         repoID,
		Service:      serviceName,
		Language:     languageForPath(relPath),
		Kind:         reviewgraph.NodeAsyncTask,
		Symbol:       target.Symbol,
		FilePath:     relPath,
		NodeRole:     reviewgraph.RoleBoundary,
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": "async_v2", "kind": "async_task", "line": lineNumber}),
	}
	b.nodesByID[bridge.ID] = bridge
	b.addEdge(reviewgraph.Edge{
		ID:           reviewgraph.EdgeIDWithEvidence(owner.ID, reviewgraph.EdgeSpawnsAsync, bridge.ID, relPath, line),
		SnapshotID:   b.snapshotID,
		SrcID:        owner.ID,
		DstID:        bridge.ID,
		EdgeType:     reviewgraph.EdgeSpawnsAsync,
		FlowKind:     reviewgraph.FlowAsync,
		Confidence:   0.95,
		EvidenceFile: relPath,
		EvidenceLine: lineNumber,
		EvidenceText: line,
		Transport:    "inproc_async",
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": "async_v2", "version": reviewgraph.AsyncV2Version}),
	})
	b.addEdge(reviewgraph.Edge{
		ID:           reviewgraph.EdgeIDWithEvidence(bridge.ID, reviewgraph.EdgeRunsAsync, target.ID, relPath, line),
		SnapshotID:   b.snapshotID,
		SrcID:        bridge.ID,
		DstID:        target.ID,
		EdgeType:     reviewgraph.EdgeRunsAsync,
		FlowKind:     reviewgraph.FlowAsync,
		Confidence:   0.95,
		EvidenceFile: relPath,
		EvidenceLine: lineNumber,
		EvidenceText: line,
		Transport:    "inproc_async",
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": "async_v2", "version": reviewgraph.AsyncV2Version}),
	})
	b.promoteNodeRole(owner.ID, reviewgraph.RoleAsyncProducer)
	if target.Kind != reviewgraph.NodeFile {
		b.promoteNodeRole(target.ID, reviewgraph.RoleAsyncConsumer)
	}
}

func (b *builder) attachInProcChannel(repoID, serviceName, relPath, line string, lineNumber int, owner reviewgraph.Node, rawChannel string, producer bool) {
	channel := normalizeChannelName(rawChannel)
	if channel == "" {
		return
	}
	bridge := reviewgraph.Node{
		ID:           reviewgraph.InProcChannelNodeID(relPath, channel),
		SnapshotID:   b.snapshotID,
		Repo:         repoID,
		Service:      serviceName,
		Language:     languageForPath(relPath),
		Kind:         reviewgraph.NodeInProcChannel,
		Symbol:       channel,
		FilePath:     relPath,
		NodeRole:     reviewgraph.RoleBoundary,
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"source": "async_v2", "kind": "inproc_channel", "line": lineNumber}),
	}
	b.nodesByID[bridge.ID] = bridge
	edge := reviewgraph.Edge{
		SnapshotID:     b.snapshotID,
		FlowKind:       reviewgraph.FlowAsync,
		Confidence:     0.9,
		EvidenceFile:   relPath,
		EvidenceLine:   lineNumber,
		EvidenceText:   line,
		Transport:      "inproc_channel",
		TopicOrChannel: channel,
		MetadataJSON:   reviewsqlite.EncodeJSON(map[string]any{"source": "async_v2", "version": reviewgraph.AsyncV2Version}),
	}
	if producer {
		edge.ID = reviewgraph.EdgeIDWithEvidence(owner.ID, reviewgraph.EdgeSendsToChannel, bridge.ID, relPath, line)
		edge.SrcID = owner.ID
		edge.DstID = bridge.ID
		edge.EdgeType = reviewgraph.EdgeSendsToChannel
		b.promoteNodeRole(owner.ID, reviewgraph.RoleAsyncProducer)
	} else {
		edge.ID = reviewgraph.EdgeIDWithEvidence(bridge.ID, reviewgraph.EdgeReceivesFromChannel, owner.ID, relPath, line)
		edge.SrcID = bridge.ID
		edge.DstID = owner.ID
		edge.EdgeType = reviewgraph.EdgeReceivesFromChannel
		b.promoteNodeRole(owner.ID, reviewgraph.RoleAsyncConsumer)
	}
	b.addEdge(edge)
}

func parseAsyncLine(repoID, serviceName, relPath, transport, line string) (reviewgraph.Node, bool) {
	if transport == "rabbitmq" || transport == "amqp" {
		if match := rabbitPublishPattern.FindStringSubmatch(line); len(match) == 2 {
			value := strings.TrimSpace(match[1])
			if value != "" {
				return bridgeNodeForMatch(repoID, serviceName, relPath, transport, "queue", value, 0, "source")
			}
		}
	}
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
	if transport == "kafka" || transport == "rabbitmq" || transport == "amqp" || transport == "nats" || transport == "sqs" || transport == "sns" || transport == "pubsub" || transport == "redis" {
		if topicMatch := topicConfigPattern.FindStringSubmatch(line); len(topicMatch) == 3 {
			kindKey := strings.ToLower(strings.TrimSpace(topicMatch[1]))
			value := strings.TrimSpace(topicMatch[2])
			if value != "" {
				if node, ok := bridgeNodeForMatch(repoID, serviceName, relPath, transport, kindKey, value, 0, "source"); ok {
					return node, true
				}
			}
		}
	}
	return reviewgraph.Node{}, false
}

func bridgeNodeForMatch(repoID, serviceName, relPath, transport, kindKey, value string, line int, source string) (reviewgraph.Node, bool) {
	transport = firstNonEmpty(transport, detectTransport(relPath, kindKey+" "+value))
	language := languageForPath(relPath)
	switch kindKey {
	case "topic", "producer", "consumer":
		switch transport {
		case "kafka":
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
		case "rabbitmq", "amqp", "sqs":
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
		default:
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
		}
	case "channel", "subject", "stream", "exchange":
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

func (b *builder) resolveRunnableTarget(repoID, relPath, rawTarget string) (reviewgraph.Node, bool) {
	target := normalizeCallTarget(rawTarget)
	if target == "" {
		return reviewgraph.Node{}, false
	}
	type scoredTarget struct {
		node  reviewgraph.Node
		score int
	}
	candidates := []scoredTarget{}
	short := lastIdentifier(target)
	for _, node := range b.nodesByID {
		if !isRunnableNode(node) || node.Repo != repoID {
			continue
		}
		score := 0
		if filepath.ToSlash(node.FilePath) == filepath.ToSlash(relPath) {
			score += 4
		}
		if node.Symbol == target {
			score += 8
		}
		if strings.HasSuffix(node.Symbol, "."+target) {
			score += 6
		}
		if short != "" && node.Symbol == short {
			score += 4
		}
		if short != "" && strings.HasSuffix(node.Symbol, "."+short) {
			score += 3
		}
		if score > 0 {
			candidates = append(candidates, scoredTarget{node: node, score: score})
		}
	}
	if len(candidates) == 0 {
		return reviewgraph.Node{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].node.ID < candidates[j].node.ID
	})
	best := candidates[0]
	if len(candidates) > 1 && candidates[1].score == best.score && candidates[1].node.ID != best.node.ID {
		return reviewgraph.Node{}, false
	}
	return best.node, true
}

func (b *builder) resolveWorkerTarget(repoID, relPath, rawTarget string) (reviewgraph.Node, bool) {
	cleaned := strings.TrimSpace(rawTarget)
	if cleaned == "" {
		return reviewgraph.Node{}, false
	}
	cleaned = strings.Trim(cleaned, `"'`)
	targetPath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(relPath), cleaned)))
	for _, node := range b.nodesByID {
		if node.Repo != repoID {
			continue
		}
		if node.Kind == reviewgraph.NodeFile && filepath.ToSlash(node.FilePath) == targetPath {
			return node, true
		}
	}
	for _, node := range b.nodesByID {
		if node.Repo != repoID {
			continue
		}
		if filepath.ToSlash(node.FilePath) == targetPath {
			return node, true
		}
	}
	return reviewgraph.Node{}, false
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
	case strings.Contains(lower, "rabbitmq") || strings.Contains(lower, "amqp"):
		return "rabbitmq"
	case strings.Contains(lower, "kafka"):
		return "kafka"
	case strings.Contains(lower, "nats"):
		return "nats"
	case strings.Contains(lower, "sqs"):
		return "sqs"
	case strings.Contains(lower, "sns"):
		return "sns"
	case strings.Contains(lower, "celery"):
		return "celery"
	case strings.Contains(lower, "bullmq") || strings.Contains(lower, "bull "):
		return "bullmq"
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
	return strings.Contains(lower, "kafka") ||
		strings.Contains(lower, "redis") ||
		strings.Contains(lower, "pubsub") ||
		strings.Contains(lower, "queue") ||
		strings.Contains(lower, "cron") ||
		strings.Contains(lower, "schedule") ||
		strings.Contains(lower, "rabbitmq") ||
		strings.Contains(lower, "amqp") ||
		strings.Contains(lower, "asyncio") ||
		strings.Contains(lower, "create_task") ||
		strings.Contains(lower, "thread") ||
		strings.Contains(lower, "worker") ||
		strings.Contains(lower, "postmessage") ||
		strings.Contains(lower, "parentport") ||
		strings.HasPrefix(lower, "go ") ||
		strings.Contains(lower, " go ")
}

func isConsumerCandidate(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "subscribe") ||
		strings.Contains(lower, "consume") ||
		strings.Contains(lower, "listen") ||
		strings.Contains(lower, " on(") ||
		strings.Contains(lower, ".on(") ||
		strings.Contains(lower, "dequeue") ||
		strings.Contains(lower, "pull(") ||
		strings.Contains(lower, "pop(") ||
		strings.Contains(lower, "onmessage") ||
		strings.Contains(lower, "addEventListener(\"message\"") ||
		strings.Contains(lower, "addEventListener('message'") ||
		strings.Contains(lower, "parentport.on")
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
	case reviewgraph.EdgeConsumesEvent, reviewgraph.EdgeSubscribesMessage, reviewgraph.EdgeDequeuesJob, reviewgraph.EdgeRunsAsync, reviewgraph.EdgeReceivesFromChannel:
		return reviewgraph.RoleAsyncConsumer
	case reviewgraph.EdgeSchedulesTask:
		return reviewgraph.RoleScheduler
	default:
		return reviewgraph.RoleAsyncProducer
	}
}

func normalizeCallTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "&*")
	raw = strings.TrimSuffix(raw, "()")
	raw = strings.Trim(raw, " ")
	raw = strings.TrimPrefix(raw, "await ")
	raw = strings.TrimPrefix(raw, "return ")
	return raw
}

func normalizeChannelName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ".")
	return parts[len(parts)-1]
}

func lastIdentifier(raw string) string {
	raw = normalizeCallTarget(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ".")
	return parts[len(parts)-1]
}

func extractCallTargets(line string) []string {
	results := []string{}
	matches := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`).FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		target := normalizeCallTarget(match[1])
		if strings.HasSuffix(target, ".gather") || strings.HasSuffix(target, ".create_task") || strings.HasSuffix(target, ".ensure_future") {
			continue
		}
		results = appendIfMissing(results, target)
	}
	return results
}

func isRunnableNode(node reviewgraph.Node) bool {
	switch node.Kind {
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod, reviewgraph.NodeFile:
		return true
	default:
		return false
	}
}

func (b *builder) appendWeakAsyncDiagnostic(filePath string, line int, evidence, message string) {
	b.counts.WeakAsyncMatches++
	b.diagnostics = append(b.diagnostics, reviewgraph.ImportDiagnostic{
		Category: "weak_async_match",
		Message:  message,
		FilePath: filePath,
		Line:     line,
		Evidence: evidence,
	})
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

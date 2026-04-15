package reviewgraph_select

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/services/reviewgraph_paths"
)

type Request struct {
	DBPath  string `json:"db_path"`
	Mode    string `json:"mode"`
	Symbol  string `json:"symbol,omitempty"`
	File    string `json:"file,omitempty"`
	Topic   string `json:"topic,omitempty"`
	OutPath string `json:"out_path,omitempty"`
}

type Result struct {
	DBPath   string                      `json:"db_path"`
	OutPath  string                      `json:"out_path"`
	Targets  []reviewgraph.ResolvedTarget `json:"targets"`
}

type Service struct {
	paths reviewgraph_paths.Service
}

func New(paths reviewgraph_paths.Service) Service {
	return Service{paths: paths}
}

func (s Service) Select(req Request) (Result, error) {
	if req.DBPath == "" {
		return Result{}, fmt.Errorf("db path is required")
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "workflow"
	}
	store, err := reviewsqlite.New(req.DBPath)
	if err != nil {
		return Result{}, err
	}
	defer store.Close()

	snapshotID, err := store.SnapshotID()
	if err != nil {
		return Result{}, err
	}
	nodes, err := store.ListNodes()
	if err != nil {
		return Result{}, err
	}

	targets := []reviewgraph.ResolvedTarget{}
	switch mode {
	case "manual":
		targets = append(targets, resolveBySymbol(nodes, req.Symbol)...)
		targets = append(targets, resolveByFile(nodes, req.File)...)
		targets = append(targets, resolveByTopic(nodes, req.Topic)...)
	case "workflow":
		targets = workflowTargets(nodes)
	case "entrypoints":
		targets = entrypointTargets(nodes)
	default:
		return Result{}, fmt.Errorf("unsupported mode: %s", mode)
	}
	targets = dedupeTargets(targets)
	if len(targets) == 0 {
		return Result{}, fmt.Errorf("no startpoints resolved")
	}

	outPath := req.OutPath
	if outPath == "" {
		outPath = filepath.Join(s.paths.ReviewDirFromDBPath(req.DBPath), "resolved_targets.json")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return Result{}, err
	}
	data, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return Result{}, err
	}
	artifact := reviewgraph.Artifact{
		ID:           reviewgraph.ArtifactID(reviewgraph.ArtifactResolvedTargets, "", outPath),
		SnapshotID:   snapshotID,
		ArtifactType: reviewgraph.ArtifactResolvedTargets,
		Path:         outPath,
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"mode": mode, "count": len(targets)}),
	}
	if err := store.UpsertArtifact(artifact); err != nil {
		return Result{}, err
	}
	return Result{DBPath: req.DBPath, OutPath: outPath, Targets: targets}, nil
}

func workflowTargets(nodes []reviewgraph.Node) []reviewgraph.ResolvedTarget {
	targets := []reviewgraph.ResolvedTarget{}
	seen := map[string]struct{}{}
	entrypointFileCandidates := map[string][]reviewgraph.Node{}
	for _, node := range nodes {
		if isPrimaryWorkflowTargetNode(node) {
			targets = append(targets, reviewgraph.ResolvedTarget{
				TargetNodeID: node.ID,
				DisplayName:  node.Symbol,
				Reason:       "workflow_root",
				SourceInput:  string(node.NodeRole),
			})
			seen[node.ID] = struct{}{}
			continue
		}
		if !isWorkflowEntrypointFileCandidate(node) {
			continue
		}
		entrypointFileCandidates[filepath.ToSlash(node.FilePath)] = append(entrypointFileCandidates[filepath.ToSlash(node.FilePath)], node)
	}
	files := make([]string, 0, len(entrypointFileCandidates))
	for file := range entrypointFileCandidates {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		for _, candidate := range selectWorkflowFileTargets(entrypointFileCandidates[file]) {
			if _, ok := seen[candidate.ID]; ok {
				continue
			}
			targets = append(targets, reviewgraph.ResolvedTarget{
				TargetNodeID: candidate.ID,
				DisplayName:  candidate.Symbol,
				Reason:       "workflow_root",
				SourceInput:  string(candidate.NodeRole),
			})
			seen[candidate.ID] = struct{}{}
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		left := workflowTargetPriority(targets[i], nodes)
		right := workflowTargetPriority(targets[j], nodes)
		if left != right {
			return left < right
		}
		return targets[i].DisplayName < targets[j].DisplayName
	})
	return dedupeTargets(targets)
}

func resolveBySymbol(nodes []reviewgraph.Node, raw string) []reviewgraph.ResolvedTarget {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	matches := []reviewgraph.Node{}
	for _, node := range nodes {
		if !isManualAnchorNode(node) {
			continue
		}
		if strings.EqualFold(node.Symbol, raw) {
			matches = append(matches, node)
		}
	}
	if len(matches) == 0 {
		for _, node := range nodes {
			if !isManualAnchorNode(node) {
				continue
			}
			if strings.Contains(strings.ToLower(node.Symbol), strings.ToLower(raw)) {
				matches = append(matches, node)
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Symbol < matches[j].Symbol })
	result := make([]reviewgraph.ResolvedTarget, 0, len(matches))
	for _, match := range matches {
		result = append(result, reviewgraph.ResolvedTarget{
			TargetNodeID: match.ID,
			DisplayName:  match.Symbol,
			Reason:       "manual_symbol",
			SourceInput:  raw,
		})
	}
	return result
}

func resolveByFile(nodes []reviewgraph.Node, raw string) []reviewgraph.ResolvedTarget {
	raw = filepath.ToSlash(strings.TrimSpace(raw))
	if raw == "" {
		return nil
	}
	candidates := []reviewgraph.Node{}
	for _, node := range nodes {
		if filepath.ToSlash(node.FilePath) == raw {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := fileAnchorPriority(candidates[i])
		right := fileAnchorPriority(candidates[j])
		if left != right {
			return left < right
		}
		if candidates[i].Symbol != candidates[j].Symbol {
			return candidates[i].Symbol < candidates[j].Symbol
		}
		return candidates[i].ID < candidates[j].ID
	})
	result := []reviewgraph.ResolvedTarget{}
	for _, candidate := range candidates {
		if candidate.Kind == reviewgraph.NodeFile && len(result) > 0 {
			continue
		}
		result = append(result, reviewgraph.ResolvedTarget{
			TargetNodeID: candidate.ID,
			DisplayName:  candidate.Symbol,
			Reason:       "manual_file",
			SourceInput:  raw,
			Metadata: map[string]any{
				"priority": fileAnchorPriority(candidate),
			},
		})
	}
	return result
}

func resolveByTopic(nodes []reviewgraph.Node, raw string) []reviewgraph.ResolvedTarget {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	result := []reviewgraph.ResolvedTarget{}
	for _, node := range nodes {
		switch node.Kind {
		case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
		default:
			continue
		}
		if strings.EqualFold(node.Symbol, raw) {
			result = append(result, reviewgraph.ResolvedTarget{
				TargetNodeID: node.ID,
				DisplayName:  node.Symbol,
				Reason:       "manual_topic",
				SourceInput:  raw,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DisplayName < result[j].DisplayName })
	return result
}

func entrypointTargets(nodes []reviewgraph.Node) []reviewgraph.ResolvedTarget {
	result := []reviewgraph.ResolvedTarget{}
	for _, node := range nodes {
		switch node.NodeRole {
		case reviewgraph.RoleEntrypoint, reviewgraph.RoleAsyncProducer, reviewgraph.RoleAsyncConsumer, reviewgraph.RoleScheduler:
			if isEntrypointTargetNode(node) {
				result = append(result, reviewgraph.ResolvedTarget{
					TargetNodeID: node.ID,
					DisplayName:  node.Symbol,
					Reason:       "entrypoint_scan",
					SourceInput:  string(node.NodeRole),
				})
			}
		}
		switch node.Kind {
		case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob:
			result = append(result, reviewgraph.ResolvedTarget{
				TargetNodeID: node.ID,
				DisplayName:  node.Symbol,
				Reason:       "entrypoint_scan",
				SourceInput:  string(node.Kind),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := targetPriority(result[i], nodes)
		right := targetPriority(result[j], nodes)
		if left != right {
			return left < right
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return dedupeTargets(result)
}

func isPrimaryWorkflowTargetNode(node reviewgraph.Node) bool {
	switch node.Kind {
	case reviewgraph.NodeWorkflow, reviewgraph.NodeSchedulerJob:
		return true
	case reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
		switch node.NodeRole {
		case reviewgraph.RoleEntrypoint, reviewgraph.RoleBoundary, reviewgraph.RolePublicAPI, reviewgraph.RoleScheduler:
			return true
		default:
			return false
		}
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod:
		return false
	default:
		return false
	}
}

func isWorkflowEntrypointFileCandidate(node reviewgraph.Node) bool {
	switch node.Kind {
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod:
	default:
		return false
	}
	return node.NodeRole == reviewgraph.RoleEntrypoint
}

func selectWorkflowFileTargets(candidates []reviewgraph.Node) []reviewgraph.Node {
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := workflowFileCandidatePriority(candidates[i])
		right := workflowFileCandidatePriority(candidates[j])
		if left != right {
			return left < right
		}
		if candidates[i].FilePath != candidates[j].FilePath {
			return candidates[i].FilePath < candidates[j].FilePath
		}
		return candidates[i].Symbol < candidates[j].Symbol
	})
	selected := []reviewgraph.Node{}
	for _, candidate := range candidates {
		if isBootstrapWorkflowTargetNode(candidate) {
			selected = append(selected, candidate)
		}
	}
	if len(selected) > 0 {
		return selected
	}
	if len(candidates) == 1 {
		return []reviewgraph.Node{candidates[0]}
	}
	return nil
}

func isBootstrapWorkflowTargetNode(node reviewgraph.Node) bool {
	return workflowBootstrapPriority(node) < 100
}

func workflowBootstrapPriority(node reviewgraph.Node) int {
	leaf := strings.ToLower(workflowSymbolLeaf(node.Symbol))
	switch {
	case leaf == "main":
		return 0
	case leaf == "execute":
		return 1
	case leaf == "run":
		return 2
	case strings.HasPrefix(leaf, "run"):
		return 3
	case leaf == "start":
		return 4
	case strings.HasPrefix(leaf, "start"):
		return 5
	case leaf == "serve":
		return 6
	case strings.HasPrefix(leaf, "serve"):
		return 7
	case leaf == "bootstrap":
		return 8
	case strings.HasPrefix(leaf, "bootstrap"):
		return 9
	case leaf == "listen":
		return 10
	case strings.HasPrefix(leaf, "listen"):
		return 11
	default:
		return 100
	}
}

func workflowFileCandidatePriority(node reviewgraph.Node) int {
	priority := workflowBootstrapPriority(node)
	if priority != 100 {
		return priority
	}
	if node.Kind == reviewgraph.NodeFunction {
		return 150
	}
	return 160
}

func workflowSymbolLeaf(symbol string) string {
	parts := strings.Split(symbol, ".")
	if len(parts) == 0 {
		return symbol
	}
	return parts[len(parts)-1]
}

func fileAnchorPriority(node reviewgraph.Node) int {
	switch node.NodeRole {
	case reviewgraph.RoleEntrypoint, reviewgraph.RoleBoundary:
		return 0
	case reviewgraph.RolePublicAPI:
		return 1
	}
	if node.Kind == reviewgraph.NodeFile {
		return 3
	}
	return 2
}

func isManualAnchorNode(node reviewgraph.Node) bool {
	switch node.Kind {
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeWorkflow, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod, reviewgraph.NodeFile, reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob, reviewgraph.NodeAsyncTask, reviewgraph.NodeInProcChannel:
		return true
	default:
		return false
	}
}

func isEntrypointTargetNode(node reviewgraph.Node) bool {
	switch node.Kind {
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
		return true
	default:
		return false
	}
}

func targetPriority(target reviewgraph.ResolvedTarget, nodes []reviewgraph.Node) int {
	for _, node := range nodes {
		if node.ID != target.TargetNodeID {
			continue
		}
		switch node.Kind {
		case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
			return 0
		case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob:
			return 1
		default:
			return 2
		}
	}
	return 3
}

func workflowTargetPriority(target reviewgraph.ResolvedTarget, nodes []reviewgraph.Node) int {
	for _, node := range nodes {
		if node.ID != target.TargetNodeID {
			continue
		}
		switch node.Kind {
		case reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod:
			return 0
		case reviewgraph.NodeSchedulerJob, reviewgraph.NodeWorkflow:
			return 1
		case reviewgraph.NodeFunction, reviewgraph.NodeMethod:
			return 2 + workflowFileCandidatePriority(node)
		default:
			return 200
		}
	}
	return 300
}

func dedupeTargets(targets []reviewgraph.ResolvedTarget) []reviewgraph.ResolvedTarget {
	seen := map[string]struct{}{}
	result := make([]reviewgraph.ResolvedTarget, 0, len(targets))
	for _, target := range targets {
		if _, ok := seen[target.TargetNodeID]; ok {
			continue
		}
		seen[target.TargetNodeID] = struct{}{}
		result = append(result, target)
	}
	return result
}

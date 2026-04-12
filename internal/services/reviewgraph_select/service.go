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
	if req.Mode == "" {
		return Result{}, fmt.Errorf("mode is required")
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
	switch req.Mode {
	case "manual":
		targets = append(targets, resolveBySymbol(nodes, req.Symbol)...)
		targets = append(targets, resolveByFile(nodes, req.File)...)
		targets = append(targets, resolveByTopic(nodes, req.Topic)...)
	case "entrypoints":
		targets = entrypointTargets(nodes)
	default:
		return Result{}, fmt.Errorf("unsupported mode: %s", req.Mode)
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
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"mode": req.Mode, "count": len(targets)}),
	}
	if err := store.UpsertArtifact(artifact); err != nil {
		return Result{}, err
	}
	return Result{DBPath: req.DBPath, OutPath: outPath, Targets: targets}, nil
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
		case reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob:
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
			result = append(result, reviewgraph.ResolvedTarget{
				TargetNodeID: node.ID,
				DisplayName:  node.Symbol,
				Reason:       "entrypoint_scan",
				SourceInput:  string(node.NodeRole),
			})
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
		if result[i].Reason != result[j].Reason {
			return result[i].Reason < result[j].Reason
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return dedupeTargets(result)
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
	case reviewgraph.NodeFunction, reviewgraph.NodeMethod, reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod, reviewgraph.NodeFile, reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob:
		return true
	default:
		return false
	}
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

package reviewgraph_export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/services/reviewgraph_paths"
	"analysis-module/internal/services/reviewgraph_traverse"
)

type Request struct {
	DBPath        string `json:"db_path"`
	TargetsFile   string `json:"targets_file"`
	Mode          string `json:"mode,omitempty"`
	RenderMode    string `json:"render_mode,omitempty"`
	CompanionView string `json:"companion_view,omitempty"`
	IncludeAsync  bool   `json:"include_async"`
	ForwardDepth  int    `json:"forward_depth,omitempty"`
	ReverseDepth  int    `json:"reverse_depth,omitempty"`
	OutDir        string `json:"out_dir,omitempty"`
}

type Result struct {
	DBPath              string   `json:"db_path"`
	ReviewDir           string   `json:"review_dir"`
	IndexPath           string   `json:"index_path"`
	FlowPaths           []string `json:"flow_paths"`
	ThreadsIndexPath    string   `json:"threads_index_path,omitempty"`
	ThreadOverviewPaths []string `json:"thread_overview_paths,omitempty"`
	ThreadFocusPaths    []string `json:"thread_focus_paths,omitempty"`
	ResidualPath        string   `json:"residual_path"`
	DiagnosticsPath     string   `json:"diagnostics_path"`
	RunManifestPath     string   `json:"run_manifest_path"`
}

type Service struct {
	paths    reviewgraph_paths.Service
	traverse reviewgraph_traverse.Service
}

func New(paths reviewgraph_paths.Service, traverse reviewgraph_traverse.Service) Service {
	return Service{paths: paths, traverse: traverse}
}

func (s Service) Export(req Request) (Result, error) {
	if req.DBPath == "" {
		return Result{}, fmt.Errorf("db path is required")
	}
	if req.TargetsFile == "" {
		return Result{}, fmt.Errorf("targets file is required")
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
	edges, err := store.ListEdges()
	if err != nil {
		return Result{}, err
	}
	artifacts, err := store.ListArtifacts()
	if err != nil {
		return Result{}, err
	}
	targets, err := loadTargets(req.TargetsFile)
	if err != nil {
		return Result{}, err
	}
	mode := reviewgraph.TraversalMode(firstNonEmpty(req.Mode, string(reviewgraph.TraversalFullFlow)))
	if mode != reviewgraph.TraversalFullFlow && mode != reviewgraph.TraversalBounded {
		return Result{}, fmt.Errorf("unsupported traversal mode: %s", req.Mode)
	}
	renderMode := firstNonEmpty(req.RenderMode, "grouped")
	if renderMode != "grouped" && renderMode != "raw" {
		return Result{}, fmt.Errorf("unsupported render mode: %s", req.RenderMode)
	}
	companionView := firstNonEmpty(req.CompanionView, "all")
	if companionView != "none" && companionView != "overview" && companionView != "all" {
		return Result{}, fmt.Errorf("unsupported companion view: %s", req.CompanionView)
	}
	reviewDir := firstNonEmpty(req.OutDir, s.paths.ReviewDirFromDBPath(req.DBPath))
	flowsDir := filepath.Join(reviewDir, "flows")
	threadsDir := filepath.Join(reviewDir, "threads")
	summariesDir := filepath.Join(reviewDir, "summaries")
	dirs := []string{reviewDir, flowsDir, summariesDir}
	if companionView != "none" {
		dirs = append(dirs, threadsDir)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Result{}, err
		}
	}

	graph := s.traverse.BuildGraph(nodes, edges)
	nodeByID := graph.NodeByID
	importManifest := findImportManifest(artifacts)
	caps := reviewgraph.DefaultTraversalCaps()
	flowPaths := []string{}
	threadOverviewPaths := []string{}
	threadFocusPaths := []string{}
	threadEntries := []threadArtifactEntry{}
	slugCounts := map[string]int{}
	unionNodes := map[string]struct{}{}
	unionEdges := map[string]struct{}{}
	for index, target := range targets {
		result, err := s.traverse.Traverse(graph, target.TargetNodeID, mode, req.IncludeAsync, req.ForwardDepth, req.ReverseDepth, caps)
		if err != nil {
			return Result{}, err
		}
		for _, nodeID := range result.CoveredNodeIDs {
			unionNodes[nodeID] = struct{}{}
		}
		for _, edgeID := range result.CoveredEdgeIDs {
			unionEdges[edgeID] = struct{}{}
		}
		targetNode := nodeByID[target.TargetNodeID]
		slug := reviewgraph.FlowSlug(target.DisplayName)
		if slugCounts[slug] > 0 {
			slug = reviewgraph.FlowSlugWithCollision(target.DisplayName, target.TargetNodeID)
		}
		slugCounts[slug]++
		flowPath := filepath.Join(flowsDir, fmt.Sprintf("%02d_%s.md", index+1, slug))
		relatedDiagnostics := filterDiagnostics(importManifest.Diagnostics, result.AffectedFiles)
		content := renderFlowMarkdown(index+1, target, targetNode, result, relatedDiagnostics, nodeByID, renderMode)
		if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
			return Result{}, err
		}
		flowPaths = append(flowPaths, flowPath)
		if err := store.UpsertArtifact(reviewgraph.Artifact{
			ID:           reviewgraph.ArtifactID(reviewgraph.ArtifactReviewFlow, target.TargetNodeID, flowPath),
			SnapshotID:   snapshotID,
			ArtifactType: reviewgraph.ArtifactReviewFlow,
			TargetNodeID: target.TargetNodeID,
			Path:         flowPath,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"index": index + 1, "slug": slug}),
		}); err != nil {
			return Result{}, err
		}
		if companionView != "none" {
			threadDir := filepath.Join(threadsDir, fmt.Sprintf("%02d_%s", index+1, slug))
			companionResult, err := exportThreadCompanionFiles(threadDir, reviewDir, flowPath, index+1, target, targetNode, result, nodeByID, companionView)
			if err != nil {
				return Result{}, err
			}
			if companionResult.OverviewPath != "" {
				threadOverviewPaths = append(threadOverviewPaths, companionResult.OverviewPath)
				if err := store.UpsertArtifact(reviewgraph.Artifact{
					ID:           reviewgraph.ArtifactID(reviewgraph.ArtifactReviewThreadOverview, target.TargetNodeID, companionResult.OverviewPath),
					SnapshotID:   snapshotID,
					ArtifactType: reviewgraph.ArtifactReviewThreadOverview,
					TargetNodeID: target.TargetNodeID,
					Path:         companionResult.OverviewPath,
					MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"index": index + 1, "slug": slug}),
				}); err != nil {
					return Result{}, err
				}
			}
			for _, focus := range companionResult.FocusFiles {
				threadFocusPaths = append(threadFocusPaths, focus.Path)
				if err := store.UpsertArtifact(reviewgraph.Artifact{
					ID:           reviewgraph.ArtifactID(reviewgraph.ArtifactReviewThreadFocus, target.TargetNodeID, focus.Path),
					SnapshotID:   snapshotID,
					ArtifactType: reviewgraph.ArtifactReviewThreadFocus,
					TargetNodeID: target.TargetNodeID,
					Path:         focus.Path,
					MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"bucket_id": focus.BucketID, "bucket_kind": focus.Kind, "bucket_label": focus.Label}),
				}); err != nil {
					return Result{}, err
				}
			}
			threadEntries = append(threadEntries, threadArtifactEntry{
				Target:       target,
				FlowPath:     flowPath,
				OverviewPath: companionResult.OverviewPath,
				FocusFiles:   companionResult.FocusFiles,
			})
		}
	}

	indexPath := filepath.Join(reviewDir, "00_index.md")
	threadsIndexPath := ""
	residualPath := filepath.Join(summariesDir, "98_orphans_and_residuals.md")
	diagnosticsPath := filepath.Join(summariesDir, "99_diagnostics.md")
	runManifestPath := filepath.Join(reviewDir, "run_manifest.json")

	residualGroups := buildResidualGroups(nodes, edges, unionNodes)
	qualityReport := buildQualityReport(importManifest)
	diagnosticsContent := renderDiagnosticsMarkdown(importManifest, qualityReport, len(unionNodes), len(unionEdges))
	residualContent := renderResidualMarkdown(residualGroups)
	if companionView != "none" {
		threadsIndexPath = filepath.Join(threadsDir, "00_index.md")
		threadsIndexContent := renderThreadIndexMarkdown(snapshotID, reviewDir, threadsDir, threadEntries)
		if err := os.WriteFile(threadsIndexPath, []byte(threadsIndexContent), 0o644); err != nil {
			return Result{}, err
		}
		if err := store.UpsertArtifact(reviewgraph.Artifact{
			ID:           reviewgraph.ArtifactID(reviewgraph.ArtifactReviewThreadIndex, "", threadsIndexPath),
			SnapshotID:   snapshotID,
			ArtifactType: reviewgraph.ArtifactReviewThreadIndex,
			Path:         threadsIndexPath,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"count": len(threadEntries)}),
		}); err != nil {
			return Result{}, err
		}
	}
	indexContent := renderIndexMarkdown(snapshotID, reviewDir, indexPath, threadsIndexPath, nodes, edges, flowPaths, len(targets), unionNodes, unionEdges, residualGroups, importManifest, qualityReport)

	if err := os.WriteFile(indexPath, []byte(indexContent), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(residualPath, []byte(residualContent), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(diagnosticsPath, []byte(diagnosticsContent), 0o644); err != nil {
		return Result{}, err
	}

	runManifest := reviewgraph.RunManifest{
		WorkspaceID:     importManifest.WorkspaceID,
		SnapshotID:      snapshotID,
		ImporterVersion: importManifest.ImporterVersion,
		AsyncVersion:    importManifest.AsyncVersion,
		GeneratedAt:     time.Now().UTC(),
		InputPaths:      importManifest.InputPaths,
		IgnoreFiles:     importManifest.IgnoreFiles,
		IgnoreRules:     importManifest.IgnoreRules,
		DroppedCounts:   importManifest.Counts,
		TargetFile:      req.TargetsFile,
	}
	runManifest.TraversalDefaults.Mode = mode
	runManifest.TraversalDefaults.IncludeAsync = req.IncludeAsync
	runManifest.TraversalDefaults.Caps = caps
	runManifest.TraversalDefaults.ForwardDepth = req.ForwardDepth
	runManifest.TraversalDefaults.ReverseDepth = req.ReverseDepth
	data, err := json.MarshalIndent(runManifest, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(runManifestPath, data, 0o644); err != nil {
		return Result{}, err
	}

	for _, artifact := range []reviewgraph.Artifact{
		{ID: reviewgraph.ArtifactID(reviewgraph.ArtifactReviewIndex, "", indexPath), SnapshotID: snapshotID, ArtifactType: reviewgraph.ArtifactReviewIndex, Path: indexPath},
		{ID: reviewgraph.ArtifactID(reviewgraph.ArtifactResidualSummary, "", residualPath), SnapshotID: snapshotID, ArtifactType: reviewgraph.ArtifactResidualSummary, Path: residualPath},
		{ID: reviewgraph.ArtifactID(reviewgraph.ArtifactDiagnostics, "", diagnosticsPath), SnapshotID: snapshotID, ArtifactType: reviewgraph.ArtifactDiagnostics, Path: diagnosticsPath},
		{ID: reviewgraph.ArtifactID(reviewgraph.ArtifactRunManifest, "", runManifestPath), SnapshotID: snapshotID, ArtifactType: reviewgraph.ArtifactRunManifest, Path: runManifestPath, MetadataJSON: reviewsqlite.EncodeJSON(runManifest)},
	} {
		if err := store.UpsertArtifact(artifact); err != nil {
			return Result{}, err
		}
	}

	return Result{
		DBPath:          req.DBPath,
		ReviewDir:       reviewDir,
		IndexPath:       indexPath,
		FlowPaths:       flowPaths,
		ThreadsIndexPath: threadsIndexPath,
		ThreadOverviewPaths: threadOverviewPaths,
		ThreadFocusPaths: threadFocusPaths,
		ResidualPath:    residualPath,
		DiagnosticsPath: diagnosticsPath,
		RunManifestPath: runManifestPath,
	}, nil
}

func loadTargets(path string) ([]reviewgraph.ResolvedTarget, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	targets := []reviewgraph.ResolvedTarget{}
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func findImportManifest(artifacts []reviewgraph.Artifact) reviewgraph.ImportManifest {
	for _, artifact := range artifacts {
		if artifact.ArtifactType != reviewgraph.ArtifactImportManifest || artifact.MetadataJSON == "" {
			continue
		}
		var manifest reviewgraph.ImportManifest
		if err := json.Unmarshal([]byte(artifact.MetadataJSON), &manifest); err == nil {
			return manifest
		}
	}
	return reviewgraph.ImportManifest{}
}

func buildQualityReport(manifest reviewgraph.ImportManifest) quality.AnalysisQualityReport {
	result := quality.AnalysisQualityReport{SnapshotID: manifest.SnapshotID}
	if raw := manifest.Metadata["quality_report_issue_counts"]; raw != nil {
		data, _ := json.Marshal(raw)
		_ = json.Unmarshal(data, &result.IssueCounts)
	}
	if raw := manifest.Metadata["quality_report_gaps"]; raw != nil {
		data, _ := json.Marshal(raw)
		_ = json.Unmarshal(data, &result.Gaps)
	}
	return result
}

func filterDiagnostics(diagnostics []reviewgraph.ImportDiagnostic, files []string) []reviewgraph.ImportDiagnostic {
	if len(files) == 0 || len(diagnostics) == 0 {
		return nil
	}
	fileSet := map[string]struct{}{}
	for _, file := range files {
		fileSet[file] = struct{}{}
	}
	result := []reviewgraph.ImportDiagnostic{}
	for _, diagnostic := range diagnostics {
		if diagnostic.FilePath == "" {
			continue
		}
		if _, ok := fileSet[filepath.ToSlash(diagnostic.FilePath)]; ok {
			result = append(result, diagnostic)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FilePath != result[j].FilePath {
			return result[i].FilePath < result[j].FilePath
		}
		return result[i].Line < result[j].Line
	})
	return result
}

type residualGroup struct {
	Key        string
	Header     string
	Count      int
	SampleNodes []reviewgraph.Node
}

func buildResidualGroups(nodes []reviewgraph.Node, edges []reviewgraph.Edge, covered map[string]struct{}) []residualGroup {
	componentSize := connectedComponentSizes(nodes, edges)
	grouped := map[string]*residualGroup{}
	totalSamples := 0
	for _, node := range nodes {
		if _, ok := covered[node.ID]; ok {
			continue
		}
		topDir := topDirectory(node.FilePath)
		componentBucket := bucketComponentSize(componentSize[node.ID])
		key := strings.Join([]string{node.Repo, firstNonEmpty(node.Service, "unowned"), string(node.Kind), topDir, componentBucket}, "|")
		group, ok := grouped[key]
		if !ok {
			group = &residualGroup{Key: key, Header: fmt.Sprintf("%s / %s / %s / %s / %s", firstNonEmpty(node.Repo, "unknown_repo"), firstNonEmpty(node.Service, "unowned"), node.Kind, topDir, componentBucket)}
			grouped[key] = group
		}
		group.Count++
		if totalSamples < 100 && len(group.SampleNodes) < 10 {
			group.SampleNodes = append(group.SampleNodes, node)
			totalSamples++
		}
	}
	result := make([]residualGroup, 0, len(grouped))
	for _, group := range grouped {
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Header < result[j].Header
	})
	return result
}

func connectedComponentSizes(nodes []reviewgraph.Node, edges []reviewgraph.Edge) map[string]int {
	adj := map[string][]string{}
	for _, edge := range edges {
		adj[edge.SrcID] = append(adj[edge.SrcID], edge.DstID)
		adj[edge.DstID] = append(adj[edge.DstID], edge.SrcID)
	}
	sizes := map[string]int{}
	visited := map[string]struct{}{}
	for _, node := range nodes {
		if _, ok := visited[node.ID]; ok {
			continue
		}
		queue := []string{node.ID}
		component := []string{}
		visited[node.ID] = struct{}{}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)
			for _, next := range adj[current] {
				if _, ok := visited[next]; ok {
					continue
				}
				visited[next] = struct{}{}
				queue = append(queue, next)
			}
		}
		for _, id := range component {
			sizes[id] = len(component)
		}
	}
	return sizes
}

func topDirectory(path string) string {
	path = filepath.ToSlash(path)
	if path == "" {
		return "(none)"
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "(none)"
	}
	return parts[0]
}

func bucketComponentSize(size int) string {
	switch {
	case size <= 1:
		return "component_1"
	case size <= 3:
		return "component_2_3"
	case size <= 10:
		return "component_4_10"
	default:
		return "component_11_plus"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

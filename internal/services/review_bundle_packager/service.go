package review_bundle_packager

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewbundle"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/workspace"
)

type Request struct {
	WorkspaceID         string
	Snapshot            graph.GraphSnapshot
	QualityReport       quality.AnalysisQualityReport
	ArtifactRefs        []artifact.Ref
	BuildSnapshotResult any
	OutDir              string
	BundleVersion       string
}

type Result struct {
	Bundle           reviewbundle.Bundle
	BundleDir        string
	ReviewBundlePath string
	FileCount        int
	Warnings         []string
}

type Service struct {
	artifactRoot string
	reporter     progress.Reporter
}

func New(artifactRoot string, reporter progress.Reporter) Service {
	if reporter == nil {
		reporter = progress.NoopReporter{}
	}
	return Service{artifactRoot: artifactRoot, reporter: reporter}
}

func (s Service) Package(req Request) (Result, error) {
	if req.WorkspaceID == "" {
		return Result{}, errors.New("workspace id is required")
	}
	if req.Snapshot.ID == "" {
		return Result{}, errors.New("snapshot id is required")
	}
	bundleVersion := req.BundleVersion
	if bundleVersion == "" {
		bundleVersion = reviewbundle.BundleVersionV1
	}
	s.reporter.StartStage("bundle", 0)
	bundleDir, err := s.prepareBundleDir(req.OutDir, req.WorkspaceID, req.Snapshot.ID)
	if err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}

	workspaceManifest, err := loadArtifact[workspace.Manifest](req.ArtifactRefs, artifact.TypeWorkspaceManifest)
	if err != nil {
		return Result{}, err
	}
	repositoryManifests, err := loadArtifact[[]repository.Manifest](req.ArtifactRefs, artifact.TypeRepositoryManifests)
	if err != nil {
		return Result{}, err
	}
	serviceManifests, err := loadArtifact[[]service.Manifest](req.ArtifactRefs, artifact.TypeServiceManifests)
	if err != nil {
		return Result{}, err
	}
	scanWarnings, err := loadArtifact[[]string](req.ArtifactRefs, artifact.TypeScanWarnings)
	if err != nil {
		return Result{}, err
	}

	rawFiles := []struct {
		artifactType string
		fileName     string
		contentType  string
		previewMode  reviewbundle.PreviewMode
		payload      any
	}{
		{artifactType: string(artifact.TypeWorkspaceManifest), fileName: "workspace_manifest.json", contentType: "application/json", previewMode: reviewbundle.PreviewModeEmbeddedJSON, payload: workspaceManifest},
		{artifactType: string(artifact.TypeRepositoryManifests), fileName: "repository_manifests.json", contentType: "application/json", previewMode: reviewbundle.PreviewModeEmbeddedJSON, payload: repositoryManifests},
		{artifactType: string(artifact.TypeServiceManifests), fileName: "service_manifests.json", contentType: "application/json", previewMode: reviewbundle.PreviewModeEmbeddedJSON, payload: serviceManifests},
		{artifactType: string(artifact.TypeScanWarnings), fileName: "scan_warnings.json", contentType: "application/json", previewMode: reviewbundle.PreviewModeEmbeddedJSON, payload: scanWarnings},
		{artifactType: string(artifact.TypeBuildSnapshotResult), fileName: "build_snapshot_result.json", contentType: "application/json", previewMode: reviewbundle.PreviewModeListedOnly, payload: req.BuildSnapshotResult},
		{artifactType: string(artifact.TypeQualityReport), fileName: "quality_report.json", contentType: "application/json", previewMode: reviewbundle.PreviewModeEmbeddedJSON, payload: req.QualityReport},
	}
	files := []reviewbundle.File{
		{
			ArtifactType: string(artifact.TypeReviewBundle),
			RelativePath: "review_bundle.json",
			ContentType:  "application/json",
			PreviewMode:  reviewbundle.PreviewModeListedOnly,
		},
	}
	for _, rawFile := range rawFiles {
		path := filepath.Join(bundleDir, rawFile.fileName)
		if err := writeJSON(path, rawFile.payload); err != nil {
			s.reporter.FinishStage("bundle failed")
			return Result{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			s.reporter.FinishStage("bundle failed")
			return Result{}, err
		}
		file := reviewbundle.File{
			ArtifactType: rawFile.artifactType,
			RelativePath: rawFile.fileName,
			ContentType:  rawFile.contentType,
			SizeBytes:    info.Size(),
			PreviewMode:  rawFile.previewMode,
		}
		if rawFile.previewMode == reviewbundle.PreviewModeEmbeddedJSON {
			file.EmbeddedJSON = rawFile.payload
		}
		files = append(files, file)
		s.reporter.Status("files_written=" + strconv.Itoa(len(files)))
	}

	nodePath := filepath.Join(bundleDir, "graph_nodes.jsonl")
	if err := writeJSONL(nodePath, req.Snapshot.Nodes); err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	nodeInfo, err := os.Stat(nodePath)
	if err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	files = append(files, reviewbundle.File{
		ArtifactType: string(artifact.TypeGraphNodes),
		RelativePath: "graph_nodes.jsonl",
		ContentType:  "application/x-ndjson",
		SizeBytes:    nodeInfo.Size(),
		PreviewMode:  reviewbundle.PreviewModeListedOnly,
	})

	edgePath := filepath.Join(bundleDir, "graph_edges.jsonl")
	if err := writeJSONL(edgePath, req.Snapshot.Edges); err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	edgeInfo, err := os.Stat(edgePath)
	if err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	files = append(files, reviewbundle.File{
		ArtifactType: string(artifact.TypeGraphEdges),
		RelativePath: "graph_edges.jsonl",
		ContentType:  "application/x-ndjson",
		SizeBytes:    edgeInfo.Size(),
		PreviewMode:  reviewbundle.PreviewModeListedOnly,
	})

	bundle := reviewbundle.Bundle{
		WorkspaceID:         req.WorkspaceID,
		SnapshotID:          req.Snapshot.ID,
		BundleVersion:       bundleVersion,
		GeneratedAt:         time.Now().UTC(),
		WorkspaceManifest:   workspaceManifest,
		RepositoryManifests: repositoryManifests,
		ServiceManifests:    serviceManifests,
		Snapshot: reviewbundle.Snapshot{
			ID:          req.Snapshot.ID,
			WorkspaceID: req.Snapshot.WorkspaceID,
			CreatedAt:   req.Snapshot.CreatedAt,
			Metadata:    req.Snapshot.Metadata,
		},
		QualityReport: req.QualityReport,
		Graph: reviewbundle.Graph{
			Nodes:          append([]graph.Node(nil), req.Snapshot.Nodes...),
			Edges:          append([]graph.Edge(nil), req.Snapshot.Edges...),
			NodeKindCounts: countNodeKinds(req.Snapshot.Nodes),
			EdgeKindCounts: countEdgeKinds(req.Snapshot.Edges),
			TotalNodeCount: len(req.Snapshot.Nodes),
			TotalEdgeCount: len(req.Snapshot.Edges),
		},
		Files: files,
	}

	reviewBundlePath := filepath.Join(bundleDir, "review_bundle.json")
	if err := writeJSON(reviewBundlePath, bundle); err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	reviewBundleInfo, err := os.Stat(reviewBundlePath)
	if err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	bundle.Files[0].SizeBytes = reviewBundleInfo.Size()
	if err := writeJSON(reviewBundlePath, bundle); err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	reviewBundleInfo, err = os.Stat(reviewBundlePath)
	if err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	bundle.Files[0].SizeBytes = reviewBundleInfo.Size()
	if err := writeJSON(reviewBundlePath, bundle); err != nil {
		s.reporter.FinishStage("bundle failed")
		return Result{}, err
	}
	s.reporter.FinishStage("files_written=" + strconv.Itoa(len(bundle.Files)))

	return Result{
		Bundle:           bundle,
		BundleDir:        bundleDir,
		ReviewBundlePath: reviewBundlePath,
		FileCount:        len(bundle.Files),
		Warnings:         append([]string{}, scanWarnings...),
	}, nil
}

func (s Service) prepareBundleDir(outDir, workspaceID, snapshotID string) (string, error) {
	bundleDir := outDir
	if bundleDir == "" {
		bundleDir = filepath.Join(s.artifactRoot, "workspaces", workspaceID, "snapshots", snapshotID, "review_bundle")
	}
	bundleDir = filepath.Clean(bundleDir)
	info, err := os.Stat(bundleDir)
	if err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("bundle output path is not a directory: %s", bundleDir)
		}
		entries, err := os.ReadDir(bundleDir)
		if err != nil {
			return "", err
		}
		if len(entries) > 0 {
			return "", fmt.Errorf("bundle output directory must be empty: %s", bundleDir)
		}
		return bundleDir, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return "", err
	}
	return bundleDir, nil
}

func loadArtifact[T any](refs []artifact.Ref, artifactType artifact.Type) (T, error) {
	var zero T
	for _, ref := range refs {
		if ref.Type != artifactType {
			continue
		}
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			return zero, err
		}
		var payload T
		if err := json.Unmarshal(data, &payload); err != nil {
			return zero, err
		}
		return payload, nil
	}
	return zero, fmt.Errorf("required artifact not found: %s", artifactType)
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeJSONL[T any](path string, payload []T) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	encoder := json.NewEncoder(writer)
	for _, item := range payload {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func countNodeKinds(nodes []graph.Node) map[graph.NodeKind]int {
	counts := map[graph.NodeKind]int{}
	for _, node := range nodes {
		counts[node.Kind]++
	}
	return counts
}

func countEdgeKinds(edges []graph.Edge) map[graph.EdgeKind]int {
	counts := map[graph.EdgeKind]int{}
	for _, edge := range edges {
		counts[edge.Kind]++
	}
	return counts
}

func SortedFiles(files []reviewbundle.File) []reviewbundle.File {
	result := append([]reviewbundle.File(nil), files...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].RelativePath < result[j].RelativePath
	})
	return result
}

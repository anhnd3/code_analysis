package golden

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reviewbundle"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_review_bundle"
)

var updateGoldens = flag.Bool("update-goldens", false, "update golden files")

func TestReviewBundleGoldenSingleGoService(t *testing.T) {
	bundle := generateBundle(t, "single_go_service")
	assertGoldenJSON(t, filepath.Join("testdata", "review_bundle_single_go_service.golden.json"), bundleProjection(bundle))
	assertGoldenJSON(t, filepath.Join("testdata", "review_bundle_files_single_go_service.golden.json"), fileProjection(bundle.Files))
}

func TestReviewBundleGoldenMultiRepoDiscovery(t *testing.T) {
	bundle := generateBundle(t, "multi_repo_discovery")
	assertGoldenJSON(t, filepath.Join("testdata", "review_bundle_multi_repo_discovery.golden.json"), bundleProjection(bundle))
}

func TestReviewBundleGoldenBoundaryHints(t *testing.T) {
	bundle := generateBundle(t, "boundary_hints")
	assertGoldenJSON(t, filepath.Join("testdata", "review_bundle_boundary_hints.golden.json"), bundleProjection(bundle))
}

func generateBundle(t *testing.T, fixtureName string) reviewbundle.Bundle {
	t.Helper()
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	result, err := app.BuildReviewBundle.Run(build_review_bundle.Request{
		WorkspacePath: fixtures.WorkspacePath(t, fixtureName),
	})
	if err != nil {
		t.Fatalf("build review bundle for %s: %v", fixtureName, err)
	}
	data, err := os.ReadFile(result.ReviewBundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	return bundle
}

func bundleProjection(bundle reviewbundle.Bundle) any {
	type repositorySummary struct {
		Name         string   `json:"name"`
		Role         string   `json:"role"`
		Languages    []string `json:"languages"`
		GoFiles      int      `json:"go_files"`
		ConfigFiles  int      `json:"config_files"`
		ServiceNames []string `json:"service_names"`
	}
	type serviceSummary struct {
		Name         string   `json:"name"`
		RepositoryID string   `json:"repository_id"`
		Entrypoints  []string `json:"entrypoints"`
		Boundaries   []string `json:"boundaries"`
	}
	repositories := make([]repositorySummary, 0, len(bundle.RepositoryManifests))
	for _, repo := range bundle.RepositoryManifests {
		serviceNames := make([]string, 0, len(repo.CandidateServices))
		for _, svc := range repo.CandidateServices {
			serviceNames = append(serviceNames, svc.Name)
		}
		sort.Strings(serviceNames)
		languages := make([]string, 0, len(repo.TechStack.Languages))
		for _, lang := range repo.TechStack.Languages {
			languages = append(languages, string(lang))
		}
		repositories = append(repositories, repositorySummary{
			Name:         repo.Name,
			Role:         string(repo.Role),
			Languages:    languages,
			GoFiles:      len(repo.GoFiles),
			ConfigFiles:  len(repo.ConfigFiles),
			ServiceNames: serviceNames,
		})
	}
	services := make([]serviceSummary, 0, len(bundle.ServiceManifests))
	for _, svc := range bundle.ServiceManifests {
		boundaries := make([]string, 0, len(svc.Boundaries))
		for _, boundary := range svc.Boundaries {
			boundaries = append(boundaries, string(boundary))
		}
		services = append(services, serviceSummary{
			Name:         svc.Name,
			RepositoryID: svc.RepositoryID,
			Entrypoints:  append([]string(nil), svc.Entrypoints...),
			Boundaries:   boundaries,
		})
	}
	sort.Slice(services, func(i, j int) bool {
		if services[i].RepositoryID == services[j].RepositoryID {
			return services[i].Name < services[j].Name
		}
		return services[i].RepositoryID < services[j].RepositoryID
	})
	gapCategories := make([]string, 0, len(bundle.QualityReport.Gaps))
	for _, gap := range bundle.QualityReport.Gaps {
		gapCategories = append(gapCategories, gap.Category)
	}
	sort.Strings(gapCategories)
	searchableSymbols := make([]string, 0)
	for _, node := range bundle.Graph.Nodes {
		if node.Kind == graph.NodeSymbol || node.Kind == graph.NodeTest {
			searchableSymbols = append(searchableSymbols, node.CanonicalName)
		}
	}
	sort.Strings(searchableSymbols)
	return map[string]any{
		"bundle_version":         bundle.BundleVersion,
		"workspace_languages":    append([]string(nil), bundle.WorkspaceManifest.Languages...),
		"repository_summaries":   repositories,
		"service_summaries":      services,
		"snapshot_metadata":      bundle.Snapshot.Metadata,
		"quality_gap_categories": gapCategories,
		"node_kind_counts":       normalizeNodeKindCounts(bundle.Graph.NodeKindCounts),
		"edge_kind_counts":       normalizeEdgeKindCounts(bundle.Graph.EdgeKindCounts),
		"searchable_symbols":     searchableSymbols,
	}
}

func fileProjection(files []reviewbundle.File) any {
	type projectedFile struct {
		ArtifactType    string `json:"artifact_type"`
		RelativePath    string `json:"relative_path"`
		ContentType     string `json:"content_type"`
		PreviewMode     string `json:"preview_mode"`
		HasEmbeddedJSON bool   `json:"has_embedded_json"`
		HasEmbeddedText bool   `json:"has_embedded_text"`
	}
	projected := make([]projectedFile, 0, len(files))
	for _, file := range files {
		projected = append(projected, projectedFile{
			ArtifactType:    file.ArtifactType,
			RelativePath:    file.RelativePath,
			ContentType:     file.ContentType,
			PreviewMode:     string(file.PreviewMode),
			HasEmbeddedJSON: file.EmbeddedJSON != nil,
			HasEmbeddedText: file.EmbeddedText != "",
		})
	}
	sort.Slice(projected, func(i, j int) bool {
		return projected[i].RelativePath < projected[j].RelativePath
	})
	return projected
}

func normalizeNodeKindCounts(counts map[graph.NodeKind]int) map[string]int {
	normalized := map[string]int{}
	for kind, count := range counts {
		normalized[string(kind)] = count
	}
	return normalized
}

func normalizeEdgeKindCounts(counts map[graph.EdgeKind]int) map[string]int {
	normalized := map[string]int{}
	for kind, count := range counts {
		normalized[string(kind)] = count
	}
	return normalized
}

func assertGoldenJSON(t *testing.T, relativePath string, payload any) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden payload: %v", err)
	}
	path := filepath.Join(relativePath)
	if *updateGoldens || os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	actual := append(data, '\n')
	if !slices.Equal(actual, expected) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s", path, string(expected), string(actual))
	}
}

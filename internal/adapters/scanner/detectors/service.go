package detectors

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	adapterfs "analysis-module/internal/adapters/scanner/filesystem"
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	scannerport "analysis-module/internal/ports/scanner"
	"analysis-module/pkg/ids"
)

type ServiceDetector struct{}

func NewServiceDetector() scannerport.ServiceDetector {
	return ServiceDetector{}
}

func (ServiceDetector) Detect(repo repository.Manifest, policy analysis.IgnorePolicy) ([]service.Manifest, []repository.BoundaryHint, error) {
	walkResult, err := adapterfs.Walk(repo.RootPath, policy, nil)
	if err != nil {
		return nil, nil, err
	}

	hints := make([]repository.BoundaryHint, 0, 8)
	candidates := map[string]*service.Manifest{}
	for _, entry := range walkResult.Entries {
		if entry.IsDir {
			continue
		}
		path := entry.Path
		rel, _ := filepath.Rel(repo.RootPath, path)
		rel = filepath.ToSlash(rel)
		lower := strings.ToLower(rel)
		base := filepath.Base(rel)

		switch {
		case strings.HasSuffix(lower, ".proto"):
			hints = append(hints, repository.BoundaryHint{Type: "grpc", Path: rel, Details: "proto definition"})
		case strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger"):
			hints = append(hints, repository.BoundaryHint{Type: "http", Path: rel, Details: "API description"})
		case strings.Contains(lower, "kafka"):
			hints = append(hints, repository.BoundaryHint{Type: "kafka", Path: rel, Details: "Kafka-related configuration or code"})
		}

		switch {
		case strings.EqualFold(base, "main.go"):
			addServiceCandidate(candidates, repo, filepath.ToSlash(filepath.Dir(rel)), rel)
		case strings.EqualFold(base, "main.py"), strings.EqualFold(base, "app.py"):
			addServiceCandidate(candidates, repo, filepath.ToSlash(filepath.Dir(rel)), rel)
		case strings.EqualFold(base, "server.js"), strings.EqualFold(base, "server.ts"), strings.EqualFold(rel, "src/index.ts"), strings.EqualFold(rel, "src/main.ts"):
			addServiceCandidate(candidates, repo, filepath.ToSlash(filepath.Dir(rel)), rel)
		case strings.EqualFold(base, "package.json"):
			if hasNodeServiceScript(path) {
				addServiceCandidate(candidates, repo, filepath.ToSlash(filepath.Dir(rel)), rel)
			}
		}
	}

	services := make([]service.Manifest, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Boundaries = inferServiceBoundaries(hints)
		services = append(services, *candidate)
	}
	sort.Slice(services, func(i, j int) bool {
		if services[i].RootPath == services[j].RootPath {
			return services[i].Name < services[j].Name
		}
		return services[i].RootPath < services[j].RootPath
	})
	return services, hints, nil
}

func inferServiceBoundaries(hints []repository.BoundaryHint) []service.BoundaryType {
	seen := map[service.BoundaryType]struct{}{}
	for _, hint := range hints {
		switch hint.Type {
		case "grpc":
			seen[service.BoundaryGRPC] = struct{}{}
		case "http":
			seen[service.BoundaryHTTP] = struct{}{}
		case "kafka":
			seen[service.BoundaryKafka] = struct{}{}
		}
	}
	result := make([]service.BoundaryType, 0, len(seen))
	for boundary := range seen {
		result = append(result, boundary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func addServiceCandidate(candidates map[string]*service.Manifest, repo repository.Manifest, rootRelPath, entrypoint string) {
	rootRelPath = strings.Trim(filepath.ToSlash(rootRelPath), "/")
	if rootRelPath == "." {
		rootRelPath = ""
	}
	rootPath := repo.RootPath
	serviceName := repo.Name
	if rootRelPath != "" {
		rootPath = filepath.Join(repo.RootPath, filepath.FromSlash(rootRelPath))
		serviceName = filepath.Base(rootRelPath)
	}
	key := filepath.ToSlash(rootRelPath)
	if existing, ok := candidates[key]; ok {
		if !containsString(existing.Entrypoints, entrypoint) {
			existing.Entrypoints = append(existing.Entrypoints, entrypoint)
			sort.Strings(existing.Entrypoints)
		}
		return
	}
	candidates[key] = &service.Manifest{
		ID:           service.ID(ids.Stable("svc", repo.RootPath, key, serviceName)),
		Name:         serviceName,
		RepositoryID: string(repo.ID),
		RootPath:     rootPath,
		Entrypoints:  []string{entrypoint},
	}
}

func hasNodeServiceScript(path string) bool {
	data := adapterfs.ReadText(path)
	if data == "" {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(data), &pkg); err != nil {
		return false
	}
	for _, key := range []string{"start", "dev", "serve"} {
		if strings.TrimSpace(pkg.Scripts[key]) != "" {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

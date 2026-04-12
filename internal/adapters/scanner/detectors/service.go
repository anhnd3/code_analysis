package detectors

import (
	"io/fs"
	"path/filepath"
	"strings"

	"analysis-module/internal/domain/repository"
	scannerport "analysis-module/internal/ports/scanner"
)

type ServiceDetector struct{}

func NewServiceDetector() scannerport.ServiceDetector {
	return ServiceDetector{}
}

func (ServiceDetector) Detect(repo repository.Manifest) ([]string, []repository.BoundaryHint, error) {
	entrypoints := []string{}
	hints := []repository.BoundaryHint{}
	err := filepath.WalkDir(repo.RootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" || d.Name() == "artifacts" || d.Name() == "__pycache__" || d.Name() == ".venv" || d.Name() == ".pytest_cache" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(repo.RootPath, path)
		lower := strings.ToLower(rel)
		if strings.HasSuffix(rel, "main.go") {
			entrypoints = append(entrypoints, rel)
		}
		switch {
		case strings.HasSuffix(lower, ".proto"):
			hints = append(hints, repository.BoundaryHint{Type: "grpc", Path: rel, Details: "proto definition"})
		case strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger"):
			hints = append(hints, repository.BoundaryHint{Type: "http", Path: rel, Details: "API description"})
		case strings.Contains(lower, "kafka"):
			hints = append(hints, repository.BoundaryHint{Type: "kafka", Path: rel, Details: "Kafka-related configuration or code"})
		}
		return nil
	})
	return entrypoints, hints, err
}

package detectors

import (
	"path/filepath"
	"sort"
	"strconv"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/analysis"
	scannerport "analysis-module/internal/ports/scanner"

	adapterfs "analysis-module/internal/adapters/scanner/filesystem"
)

type RepoRootDetector struct {
	reporter progress.Reporter
}

var repoSignals = []string{
	"go.mod",
	"package.json",
	"pyproject.toml",
	"requirements.txt",
	"setup.py",
	"setup.cfg",
	"Pipfile",
	"poetry.lock",
	"Cargo.toml",
	"pom.xml",
	"build.gradle",
	"settings.gradle",
}

func NewRepoRootDetector(reporter progress.Reporter) scannerport.RepoRootDetector {
	if reporter == nil {
		reporter = progress.NoopReporter{}
	}
	return RepoRootDetector{reporter: reporter}
}

func (d RepoRootDetector) Detect(root string, policy analysis.IgnorePolicy) ([]string, error) {
	entryCount := 0
	result, err := adapterfs.Walk(root, policy, func(entry adapterfs.Entry) {
		entryCount++
		if entryCount == 1 || entryCount%250 == 0 {
			d.reporter.Status("entries=" + strconv.Itoa(entryCount))
		}
	})
	if err != nil {
		return nil, err
	}
	roots := map[string]struct{}{}
	for _, entry := range result.Entries {
		if !entry.IsDir {
			base := filepath.Base(entry.Path)
			for _, signal := range repoSignals {
				if base == signal {
					roots[filepath.Dir(entry.Path)] = struct{}{}
				}
			}
			continue
		}
		if filepath.Base(entry.Path) == ".git" {
			roots[filepath.Dir(entry.Path)] = struct{}{}
		}
	}
	if len(roots) == 0 {
		roots[root] = struct{}{}
	}
	paths := make([]string, 0, len(roots))
	for path := range roots {
		paths = append(paths, filepath.Clean(path))
	}
	sort.Strings(paths)
	d.reporter.Status("entries=" + strconv.Itoa(entryCount) + " repos=" + strconv.Itoa(len(paths)))
	return paths, nil
}

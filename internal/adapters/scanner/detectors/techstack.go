package detectors

import (
	"path/filepath"
	"sort"
	"strings"

	adapterfs "analysis-module/internal/adapters/scanner/filesystem"
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/repository"
	scannerport "analysis-module/internal/ports/scanner"
)

type TechStackDetector struct{}

func NewTechStackDetector() scannerport.TechStackDetector {
	return TechStackDetector{}
}

func (TechStackDetector) Detect(repoPath string, policy analysis.IgnorePolicy) (repository.TechStackProfile, error) {
	langs := map[repository.Language]struct{}{}
	buildFiles := []string{}
	testFrameworks := map[string]struct{}{}
	frameworkHints := map[string]struct{}{}
	walkResult, err := adapterfs.Walk(repoPath, policy, nil)
	if err != nil {
		return repository.TechStackProfile{}, err
	}
	for _, entry := range walkResult.Entries {
		if entry.IsDir {
			continue
		}
		path := entry.Path
		rel, _ := filepath.Rel(repoPath, path)
		base := filepath.Base(path)
		switch filepath.Ext(path) {
		case ".go":
			langs[repository.LanguageGo] = struct{}{}
			if strings.HasSuffix(path, "_test.go") {
				testFrameworks["go_test"] = struct{}{}
			}
		case ".py":
			langs[repository.LanguagePython] = struct{}{}
			if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
				testFrameworks["pytest"] = struct{}{}
			}
		case ".js":
			langs[repository.LanguageJS] = struct{}{}
			if strings.HasSuffix(base, ".test.js") || strings.HasSuffix(base, ".spec.js") {
				testFrameworks["jest"] = struct{}{}
			}
		case ".ts":
			langs[repository.LanguageTS] = struct{}{}
			if strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".spec.ts") {
				testFrameworks["jest"] = struct{}{}
			}
		case ".java":
			langs[repository.LanguageJava] = struct{}{}
		case ".yaml", ".yml":
			langs[repository.LanguageYAML] = struct{}{}
		case ".json":
			langs[repository.LanguageJSON] = struct{}{}
		}
		switch base {
		case "go.mod", "package.json", "pyproject.toml", "requirements.txt", "setup.py", "setup.cfg", "Pipfile", "poetry.lock", "Cargo.toml", "pom.xml", "build.gradle", "settings.gradle":
			buildFiles = append(buildFiles, rel)
		}
		contentHintPath := strings.ToLower(rel)
		if strings.Contains(contentHintPath, "openapi") || strings.Contains(contentHintPath, "swagger") {
			frameworkHints["openapi"] = struct{}{}
		}
		if strings.HasSuffix(base, ".proto") {
			frameworkHints["grpc"] = struct{}{}
		}
		if strings.Contains(contentHintPath, "kafka") {
			frameworkHints["kafka"] = struct{}{}
		}
	}
	languages := make([]repository.Language, 0, len(langs))
	for lang := range langs {
		languages = append(languages, lang)
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i] < languages[j] })
	frameworks := make([]string, 0, len(frameworkHints))
	for hint := range frameworkHints {
		frameworks = append(frameworks, hint)
	}
	sort.Strings(buildFiles)
	sort.Strings(frameworks)
	tests := make([]string, 0, len(testFrameworks))
	for framework := range testFrameworks {
		tests = append(tests, framework)
	}
	sort.Strings(tests)
	return repository.TechStackProfile{
		Languages:      languages,
		BuildFiles:     buildFiles,
		TestFrameworks: tests,
		FrameworkHints: frameworks,
	}, nil
}

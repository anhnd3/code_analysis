package reviewgraph

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var builtInIgnoredDirs = map[string]struct{}{
	"tests":        {},
	"test":         {},
	"__tests__":    {},
	"dist":         {},
	"vendor":       {},
	"node_modules": {},
	"artifacts":    {},
	"coverage":     {},
	"testdata":     {},
}

var builtInTestFilePatterns = []string{
	"*_test.go",
	"*.spec.ts",
	"*.test.ts",
	"*.spec.js",
	"*.test.js",
}

var builtInGeneratedFilePatterns = []string{
	"*.pb.go",
	"*_grpc.pb.go",
	"*.pb.gw.go",
}

var builtInGeneratedPathPatterns = []string{
	"pkg/proto/",
	"proto/gen/",
	"proto_gen/",
}

type IgnoreRules struct {
	Patterns []string `json:"patterns"`
}

func LoadIgnoreRules(paths ...string) (IgnoreRules, []string, error) {
	patterns := []string{}
	loadedFrom := []string{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return IgnoreRules{}, nil, err
		}
		loadedFrom = append(loadedFrom, filepath.Clean(path))
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = filepath.ToSlash(line)
			line = strings.TrimPrefix(line, "./")
			line = strings.Trim(line, "/")
			if line != "" {
				patterns = append(patterns, line)
			}
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return IgnoreRules{}, nil, err
		}
		_ = file.Close()
	}
	patterns = dedupeStrings(patterns)
	return IgnoreRules{Patterns: patterns}, loadedFrom, nil
}

func (r IgnoreRules) AllPatterns() []string {
	patterns := append([]string{}, builtInTestFilePatterns...)
	patterns = append(patterns, builtInGeneratedFilePatterns...)
	for dir := range builtInIgnoredDirs {
		patterns = append(patterns, dir+"/")
	}
	patterns = append(patterns, builtInGeneratedPathPatterns...)
	patterns = append(patterns, r.Patterns...)
	sort.Strings(patterns)
	return patterns
}

func (r IgnoreRules) Match(relPath string) (matched bool, generated bool) {
	relPath = normalizeImportPath(relPath)
	if relPath == "" {
		return false, false
	}
	base := pathBase(relPath)
	for _, pattern := range builtInTestFilePatterns {
		if ok, _ := filepath.Match(pattern, base); ok {
			return true, false
		}
	}
	for _, pattern := range builtInGeneratedFilePatterns {
		if ok, _ := filepath.Match(pattern, base); ok {
			return true, true
		}
	}
	parts := strings.Split(relPath, "/")
	for _, part := range parts[:len(parts)-1] {
		if _, ok := builtInIgnoredDirs[part]; ok {
			generated = part != "tests" && part != "test" && part != "__tests__"
			return true, generated
		}
	}
	for _, pattern := range builtInGeneratedPathPatterns {
		if matchesIgnorePattern(relPath, pattern) {
			return true, true
		}
	}
	lower := strings.ToLower(relPath)
	if strings.Contains(lower, "/mocks/") || strings.Contains(lower, "/fixtures/") {
		if strings.Contains(lower, "test") {
			return true, false
		}
	}
	for _, pattern := range r.Patterns {
		if matchesIgnorePattern(relPath, pattern) {
			return true, false
		}
	}
	return false, false
}

func matchesIgnorePattern(relPath, pattern string) bool {
	pattern = normalizeImportPath(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
	}
	if strings.ContainsAny(pattern, "*?[]") {
		if ok, _ := filepath.Match(pattern, relPath); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, pathBase(relPath)); ok {
			return true
		}
	}
	if relPath == pattern || strings.HasPrefix(relPath, pattern+"/") {
		return true
	}
	parts := strings.Split(relPath, "/")
	for _, part := range parts {
		if part == pattern {
			return true
		}
	}
	return false
}

func normalizeImportPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	path = strings.Trim(path, "/")
	return path
}

func pathBase(path string) string {
	parts := strings.Split(normalizeImportPath(path), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

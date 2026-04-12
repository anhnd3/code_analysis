package analysis

import (
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/pkg/ids"
)

var defaultIgnoredDirs = map[string]struct{}{
	".git":          {},
	"artifacts":     {},
	".pytest_cache": {},
	"__pycache__":   {},
	".venv":         {},
	"node_modules":  {},
	"testdata":      {},
	"vendor":        {},
}

type IgnorePolicy struct {
	Patterns  []string `json:"patterns"`
	Signature string   `json:"signature"`
}

func NewIgnorePolicy(patterns []string) IgnorePolicy {
	normalized := make([]string, 0, len(patterns))
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		pattern = normalizePattern(pattern)
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		normalized = append(normalized, pattern)
	}
	sort.Strings(normalized)
	return IgnorePolicy{
		Patterns:  normalized,
		Signature: ids.Stable("ignore", strings.Join(normalized, "|")),
	}
}

func (p IgnorePolicy) ShouldIgnore(root, path string, isDir bool) bool {
	rel := relativeToRoot(root, path)
	if rel == "." || rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if _, ok := defaultIgnoredDirs[part]; ok {
			return true
		}
	}
	for _, pattern := range p.Patterns {
		if pattern == "" {
			continue
		}
		if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
			return true
		}
		if strings.Contains(rel, pattern) {
			return true
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		if isDir {
			base := parts[len(parts)-1]
			if base == pattern {
				return true
			}
		}
	}
	return false
}

func normalizePattern(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = filepath.ToSlash(raw)
	raw = strings.TrimPrefix(raw, "./")
	raw = strings.Trim(raw, "/")
	return raw
}

func relativeToRoot(root, path string) string {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

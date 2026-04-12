package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	Path  string
	IsDir bool
}

type WalkObserver func(Entry)

func Walk(root string, ignorePatterns []string, observer WalkObserver) ([]Entry, error) {
	entries := make([]Entry, 0, 128)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && shouldIgnore(path, ignorePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entry := Entry{Path: path, IsDir: d.IsDir()}
		entries = append(entries, entry)
		if observer != nil {
			observer(entry)
		}
		return nil
	})
	return entries, err
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ReadText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func shouldIgnore(path string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

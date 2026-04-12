package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"

	"analysis-module/internal/domain/analysis"
)

type Entry struct {
	Path  string
	IsDir bool
}

type WalkObserver func(Entry)

type WalkResult struct {
	Entries           []Entry
	SkippedEntryCount int
}

func Walk(root string, policy analysis.IgnorePolicy, observer WalkObserver) (WalkResult, error) {
	entries := make([]Entry, 0, 128)
	skipped := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && policy.ShouldIgnore(root, path, d.IsDir()) {
			skipped++
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
	return WalkResult{Entries: entries, SkippedEntryCount: skipped}, err
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

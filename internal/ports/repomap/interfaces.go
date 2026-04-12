package repomap

import "analysis-module/internal/domain/repomap"

type Builder interface {
	Build(target string, snapshotID string) (repomap.RepoMap, error)
}

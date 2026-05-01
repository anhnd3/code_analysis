package repomap_build

import (
	"analysis-module/internal/app"
	"analysis-module/internal/domain/repomap"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Build(target, snapshotID string) (repomap.RepoMap, error) {
	return repomap.RepoMap{}, app.NotImplemented("repo-map generation is deferred in this pass")
}

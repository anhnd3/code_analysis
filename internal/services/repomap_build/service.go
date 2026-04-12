package repomap_build

import (
	apperrors "analysis-module/internal/app/errors"
	"analysis-module/internal/domain/repomap"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Build(target, snapshotID string) (repomap.RepoMap, error) {
	return repomap.RepoMap{}, apperrors.NotImplemented("repo-map generation is deferred in this pass")
}

package build_repomap

import (
	apperrors "analysis-module/internal/app/errors"
	"analysis-module/internal/domain/repomap"
)

type Request struct {
	SnapshotID string `json:"snapshot_id"`
	Target     string `json:"target"`
}

type Result struct {
	RepoMap repomap.RepoMap `json:"repo_map"`
}

func Run(Request) (Result, error) {
	return Result{}, apperrors.NotImplemented("build-repomap workflow is deferred in this pass")
}

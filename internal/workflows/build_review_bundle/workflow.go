package build_review_bundle

import (
	"fmt"
	"path/filepath"

	"analysis-module/internal/domain/reviewbundle"
	"analysis-module/internal/services/review_bundle_packager"
	"analysis-module/internal/workflows/build_snapshot"
)

type Request struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
	OutDir         string   `json:"out_dir"`
}

type Result struct {
	WorkspaceID      string   `json:"workspace_id"`
	SnapshotID       string   `json:"snapshot_id"`
	BundleVersion    string   `json:"bundle_version"`
	BundleDir        string   `json:"bundle_dir"`
	ReviewBundlePath string   `json:"review_bundle_path"`
	FileCount        int      `json:"file_count"`
	Warnings         []string `json:"warnings"`
}

type Workflow struct {
	buildSnapshot build_snapshot.Workflow
	packager      review_bundle_packager.Service
}

func New(buildSnapshot build_snapshot.Workflow, packager review_bundle_packager.Service) Workflow {
	return Workflow{
		buildSnapshot: buildSnapshot,
		packager:      packager,
	}
}

func (w Workflow) Run(req Request) (Result, error) {
	if req.WorkspacePath == "" {
		return Result{}, fmt.Errorf("workspace path is required")
	}
	snapshotResult, err := w.buildSnapshot.Run(build_snapshot.Request{
		WorkspacePath:  filepath.Clean(req.WorkspacePath),
		IgnorePatterns: append([]string(nil), req.IgnorePatterns...),
		TargetHints:    append([]string(nil), req.TargetHints...),
	})
	if err != nil {
		return Result{}, err
	}
	packageResult, err := w.packager.Package(review_bundle_packager.Request{
		WorkspaceID:         snapshotResult.WorkspaceID,
		Snapshot:            snapshotResult.Snapshot,
		QualityReport:       snapshotResult.QualityReport,
		ArtifactRefs:        snapshotResult.ArtifactRefs,
		BuildSnapshotResult: snapshotResult,
		OutDir:              req.OutDir,
		BundleVersion:       reviewbundle.BundleVersionV1,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		WorkspaceID:      snapshotResult.WorkspaceID,
		SnapshotID:       snapshotResult.Snapshot.ID,
		BundleVersion:    reviewbundle.BundleVersionV1,
		BundleDir:        packageResult.BundleDir,
		ReviewBundlePath: packageResult.ReviewBundlePath,
		FileCount:        packageResult.FileCount,
		Warnings:         packageResult.Warnings,
	}, nil
}

package dto

import (
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
)

type AnalyzeWorkspaceRequest = analyze_workspace.Request
type AnalyzeWorkspaceResponse = analyze_workspace.Result

type BuildSnapshotRequest = build_snapshot.Request
type BuildSnapshotResponse = build_snapshot.Result

type BlastRadiusRequest = blast_radius.Request
type BlastRadiusResponse = blast_radius.Result

type ImpactedTestsRequest = impacted_tests.Request
type ImpactedTestsResponse = impacted_tests.Result

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

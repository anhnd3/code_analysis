package httpapi

import (
	"encoding/json"
	"net/http"

	apperrors "analysis-module/internal/app/errors"
	"analysis-module/internal/adapters/api/dto"
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
)

type Handler struct {
	analyzeWorkflow      analyze_workspace.Workflow
	buildSnapshotWorkflow build_snapshot.Workflow
	blastRadiusWorkflow  blast_radius.Workflow
	impactedTestsWorkflow impacted_tests.Workflow
}

func New(analyzeWorkflow analyze_workspace.Workflow, buildSnapshotWorkflow build_snapshot.Workflow, blastRadiusWorkflow blast_radius.Workflow, impactedTestsWorkflow impacted_tests.Workflow) http.Handler {
	h := Handler{
		analyzeWorkflow:       analyzeWorkflow,
		buildSnapshotWorkflow: buildSnapshotWorkflow,
		blastRadiusWorkflow:   blastRadiusWorkflow,
		impactedTestsWorkflow: impactedTestsWorkflow,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/v1/workspaces/analyze", h.analyzeWorkspace)
	mux.HandleFunc("/v1/snapshots/build", h.buildSnapshot)
	mux.HandleFunc("/v1/queries/blast-radius", h.blastRadius)
	mux.HandleFunc("/v1/queries/impacted-tests", h.impactedTests)
	return mux
}

func (h Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) analyzeWorkspace(w http.ResponseWriter, r *http.Request) {
	var req dto.AnalyzeWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.InvalidArgument("invalid analyze-workspace request"))
		return
	}
	result, err := h.analyzeWorkflow.Run(req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.AnalyzeWorkspaceResponse(result))
}

func (h Handler) buildSnapshot(w http.ResponseWriter, r *http.Request) {
	var req dto.BuildSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.InvalidArgument("invalid build-snapshot request"))
		return
	}
	result, err := h.buildSnapshotWorkflow.Run(req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.BuildSnapshotResponse(result))
}

func (h Handler) blastRadius(w http.ResponseWriter, r *http.Request) {
	var req dto.BlastRadiusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.InvalidArgument("invalid blast-radius request"))
		return
	}
	result, err := h.blastRadiusWorkflow.Run(req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.BlastRadiusResponse(result))
}

func (h Handler) impactedTests(w http.ResponseWriter, r *http.Request) {
	var req dto.ImpactedTestsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.InvalidArgument("invalid impacted-tests request"))
		return
	}
	result, err := h.impactedTestsWorkflow.Run(req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.ImpactedTestsResponse(result))
}

func writeError(w http.ResponseWriter, err error) {
	status := apperrors.StatusOf(err)
	payload := dto.ErrorResponse{Code: "internal", Message: err.Error()}
	if typed, ok := err.(*apperrors.Error); ok {
		payload.Code = string(typed.Code)
		payload.Message = typed.Message
	}
	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

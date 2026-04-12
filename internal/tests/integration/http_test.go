package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/tests/fixtures"
)

func TestHTTPEndpoints(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = cfg.ArtifactRoot + "/analysis.sqlite"
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRes := httptest.NewRecorder()
	app.HTTPHandler.ServeHTTP(healthRes, healthReq)
	if healthRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from healthz, got %d", healthRes.Code)
	}

	body, _ := json.Marshal(map[string]any{
		"workspace_path": fixtures.WorkspacePath(t, "single_go_service"),
	})
	analyzeReq := httptest.NewRequest(http.MethodPost, "/v1/workspaces/analyze", bytes.NewReader(body))
	analyzeRes := httptest.NewRecorder()
	app.HTTPHandler.ServeHTTP(analyzeRes, analyzeReq)
	if analyzeRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from analyze endpoint, got %d", analyzeRes.Code)
	}
}

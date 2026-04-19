package boundary_detect

import (
	"os"
	"path/filepath"
	"testing"

	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/boundary/go/frameworks"
	"analysis-module/internal/domain/repository"
)

func TestDetectAllDetailedPreparesImportedGinProviderPackages(t *testing.T) {
	workspace := t.TempDir()

	writeBoundaryTestFile(t, filepath.Join(workspace, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"))
	writeBoundaryTestFile(t, filepath.Join(workspace, "cmd", "router.go"), []byte(`package cmd

import (
	"github.com/gin-gonic/gin"

	camera_v2 "example.com/demo/service/camera_v2"
)

func register() {
	engine := gin.New()
	handlersV2 := camera_v2.InitCameraV2()
	engine.POST("/detect-qr", handlersV2.DetectQR)
}
`))
	writeBoundaryTestFile(t, filepath.Join(workspace, "service", "camera_v2", "camera_v2.go"), []byte(`package camera_v2

import handler_v2 "example.com/demo/service/camera_v2/handler"

func InitCameraV2() handler_v2.CameraV2Handler {
	return handler_v2.NewCameraHandler()
}
`))

	registry := boundary.NewRegistry()
	registry.Register(frameworks.NewGinDetector())
	service := New(registry)

	inventory := repository.Inventory{
		WorkspaceID: "ws_demo",
		Repositories: []repository.Manifest{{
			ID:       "repo_demo",
			Name:     "demo",
			RootPath: workspace,
			Role:     repository.RoleService,
			TechStack: repository.TechStackProfile{
				Languages: []repository.Language{repository.LanguageGo},
			},
			GoFiles: []string{
				"cmd/router.go",
				"service/camera_v2/camera_v2.go",
			},
		}},
	}

	result, err := service.DetectAllDetailed(inventory, nil)
	if err != nil {
		t.Fatalf("detect boundaries: %v", err)
	}
	if len(result.Roots) != 1 {
		t.Fatalf("expected one root, got %d: %+v", len(result.Roots), result.Roots)
	}
	if result.Roots[0].CanonicalName != "POST /detect-qr" {
		t.Fatalf("expected detect-qr root, got %+v", result.Roots[0])
	}
	if result.Roots[0].HandlerTarget != "handler_v2.DetectQR" {
		t.Fatalf("expected imported provider wrapper to canonicalize to handler_v2, got %+v", result.Roots[0])
	}
}

func writeBoundaryTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

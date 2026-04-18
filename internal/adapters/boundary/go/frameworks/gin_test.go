package frameworks

import (
	"slices"
	"testing"

	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/boundaryroot"
)

func TestGinDetectorHardensReceiverProvenanceAndComposesFullPaths(t *testing.T) {
	source := []byte(`package main

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type fakeBuilder struct{}
func (fakeBuilder) POST(string, interface{}) {}

type handlers struct{}
func (handlers) Health(*gin.Context) {}
func (handlers) DetectQR(*gin.Context) {}
func (handlers) DetectQRV2(*gin.Context) {}

func main() {
	r := gin.Default()
	api := r

	r.GET("/health", handlers{}.Health)

	v1 := api.Group("/v1")
	cameraV1 := v1.Group("/camera")
	cameraV1.POST("/detect-qr", handlers{}.DetectQR)

	v2 := r.Group("/v2")
	cameraV2 := v2.Group("/camera")
	cameraV2.POST("/detect-qr", handlers{}.DetectQRV2)

	r.Group("/metrics").GET("/prom", gin.WrapH(promhttp.Handler()))

	logger := zap.NewNop()
	logger.Any("zlp_token", "secret")
	logger.Any("error", nil)

	other := fakeBuilder{}
	other.POST("/detect-qr", handlers{}.DetectQR)
}`)

	roots := detectGinRoots(t, source)
	if len(roots) != 4 {
		t.Fatalf("expected 4 Gin boundaries, got %d: %+v", len(roots), roots)
	}

	health := rootByCanonicalName(t, roots, "GET /health")
	if health.HandlerTarget != "handlers{}.Health" {
		t.Fatalf("expected exact health handler target, got %q", health.HandlerTarget)
	}

	v1Detect := rootByCanonicalName(t, roots, "POST /v1/camera/detect-qr")
	v2Detect := rootByCanonicalName(t, roots, "POST /v2/camera/detect-qr")
	if v1Detect.ID == v2Detect.ID {
		t.Fatalf("expected distinct stable IDs for v1/v2 detect-qr routes, got %q", v1Detect.ID)
	}
	if v1Detect.HandlerTarget != "handlers{}.DetectQR" {
		t.Fatalf("expected v1 handler target to preserve exact registration target, got %q", v1Detect.HandlerTarget)
	}
	if v2Detect.HandlerTarget != "handlers{}.DetectQRV2" {
		t.Fatalf("expected v2 handler target to preserve exact registration target, got %q", v2Detect.HandlerTarget)
	}

	metrics := rootByCanonicalName(t, roots, "GET /metrics/prom")
	if metrics.HandlerTarget != "gin.WrapH" {
		t.Fatalf("expected wrapper target to remain exact outer registration target, got %q", metrics.HandlerTarget)
	}

	for _, root := range roots {
		if root.RepositoryID != "repo1" {
			t.Fatalf("expected repo-scoped root, got repo=%q for %+v", root.RepositoryID, root)
		}
		if root.SourceFile != "main.go" {
			t.Fatalf("expected source file main.go, got %q", root.SourceFile)
		}
		if root.SourceEndByte <= root.SourceStartByte {
			t.Fatalf("expected non-empty source span for %+v", root)
		}
		if root.ID == "" || root.ID != boundaryroot.StableID(root) {
			t.Fatalf("expected stable root ID for %+v", root)
		}
	}

	canonicals := make([]string, 0, len(roots))
	for _, root := range roots {
		canonicals = append(canonicals, root.CanonicalName)
	}
	if slices.Contains(canonicals, "Any /zlp_token") || slices.Contains(canonicals, "Any /error") || slices.Contains(canonicals, "POST /detect-qr") {
		t.Fatalf("expected non-Gin selectors to be rejected, got canonicals=%v", canonicals)
	}
}

func detectGinRoots(t *testing.T, source []byte) []boundaryroot.Root {
	t.Helper()

	parser := treesitter.NewGoParser()
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	defer tree.Close()

	file := boundary.ParsedGoFile{
		RepositoryID: "repo1",
		Path:         "main.go",
		Content:      source,
		Root:         tree.RootNode(),
	}

	roots, _ := NewGinDetector().DetectBoundaries(file, nil)
	return roots
}

func rootByCanonicalName(t *testing.T, roots []boundaryroot.Root, canonical string) boundaryroot.Root {
	t.Helper()
	for _, root := range roots {
		if root.CanonicalName == canonical {
			return root
		}
	}
	t.Fatalf("root %q not found in %+v", canonical, roots)
	return boundaryroot.Root{}
}

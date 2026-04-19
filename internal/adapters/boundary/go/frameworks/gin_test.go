package frameworks

import (
	"slices"
	"strings"
	"testing"

	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
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

	files := parseGinFiles(t, map[string][]byte{"main.go": source})
	detector := NewGinDetector()
	roots, diags := detector.DetectBoundaries(files[0], nil)
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

	diagCategories := collectDiagnosticCategories(diags)
	if !slices.Contains(diagCategories, "boundary_unproven_receiver") {
		t.Fatalf("expected unproven receiver diagnostics for rejected route-like calls, got %v", diagCategories)
	}
}

func TestGinDetectorRecoversCrossFileAccessorRoutes(t *testing.T) {
	files := parseGinFiles(t, map[string][]byte{
		"server.go": []byte(`package cmd

import "github.com/gin-gonic/gin"

type service struct {
	engine *gin.Engine
}

func NewService() *service {
	engine := gin.New()
	return &service{engine: engine}
}

func (s *service) Engine() *gin.Engine {
	return s.engine
}`),
		"router.go": []byte(`package cmd

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type handlers struct{}

func (handlers) DetectQR(*gin.Context) {}

func (handlers) GetConfigCameraZPA() gin.HandlerFunc {
	return func(c *gin.Context) {}
}

func (handlers) DetectQRHandler() gin.HandlerFunc {
	return func(c *gin.Context) {}
}

func (s *service) withRouter() {
	engine := s.Engine()
	engine.GET("/health", func(c *gin.Context) {})
	engine.GET("/metrics/prom", gin.WrapH(promhttp.Handler()))

	routeEngine := engine
	v1 := routeEngine.Group("/v1")
	cameraV1 := v1.Group("/camera")
	h := handlers{}
	cameraV1.GET("/config/all", h.GetConfigCameraZPA())
	cameraV1.POST("/detect-qr", h.DetectQRHandler())

	v2 := engine.Group("/v2")
	cameraV2 := v2.Group("/camera")
	cameraV2.POST("/detect-qr", h.DetectQR)
}`),
	})

	detector := NewGinDetector()
	if diags := detector.PreparePackage(files, nil); len(diags) != 0 {
		t.Fatalf("expected package prep diagnostics to stay empty for bounded supported patterns, got %+v", diags)
	}
	state := detector.packageStates[detector.packageKey(files[0])]
	if state == nil {
		t.Fatal("expected prepared package state for cross-file accessor recovery")
	}
	if _, ok := state.fieldContext("service", "engine"); !ok {
		t.Fatalf("expected prepared state to recover service.engine as a Gin context, got %+v; server debug=%s", state.fieldContexts, describeFunctionBodies(files))
	}
	if _, ok := state.methodProvider("service", "Engine"); !ok {
		t.Fatalf("expected prepared state to recover service.Engine as a Gin provider, got %+v", state.methodProviders)
	}

	var router boundary.ParsedGoFile
	for _, file := range files {
		if file.Path == "router.go" {
			router = file
			break
		}
	}
	healthClosure := firstNodeOfKind(router.Root, "func_literal")
	if healthClosure == nil {
		t.Fatal("expected inline health handler closure in router.go")
	}

	symbols := []symbol.Symbol{
		{
			ID:           "synthetic_health_handler",
			RepositoryID: "repo1",
			FilePath:     "router.go",
			Location: symbol.CodeLocation{
				StartLine: uint32(healthClosure.StartPosition().Row + 1),
				StartCol:  uint32(healthClosure.StartPosition().Column + 1),
			},
		},
	}

	roots, diags := detector.DetectBoundaries(router, symbols)
	if len(roots) != 5 {
		t.Fatalf("expected 5 recovered Gin boundaries, got %d: %+v", len(roots), roots)
	}

	required := []string{
		"GET /health",
		"GET /metrics/prom",
		"GET /v1/camera/config/all",
		"POST /v1/camera/detect-qr",
		"POST /v2/camera/detect-qr",
	}
	for _, canonical := range required {
		root := rootByCanonicalName(t, roots, canonical)
		if root.ID != boundaryroot.StableID(root) {
			t.Fatalf("expected stable root ID for %s, got %q", canonical, root.ID)
		}
	}

	if actual := rootByCanonicalName(t, roots, "GET /health").HandlerTarget; actual != "synthetic_health_handler" {
		t.Fatalf("expected inline closure to preserve exact synthetic handler target, got %q", actual)
	}
	if rootByCanonicalName(t, roots, "GET /v1/camera/config/all").HandlerTarget != "h.GetConfigCameraZPA" {
		t.Fatalf("expected factory route target to preserve exact outer callee")
	}
	if rootByCanonicalName(t, roots, "POST /v1/camera/detect-qr").HandlerTarget != "h.DetectQRHandler" {
		t.Fatalf("expected factory route target to preserve exact outer callee")
	}
	if rootByCanonicalName(t, roots, "POST /v2/camera/detect-qr").HandlerTarget != "h.DetectQR" {
		t.Fatalf("expected direct method route target to preserve exact selector target")
	}

	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for supported accessor recovery, got %+v", diags)
	}
}

func parseGinFiles(t *testing.T, files map[string][]byte) []boundary.ParsedGoFile {
	t.Helper()

	parser := treesitter.NewGoParser()
	parsed := make([]boundary.ParsedGoFile, 0, len(files))
	for path, source := range files {
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		t.Cleanup(tree.Close)
		parsed = append(parsed, boundary.ParsedGoFile{
			RepositoryID: "repo1",
			Path:         path,
			PackageName:  packageNameForTest(source),
			Content:      source,
			Root:         tree.RootNode(),
		})
	}
	slices.SortFunc(parsed, func(left, right boundary.ParsedGoFile) int {
		switch {
		case left.Path < right.Path:
			return -1
		case left.Path > right.Path:
			return 1
		default:
			return 0
		}
	})
	return parsed
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

func collectDiagnosticCategories(diags []symbol.Diagnostic) []string {
	categories := make([]string, 0, len(diags))
	for _, diag := range diags {
		categories = append(categories, diag.Category)
	}
	slices.Sort(categories)
	return categories
}

func packageNameForTest(source []byte) string {
	for _, line := range strings.Split(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "package "))
		}
	}
	return ""
}

func describeFunctionBodies(files []boundary.ParsedGoFile) string {
	parts := make([]string, 0, len(files))
	for _, file := range files {
		walk(file.Root, func(node *tree_sitter.Node) bool {
			switch node.Kind() {
			case "function_declaration", "method_declaration":
				body := node.ChildByFieldName("body")
				if body == nil {
					return false
				}
				stmtKinds := make([]string, 0)
				callable := callableName(node, file.Content)
				stmtList := statementListNode(body)
				for i := 0; i < int(stmtList.NamedChildCount()); i++ {
					stmt := stmtList.NamedChild(uint(i))
					entry := stmt.Kind()
					if stmt.Kind() == "return_statement" && stmt.NamedChildCount() > 0 {
						entry += "(" + stmt.NamedChild(0).Kind() + ")"
						if callable == "NewService" {
							desc := []string{}
							walk(stmt, func(current *tree_sitter.Node) bool {
								desc = append(desc, current.Kind())
								return true
							})
							entry += "[" + strings.Join(desc, ">") + "]"
						}
					}
					if stmt.Kind() == "short_var_declaration" && stmt.NamedChildCount() > 1 {
						entry += "(" + stmt.NamedChild(0).Kind() + "->" + stmt.NamedChild(1).Kind() + ")"
					}
					stmtKinds = append(stmtKinds, entry)
				}
				parts = append(parts, file.Path+":"+callable+":"+strings.Join(stmtKinds, ","))
				return false
			default:
				return true
			}
		})
	}
	return strings.Join(parts, " | ")
}

func firstNodeOfKind(root *tree_sitter.Node, kind string) *tree_sitter.Node {
	var found *tree_sitter.Node
	walk(root, func(node *tree_sitter.Node) bool {
		if node.Kind() == kind {
			found = node
			return false
		}
		return true
	})
	return found
}

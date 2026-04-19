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
	packageSymbols := []symbol.Symbol{
		{ID: "sym_health", RepositoryID: "repo1", FilePath: "main.go", PackageName: "main", Name: "Health", Receiver: "handlers", CanonicalName: "main.handlers.Health", Kind: symbol.KindMethod},
		{ID: "sym_detect_v1", RepositoryID: "repo1", FilePath: "main.go", PackageName: "main", Name: "DetectQR", Receiver: "handlers", CanonicalName: "main.handlers.DetectQR", Kind: symbol.KindMethod},
		{ID: "sym_detect_v2", RepositoryID: "repo1", FilePath: "main.go", PackageName: "main", Name: "DetectQRV2", Receiver: "handlers", CanonicalName: "main.handlers.DetectQRV2", Kind: symbol.KindMethod},
	}
	if diags := detector.PreparePackage(files, packageSymbols); len(diags) != 0 {
		t.Fatalf("expected package prep diagnostics to stay empty, got %+v", diags)
	}
	roots, diags := detector.DetectBoundaries(files[0], nil)
	if len(roots) != 4 {
		t.Fatalf("expected 4 Gin boundaries, got %d: %+v", len(roots), roots)
	}

	health := rootByCanonicalName(t, roots, "GET /health")
	if health.HandlerTarget != "main.handlers.Health" {
		t.Fatalf("expected exact health handler target, got %q", health.HandlerTarget)
	}

	v1Detect := rootByCanonicalName(t, roots, "POST /v1/camera/detect-qr")
	v2Detect := rootByCanonicalName(t, roots, "POST /v2/camera/detect-qr")
	if v1Detect.ID == v2Detect.ID {
		t.Fatalf("expected distinct stable IDs for v1/v2 detect-qr routes, got %q", v1Detect.ID)
	}
	if v1Detect.HandlerTarget != "main.handlers.DetectQR" {
		t.Fatalf("expected v1 handler target to preserve exact registration target, got %q", v1Detect.HandlerTarget)
	}
	if v2Detect.HandlerTarget != "main.handlers.DetectQRV2" {
		t.Fatalf("expected v2 handler target to preserve exact registration target, got %q", v2Detect.HandlerTarget)
	}

	metrics := rootByCanonicalName(t, roots, "GET /metrics/prom")
	if metrics.HandlerTarget != "" {
		t.Fatalf("expected support wrapper target to remain unresolved, got %q", metrics.HandlerTarget)
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
	if !slices.Contains(diagCategories, "boundary_unresolved_handler_target") {
		t.Fatalf("expected unresolved handler diagnostic for unsupported wrapper target, got %v", diagCategories)
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
			ID:            "sym_config",
			RepositoryID:  "repo1",
			FilePath:      "router.go",
			PackageName:   "cmd",
			Name:          "GetConfigCameraZPA",
			Receiver:      "handlers",
			CanonicalName: "cmd.handlers.GetConfigCameraZPA",
			Kind:          symbol.KindMethod,
		},
		{
			ID:            "sym_detect_handler",
			RepositoryID:  "repo1",
			FilePath:      "router.go",
			PackageName:   "cmd",
			Name:          "DetectQRHandler",
			Receiver:      "handlers",
			CanonicalName: "cmd.handlers.DetectQRHandler",
			Kind:          symbol.KindMethod,
		},
		{
			ID:            "sym_detect_v2",
			RepositoryID:  "repo1",
			FilePath:      "router.go",
			PackageName:   "cmd",
			Name:          "DetectQR",
			Receiver:      "handlers",
			CanonicalName: "cmd.handlers.DetectQR",
			Kind:          symbol.KindMethod,
		},
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
	if diags := detector.PreparePackage(files, symbols); len(diags) != 0 {
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
	if rootByCanonicalName(t, roots, "GET /v1/camera/config/all").HandlerTarget != "cmd.handlers.GetConfigCameraZPA" {
		t.Fatalf("expected factory route target to preserve exact outer callee")
	}
	if rootByCanonicalName(t, roots, "POST /v1/camera/detect-qr").HandlerTarget != "cmd.handlers.DetectQRHandler" {
		t.Fatalf("expected factory route target to preserve exact outer callee")
	}
	if rootByCanonicalName(t, roots, "POST /v2/camera/detect-qr").HandlerTarget != "cmd.handlers.DetectQR" {
		t.Fatalf("expected direct method route target to preserve exact selector target")
	}
	if rootByCanonicalName(t, roots, "GET /metrics/prom").HandlerTarget != "" {
		t.Fatalf("expected unsupported wrapper target to remain unresolved")
	}

	if !slices.Equal(collectDiagnosticCategories(diags), []string{"boundary_unresolved_handler_target"}) {
		t.Fatalf("expected only unresolved-wrapper diagnostics, got %+v", diags)
	}
}

func TestGinDetectorCanonicalizesConstructorBackedSelectorsAndRejectsDeepFactoryChains(t *testing.T) {
	files := parseGinFiles(t, map[string][]byte{
		"router.go": []byte(`package cmd

import (
	"github.com/gin-gonic/gin"

	"example.com/demo/camerahandler"
)

type cameraHandler interface {
	GetABTestingInfo() gin.HandlerFunc
}

func makeHandler() cameraHandler {
	return camerahandler.NewCameraHandler()
}

func makeDeepHandler() cameraHandler {
	return makeHandler()
}

func register() {
	engine := gin.New()
	handlers := camerahandler.NewCameraHandler()
	alias := handlers
	fromFactory := makeHandler()
	fromDeepFactory := makeDeepHandler()

	engine.GET("/abtest", handlers.GetABTestingInfo())
	engine.GET("/alias", alias.GetABTestingInfo())
	engine.GET("/factory", fromFactory.GetABTestingInfo())
	engine.GET("/deep", fromDeepFactory.GetABTestingInfo())
}`),
	})

	detector := NewGinDetector()
	if diags := detector.PreparePackage(files, nil); len(diags) != 0 {
		t.Fatalf("expected package prep diagnostics for bounded constructor recovery to stay empty, got %+v", diags)
	}
	roots, diags := detector.DetectBoundaries(files[0], nil)
	if len(roots) != 4 {
		t.Fatalf("expected 4 constructor-backed routes, got %d: %+v", len(roots), roots)
	}

	for _, canonical := range []string{"GET /abtest", "GET /alias", "GET /factory"} {
		root := rootByCanonicalName(t, roots, canonical)
		if root.HandlerTarget != "camerahandler.GetABTestingInfo" {
			t.Fatalf("expected package-qualified handler hint for %s, got %+v", canonical, root)
		}
	}

	if rootByCanonicalName(t, roots, "GET /deep").HandlerTarget != "" {
		t.Fatalf("expected deeper factory chain to remain unresolved")
	}

	if !slices.Contains(collectDiagnosticCategories(diags), "boundary_unresolved_handler_target") {
		t.Fatalf("expected unresolved diagnostic for deeper factory chain, got %+v", diags)
	}
}

func TestGinDetectorResolvesImportedProviderWrapperSelectors(t *testing.T) {
	imported := parseGinFiles(t, map[string][]byte{
		"service/camera_v2/camera_v2.go": []byte(`package camera_v2

import handler_v2 "example.com/demo/service/camera_v2/handler"

func InitCameraV2() handler_v2.CameraV2Handler {
	return handler_v2.NewCameraHandler()
}`),
	})
	router := parseGinFiles(t, map[string][]byte{
		"cmd/router.go": []byte(`package cmd

import (
	"github.com/gin-gonic/gin"

	camera_v2 "example.com/demo/service/camera_v2"
)

func register() {
	engine := gin.New()
	handlersV2 := camera_v2.InitCameraV2()
	engine.POST("/detect-qr", handlersV2.DetectQR)
}`),
	})

	detector := NewGinDetector()
	if diags := detector.PreparePackage(imported, nil); len(diags) != 0 {
		t.Fatalf("expected imported package prep diagnostics to stay empty, got %+v", diags)
	}
	if diags := detector.PreparePackage(router, nil); len(diags) != 0 {
		t.Fatalf("expected router package prep diagnostics to stay empty, got %+v", diags)
	}

	roots, diags := detector.DetectBoundaries(router[0], nil)
	if len(roots) != 1 {
		t.Fatalf("expected one route, got %d: %+v", len(roots), roots)
	}

	root := rootByCanonicalName(t, roots, "POST /detect-qr")
	if root.HandlerTarget != "handler_v2.DetectQR" {
		t.Fatalf("expected imported wrapper selector to resolve against handler_v2, got %+v", root)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for supported imported wrapper, got %+v", diags)
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

package frameworks

import (
	"slices"
	"testing"

	"analysis-module/internal/domain/boundaryroot"
)

func TestGRPCGatewayDetectorRequiresProvenGatewayMux(t *testing.T) {
	files := parseGinFiles(t, map[string][]byte{
		"gateway.go": []byte(`package main

import (
	"context"

	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

type fakeMux struct{}

type ServeMux struct{}

type server struct {
	mux *gwruntime.ServeMux
}

type localServer struct {
	mux *ServeMux
}

func RegisterLocalHandler(ctx context.Context, mux *gwruntime.ServeMux, conn interface{}) error {
	return nil
}

func configure(ctx context.Context, s *server, conn interface{}) {
	mux := gwruntime.NewServeMux()
	alias := mux

	RegisterUsersHandlerFromEndpoint(ctx, alias, "localhost:8080")
	RegisterLocalHandler(ctx, mux, conn)
	RegisterFieldHandler(ctx, s.mux, conn)

	local := localServer{}
	RegisterShadowHandler(ctx, local.mux, conn)

	fake := fakeMux{}
	RegisterBadHandler(ctx, fake, conn)
}`),
	})

	detector := NewGRPCGatewayDetector()
	if diags := detector.PreparePackage(files, nil); len(diags) != 0 {
		t.Fatalf("expected no preparation diagnostics, got %+v", diags)
	}

	roots, diags := detector.DetectBoundaries(files[0], nil)
	if len(roots) != 3 {
		t.Fatalf("expected 3 grpc-gateway roots, got %d: %+v", len(roots), roots)
	}

	users := rootByCanonicalName(t, roots, "PROXY RegisterUsersHandlerFromEndpoint")
	if users.Path != "RegisterUsersHandlerFromEndpoint" || users.Method != "PROXY" {
		t.Fatalf("expected deterministic proxy labeling, got %+v", users)
	}
	if users.HandlerTarget != "RegisterUsersHandlerFromEndpoint" {
		t.Fatalf("expected register function name to remain the handler target, got %q", users.HandlerTarget)
	}

	local := rootByCanonicalName(t, roots, "PROXY RegisterLocalHandler")
	if local.HandlerTarget != "RegisterLocalHandler" {
		t.Fatalf("expected local register symbol name, got %q", local.HandlerTarget)
	}

	field := rootByCanonicalName(t, roots, "PROXY RegisterFieldHandler")
	if field.ID == "" || field.ID != boundaryroot.StableID(field) {
		t.Fatalf("expected stable root ID for %+v", field)
	}

	diagCategories := collectDiagnosticCategories(diags)
	if !slices.Contains(diagCategories, "boundary_unproven_receiver") {
		t.Fatalf("expected unproven receiver diagnostic for fake gateway mux, got %+v", diags)
	}
}

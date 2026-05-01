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

func (s *server) configure(ctx context.Context, conn interface{}) error {
	mux := gwruntime.NewServeMux()
	alias := mux

	RegisterUsersHandlerFromEndpoint(ctx, alias, "localhost:8080")
	RegisterLocalHandler(ctx, mux, conn)
	RegisterFieldHandler(ctx, s.mux, conn)
	shortErr := RegisterShortVarHandler(ctx, mux, conn)
	_ = shortErr

	var specErr = RegisterVarSpecHandler(ctx, mux, conn)
	_ = specErr

	var assignErr error
	assignErr = RegisterAssignedHandler(ctx, mux, conn)
	_ = assignErr

	go RegisterGoHandler(ctx, mux, conn)
	defer RegisterDeferHandler(ctx, mux, conn)

	local := localServer{}
	RegisterShadowHandler(ctx, local.mux, conn)

	fake := fakeMux{}
	RegisterBadHandler(ctx, fake, conn)

	return RegisterReturnedHandler(ctx, mux, conn)
}`),
	})

	detector := NewGRPCGatewayDetector()
	if diags := detector.PreparePackage(files, nil); len(diags) != 0 {
		t.Fatalf("expected no preparation diagnostics, got %+v", diags)
	}

	roots, diags := detector.DetectBoundaries(files[0], nil)
	if len(roots) != 9 {
		t.Fatalf("expected 9 grpc-gateway roots, got %d: %+v", len(roots), roots)
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

	shortVar := rootByCanonicalName(t, roots, "PROXY RegisterShortVarHandler")
	if shortVar.HandlerTarget != "RegisterShortVarHandler" {
		t.Fatalf("expected short-var registration handler target, got %q", shortVar.HandlerTarget)
	}

	varSpec := rootByCanonicalName(t, roots, "PROXY RegisterVarSpecHandler")
	if varSpec.HandlerTarget != "RegisterVarSpecHandler" {
		t.Fatalf("expected var-spec registration handler target, got %q", varSpec.HandlerTarget)
	}

	assigned := rootByCanonicalName(t, roots, "PROXY RegisterAssignedHandler")
	if assigned.HandlerTarget != "RegisterAssignedHandler" {
		t.Fatalf("expected assignment registration handler target, got %q", assigned.HandlerTarget)
	}

	returned := rootByCanonicalName(t, roots, "PROXY RegisterReturnedHandler")
	if returned.HandlerTarget != "RegisterReturnedHandler" {
		t.Fatalf("expected return-statement registration handler target, got %q", returned.HandlerTarget)
	}

	goCall := rootByCanonicalName(t, roots, "PROXY RegisterGoHandler")
	if goCall.HandlerTarget != "RegisterGoHandler" {
		t.Fatalf("expected go-statement registration handler target, got %q", goCall.HandlerTarget)
	}

	deferred := rootByCanonicalName(t, roots, "PROXY RegisterDeferHandler")
	if deferred.HandlerTarget != "RegisterDeferHandler" {
		t.Fatalf("expected defer-statement registration handler target, got %q", deferred.HandlerTarget)
	}

	diagCategories := collectDiagnosticCategories(diags)
	if !slices.Contains(diagCategories, "boundary_unproven_receiver") {
		t.Fatalf("expected unproven receiver diagnostic for fake gateway mux, got %+v", diags)
	}
}

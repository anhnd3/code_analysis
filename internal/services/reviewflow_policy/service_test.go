package reviewflow_policy

import (
	"reflect"
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/reviewpack"
)

func TestResolve_ManifestFamilyHasPriority(t *testing.T) {
	service := New()
	result := service.Resolve(ResolveInput{
		Root: entrypoint.Root{
			RootType:  entrypoint.RootHTTP,
			Method:    "GET",
			Path:      "/v1/camera/config/all",
			Framework: "gin",
		},
		ExpectedFamily: FamilyDetectorPipeline,
	})

	if result.Source != reviewpack.PolicySourceManifest {
		t.Fatalf("expected manifest policy source, got %s", result.Source)
	}
	if result.Policy.Family != FamilyDetectorPipeline {
		t.Fatalf("expected detector pipeline policy, got %+v", result.Policy)
	}
}

func TestResolve_RouteMetadataWhenNoManifestFamily(t *testing.T) {
	service := New()
	result := service.Resolve(ResolveInput{
		Root: entrypoint.Root{
			RootType:  entrypoint.RootHTTP,
			Method:    "POST",
			Path:      "/scan360/v1/blacklist/checkimage",
			Framework: "gin",
		},
	})

	if result.Source != reviewpack.PolicySourceRouteMetadata {
		t.Fatalf("expected route metadata source, got %s", result.Source)
	}
	if result.Policy.Family != FamilyBlacklistGate {
		t.Fatalf("expected blacklist gate family, got %+v", result.Policy)
	}
}

func TestResolve_DefaultWhenNoManifestAndNoRouteSignal(t *testing.T) {
	service := New()
	result := service.Resolve(ResolveInput{
		Root: entrypoint.Root{
			RootType: entrypoint.RootHTTP,
			Method:   "POST",
			Path:     "/internal/ping",
		},
	})

	if result.Source != reviewpack.PolicySourceDefault {
		t.Fatalf("expected default source, got %s", result.Source)
	}
	if result.Policy.Family != FamilyDefault {
		t.Fatalf("expected default family, got %+v", result.Policy)
	}
}

func TestResolve_DoesNotDependOnServiceNameOrWorkspacePath(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		RootType:  entrypoint.RootHTTP,
		Method:    "POST",
		Path:      "/v1/camera/detect-qr",
		Framework: "gin",
	}

	first := service.Resolve(ResolveInput{
		Root:          root,
		ServiceName:   "zpa-camera-config-be",
		WorkspacePath: "/tmp/a",
	})
	second := service.Resolve(ResolveInput{
		Root:          root,
		ServiceName:   "totally-different-service",
		WorkspacePath: "/tmp/b",
	})

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected policy resolution to ignore service/workspace context, got %+v vs %+v", first, second)
	}
}

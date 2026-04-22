package participant_classify

import (
	"testing"

	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
)

func TestClassify_RemoteParticipant(t *testing.T) {
	s := New()
	snapshot := graph.GraphSnapshot{}

	node := graph.Node{
		ID:            "unresolved_github.com/stripe/stripe-go",
		CanonicalName: "github.com/stripe/stripe-go",
		Properties:    map[string]string{},
	}

	class := s.Classify(node, snapshot)
	if class.Role != reduced.RoleRemote {
		t.Errorf("expected RoleRemote, got %s", class.Role)
	}
	if class.ShortName != "StripeAPI" {
		t.Errorf("expected StripeAPI, got %s", class.ShortName)
	}
}

func TestClassify_HandlerFromEdge(t *testing.T) {
	s := New()
	nodeID := "node-1"
	snapshot := graph.GraphSnapshot{
		Edges: []graph.Edge{
			{
				Kind: graph.EdgeRegistersBoundary,
				To:   nodeID,
			},
		},
	}

	node := graph.Node{
		ID:         nodeID,
		Properties: map[string]string{"name": "CreateOrder"},
	}

	class := s.Classify(node, snapshot)
	if class.Role != reduced.RoleHandler {
		t.Errorf("expected RoleHandler, got %s", class.Role)
	}
}

func TestClassify_LocalService(t *testing.T) {
	s := New()
	node := graph.Node{
		ID:         "n1",
		Properties: map[string]string{"kind": "struct", "name": "OrderService"},
	}

	class := s.Classify(node, graph.GraphSnapshot{})
	if class.Role != reduced.RoleService {
		t.Errorf("expected RoleService, got %s", class.Role)
	}
}

func TestProfile_InlineHandlerCollapsesToHandlerBucket(t *testing.T) {
	s := New()
	node := graph.Node{
		ID:            "n1",
		CanonicalName: "main.main.$inline_handler_0",
		Properties: map[string]string{
			"name":           "$inline_handler_0",
			"synthetic":      "true",
			"synthetic_kind": "inline_handler",
		},
	}

	profile := s.Profile(node, graph.GraphSnapshot{})
	if profile.Bucket != BucketHandler {
		t.Fatalf("expected handler bucket, got %s", profile.Bucket)
	}
	if !profile.IsInlineHandler {
		t.Fatal("expected inline handler flag")
	}
	if profile.DisplayLabel != "Handler" {
		t.Fatalf("expected Handler display label, got %q", profile.DisplayLabel)
	}
}

func TestProfile_ResponseHelperMapsToResponseBucket(t *testing.T) {
	s := New()
	node := graph.Node{
		ID:            "n2",
		CanonicalName: "http.respondJSON",
		Properties: map[string]string{
			"name": "respondJSON",
		},
	}

	profile := s.Profile(node, graph.GraphSnapshot{})
	if profile.Bucket != BucketResponse {
		t.Fatalf("expected response bucket, got %s", profile.Bucket)
	}
	if !profile.IsResponseHelper {
		t.Fatal("expected response helper flag")
	}
	if profile.DisplayLabel != "Response" {
		t.Fatalf("expected Response display label, got %q", profile.DisplayLabel)
	}
}

func TestProfile_ServiceReceiverMethodMapsToServiceBucket(t *testing.T) {
	s := New()
	node := graph.Node{
		ID:            "n3",
		CanonicalName: "service.service.Predict",
		Properties: map[string]string{
			"name": "Predict",
			"kind": "method",
		},
	}

	profile := s.Profile(node, graph.GraphSnapshot{})
	if profile.Role != reduced.RoleService {
		t.Fatalf("expected service role, got %s", profile.Role)
	}
	if profile.Bucket != BucketService {
		t.Fatalf("expected service bucket, got %s", profile.Bucket)
	}
}

func TestProfile_CheckImageInBlacklistIsBusinessNotValidation(t *testing.T) {
	s := New()
	node := graph.Node{
		ID:            "n4",
		CanonicalName: "service.service.CheckImageInBlacklist",
		Properties: map[string]string{
			"name": "CheckImageInBlacklist",
			"kind": "method",
		},
	}

	profile := s.Profile(node, graph.GraphSnapshot{})
	if profile.IsValidationHelper {
		t.Fatal("expected blacklist service method to avoid validation-helper classification")
	}
	if profile.Bucket != BucketService {
		t.Fatalf("expected blacklist service method to map to service bucket, got %s", profile.Bucket)
	}
}

func TestProfile_InstrumentingRecordMethodMapsToHelper(t *testing.T) {
	s := New()
	node := graph.Node{
		ID:            "n5",
		CanonicalName: "utils.InstrumentingServices.RecordPredictBlacklistResult",
		Properties: map[string]string{
			"name": "RecordPredictBlacklistResult",
			"kind": "method",
		},
	}

	profile := s.Profile(node, graph.GraphSnapshot{})
	if !profile.IsObservability {
		t.Fatal("expected instrumenting service record method to be observability")
	}
	if profile.Bucket != BucketHelper {
		t.Fatalf("expected instrumenting service record method to stay helper, got %s", profile.Bucket)
	}
}

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

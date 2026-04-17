package participant_classify

import (
	"strings"

	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
)

// Classification contains the result of node classification.
type Classification struct {
	Role      reduced.NodeRole
	ShortName string
	IsRemote  bool
}

// Service determines the role and human-friendly name of flow participants.
type Service struct{}

// New creates a new participant classifier.
func New() Service {
	return Service{}
}

// Classify inspects node properties and graph context to determine participant details.
func (s Service) Classify(node graph.Node, snapshot graph.GraphSnapshot) Classification {
	kind := node.Properties["kind"]
	name := node.Properties["name"]
	if name == "" {
		name = s.deriveShortName(node.CanonicalName)
	}

	role := reduced.RoleHelper

	// Check for remote/unresolved first
	if strings.HasPrefix(node.ID, "unresolved_") || s.isRemotePath(node.CanonicalName) {
		return Classification{
			Role:      reduced.RoleRemote,
			ShortName: s.humanizeRemoteName(node.CanonicalName),
			IsRemote:  true,
		}
	}

	// Check if it's a boundary target (handler)
	if s.isBoundaryTarget(node.ID, snapshot) {
		role = reduced.RoleHandler
	} else {
		// Classify based on kind
		switch kind {
		case "route_handler", "grpc_handler":
			role = reduced.RoleHandler
		case "consumer", "producer", "processor":
			role = reduced.RoleProcessor
		case "struct", "interface":
			role = reduced.RoleService
		case "repository":
			role = reduced.RoleRepository
		}
	}

	// Special name-based heuristics if role is still helper
	if role == reduced.RoleHelper {
		if s.isConstructorName(name) {
			role = reduced.RoleConstructor
		} else if strings.HasSuffix(strings.ToLower(name), "repository") || strings.HasSuffix(strings.ToLower(name), "repo") {
			role = reduced.RoleRepository
		} else if strings.HasSuffix(strings.ToLower(name), "service") {
			role = reduced.RoleService
		}
	}

	return Classification{
		Role:      role,
		ShortName: name,
		IsRemote:  false,
	}
}

func (s Service) isBoundaryTarget(nodeID string, snapshot graph.GraphSnapshot) bool {
	for _, edge := range snapshot.Edges {
		if edge.Kind == graph.EdgeRegistersBoundary && edge.To == nodeID {
			return true
		}
	}
	return false
}

func (s Service) isRemotePath(path string) bool {
	// Simple heuristic: if it looks like a domain-separated path but not the current module
	// In a real system we'd check against the workspace module name.
	// For now, assume github.com, google.golang.org, etc are remote.
	prefixes := []string{"github.com/", "google.golang.org/", "cloud.google.com/", "aws/"}
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (s Service) humanizeRemoteName(path string) string {
	path = strings.TrimPrefix(path, "unresolved_")
	
	// Try to get the last part or a well-known service name
	if strings.Contains(path, "github.com/stripe/stripe-go") {
		return "StripeAPI"
	}
	if strings.Contains(path, "google.golang.org/grpc") {
		return "gRPC"
	}
	
	// Default to the last meaningful part of the path
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// Capitalize
		if len(last) > 0 {
			return strings.Title(last)
		}
	}
	return path
}

func (s Service) deriveShortName(canonical string) string {
	idx := strings.LastIndex(canonical, ".")
	if idx >= 0 {
		return canonical[idx+1:]
	}
	return canonical
}

func (s Service) isConstructorName(name string) bool {
	return strings.HasPrefix(name, "New") || strings.HasPrefix(name, "Create") || strings.HasPrefix(name, "Init")
}

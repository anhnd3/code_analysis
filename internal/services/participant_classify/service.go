package participant_classify

import (
	"strings"
	"unicode"

	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
)

// Classification contains the result of node classification.
type Classification struct {
	Role      reduced.NodeRole
	ShortName string
	IsRemote  bool
}

// Bucket is the review-oriented semantic grouping for a participant.
type Bucket string

const (
	BucketClient      Bucket = "client"
	BucketBoundary    Bucket = "boundary"
	BucketHandler     Bucket = "handler"
	BucketValidation  Bucket = "validation"
	BucketSessionAuth Bucket = "session_auth"
	BucketRepo        Bucket = "repo"
	BucketService     Bucket = "service"
	BucketProcessor   Bucket = "processor"
	BucketCoreEngine  Bucket = "core_engine"
	BucketGateway     Bucket = "gateway_client"
	BucketResponse    Bucket = "response"
	BucketAsyncSink   Bucket = "async_sink"
	BucketRemote      Bucket = "remote"
	BucketHelper      Bucket = "helper"
	BucketAlgorithm   Bucket = "algorithm"
)

// Profile is the richer review-oriented participant description used by reviewflow.
type Profile struct {
	Role               reduced.NodeRole
	Bucket             Bucket
	ShortName          string
	DisplayLabel       string
	IsRemote           bool
	IsBoundaryTarget   bool
	IsSynthetic        bool
	SyntheticKind      string
	ParentCanonical    string
	ParentContext      string
	IsClosure          bool
	IsInlineHandler    bool
	IsResponseHelper   bool
	IsValidationHelper bool
	IsSessionAuth      bool
	IsObservability    bool
	IsAsyncLike        bool
	PackageTokens      []string
	NameTokens         []string
	ReceiverToken      string
}

// Service determines the role and human-friendly name of flow participants.
type Service struct{}

// New creates a new participant classifier.
func New() Service {
	return Service{}
}

// Profile inspects node properties and graph context to determine review-oriented
// participant metadata. This is intentionally richer than Classify and is used by
// reviewflow to suppress synthetic closures, inline handlers, and response helpers
// without changing the low-level reduction path.
func (s Service) Profile(node graph.Node, snapshot graph.GraphSnapshot) Profile {
	kind := node.Properties["kind"]
	name := node.Properties["name"]
	if name == "" {
		name = s.deriveShortName(node.CanonicalName)
	}

	profile := Profile{
		ShortName:        name,
		DisplayLabel:     name,
		IsBoundaryTarget: s.isBoundaryTarget(node.ID, snapshot),
		IsSynthetic:      node.Properties["synthetic"] == "true",
		SyntheticKind:    node.Properties["synthetic_kind"],
		ParentCanonical:  node.Properties["parent_canonical"],
		ParentContext:    node.Properties["parent_context"],
		PackageTokens:    tokenizeTokens(packageFromCanonical(node.CanonicalName)),
		NameTokens:       tokenizeTokens(name),
		ReceiverToken:    receiverToken(node.CanonicalName),
	}

	if strings.HasPrefix(node.ID, "unresolved_") || s.isRemotePath(node.CanonicalName) {
		profile.Role = reduced.RoleRemote
		profile.Bucket = BucketRemote
		profile.DisplayLabel = s.humanizeRemoteName(node.CanonicalName)
		profile.IsRemote = true
		return profile
	}

	lowerName := strings.ToLower(name)
	lowerCanonical := strings.ToLower(node.CanonicalName)
	validationTokens := []string{"bind", "validate", "decode", "parse", "sanitize", "verify"}
	profile.IsClosure = profile.SyntheticKind == "closure_return" || strings.Contains(lowerName, "$closure_return_")
	profile.IsInlineHandler = profile.SyntheticKind == "inline_handler" || strings.Contains(lowerName, "$inline_handler_")
	profile.IsResponseHelper = containsAny(lowerName, []string{"respond", "response", "render", "writejson", "write_json", "write", "json", "wrap"}) ||
		containsAny(lowerCanonical, []string{"respond", "response", "render", "writejson", "json", "wrap"})
	profile.IsValidationHelper = containsAny(lowerName, validationTokens) ||
		containsAny(lowerCanonical, validationTokens)
	profile.IsSessionAuth = containsAny(lowerName, []string{"session", "auth", "token", "claims", "context"}) ||
		containsAny(lowerCanonical, []string{"session", "auth", "token", "claims", "context"})
	profile.IsObservability = containsAny(lowerName, []string{"trace", "tracing", "metric", "metrics", "logger", "log", "span", "telemetry"}) ||
		containsAny(lowerCanonical, []string{"trace", "tracing", "metric", "metrics", "logger", "log", "span", "telemetry", "instrument"})
	profile.IsAsyncLike = containsAny(lowerName, []string{"worker", "async", "background", "goroutine", "job"}) ||
		containsAny(lowerCanonical, []string{"worker", "async", "background", "goroutine", "job"})

	profile.Role = reduced.RoleHelper
	if profile.IsBoundaryTarget || profile.IsClosure || profile.IsInlineHandler {
		profile.Role = reduced.RoleHandler
	} else {
		switch kind {
		case "route_handler", "grpc_handler":
			profile.Role = reduced.RoleHandler
		case "consumer", "producer", "processor":
			profile.Role = reduced.RoleProcessor
		case "struct", "interface":
			profile.Role = reduced.RoleService
		case "repository":
			profile.Role = reduced.RoleRepository
		}
	}

	if profile.Role == reduced.RoleHelper {
		switch {
		case looksLikeRepositoryReceiver(profile.ReceiverToken, lowerCanonical):
			profile.Role = reduced.RoleRepository
		case looksLikeServiceReceiver(profile.ReceiverToken, lowerCanonical):
			profile.Role = reduced.RoleService
		case s.isConstructorName(name):
			profile.Role = reduced.RoleConstructor
		case strings.HasSuffix(lowerName, "repository") || strings.HasSuffix(lowerName, "repo"):
			profile.Role = reduced.RoleRepository
		case strings.HasSuffix(lowerName, "service"):
			profile.Role = reduced.RoleService
		}
	}

	switch {
	case profile.IsRemote:
		profile.Bucket = BucketRemote
	case profile.IsResponseHelper:
		profile.Bucket = BucketResponse
		profile.DisplayLabel = "Response"
	case profile.IsObservability:
		profile.Bucket = BucketHelper
		profile.DisplayLabel = "Observability"
	case profile.IsValidationHelper:
		profile.Bucket = BucketValidation
		profile.DisplayLabel = "Validation"
	case profile.IsSessionAuth:
		profile.Bucket = BucketSessionAuth
		profile.DisplayLabel = "Session/Auth"
	case profile.Role == reduced.RoleHandler:
		profile.Bucket = BucketHandler
		profile.DisplayLabel = "Handler"
	case profile.Role == reduced.RoleRepository:
		profile.Bucket = BucketRepo
	case profile.Role == reduced.RoleProcessor:
		if profile.IsAsyncLike {
			profile.Bucket = BucketAsyncSink
			profile.DisplayLabel = "Async Worker"
		} else {
			profile.Bucket = BucketProcessor
		}
	case profile.Role == reduced.RoleService:
		if containsAny(lowerName, []string{"client", "gateway", "proxy"}) || containsAny(lowerCanonical, []string{"client", "gateway", "proxy"}) {
			profile.Bucket = BucketGateway
		} else {
			profile.Bucket = BucketService
		}
	case containsAny(lowerName, []string{"sort", "merge", "classify", "detect", "transform", "select"}):
		profile.Bucket = BucketAlgorithm
	case containsAny(lowerName, []string{"engine", "core", "rule"}):
		profile.Bucket = BucketCoreEngine
	case profile.IsAsyncLike:
		profile.Bucket = BucketAsyncSink
		profile.DisplayLabel = "Async Worker"
	default:
		profile.Bucket = BucketHelper
	}

	if profile.DisplayLabel == name {
		profile.DisplayLabel = humanizeIdentifier(name)
	}
	return profile
}

// Classify inspects node properties and graph context to determine participant details.
func (s Service) Classify(node graph.Node, snapshot graph.GraphSnapshot) Classification {
	profile := s.Profile(node, snapshot)
	if profile.IsRemote {
		return Classification{
			Role:      reduced.RoleRemote,
			ShortName: profile.DisplayLabel,
			IsRemote:  true,
		}
	}

	return Classification{
		Role:      profile.Role,
		ShortName: profile.ShortName,
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

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func packageFromCanonical(canonical string) string {
	idx := strings.LastIndex(canonical, ".")
	if idx < 0 {
		return canonical
	}
	return canonical[:idx]
}

func receiverToken(canonical string) string {
	pkg := packageFromCanonical(canonical)
	lastSlash := strings.LastIndex(pkg, "/")
	if lastSlash >= 0 {
		pkg = pkg[lastSlash+1:]
	}
	parts := strings.Split(pkg, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(parts[len(parts)-1])
}

func looksLikeRepositoryReceiver(receiver, canonical string) bool {
	receiver = strings.ToLower(receiver)
	return strings.Contains(receiver, "repo") || strings.Contains(receiver, "repository") || strings.Contains(canonical, ".repo.")
}

func looksLikeServiceReceiver(receiver, canonical string) bool {
	receiver = strings.ToLower(receiver)
	switch {
	case receiver == "":
		return false
	case strings.Contains(receiver, "service"):
		return true
	case strings.Contains(receiver, "client"), strings.Contains(receiver, "gateway"), strings.Contains(receiver, "proxy"):
		return true
	default:
		return strings.Contains(canonical, ".service.") || strings.Contains(canonical, ".client.")
	}
}

func tokenizeTokens(raw string) []string {
	replacer := strings.NewReplacer("/", " ", ".", " ", "-", " ", "_", " ")
	raw = replacer.Replace(raw)
	var parts []string
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		parts = append(parts, strings.ToLower(token.String()))
		token.Reset()
	}
	for i, r := range raw {
		if unicode.IsUpper(r) && token.Len() > 0 && i > 0 {
			flush()
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			token.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return parts
}

func humanizeIdentifier(raw string) string {
	tokens := tokenizeTokens(raw)
	if len(tokens) == 0 {
		return raw
	}
	for i := range tokens {
		if tokens[i] == "qr" {
			tokens[i] = "QR"
			continue
		}
		if tokens[i] == "http" {
			tokens[i] = "HTTP"
			continue
		}
		if len(tokens[i]) > 0 {
			tokens[i] = strings.ToUpper(tokens[i][:1]) + tokens[i][1:]
		}
	}
	return strings.Join(tokens, " ")
}

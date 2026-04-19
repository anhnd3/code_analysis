package entrypoint

// RootType classifies an executable entry point.
type RootType string

const (
	RootBootstrap RootType = "bootstrap"
	RootHTTP      RootType = "http"
	RootGRPC      RootType = "grpc"
	RootCLI       RootType = "cli"
	RootWorker    RootType = "worker"
	RootConsumer  RootType = "consumer"
)

// Confidence represents how sure the resolver is about a root.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Root is a single resolved entry point in the graph.
type Root struct {
	NodeID        string     `json:"node_id"`
	CanonicalName string     `json:"canonical_name"`
	RootType      RootType   `json:"root_type"`
	Confidence    Confidence `json:"confidence"`
	RepositoryID  string     `json:"repository_id"`
	ServiceID     string     `json:"service_id,omitempty"`
	Framework     string     `json:"framework,omitempty"`
	Method        string     `json:"method,omitempty"`
	Path          string     `json:"path,omitempty"`
	Evidence      string     `json:"evidence,omitempty"`
}

// Result holds all resolved entry points for a snapshot.
type Result struct {
	Roots []Root `json:"roots"`
}

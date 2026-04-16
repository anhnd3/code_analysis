package boundaryroot

// Kind represents the overarching integration shape of the entrypoint.
type Kind string

const (
	KindHTTP        Kind = "http"
	KindGRPC        Kind = "grpc"
	KindHTTPGateway Kind = "http_gateway"
	KindCLI         Kind = "cli"
	KindWorker      Kind = "worker"
	KindConsumer    Kind = "consumer"
)

// Root acts as the generic boundary starting point for a static flow.
type Root struct {
	ID            string
	Kind          Kind
	Framework     string
	Method        string
	Path          string
	CanonicalName string // Synthetic or concrete label
	HandlerTarget string // E.g., function name or literal ref
	SourceFile    string
	SourceExpr    string
	Confidence    string
}

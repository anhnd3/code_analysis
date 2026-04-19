package boundaryroot

import (
	"analysis-module/internal/domain/targetref"
	"strconv"

	"analysis-module/pkg/ids"
)

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
	ID                string
	Kind              Kind
	Framework         string
	Method            string
	Path              string
	CanonicalName     string         // Synthetic or concrete label
	HandlerTarget     string         // E.g., function name or literal ref
	HandlerTargetKind targetref.Kind `json:"-"`
	RepositoryID      string
	SourceFile        string
	SourceStartByte   uint32
	SourceEndByte     uint32
	SourceExpr        string
	Confidence        string
}

// StableID derives a repo-scoped endpoint identifier that survives snapshot rebuilds.
func StableID(root Root) string {
	return ids.Stable(
		"boundary",
		root.RepositoryID,
		root.SourceFile,
		strconv.FormatUint(uint64(root.SourceStartByte), 10),
		strconv.FormatUint(uint64(root.SourceEndByte), 10),
		root.Framework,
		root.Method,
		root.Path,
	)
}

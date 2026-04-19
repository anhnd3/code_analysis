package entrypoint_resolve

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
)

// Service resolves real executable roots from a snapshot and inventory.
type Service struct{}

// New creates an entrypoint resolver.
func New() Service {
	return Service{}
}

// Resolve scans the snapshot and inventory for bootstrap, HTTP, gRPC, CLI,
// worker, and consumer roots, returning them with confidence classifications.
func (s Service) Resolve(snapshot graph.GraphSnapshot, inventory repository.Inventory, detectedRoots []boundaryroot.Root) (entrypoint.Result, error) {
	nodesByID := indexNodes(snapshot.Nodes)
	symbolNodes := filterSymbolNodes(snapshot.Nodes)
	entrypointEdges := filterEdges(snapshot.Edges, graph.EdgeEntrypointTo)

	var roots []entrypoint.Root

	// Track which root kinds are already covered by explicit boundary-detector output.
	// These take precedence and suppress the coarser symbol-kind fallbacks.
	httpCoveredByDetector := false
	grpcCoveredByDetector := false

	// 0. Explicitly detected boundary roots (highest confidence — always included).
	for _, br := range detectedRoots {
		target, ok := nodesByID[br.ID]
		repoID := ""
		if ok {
			repoID = target.RepositoryID
		}

		roots = append(roots, entrypoint.Root{
			NodeID:        br.ID,
			CanonicalName: br.CanonicalName,
			RootType:      mapBoundaryKind(br.Kind),
			Confidence:    entrypoint.ConfidenceHigh, // Explicit detection is highly confident
			RepositoryID:  repoID,
			Evidence:      "framework adapter: " + br.Framework,
		})

		switch br.Kind {
		case boundaryroot.KindHTTP, boundaryroot.KindHTTPGateway:
			httpCoveredByDetector = true
		case boundaryroot.KindGRPC:
			grpcCoveredByDetector = true
		}
	}

	// 0.5 Persisted endpoint roots from the snapshot graph.
	// This keeps root resolution snapshot-first even when live boundary detection
	// is unavailable or intentionally bypassed.
	persistedRoots := s.resolvePersistedBoundaryRoots(snapshot.Nodes)
	roots = append(roots, persistedRoots...)
	for _, root := range persistedRoots {
		switch root.RootType {
		case entrypoint.RootHTTP:
			httpCoveredByDetector = true
		case entrypoint.RootGRPC:
			grpcCoveredByDetector = true
		}
	}

	// 1. Bootstrap roots: main.main, cmd.Execute, Start patterns
	roots = append(roots, s.resolveBootstrapRoots(symbolNodes)...)

	// 2. HTTP route handlers: symbol kind == route_handler
	// Fallback: skip when a framework detector already covered HTTP boundaries.
	if !httpCoveredByDetector {
		roots = append(roots, s.resolveHTTPRoots(symbolNodes)...)
	}

	// 3. gRPC handlers: symbol kind == grpc_handler
	// Fallback: skip when a framework detector already covered gRPC boundaries.
	if !grpcCoveredByDetector {
		roots = append(roots, s.resolveGRPCRoots(symbolNodes)...)
	}

	// 4. CLI roots from entrypoint edges
	roots = append(roots, s.resolveCLIRoots(entrypointEdges, nodesByID)...)

	// 5. Worker/consumer roots: symbol kind == consumer or producer
	roots = append(roots, s.resolveWorkerRoots(symbolNodes)...)

	roots = deduplicateRoots(roots)
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].CanonicalName < roots[j].CanonicalName
	})

	return entrypoint.Result{Roots: roots}, nil
}

func (s Service) resolveBootstrapRoots(nodes []graph.Node) []entrypoint.Root {
	var roots []entrypoint.Root
	for _, n := range nodes {
		name := n.CanonicalName
		kind := nodeSymbolKind(n)
		if kind != string(symbol.KindFunction) && kind != string(symbol.KindMethod) {
			continue
		}
		shortName := shortSymbolName(name)
		switch {
		case shortName == "main" && strings.HasSuffix(packageName(name), "main"):
			roots = append(roots, entrypoint.Root{
				NodeID:        n.ID,
				CanonicalName: name,
				RootType:      entrypoint.RootBootstrap,
				Confidence:    entrypoint.ConfidenceHigh,
				RepositoryID:  n.RepositoryID,
				Evidence:      "main.main pattern",
			})
		case shortName == "Execute" || shortName == "Run" || shortName == "Start":
			if isLikelyCmdPackage(name) {
				roots = append(roots, entrypoint.Root{
					NodeID:        n.ID,
					CanonicalName: name,
					RootType:      entrypoint.RootBootstrap,
					Confidence:    entrypoint.ConfidenceMedium,
					RepositoryID:  n.RepositoryID,
					Evidence:      "cmd entry pattern: " + shortName,
				})
			}
		}
	}
	return roots
}

func (s Service) resolvePersistedBoundaryRoots(nodes []graph.Node) []entrypoint.Root {
	var roots []entrypoint.Root
	for _, n := range nodes {
		if n.Kind != graph.NodeEndpoint {
			continue
		}
		boundaryKind := boundaryroot.Kind(n.Properties["boundary_kind"])
		rootType := mapBoundaryKind(boundaryKind)
		if !isBoundaryRootKind(boundaryKind) {
			continue
		}
		roots = append(roots, entrypoint.Root{
			NodeID:        n.ID,
			CanonicalName: n.CanonicalName,
			RootType:      rootType,
			Confidence:    entrypoint.ConfidenceHigh,
			RepositoryID:  n.RepositoryID,
			Evidence:      "snapshot endpoint node",
		})
	}
	return roots
}

func (s Service) resolveHTTPRoots(nodes []graph.Node) []entrypoint.Root {
	var roots []entrypoint.Root
	for _, n := range nodes {
		if nodeSymbolKind(n) != string(symbol.KindRouteHandler) {
			continue
		}
		roots = append(roots, entrypoint.Root{
			NodeID:        n.ID,
			CanonicalName: n.CanonicalName,
			RootType:      entrypoint.RootHTTP,
			Confidence:    entrypoint.ConfidenceHigh,
			RepositoryID:  n.RepositoryID,
			Evidence:      "route_handler symbol kind",
		})
	}
	return roots
}

func (s Service) resolveGRPCRoots(nodes []graph.Node) []entrypoint.Root {
	var roots []entrypoint.Root
	for _, n := range nodes {
		if nodeSymbolKind(n) != string(symbol.KindGRPCHandler) {
			continue
		}
		roots = append(roots, entrypoint.Root{
			NodeID:        n.ID,
			CanonicalName: n.CanonicalName,
			RootType:      entrypoint.RootGRPC,
			Confidence:    entrypoint.ConfidenceHigh,
			RepositoryID:  n.RepositoryID,
			Evidence:      "grpc_handler symbol kind",
		})
	}
	return roots
}

func (s Service) resolveCLIRoots(entrypointEdges []graph.Edge, nodesByID map[string]graph.Node) []entrypoint.Root {
	var roots []entrypoint.Root
	for _, edge := range entrypointEdges {
		target, ok := nodesByID[edge.To]
		if !ok {
			continue
		}
		roots = append(roots, entrypoint.Root{
			NodeID:        target.ID,
			CanonicalName: target.CanonicalName,
			RootType:      entrypoint.RootCLI,
			Confidence:    entrypoint.ConfidenceHigh,
			RepositoryID:  target.RepositoryID,
			Evidence:      "ENTRYPOINT_TO edge from service",
		})
	}
	return roots
}

func (s Service) resolveWorkerRoots(nodes []graph.Node) []entrypoint.Root {
	var roots []entrypoint.Root
	for _, n := range nodes {
		kind := nodeSymbolKind(n)
		switch kind {
		case string(symbol.KindConsumer):
			roots = append(roots, entrypoint.Root{
				NodeID:        n.ID,
				CanonicalName: n.CanonicalName,
				RootType:      entrypoint.RootConsumer,
				Confidence:    entrypoint.ConfidenceHigh,
				RepositoryID:  n.RepositoryID,
				Evidence:      "consumer symbol kind",
			})
		case string(symbol.KindProducer):
			roots = append(roots, entrypoint.Root{
				NodeID:        n.ID,
				CanonicalName: n.CanonicalName,
				RootType:      entrypoint.RootWorker,
				Confidence:    entrypoint.ConfidenceMedium,
				RepositoryID:  n.RepositoryID,
				Evidence:      "producer symbol kind",
			})
		}
	}
	return roots
}

// --- helpers ---

func indexNodes(nodes []graph.Node) map[string]graph.Node {
	m := make(map[string]graph.Node, len(nodes))
	for _, n := range nodes {
		m[n.ID] = n
	}
	return m
}

func filterSymbolNodes(nodes []graph.Node) []graph.Node {
	var out []graph.Node
	for _, n := range nodes {
		if n.Kind == graph.NodeSymbol || n.Kind == graph.NodeTest {
			out = append(out, n)
		}
	}
	return out
}

func filterEdges(edges []graph.Edge, kind graph.EdgeKind) []graph.Edge {
	var out []graph.Edge
	for _, e := range edges {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func nodeSymbolKind(n graph.Node) string {
	if n.Properties == nil {
		return ""
	}
	return n.Properties["kind"]
}

func shortSymbolName(canonical string) string {
	idx := strings.LastIndex(canonical, ".")
	if idx >= 0 {
		return canonical[idx+1:]
	}
	return canonical
}

func packageName(canonical string) string {
	idx := strings.LastIndex(canonical, ".")
	if idx >= 0 {
		return canonical[:idx]
	}
	return ""
}

func isLikelyCmdPackage(canonical string) bool {
	pkg := packageName(canonical)
	parts := strings.Split(pkg, "/")
	for _, part := range parts {
		if part == "cmd" || part == "command" || part == "cli" {
			return true
		}
	}
	return false
}

func deduplicateRoots(roots []entrypoint.Root) []entrypoint.Root {
	seen := make(map[string]bool, len(roots))
	var out []entrypoint.Root
	for _, r := range roots {
		if seen[r.NodeID] {
			continue
		}
		seen[r.NodeID] = true
		out = append(out, r)
	}
	return out
}

func mapBoundaryKind(kind boundaryroot.Kind) entrypoint.RootType {
	switch kind {
	case boundaryroot.KindHTTP, boundaryroot.KindHTTPGateway:
		return entrypoint.RootHTTP
	case boundaryroot.KindGRPC:
		return entrypoint.RootGRPC
	case boundaryroot.KindCLI:
		return entrypoint.RootCLI
	case boundaryroot.KindWorker:
		return entrypoint.RootWorker
	case boundaryroot.KindConsumer:
		return entrypoint.RootConsumer
	default:
		return entrypoint.RootHTTP // Safe default
	}
}

func isBoundaryRootKind(kind boundaryroot.Kind) bool {
	switch kind {
	case boundaryroot.KindHTTP, boundaryroot.KindHTTPGateway, boundaryroot.KindGRPC, boundaryroot.KindCLI, boundaryroot.KindWorker, boundaryroot.KindConsumer:
		return true
	default:
		return false
	}
}

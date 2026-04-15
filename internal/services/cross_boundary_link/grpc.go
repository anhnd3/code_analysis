package cross_boundary_link

import (
	"strings"

	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
)

// matchGRPC performs protocol-aware gRPC cross-project linking.
//
// Required comparison rules:
//   1. proto package
//   2. service name
//   3. RPC name
//   4. request/response type names
//   5. shared field tag/type compatibility
//
// Full proto-file equality is specifically NOT required.
// Client subset of server proto is allowed.
// Additive extra fields on server are allowed.
func (s Service) matchGRPC(idx *linkIndex) []boundary.Link {
	var links []boundary.Link

	clients := idx.markersByProtocolRole("grpc", "client")
	servers := idx.markersByProtocolRole("grpc", "server")

	for _, client := range clients {
		clientNode, ok := idx.nodeByID[client.NodeID]
		if !ok {
			continue
		}
		clientContract := extractGRPCContract(client, clientNode, idx)
		if clientContract.ServiceName == "" {
			continue
		}

		matched := false
		for _, server := range servers {
			serverNode, ok := idx.nodeByID[server.NodeID]
			if !ok {
				continue
			}
			// Skip same-repo links
			if clientNode.RepositoryID == serverNode.RepositoryID {
				continue
			}

			serverContract := extractGRPCContract(server, serverNode, idx)
			if serverContract.ServiceName == "" {
				continue
			}

			status, evidence := classifyGRPCLink(clientContract, serverContract)
			if status == "" {
				continue
			}

			links = append(links, boundary.Link{
				OutboundNodeID: client.NodeID,
				InboundNodeID:  server.NodeID,
				Protocol:       boundary.ProtocolGRPC,
				Status:         status,
				OutboundRepoID: clientNode.RepositoryID,
				InboundRepoID:  serverNode.RepositoryID,
				Evidence:       evidence,
			})
			matched = true
		}

		if !matched {
			links = append(links, boundary.Link{
				OutboundNodeID: client.NodeID,
				Protocol:       boundary.ProtocolGRPC,
				Status:         boundary.StatusExternalOnly,
				OutboundRepoID: clientNode.RepositoryID,
				Evidence:       "no matching gRPC server in workspace for " + clientContract.ServiceName + "/" + clientContract.RPCName,
			})
		}
	}

	return links
}

// classifyGRPCLink compares two gRPC contracts and returns a status.
func classifyGRPCLink(client, server boundary.Contract) (boundary.LinkStatus, string) {
	// 1. Package comparison (if both present)
	if client.Package != "" && server.Package != "" {
		if normalizePackage(client.Package) != normalizePackage(server.Package) {
			return boundary.StatusMismatch, "gRPC package mismatch: " + client.Package + " vs " + server.Package
		}
	}

	// 2. Service name comparison
	if normalizeServiceName(client.ServiceName) != normalizeServiceName(server.ServiceName) {
		return boundary.StatusMismatch, "gRPC service mismatch: " + client.ServiceName + " vs " + server.ServiceName
	}

	// 3. RPC name comparison
	if client.RPCName != "" && server.RPCName != "" {
		if normalizeRPCName(client.RPCName) != normalizeRPCName(server.RPCName) {
			return boundary.StatusMismatch, "gRPC RPC mismatch: " + client.RPCName + " vs " + server.RPCName
		}
	}

	// 4. Request/response type name comparison
	typeMatch := checkTypeNames(client, server)

	// 5. Field compatibility check (subset matching)
	fieldResult := checkFieldCompatibility(client, server)

	// Classify based on combined results
	switch {
	case typeMatch == matchExact && fieldResult == fieldExact:
		return boundary.StatusConfirmed, "gRPC fully confirmed: " + client.ServiceName + "/" + client.RPCName
	case typeMatch == matchExact && fieldResult == fieldSubset:
		return boundary.StatusCompatibleSubset, "gRPC subset-compatible: client is subset of server for " + client.ServiceName + "/" + client.RPCName
	case typeMatch == matchExact && fieldResult == fieldUnknown:
		return boundary.StatusConfirmed, "gRPC confirmed by type names: " + client.ServiceName + "/" + client.RPCName
	case typeMatch == matchMissing:
		// Types not available, but service+RPC match
		if client.RPCName != "" {
			return boundary.StatusCandidate, "gRPC candidate (types unavailable): " + client.ServiceName + "/" + client.RPCName
		}
		return boundary.StatusCandidate, "gRPC candidate (service name only): " + client.ServiceName
	case fieldResult == fieldMismatch:
		return boundary.StatusMismatch, "gRPC field incompatibility: " + client.ServiceName + "/" + client.RPCName
	default:
		return boundary.StatusCandidate, "gRPC candidate: " + client.ServiceName
	}
}

// --- type matching ---

type typeMatchResult int

const (
	matchExact   typeMatchResult = iota
	matchMissing
	matchMismatch
)

func checkTypeNames(client, server boundary.Contract) typeMatchResult {
	clientHasTypes := client.RequestType != "" || client.ResponseType != ""
	serverHasTypes := server.RequestType != "" || server.ResponseType != ""

	if !clientHasTypes || !serverHasTypes {
		return matchMissing
	}

	reqMatch := true
	if client.RequestType != "" && server.RequestType != "" {
		reqMatch = normalizeTypeName(client.RequestType) == normalizeTypeName(server.RequestType)
	}

	respMatch := true
	if client.ResponseType != "" && server.ResponseType != "" {
		respMatch = normalizeTypeName(client.ResponseType) == normalizeTypeName(server.ResponseType)
	}

	if reqMatch && respMatch {
		return matchExact
	}
	return matchMismatch
}

// --- field compatibility ---

type fieldMatchResult int

const (
	fieldExact    fieldMatchResult = iota
	fieldSubset
	fieldUnknown
	fieldMismatch
)

// checkFieldCompatibility checks if the client's fields are a subset of the server's.
// Additive extra fields on server side are allowed.
// Full proto file equality is NOT required.
func checkFieldCompatibility(client, server boundary.Contract) fieldMatchResult {
	reqResult := compareFields(client.RequestFields, server.RequestFields)
	respResult := compareFields(client.ResponseFields, server.ResponseFields)

	// If neither side has fields, field check is unknown
	if reqResult == fieldUnknown && respResult == fieldUnknown {
		return fieldUnknown
	}

	// Any mismatch is a mismatch
	if reqResult == fieldMismatch || respResult == fieldMismatch {
		return fieldMismatch
	}

	// If any is a subset (and none is mismatch), overall is subset
	if reqResult == fieldSubset || respResult == fieldSubset {
		return fieldSubset
	}

	return fieldExact
}

// compareFields checks if clientFields are a compatible subset of serverFields.
func compareFields(clientFields, serverFields []boundary.ContractField) fieldMatchResult {
	if len(clientFields) == 0 || len(serverFields) == 0 {
		return fieldUnknown
	}

	serverIndex := make(map[string]boundary.ContractField, len(serverFields))
	for _, f := range serverFields {
		serverIndex[f.Name] = f
	}

	allMatched := true
	for _, cf := range clientFields {
		sf, ok := serverIndex[cf.Name]
		if !ok {
			return fieldMismatch
		}
		// Check type compatibility
		if cf.Type != "" && sf.Type != "" && normalizeTypeName(cf.Type) != normalizeTypeName(sf.Type) {
			return fieldMismatch
		}
		// Check tag compatibility (if both have tags)
		if cf.Tag > 0 && sf.Tag > 0 && cf.Tag != sf.Tag {
			return fieldMismatch
		}
		_ = allMatched
	}

	// Client has fewer fields than server? That's a valid subset.
	if len(clientFields) < len(serverFields) {
		return fieldSubset
	}

	return fieldExact
}

// --- contract extraction ---

// extractGRPCContract builds a Contract from node properties and edge evidence.
func extractGRPCContract(marker flow.BoundaryMarker, node graph.Node, idx *linkIndex) boundary.Contract {
	contract := boundary.Contract{}

	// Try to parse from the detail string (e.g., "order.OrderService/CreateOrder")
	if marker.Detail != "" {
		contract = parseGRPCDetail(marker.Detail)
	}

	// Fallback: use node canonical name to infer service/RPC
	if contract.ServiceName == "" {
		contract = inferGRPCFromCanonical(node.CanonicalName)
	}

	// Enrich from node properties
	if node.Properties != nil {
		if pkg := node.Properties["grpc_package"]; pkg != "" {
			contract.Package = pkg
		}
		if reqType := node.Properties["grpc_request_type"]; reqType != "" {
			contract.RequestType = reqType
		}
		if respType := node.Properties["grpc_response_type"]; respType != "" {
			contract.ResponseType = respType
		}
	}

	return contract
}

// parseGRPCDetail parses "package.ServiceName/RPCName" format.
func parseGRPCDetail(detail string) boundary.Contract {
	contract := boundary.Contract{}

	// Format: "package.ServiceName/RPCName" or "ServiceName/RPCName"
	slashIdx := strings.LastIndex(detail, "/")
	if slashIdx >= 0 {
		contract.RPCName = detail[slashIdx+1:]
		serviceQualified := detail[:slashIdx]
		dotIdx := strings.LastIndex(serviceQualified, ".")
		if dotIdx >= 0 {
			contract.Package = serviceQualified[:dotIdx]
			contract.ServiceName = serviceQualified[dotIdx+1:]
		} else {
			contract.ServiceName = serviceQualified
		}
	} else {
		// No slash, treat as service name
		dotIdx := strings.LastIndex(detail, ".")
		if dotIdx >= 0 {
			contract.Package = detail[:dotIdx]
			contract.ServiceName = detail[dotIdx+1:]
		} else {
			contract.ServiceName = detail
		}
	}

	return contract
}

// inferGRPCFromCanonical attempts to extract service/rpc from symbol canonical name.
func inferGRPCFromCanonical(canonical string) boundary.Contract {
	contract := boundary.Contract{}
	parts := strings.Split(canonical, ".")
	if len(parts) >= 2 {
		contract.ServiceName = parts[len(parts)-2]
		contract.RPCName = parts[len(parts)-1]
	} else if len(parts) == 1 {
		contract.ServiceName = parts[0]
	}
	return contract
}

// --- normalization ---

func normalizePackage(pkg string) string {
	return strings.ToLower(strings.TrimSpace(pkg))
}

func normalizeServiceName(name string) string {
	name = strings.TrimSpace(name)
	// Strip common suffixes for matching
	name = strings.TrimSuffix(name, "Service")
	name = strings.TrimSuffix(name, "Server")
	name = strings.TrimSuffix(name, "Client")
	return strings.ToLower(name)
}

func normalizeRPCName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeTypeName(name string) string {
	// Strip package qualifiers for cross-project comparison
	idx := strings.LastIndex(name, ".")
	if idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ToLower(strings.TrimSpace(name))
}

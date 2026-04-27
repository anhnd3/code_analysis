package cross_boundary_link

import (
	"testing"

	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
)

func TestGRPC_FullyConfirmed(t *testing.T) {
	snapshot, flows := grpcFixture(
		grpcEndpoint{nodeID: "client1", repoID: "repo_a", role: "client",
			detail: "order.OrderService/CreateOrder",
			props: map[string]string{
				"kind": "function", "name": "CreateOrder",
				"grpc_request_type": "CreateOrderRequest", "grpc_response_type": "CreateOrderResponse",
			}},
		grpcEndpoint{nodeID: "server1", repoID: "repo_b", role: "server",
			detail: "order.OrderService/CreateOrder",
			props: map[string]string{
				"kind": "grpc_handler", "name": "CreateOrder",
				"grpc_request_type": "CreateOrderRequest", "grpc_response_type": "CreateOrderResponse",
			}},
	)

	bundle, err := New().Build(snapshot, repository.Inventory{}, flows)
	if err != nil {
		t.Fatal(err)
	}

	link := findLink(t, bundle, "client1", "server1")
	if link.Status != boundary.StatusConfirmed {
		t.Errorf("expected confirmed, got %s", link.Status)
	}
	if link.Protocol != boundary.ProtocolGRPC {
		t.Errorf("expected grpc, got %s", link.Protocol)
	}
}

func TestGRPC_SubsetCompatible(t *testing.T) {
	// Client uses fewer fields than server (additive server fields allowed)
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "client1", Kind: graph.NodeSymbol, CanonicalName: "client.CreateOrder", RepositoryID: "repo_a",
				Properties: map[string]string{
					"kind": "function", "name": "CreateOrder",
					"grpc_package": "order", "grpc_request_type": "CreateOrderRequest", "grpc_response_type": "CreateOrderResponse",
				}},
			{ID: "server1", Kind: graph.NodeSymbol, CanonicalName: "server.CreateOrder", RepositoryID: "repo_b",
				Properties: map[string]string{
					"kind": "grpc_handler", "name": "CreateOrder",
					"grpc_package": "order", "grpc_request_type": "CreateOrderRequest", "grpc_response_type": "CreateOrderResponse",
				}},
		},
	}

	// Manually test classifyGRPCLink with field data
	clientContract := boundary.Contract{
		Package: "order", ServiceName: "OrderService", RPCName: "CreateOrder",
		RequestType: "CreateOrderRequest", ResponseType: "CreateOrderResponse",
		RequestFields: []boundary.ContractField{
			{Name: "user_id", Type: "string", Tag: 1},
			{Name: "amount", Type: "int64", Tag: 2},
		},
	}
	serverContract := boundary.Contract{
		Package: "order", ServiceName: "OrderService", RPCName: "CreateOrder",
		RequestType: "CreateOrderRequest", ResponseType: "CreateOrderResponse",
		RequestFields: []boundary.ContractField{
			{Name: "user_id", Type: "string", Tag: 1},
			{Name: "amount", Type: "int64", Tag: 2},
			{Name: "currency", Type: "string", Tag: 3}, // additive field
		},
	}

	status, _ := classifyGRPCLink(clientContract, serverContract)
	if status != boundary.StatusCompatibleSubset {
		t.Errorf("expected compatible_subset, got %s", status)
	}

	// Verify the snapshot still compiles through Build
	flows := flow.Bundle{
		BoundaryMarkers: []flow.BoundaryMarker{
			{NodeID: "client1", Protocol: "grpc", Role: "client", Detail: "order.OrderService/CreateOrder"},
			{NodeID: "server1", Protocol: "grpc", Role: "server", Detail: "order.OrderService/CreateOrder"},
		},
	}
	_, err := New().Build(snapshot, repository.Inventory{}, flows)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGRPC_Mismatch_DifferentService(t *testing.T) {
	clientContract := boundary.Contract{
		Package: "order", ServiceName: "OrderService", RPCName: "CreateOrder",
	}
	serverContract := boundary.Contract{
		Package: "order", ServiceName: "PaymentService", RPCName: "CreatePayment",
	}
	status, _ := classifyGRPCLink(clientContract, serverContract)
	if status != boundary.StatusMismatch {
		t.Errorf("expected mismatch, got %s", status)
	}
}

func TestGRPC_Mismatch_DifferentPackage(t *testing.T) {
	clientContract := boundary.Contract{
		Package: "order.v1", ServiceName: "OrderService", RPCName: "CreateOrder",
	}
	serverContract := boundary.Contract{
		Package: "payment.v1", ServiceName: "OrderService", RPCName: "CreateOrder",
	}
	status, _ := classifyGRPCLink(clientContract, serverContract)
	if status != boundary.StatusMismatch {
		t.Errorf("expected mismatch, got %s", status)
	}
}

func TestGRPC_Mismatch_IncompatibleFields(t *testing.T) {
	clientContract := boundary.Contract{
		ServiceName: "OrderService", RPCName: "CreateOrder",
		RequestType: "CreateOrderRequest", ResponseType: "CreateOrderResponse",
		RequestFields: []boundary.ContractField{
			{Name: "user_id", Type: "string", Tag: 1},
			{Name: "nonexistent_field", Type: "bool", Tag: 99},
		},
	}
	serverContract := boundary.Contract{
		ServiceName: "OrderService", RPCName: "CreateOrder",
		RequestType: "CreateOrderRequest", ResponseType: "CreateOrderResponse",
		RequestFields: []boundary.ContractField{
			{Name: "user_id", Type: "string", Tag: 1},
			{Name: "amount", Type: "int64", Tag: 2},
		},
	}
	status, _ := classifyGRPCLink(clientContract, serverContract)
	if status != boundary.StatusMismatch {
		t.Errorf("expected mismatch, got %s", status)
	}
}

func TestGRPC_Candidate_NoTypes(t *testing.T) {
	// Service+RPC match but no type info available
	clientContract := boundary.Contract{
		Package: "order", ServiceName: "OrderService", RPCName: "CreateOrder",
	}
	serverContract := boundary.Contract{
		Package: "order", ServiceName: "OrderService", RPCName: "CreateOrder",
	}
	status, _ := classifyGRPCLink(clientContract, serverContract)
	if status != boundary.StatusCandidate {
		t.Errorf("expected candidate, got %s", status)
	}
}

func TestGRPC_ExternalOnly(t *testing.T) {
	snapshot, flows := grpcFixture(
		grpcEndpoint{nodeID: "client1", repoID: "repo_a", role: "client",
			detail: "order.OrderService/CreateOrder",
			props:  map[string]string{"kind": "function", "name": "CreateOrder"}},
		// No matching server
	)

	bundle, err := New().Build(snapshot, repository.Inventory{}, flows)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, link := range bundle.Links {
		if link.OutboundNodeID == "client1" && link.Status == boundary.StatusExternalOnly {
			found = true
		}
	}
	if !found {
		t.Errorf("expected external_only link for client1")
	}
}

func TestGRPC_SameRepoSkipped(t *testing.T) {
	snapshot, flows := grpcFixture(
		grpcEndpoint{nodeID: "client1", repoID: "repo_a", role: "client",
			detail: "order.OrderService/CreateOrder",
			props:  map[string]string{"kind": "function", "name": "CreateOrder"}},
		grpcEndpoint{nodeID: "server1", repoID: "repo_a", role: "server",
			detail: "order.OrderService/CreateOrder",
			props:  map[string]string{"kind": "grpc_handler", "name": "CreateOrder"}},
	)

	bundle, err := New().Build(snapshot, repository.Inventory{}, flows)
	if err != nil {
		t.Fatal(err)
	}

	// Should not create a cross-project link for same-repo
	for _, link := range bundle.Links {
		if link.OutboundNodeID == "client1" && link.InboundNodeID == "server1" && link.Status == boundary.StatusConfirmed {
			t.Errorf("should not link endpoints in the same repo")
		}
	}
}

func TestGRPC_ParseDetail(t *testing.T) {
	cases := []struct {
		detail  string
		pkg     string
		svc     string
		rpc     string
	}{
		{"order.OrderService/CreateOrder", "order", "OrderService", "CreateOrder"},
		{"OrderService/CreateOrder", "", "OrderService", "CreateOrder"},
		{"com.example.OrderService/ListOrders", "com.example", "OrderService", "ListOrders"},
		{"OrderService", "", "OrderService", ""},
		{"order.v1.OrderService/GetOrder", "order.v1", "OrderService", "GetOrder"},
	}
	for _, tc := range cases {
		c := parseGRPCDetail(tc.detail)
		if c.Package != tc.pkg {
			t.Errorf("parseGRPCDetail(%q): package=%q, want %q", tc.detail, c.Package, tc.pkg)
		}
		if c.ServiceName != tc.svc {
			t.Errorf("parseGRPCDetail(%q): service=%q, want %q", tc.detail, c.ServiceName, tc.svc)
		}
		if c.RPCName != tc.rpc {
			t.Errorf("parseGRPCDetail(%q): rpc=%q, want %q", tc.detail, c.RPCName, tc.rpc)
		}
	}
}

func TestGRPC_NormalizeServiceName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"OrderService", "order"},
		{"OrderServer", "order"},
		{"OrderClient", "order"},
		{"Order", "order"},
	}
	for _, tc := range cases {
		got := normalizeServiceName(tc.input)
		if got != tc.want {
			t.Errorf("normalizeServiceName(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFieldCompatibility_Exact(t *testing.T) {
	client := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
		{Name: "name", Type: "string", Tag: 2},
	}
	server := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
		{Name: "name", Type: "string", Tag: 2},
	}
	result := compareFields(client, server)
	if result != fieldExact {
		t.Errorf("expected fieldExact, got %d", result)
	}
}

func TestFieldCompatibility_Subset(t *testing.T) {
	client := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
	}
	server := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
		{Name: "extra_field", Type: "string", Tag: 2},
	}
	result := compareFields(client, server)
	if result != fieldSubset {
		t.Errorf("expected fieldSubset, got %d", result)
	}
}

func TestFieldCompatibility_MissingField(t *testing.T) {
	client := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
		{Name: "missing", Type: "bool", Tag: 5},
	}
	server := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
		{Name: "name", Type: "string", Tag: 2},
	}
	result := compareFields(client, server)
	if result != fieldMismatch {
		t.Errorf("expected fieldMismatch, got %d", result)
	}
}

func TestFieldCompatibility_TagMismatch(t *testing.T) {
	client := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
	}
	server := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 99},
	}
	result := compareFields(client, server)
	if result != fieldMismatch {
		t.Errorf("expected fieldMismatch, got %d", result)
	}
}

func TestFieldCompatibility_TypeMismatch(t *testing.T) {
	client := []boundary.ContractField{
		{Name: "id", Type: "string", Tag: 1},
	}
	server := []boundary.ContractField{
		{Name: "id", Type: "int64", Tag: 1},
	}
	result := compareFields(client, server)
	if result != fieldMismatch {
		t.Errorf("expected fieldMismatch, got %d", result)
	}
}

// --- test fixture helpers ---

type grpcEndpoint struct {
	nodeID string
	repoID string
	role   string
	detail string
	props  map[string]string
}

func grpcFixture(endpoints ...grpcEndpoint) (graph.GraphSnapshot, flow.Bundle) {
	var nodes []graph.Node
	var markers []flow.BoundaryMarker

	for _, ep := range endpoints {
		nodes = append(nodes, graph.Node{
			ID:            ep.nodeID,
			Kind:          graph.NodeSymbol,
			CanonicalName: ep.detail,
			RepositoryID:  ep.repoID,
			Properties:    ep.props,
		})
		markers = append(markers, flow.BoundaryMarker{
			NodeID:   ep.nodeID,
			Protocol: "grpc",
			Role:     ep.role,
			Detail:   ep.detail,
		})
	}

	return graph.GraphSnapshot{Nodes: nodes}, flow.Bundle{BoundaryMarkers: markers}
}

func findLink(t *testing.T, bundle boundary.Bundle, outbound, inbound string) boundary.Link {
	t.Helper()
	for _, l := range bundle.Links {
		if l.OutboundNodeID == outbound && l.InboundNodeID == inbound {
			return l
		}
	}
	t.Fatalf("link from %s to %s not found (have %d links)", outbound, inbound, len(bundle.Links))
	return boundary.Link{}
}

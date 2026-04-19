package flow_stitch

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
)

func TestBuild_BootstrapChain(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "app.NewServer", "function"),
			testSymbolNode("n3", "app.Server.Start", "method"),
		},
		Edges: []graph.Edge{
			callEdge("e1", "n1", "n2"),
			callEdge("e2", "n2", "n3"),
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", CanonicalName: "main.main", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(bundle.Chains))
	}
	chain := bundle.Chains[0]
	if chain.RootNodeID != "n1" {
		t.Errorf("expected root n1, got %s", chain.RootNodeID)
	}
	if len(chain.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(chain.Steps))
	}
}

func TestBuild_ConstructorDetection(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "app.NewService", "function"),
		},
		Edges: []graph.Edge{
			callEdge("e1", "n1", "n2"),
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 || len(bundle.Chains[0].Steps) != 1 {
		t.Fatalf("expected 1 chain with 1 step")
	}
	if bundle.Chains[0].Steps[0].Kind != flow.StepConstruct {
		t.Errorf("expected construct step, got %s", bundle.Chains[0].Steps[0].Kind)
	}
}

func TestBuild_BoundaryMarkerHTTP(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{
				ID: "n1", Kind: graph.NodeSymbol, CanonicalName: "api.GetUser",
				Properties: map[string]string{"kind": string(symbol.KindRouteHandler), "name": "GetUser"},
			},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootHTTP, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range bundle.BoundaryMarkers {
		if m.NodeID == "n1" && m.Protocol == "http" && m.Role == "server" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected HTTP server boundary marker for n1")
	}
}

func TestBuild_BoundaryMarkerOutboundGRPC(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "client.Call", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e1", Kind: graph.EdgeCalls, From: "n1", To: "n2", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e2", Kind: graph.EdgeCallsGRPC, From: "n2", To: "remote1", Evidence: graph.Evidence{Details: "order.OrderService/CreateOrder"}, Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range bundle.BoundaryMarkers {
		if m.NodeID == "n2" && m.Protocol == "grpc" && m.Role == "client" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gRPC client boundary marker for n2")
	}
}

func TestBuild_CycleProtection(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "a.Foo", "function"),
			testSymbolNode("n2", "a.Bar", "function"),
		},
		Edges: []graph.Edge{
			callEdge("e1", "n1", "n2"),
			callEdge("e2", "n2", "n1"),
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(bundle.Chains))
	}
	// Should not loop forever
	if len(bundle.Chains[0].Steps) != 1 {
		t.Errorf("expected 1 step (cycle broken), got %d", len(bundle.Chains[0].Steps))
	}
}

func TestBuild_InferredEdge(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "util.Helper", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e1", Kind: graph.EdgeCalls, From: "n1", To: "n2",
				Confidence: graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.6}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains[0].Steps) != 1 {
		t.Fatalf("expected 1 step")
	}
	if !bundle.Chains[0].Steps[0].Inferred {
		t.Errorf("expected step to be marked as inferred")
	}
}

func TestBuild_EmptySnapshot(t *testing.T) {
	snapshot := graph.GraphSnapshot{}
	roots := entrypoint.Result{}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 0 {
		t.Errorf("expected 0 chains, got %d", len(bundle.Chains))
	}
}

func TestBuild_ExecutionOrder(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("call_target", "app.Call", "function"),
			testSymbolNode("branch_target", "app.Branch", "function"),
			testSymbolNode("spawn_target", "app.Spawn", "function"),
			testSymbolNode("wait_target", "app.Wait", "function"),
			testSymbolNode("defer_target", "app.Defer", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_defer", Kind: graph.EdgeDefers, From: "n1", To: "defer_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_wait", Kind: graph.EdgeWaitsOn, From: "n1", To: "wait_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_spawn", Kind: graph.EdgeSpawns, From: "n1", To: "spawn_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_branch", Kind: graph.EdgeBranches, From: "n1", To: "branch_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_call", Kind: graph.EdgeCalls, From: "n1", To: "call_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}

	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain")
	}
	steps := bundle.Chains[0].Steps
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	// Order: Call (0), Branch (1), Spawn (2), Wait (3), Defer (4)
	expectedOrders := []flow.StepKind{
		flow.StepCall,
		flow.StepBranch,
		flow.StepAsync,
		flow.StepWait,
		flow.StepDefer,
	}

	for i, expectedKind := range expectedOrders {
		if steps[i].Kind != expectedKind {
			t.Errorf("step %d: expected kind %s, got %s", i, expectedKind, steps[i].Kind)
		}
	}
}

func TestBuild_HTTPRootUsesSemanticSpineAndPreservesSideEdges(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			endpointNode("endpoint_orders", "GET /orders"),
			testSymbolNode("handler_factory", "api.MakeOrdersHandler", "function"),
			testSymbolNode("handler_closure", "api.MakeOrdersHandler.$closure_return_0", "function"),
			testSymbolNode("service_target", "svc.OrderService", "function"),
			testSymbolNode("log_target", "obs.LogRequest", "function"),
			testSymbolNode("branch_target", "api.BranchArm", "function"),
			testSymbolNode("spawn_target", "api.SpawnWorker", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_orders", To: "handler_factory", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_return", Kind: graph.EdgeReturnsHandler, From: "handler_factory", To: "handler_closure", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_log", Kind: graph.EdgeCalls, From: "handler_closure", To: "log_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_service", Kind: graph.EdgeCalls, From: "handler_closure", To: "service_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_branch", Kind: graph.EdgeBranches, From: "handler_closure", To: "branch_target", Evidence: graph.Evidence{Source: "if featureEnabled"}, Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_spawn", Kind: graph.EdgeSpawns, From: "handler_closure", To: "spawn_target", Evidence: graph.Evidence{Source: "go dispatch()"}, Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "endpoint_orders",
			CanonicalName: "GET /orders",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(bundle.Chains))
	}

	steps := bundle.Chains[0].Steps
	if len(steps) != 5 {
		t.Fatalf("expected 5 semantic-spine steps, got %d: %+v", len(steps), steps)
	}
	if steps[0].ToNodeID != "handler_factory" || steps[0].Kind != flow.StepBoundary {
		t.Fatalf("expected boundary registration step first, got %+v", steps[0])
	}
	if steps[1].ToNodeID != "handler_closure" || steps[1].Kind != flow.StepCall {
		t.Fatalf("expected RETURNS_HANDLER peel second, got %+v", steps[1])
	}
	if steps[2].ToNodeID != "service_target" || steps[2].Kind != flow.StepCall {
		t.Fatalf("expected business service call to win over noise, got %+v", steps[2])
	}
	if steps[3].Kind != flow.StepBranch || steps[4].Kind != flow.StepAsync {
		t.Fatalf("expected side structure after the selected mainline, got %+v", steps[3:])
	}
}

func TestBuild_HTTPRootPeelsReturnsHandlerBeforeSearchingCalls(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			endpointNode("endpoint_orders", "GET /orders"),
			testSymbolNode("handler_factory", "api.MakeOrdersHandler", "function"),
			testSymbolNode("handler_closure", "api.MakeOrdersHandler.$closure_return_0", "function"),
			testSymbolNode("factory_helper", "svc.FactoryHelper", "function"),
			testSymbolNode("service_target", "svc.OrderService", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_orders", To: "handler_factory", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_return", Kind: graph.EdgeReturnsHandler, From: "handler_factory", To: "handler_closure", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_factory_helper", Kind: graph.EdgeCalls, From: "handler_factory", To: "factory_helper", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_service", Kind: graph.EdgeCalls, From: "handler_closure", To: "service_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "endpoint_orders",
			CanonicalName: "GET /orders",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(bundle.Chains))
	}
	steps := bundle.Chains[0].Steps
	if len(steps) < 3 {
		t.Fatalf("expected boundary, peel, and business call, got %+v", steps)
	}
	if steps[1].ToNodeID != "handler_closure" {
		t.Fatalf("expected RETURNS_HANDLER peel before call search, got %+v", steps)
	}
	if steps[2].ToNodeID != "service_target" {
		t.Fatalf("expected business call from closure to win after peel, got %+v", steps)
	}
}

func TestBuild_HTTPRootBeamSearchUsesDeterministicTieBreaks(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("handler_root", "api.HandleOrders", string(symbol.KindRouteHandler)),
			testSymbolNode("service_b", "svc.BillingService", "function"),
			testSymbolNode("service_a", "svc.AccountService", "function"),
		},
		Edges: []graph.Edge{
			{
				ID:         "e_billing",
				Kind:       graph.EdgeCalls,
				From:       "handler_root",
				To:         "service_b",
				Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1},
				Properties: map[string]string{"order_index": "2"},
			},
			{
				ID:         "e_account",
				Kind:       graph.EdgeCalls,
				From:       "handler_root",
				To:         "service_a",
				Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1},
				Properties: map[string]string{"order_index": "1"},
			},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "handler_root",
			CanonicalName: "api.HandleOrders",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 || len(bundle.Chains[0].Steps) != 1 {
		t.Fatalf("expected one selected semantic step, got %+v", bundle.Chains)
	}
	if got := bundle.Chains[0].Steps[0].ToNodeID; got != "service_a" {
		t.Fatalf("expected lower order_index edge to win deterministic tie-break, got %s", got)
	}
}

func TestBuild_HTTPRootBeamSearchStopsAtMaxDepth(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("handler_root", "api.HandleOrders", string(symbol.KindRouteHandler)),
			testSymbolNode("service_one", "svc.OrderServiceOne", "function"),
			testSymbolNode("service_two", "svc.OrderServiceTwo", "function"),
			testSymbolNode("service_three", "svc.OrderServiceThree", "function"),
			testSymbolNode("service_four", "svc.OrderServiceFour", "function"),
			testSymbolNode("service_five", "svc.OrderServiceFive", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e1", Kind: graph.EdgeCalls, From: "handler_root", To: "service_one", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e2", Kind: graph.EdgeCalls, From: "service_one", To: "service_two", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e3", Kind: graph.EdgeCalls, From: "service_two", To: "service_three", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e4", Kind: graph.EdgeCalls, From: "service_three", To: "service_four", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e5", Kind: graph.EdgeCalls, From: "service_four", To: "service_five", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "handler_root",
			CanonicalName: "api.HandleOrders",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %+v", bundle.Chains)
	}
	steps := bundle.Chains[0].Steps
	if len(steps) != semanticMaxDepth {
		t.Fatalf("expected semantic search to stop at max depth %d, got %+v", semanticMaxDepth, steps)
	}
	if got := steps[len(steps)-1].ToNodeID; got != "service_four" {
		t.Fatalf("expected bounded search to stop before service_five, got %s", got)
	}
}

func TestBuild_HTTPFallbackRootAllowsInferredEligibleBusinessCall(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("handler_root", "api.HandleOrders", string(symbol.KindRouteHandler)),
			testSymbolNode("service_target", "svc.OrderService", "function"),
			testSymbolNode("log_target", "obs.LogOrders", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_inferred_service", Kind: graph.EdgeCalls, From: "handler_root", To: "service_target", Confidence: graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.6}},
			{ID: "e_confirmed_log", Kind: graph.EdgeCalls, From: "handler_root", To: "log_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "handler_root",
			CanonicalName: "api.HandleOrders",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 || len(bundle.Chains[0].Steps) != 1 {
		t.Fatalf("expected one semantic step, got %+v", bundle.Chains)
	}
	if got := bundle.Chains[0].Steps[0].ToNodeID; got != "service_target" {
		t.Fatalf("expected inferred service call to remain eligible and win on score, got %s", got)
	}
	if !bundle.Chains[0].Steps[0].Inferred {
		t.Fatalf("expected inferred service edge to remain marked inferred")
	}
}

func TestBuildAudit_ClassifiesBusinessWarnings(t *testing.T) {
	service := New()

	t.Run("noise_frontier", func(t *testing.T) {
		snapshot := graph.GraphSnapshot{
			WorkspaceID: "ws_noise",
			ID:          "snap_noise",
			Nodes: []graph.Node{
				endpointNode("endpoint_noise", "GET /noise"),
				testSymbolNode("handler_noise", "api.HandleNoise", "function"),
				testSymbolNode("log_target", "obs.LogRequest", "function"),
			},
			Edges: []graph.Edge{
				{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_noise", To: "handler_noise", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
				{ID: "e_log", Kind: graph.EdgeCalls, From: "handler_noise", To: "log_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			},
		}
		roots := entrypoint.Result{
			Roots: []entrypoint.Root{{
				NodeID:        "endpoint_noise",
				CanonicalName: "GET /noise",
				RootType:      entrypoint.RootHTTP,
				RepositoryID:  "repo1",
			}},
		}

		bundle, err := service.Build(snapshot, roots, repository.Inventory{})
		if err != nil {
			t.Fatal(err)
		}
		audit := service.BuildAudit(snapshot, roots, bundle)
		if len(audit.Roots) != 1 {
			t.Fatalf("expected 1 audit root, got %+v", audit.Roots)
		}
		if len(audit.Roots[0].FirstBusinessCalls) != 0 {
			t.Fatalf("expected no first business calls for noise-only frontier, got %+v", audit.Roots[0].FirstBusinessCalls)
		}
		assertAuditWarningKind(t, audit.Roots[0].Warnings, "business_frontier_blocked_by_noise")
	})

	t.Run("tiny_handler", func(t *testing.T) {
		snapshot := graph.GraphSnapshot{
			WorkspaceID: "ws_tiny",
			ID:          "snap_tiny",
			Nodes: []graph.Node{
				endpointNode("endpoint_tiny", "GET /tiny"),
				testSymbolNode("handler_tiny", "api.HandleTiny", "function"),
			},
			Edges: []graph.Edge{
				{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_tiny", To: "handler_tiny", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			},
		}
		roots := entrypoint.Result{
			Roots: []entrypoint.Root{{
				NodeID:        "endpoint_tiny",
				CanonicalName: "GET /tiny",
				RootType:      entrypoint.RootHTTP,
				RepositoryID:  "repo1",
			}},
		}

		bundle, err := service.Build(snapshot, roots, repository.Inventory{})
		if err != nil {
			t.Fatal(err)
		}
		audit := service.BuildAudit(snapshot, roots, bundle)
		if len(audit.Roots) != 1 {
			t.Fatalf("expected 1 audit root, got %+v", audit.Roots)
		}
		assertAuditWarningKind(t, audit.Roots[0].Warnings, "no_business_call_after_handler")
	})
}

func TestBuildAudit_CollectsSiblingBusinessCallsBeforeDeeperPath(t *testing.T) {
	service := New()
	snapshot := graph.GraphSnapshot{
		WorkspaceID: "ws_business",
		ID:          "snap_business",
		Nodes: []graph.Node{
			endpointNode("endpoint_detect", "POST /detect"),
			testSymbolNode("handler_root", "api.HandleDetect", "function"),
			testSymbolNode("session_target", "session.SessionService.GetSessionByZlpToken", "function"),
			testSymbolNode("repo_target", "repo.CameraRepo.DetectQR", "function"),
			testSymbolNode("helper_target", "repo.detectQR", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_detect", To: "handler_root", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_repo", Kind: graph.EdgeCalls, From: "handler_root", To: "repo_target", Confidence: graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.82}, Properties: map[string]string{"resolution_basis": "package_method_hint"}},
			{ID: "e_session", Kind: graph.EdgeCalls, From: "handler_root", To: "session_target", Confidence: graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.82}, Properties: map[string]string{"resolution_basis": "package_method_hint"}},
			{ID: "e_helper", Kind: graph.EdgeCalls, From: "repo_target", To: "helper_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}, Properties: map[string]string{"resolution_basis": "exact_canonical"}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "endpoint_detect",
			CanonicalName: "POST /detect",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := service.Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	audit := service.BuildAudit(snapshot, roots, bundle)
	if len(audit.Roots) != 1 {
		t.Fatalf("expected 1 audit root, got %+v", audit.Roots)
	}
	got := audit.Roots[0].FirstBusinessCalls
	if len(got) != 3 {
		t.Fatalf("expected three business calls from sibling + deeper path, got %+v", got)
	}
	if got[0].ToNodeID != "repo_target" && got[0].ToNodeID != "session_target" {
		t.Fatalf("expected sibling business call first, got %+v", got)
	}
	if got[1].ToNodeID != "repo_target" && got[1].ToNodeID != "session_target" {
		t.Fatalf("expected both sibling business calls to be preserved, got %+v", got)
	}
	if got[0].ToNodeID == got[1].ToNodeID {
		t.Fatalf("expected distinct sibling business calls, got %+v", got)
	}
	if got[2].ToNodeID != "helper_target" {
		t.Fatalf("expected deeper business call after sibling calls, got %+v", got)
	}
}

func TestBuildAudit_FunctionAnchorRefinementDoesNotChangeMainline(t *testing.T) {
	service := New()
	snapshot := graph.GraphSnapshot{
		WorkspaceID: "ws_anchor_refine",
		ID:          "snap_anchor_refine",
		Nodes: []graph.Node{
			endpointNode("endpoint_anchor", "GET /anchor"),
			testSymbolNode("handler_factory", "api.MakeOrdersHandler", "function"),
			testSymbolNode("handler_closure", "api.MakeOrdersHandler.$closure_return_0", "function"),
			testSymbolNode("service_target", "svc.OrderService", "function"),
			testSymbolNode("log_target", "obs.LogRequest", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_anchor", To: "handler_factory", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_return", Kind: graph.EdgeReturnsHandler, From: "handler_factory", To: "handler_closure", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_factory_service", Kind: graph.EdgeCalls, From: "handler_factory", To: "service_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
			{ID: "e_closure_log", Kind: graph.EdgeCalls, From: "handler_closure", To: "log_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "endpoint_anchor",
			CanonicalName: "GET /anchor",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := service.Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %+v", bundle.Chains)
	}
	steps := bundle.Chains[0].Steps
	if len(steps) != 3 {
		t.Fatalf("expected boundary, handler peel, and closure call, got %+v", steps)
	}
	if got := steps[2].ToNodeID; got != "log_target" {
		t.Fatalf("expected mainline chain to remain rooted in the closure body, got %s", got)
	}

	audit := service.BuildAudit(snapshot, roots, bundle)
	if len(audit.Roots) != 1 {
		t.Fatalf("expected 1 audit root, got %+v", audit.Roots)
	}
	if len(audit.Roots[0].FirstBusinessCalls) != 1 {
		t.Fatalf("expected audit refinement to recover one handler-anchor business call, got %+v", audit.Roots[0].FirstBusinessCalls)
	}
	if got := audit.Roots[0].FirstBusinessCalls[0].ToNodeID; got != "service_target" {
		t.Fatalf("expected audit-only handler anchor refinement to recover service_target, got %+v", audit.Roots[0].FirstBusinessCalls)
	}
}

func TestBuildAudit_FlagsExternalGatewayProxyTarget(t *testing.T) {
	service := New()
	snapshot := graph.GraphSnapshot{
		WorkspaceID: "ws_gateway",
		ID:          "snap_gateway",
		Nodes: []graph.Node{
			endpointNode("endpoint_gateway", "PROXY RegisterUsersHandlerFromEndpoint"),
		},
		Edges: []graph.Edge{
			{ID: "e_register", Kind: graph.EdgeRegistersBoundary, From: "endpoint_gateway", To: "unresolved_RegisterUsersHandlerFromEndpoint", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{{
			NodeID:        "endpoint_gateway",
			CanonicalName: "PROXY RegisterUsersHandlerFromEndpoint",
			RootType:      entrypoint.RootHTTP,
			RepositoryID:  "repo1",
		}},
	}

	bundle, err := service.Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	audit := service.BuildAudit(snapshot, roots, bundle)
	if len(audit.Roots) != 1 {
		t.Fatalf("expected 1 audit root, got %+v", audit.Roots)
	}
	assertAuditWarningKind(t, audit.Roots[0].Warnings, "gateway_proxy_external")
}

// --- test helpers ---

func testSymbolNode(id, canonical, kind string) graph.Node {
	name := canonical
	if idx := len(canonical) - 1; idx >= 0 {
		for i := len(canonical) - 1; i >= 0; i-- {
			if canonical[i] == '.' {
				name = canonical[i+1:]
				break
			}
		}
	}
	return graph.Node{
		ID:            id,
		Kind:          graph.NodeSymbol,
		CanonicalName: canonical,
		RepositoryID:  "repo1",
		Properties: map[string]string{
			"kind": kind,
			"name": name,
		},
	}
}

func callEdge(id, from, to string) graph.Edge {
	return graph.Edge{
		ID:         id,
		Kind:       graph.EdgeCalls,
		From:       from,
		To:         to,
		Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.95},
	}
}

func endpointNode(id, canonical string) graph.Node {
	return graph.Node{
		ID:            id,
		Kind:          graph.NodeEndpoint,
		CanonicalName: canonical,
		RepositoryID:  "repo1",
		Properties: map[string]string{
			"boundary_kind": "http",
		},
	}
}

func assertAuditWarningKind(t *testing.T, warnings []SemanticAuditWarning, kind string) {
	t.Helper()
	for _, warning := range warnings {
		if warning.Kind == kind {
			return
		}
	}
	t.Fatalf("warning %q not found in %+v", kind, warnings)
}

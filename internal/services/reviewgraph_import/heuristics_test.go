package reviewgraph_import

import (
	"strings"
	"testing"

	"analysis-module/internal/domain/reviewgraph"
)

func TestScanSourceFileAddsGoAsyncTaskAndChannel(t *testing.T) {
	b := newHeuristicBuilder()
	b.nodesByID["owner"] = reviewgraph.Node{
		ID:         "owner",
		SnapshotID: "snap",
		Repo:       "repo",
		Service:    "svc",
		Language:   "go",
		Kind:       reviewgraph.NodeFunction,
		Symbol:     "svc.Start",
		FilePath:   "main.go",
		StartLine:  2,
		EndLine:    6,
		NodeRole:   reviewgraph.RoleEntrypoint,
	}
	b.nodesByID["target"] = reviewgraph.Node{
		ID:         "target",
		SnapshotID: "snap",
		Repo:       "repo",
		Service:    "svc",
		Language:   "go",
		Kind:       reviewgraph.NodeFunction,
		Symbol:     "worker.Run",
		FilePath:   "main.go",
		StartLine:  8,
		EndLine:    9,
	}

	contents := strings.Join([]string{
		"package main",
		"func Start() {",
		"  go worker.Run()",
		"  jobs <- payload",
		"  result := <-jobs",
		"}",
		"",
		"func Run() {",
		"}",
	}, "\n")
	b.scanSourceFile("repo", "svc", "main.go", "", contents)

	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeSpawnsAsync, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeRunsAsync, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeSendsToChannel, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeReceivesFromChannel, 1)
	assertNodeKindCount(t, b.nodesByID, reviewgraph.NodeAsyncTask, 1)
	assertNodeKindCount(t, b.nodesByID, reviewgraph.NodeInProcChannel, 1)
	if got := b.nodesByID["owner"].NodeRole; got != reviewgraph.RoleAsyncProducer {
		t.Fatalf("expected owner to be async producer, got %s", got)
	}
	if got := b.nodesByID["target"].NodeRole; got != reviewgraph.RoleAsyncConsumer {
		t.Fatalf("expected goroutine target to be async consumer, got %s", got)
	}
}

func TestScanSourceFileAddsPythonAsyncTaskAndWeakDiagnostic(t *testing.T) {
	b := newHeuristicBuilder()
	b.nodesByID["owner"] = reviewgraph.Node{
		ID:         "owner",
		SnapshotID: "snap",
		Repo:       "repo",
		Service:    "svc",
		Language:   "python",
		Kind:       reviewgraph.NodeFunction,
		Symbol:     "jobs.start",
		FilePath:   "jobs.py",
		StartLine:  1,
		EndLine:    5,
		NodeRole:   reviewgraph.RoleEntrypoint,
	}
	b.nodesByID["target"] = reviewgraph.Node{
		ID:         "target",
		SnapshotID: "snap",
		Repo:       "repo",
		Service:    "svc",
		Language:   "python",
		Kind:       reviewgraph.NodeFunction,
		Symbol:     "worker.run",
		FilePath:   "jobs.py",
		StartLine:  7,
		EndLine:    8,
	}

	contents := strings.Join([]string{
		"def start():",
		"    asyncio.create_task(worker.run())",
		"    jobs.put(item)",
		"    pending = asyncio.create_task(missing.run())",
		"",
		"def run():",
		"    return None",
	}, "\n")
	b.scanSourceFile("repo", "svc", "jobs.py", "", contents)

	assertNodeKindCount(t, b.nodesByID, reviewgraph.NodeAsyncTask, 1)
	assertNodeKindCount(t, b.nodesByID, reviewgraph.NodeInProcChannel, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeSpawnsAsync, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeRunsAsync, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeSendsToChannel, 1)
	if len(b.diagnostics) == 0 {
		t.Fatal("expected weak async diagnostic for unresolved python task target")
	}
}

func TestScanSourceFileAddsJSWorkerAndRabbitMQBridge(t *testing.T) {
	b := newHeuristicBuilder()
	b.nodesByID["owner"] = reviewgraph.Node{
		ID:         "owner",
		SnapshotID: "snap",
		Repo:       "repo",
		Service:    "svc",
		Language:   "javascript",
		Kind:       reviewgraph.NodeFunction,
		Symbol:     "app.start",
		FilePath:   "src/app.js",
		StartLine:  1,
		EndLine:    3,
		NodeRole:   reviewgraph.RoleEntrypoint,
	}
	b.nodesByID[reviewgraph.FileNodeID("repo", "workers/job-worker.js")] = reviewgraph.Node{
		ID:         reviewgraph.FileNodeID("repo", "workers/job-worker.js"),
		SnapshotID: "snap",
		Repo:       "repo",
		Service:    "svc",
		Language:   "javascript",
		Kind:       reviewgraph.NodeFile,
		Symbol:     "workers/job-worker.js",
		FilePath:   "workers/job-worker.js",
	}

	jsContents := strings.Join([]string{
		"function start() {",
		"  const worker = new Worker(\"../workers/job-worker.js\")",
		"}",
	}, "\n")
	b.scanSourceFile("repo", "svc", "src/app.js", "", jsContents)
	if !hasNodeKind(b.nodesByID, reviewgraph.NodeAsyncTask) {
		t.Fatal("expected worker launch to create async_task node")
	}
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeSpawnsAsync, 1)
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeRunsAsync, 1)

	if !b.tryAddExternalAsyncLine("repo", "svc", "src/app.js", "rabbitmq", `channel.publish("orders", "orders.created", payload)`, 3) {
		t.Fatal("expected rabbitmq publish line to be recognized")
	}
	if !hasNodeWithKindAndSymbol(b.nodesByID, reviewgraph.NodeQueue, "orders.created") {
		t.Fatal("expected rabbitmq publish to create queue bridge node")
	}
	assertEdgeTypeCount(t, b.edgesByID, reviewgraph.EdgeEnqueuesJob, 1)
}

func newHeuristicBuilder() *builder {
	return &builder{
		snapshotID: "snap",
		nodesByID:  map[string]reviewgraph.Node{},
		edgesByID:  map[string]reviewgraph.Edge{},
	}
}

func assertEdgeTypeCount(t *testing.T, edges map[string]reviewgraph.Edge, edgeType reviewgraph.EdgeType, want int) {
	t.Helper()
	got := 0
	for _, edge := range edges {
		if edge.EdgeType == edgeType {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected %d edges of type %s, got %d", want, edgeType, got)
	}
}

func assertNodeKindCount(t *testing.T, nodes map[string]reviewgraph.Node, kind reviewgraph.NodeKind, want int) {
	t.Helper()
	got := 0
	for _, node := range nodes {
		if node.Kind == kind {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected %d nodes of kind %s, got %d", want, kind, got)
	}
}

func hasNodeKind(nodes map[string]reviewgraph.Node, kind reviewgraph.NodeKind) bool {
	for _, node := range nodes {
		if node.Kind == kind {
			return true
		}
	}
	return false
}

func hasNodeWithKindAndSymbol(nodes map[string]reviewgraph.Node, kind reviewgraph.NodeKind, symbol string) bool {
	for _, node := range nodes {
		if node.Kind == kind && node.Symbol == symbol {
			return true
		}
	}
	return false
}

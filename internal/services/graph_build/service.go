package graph_build

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/pkg/ids"
)

type BuildResult struct {
	Snapshot        graph.GraphSnapshot `json:"snapshot"`
	UnresolvedCalls int                 `json:"unresolved_calls"`
}

type Service struct {
	reporter progress.Reporter
}

func New(reporter progress.Reporter) Service {
	if reporter == nil {
		reporter = progress.NoopReporter{}
	}
	return Service{reporter: reporter}
}

func (s Service) Build(workspaceID, snapshotID string, inventory repository.Inventory, extraction symbol_index.Result) BuildResult {
	s.reporter.StartStage("graph", 0)
	nodes := []graph.Node{}
	edges := []graph.Edge{}
	symbolByCanonical := map[string]symbol.Symbol{}
	symbolNodeByID := map[symbol.ID]graph.Node{}
	methodsBySuffix := map[string][]symbol.Symbol{}
	workspaceNode := graph.Node{
		ID:            ids.Stable("node", snapshotID, "workspace", workspaceID),
		Kind:          graph.NodeWorkspace,
		CanonicalName: workspaceID,
		SnapshotID:    snapshotID,
	}
	nodes = append(nodes, workspaceNode)
	for _, repo := range inventory.Repositories {
		repoNode := graph.Node{
			ID:            ids.Stable("node", snapshotID, "repo", string(repo.ID)),
			Kind:          graph.NodeRepository,
			CanonicalName: repo.Name,
			RepositoryID:  string(repo.ID),
			FilePath:      repo.RootPath,
			Properties:    map[string]string{"role": string(repo.Role)},
			SnapshotID:    snapshotID,
		}
		nodes = append(nodes, repoNode)
		edges = append(edges, newEdge(snapshotID, graph.EdgeContains, workspaceNode.ID, repoNode.ID, "scanner", graph.ConfidenceConfirmed, 1.0))
		for _, svc := range repo.CandidateServices {
			serviceNode := graph.Node{
				ID:            ids.Stable("node", snapshotID, "service", string(svc.ID)),
				Kind:          graph.NodeService,
				CanonicalName: svc.Name,
				RepositoryID:  string(repo.ID),
				FilePath:      svc.RootPath,
				SnapshotID:    snapshotID,
			}
			nodes = append(nodes, serviceNode)
			edges = append(edges, newEdge(snapshotID, graph.EdgeContains, repoNode.ID, serviceNode.ID, "scanner", graph.ConfidenceConfirmed, 1.0))
		}
	}
	s.reporter.Status("repos=" + strconv.Itoa(len(inventory.Repositories)))

	fileCount := 0
	for _, repoExtraction := range extraction.Repositories {
		repo := repoExtraction.Repository
		repoNodeID := ids.Stable("node", snapshotID, "repo", string(repo.ID))
		packageNodes := map[string]string{}
		serviceNodeID := ""
		if len(repo.CandidateServices) > 0 {
			serviceNodeID = ids.Stable("node", snapshotID, "service", string(repo.CandidateServices[0].ID))
		}
		for _, fileResult := range repoExtraction.Files {
			fileCount++
			language := fileResult.Language
			if language == "" {
				language = string(primaryRepoLanguage(repo))
			}
			method := extractionMethod(language)
			fileNode := graph.Node{
				ID:            ids.Stable("node", snapshotID, "file", string(repo.ID), fileResult.FilePath),
				Kind:          graph.NodeFile,
				CanonicalName: fileResult.FilePath,
				RepositoryID:  string(repo.ID),
				FilePath:      fileResult.FilePath,
				Language:      language,
				SnapshotID:    snapshotID,
			}
			nodes = append(nodes, fileNode)
			edges = append(edges, newEdge(snapshotID, graph.EdgeContains, repoNodeID, fileNode.ID, "inventory", graph.ConfidenceConfirmed, 1.0))

			if fileResult.PackageName != "" {
				if _, ok := packageNodes[fileResult.PackageName]; !ok {
					packageNode := graph.Node{
						ID:            ids.Stable("node", snapshotID, "package", string(repo.ID), fileResult.PackageName),
						Kind:          graph.NodePackage,
						CanonicalName: fileResult.PackageName,
						RepositoryID:  string(repo.ID),
						Language:      language,
						SnapshotID:    snapshotID,
					}
					nodes = append(nodes, packageNode)
					packageNodes[fileResult.PackageName] = packageNode.ID
					edges = append(edges, newEdge(snapshotID, graph.EdgeContains, repoNodeID, packageNode.ID, method, graph.ConfidenceConfirmed, 1.0))
				}
				edges = append(edges, newEdge(snapshotID, graph.EdgeContains, packageNodes[fileResult.PackageName], fileNode.ID, method, graph.ConfidenceConfirmed, 1.0))
			}

			for _, imported := range fileResult.Imports {
				importNode := graph.Node{
					ID:            ids.Stable("node", snapshotID, "import", imported),
					Kind:          graph.NodePackage,
					CanonicalName: imported,
					Language:      language,
					SnapshotID:    snapshotID,
				}
				nodes = append(nodes, importNode)
				edges = append(edges, newEdge(snapshotID, graph.EdgeImports, fileNode.ID, importNode.ID, method, graph.ConfidenceConfirmed, 0.9))
			}

			for _, sym := range fileResult.Symbols {
				symbolByCanonical[sym.CanonicalName] = sym
				suffix := "." + sym.Name
				methodsBySuffix[suffix] = append(methodsBySuffix[suffix], sym)
				nodeKind := graph.NodeSymbol
				if sym.Kind == symbol.KindTestFunction {
					nodeKind = graph.NodeTest
				}
				symbolNode := graph.Node{
					ID:            ids.Stable("node", snapshotID, "symbol", string(sym.ID)),
					Kind:          nodeKind,
					CanonicalName: sym.CanonicalName,
					RepositoryID:  sym.RepositoryID,
					FilePath:      sym.FilePath,
					Language:      language,
					Location:      &sym.Location,
					Properties: map[string]string{
						"name": sym.Name,
						"kind": string(sym.Kind),
					},
					SnapshotID: snapshotID,
				}
				if sym.Receiver != "" {
					symbolNode.Properties["receiver"] = sym.Receiver
				}
				symbolNodeByID[sym.ID] = symbolNode
				nodes = append(nodes, symbolNode)
				edges = append(edges, newEdge(snapshotID, graph.EdgeDefines, fileNode.ID, symbolNode.ID, method, graph.ConfidenceConfirmed, 1.0))
				if serviceNodeID != "" {
					edges = append(edges, newEdge(snapshotID, graph.EdgeBelongsToService, symbolNode.ID, serviceNodeID, "inventory", graph.ConfidenceInferred, 0.7))
				}
			}
			s.reporter.Status("files=" + strconv.Itoa(fileCount) + " symbols=" + strconv.Itoa(len(symbolByCanonical)))
		}
	}

	unresolved := 0
	for _, repoExtraction := range extraction.Repositories {
		for _, fileResult := range repoExtraction.Files {
			sourceByID := map[symbol.ID]symbol.Symbol{}
			for _, sym := range fileResult.Symbols {
				sourceByID[sym.ID] = sym
			}
			for _, relation := range fileResult.Relations {
				source, ok := sourceByID[relation.SourceSymbolID]
				if !ok {
					unresolved++
					continue
				}
				resolved, confidence, ok := resolveTarget(relation.TargetCanonicalName, symbolByCanonical, methodsBySuffix)
				if !ok {
					unresolved++
					continue
				}
				fromNode := symbolNodeByID[source.ID]
				toNode := symbolNodeByID[resolved.ID]
				edges = append(edges, graph.Edge{
					ID:   ids.Stable("edge", snapshotID, fromNode.ID, toNode.ID, string(graph.EdgeCalls), relation.EvidenceSource),
					Kind: graph.EdgeCalls,
					From: fromNode.ID,
					To:   toNode.ID,
					Evidence: graph.Evidence{
						Type:             relation.EvidenceType,
						Source:           relation.EvidenceSource,
						ExtractionMethod: relation.ExtractionMethod,
						Details:          relation.TargetCanonicalName,
					},
					Confidence: confidence,
					SnapshotID: snapshotID,
				})
				if source.Kind == symbol.KindTestFunction {
					edges = append(edges, newEdge(snapshotID, graph.EdgeTestedBy, toNode.ID, fromNode.ID, relation.ExtractionMethod, graph.ConfidenceInferred, 0.8))
				}
			}
		}
	}

	dedupedNodes := dedupeNodes(nodes)
	dedupedEdges := dedupeEdges(edges)
	s.reporter.FinishStage("nodes=" + strconv.Itoa(len(dedupedNodes)) + " edges=" + strconv.Itoa(len(dedupedEdges)) + " unresolved=" + strconv.Itoa(unresolved))
	return BuildResult{
		Snapshot: graph.GraphSnapshot{
			ID:          snapshotID,
			WorkspaceID: workspaceID,
			CreatedAt:   time.Now().UTC(),
			Nodes:       dedupedNodes,
			Edges:       dedupedEdges,
			Metadata: graph.SnapshotMetadata{
				RepositoryCount: len(inventory.Repositories),
				FileCount:       fileCount,
				SymbolCount:     len(symbolByCanonical),
				EdgeCount:       len(dedupedEdges),
			},
		},
		UnresolvedCalls: unresolved,
	}
}

func resolveTarget(target string, symbolByCanonical map[string]symbol.Symbol, methodsBySuffix map[string][]symbol.Symbol) (symbol.Symbol, graph.Confidence, bool) {
	if resolved, ok := symbolByCanonical[target]; ok {
		return resolved, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.95}, true
	}
	idx := strings.LastIndex(target, ".")
	if idx < 0 {
		return symbol.Symbol{}, graph.Confidence{}, false
	}
	suffix := target[idx:]
	if suffix != "" {
		candidates := methodsBySuffix[suffix]
		if len(candidates) == 1 {
			return candidates[0], graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.6}, true
		}
	}
	return symbol.Symbol{}, graph.Confidence{}, false
}

func primaryRepoLanguage(repo repository.Manifest) repository.Language {
	if len(repo.TechStack.Languages) > 0 {
		return repo.TechStack.Languages[0]
	}
	if len(repo.GoFiles) > 0 {
		return repository.LanguageGo
	}
	if len(repo.PythonFiles) > 0 {
		return repository.LanguagePython
	}
	if len(repo.TypeScriptFiles) > 0 {
		return repository.LanguageTS
	}
	if len(repo.JavaScriptFiles) > 0 {
		return repository.LanguageJS
	}
	return repository.LanguageGo
}

func extractionMethod(language string) string {
	switch language {
	case string(repository.LanguagePython):
		return "python-regex"
	case string(repository.LanguageJS), string(repository.LanguageTS):
		return "js-regex"
	default:
		return "tree-sitter-go"
	}
}

func newEdge(snapshotID string, kind graph.EdgeKind, from, to, method string, tier graph.ConfidenceTier, score float64) graph.Edge {
	return graph.Edge{
		ID:   ids.Stable("edge", snapshotID, from, to, string(kind)),
		Kind: kind,
		From: from,
		To:   to,
		Evidence: graph.Evidence{
			Type:             "static",
			Source:           method,
			ExtractionMethod: method,
			Details:          string(kind),
		},
		Confidence: graph.Confidence{Tier: tier, Score: score},
		SnapshotID: snapshotID,
	}
}

func dedupeNodes(nodes []graph.Node) []graph.Node {
	seen := map[string]graph.Node{}
	for _, node := range nodes {
		seen[node.ID] = node
	}
	result := make([]graph.Node, 0, len(seen))
	for _, node := range seen {
		result = append(result, node)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func dedupeEdges(edges []graph.Edge) []graph.Edge {
	seen := map[string]graph.Edge{}
	for _, edge := range edges {
		seen[edge.ID] = edge
	}
	result := make([]graph.Edge, 0, len(seen))
	for _, edge := range seen {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

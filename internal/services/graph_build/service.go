package graph_build

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/pkg/ids"
)

type BuildResult struct {
	Snapshot    graph.GraphSnapshot  `json:"snapshot"`
	IssueCounts analysis.IssueCounts `json:"issue_counts"`
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

func (s Service) Build(workspaceID, snapshotID string, inventory repository.Inventory, extraction symbol_index.Result, detectedRoots []boundaryroot.Root) BuildResult {
	s.reporter.StartStage("graph", 0)
	nodes := []graph.Node{}
	edges := []graph.Edge{}
	issueCounts := inventory.IssueCounts

	symbolByCanonical := map[string]symbol.Symbol{}
	symbolByID := map[symbol.ID]symbol.Symbol{}
	symbolNodeByID := map[symbol.ID]graph.Node{}
	symbolsByFileAndName := map[string]map[string][]symbol.Symbol{}
	exportCanonicalByFile := map[string]map[string]string{}
	methodsBySuffix := map[string][]symbol.Symbol{}
	serviceNodeIDs := map[string]map[string]string{}

	for _, repoExt := range extraction.Repositories {
		for _, fileResult := range repoExt.Files {
			if _, ok := symbolsByFileAndName[fileResult.FilePath]; !ok {
				symbolsByFileAndName[fileResult.FilePath] = map[string][]symbol.Symbol{}
			}
			for _, sym := range fileResult.Symbols {
				symbolByCanonical[sym.CanonicalName] = sym
				symbolByID[sym.ID] = sym
				suffix := "." + sym.Name
				methodsBySuffix[suffix] = append(methodsBySuffix[suffix], sym)
				symbolsByFileAndName[fileResult.FilePath][sym.Name] = append(symbolsByFileAndName[fileResult.FilePath][sym.Name], sym)
			}
			exportCanonicalByFile[fileResult.FilePath] = map[string]string{}
			for _, exportBinding := range fileResult.Exports {
				exportCanonicalByFile[fileResult.FilePath][exportBinding.Name] = exportBinding.CanonicalName
			}
		}
	}

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
		serviceNodeIDs[string(repo.ID)] = map[string]string{}
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
			serviceNodeIDs[string(repo.ID)][string(svc.ID)] = serviceNode.ID
			edges = append(edges, newEdge(snapshotID, graph.EdgeContains, repoNode.ID, serviceNode.ID, "scanner", graph.ConfidenceConfirmed, 1.0))
		}
	}
	s.reporter.Status("repos=" + strconv.Itoa(len(inventory.Repositories)))

	fileCount := 0
	for _, repoExtraction := range extraction.Repositories {
		repo := repoExtraction.Repository
		repoNodeID := ids.Stable("node", snapshotID, "repo", string(repo.ID))
		packageNodes := map[string]string{}
		fileNodeIDs := map[string]string{}
		ownersByFile, repoAmbiguities := determineFileOwners(repo, repoExtraction.Files)
		issueCounts.ServiceAttributionAmbiguities += repoAmbiguities

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
			fileNodeIDs[fileResult.FilePath] = fileNode.ID
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

			exportCanonicalByFile[fileResult.FilePath] = map[string]string{}
			for _, exportBinding := range fileResult.Exports {
				exportCanonicalByFile[fileResult.FilePath][exportBinding.Name] = exportBinding.CanonicalName
			}

			for _, diag := range fileResult.Diagnostics {
				issueCounts = incrementIssueCount(issueCounts, diag.Category)
			}

			for _, sym := range fileResult.Symbols {
				nodeKind := graph.NodeSymbol
				if sym.Kind == symbol.KindTestFunction {
					nodeKind = graph.NodeTest
				}
				symbolNode := graph.Node{
					ID:            ids.Stable("node", "symbol", string(sym.ID)),
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

				owners := sortedOwnerIDs(ownersByFile[fileResult.FilePath])
				for _, ownerID := range owners {
					serviceNodeID := serviceNodeIDs[string(repo.ID)][ownerID]
					if serviceNodeID == "" {
						continue
					}
					edge := newEdge(snapshotID, graph.EdgeBelongsToService, symbolNode.ID, serviceNodeID, "inventory", graph.ConfidenceInferred, 0.75)
					if len(owners) > 1 {
						edge.Properties = map[string]string{"shared": "true"}
					}
					edges = append(edges, edge)
				}
			}
			s.reporter.Status("files=" + strconv.Itoa(fileCount) + " symbols=" + strconv.Itoa(len(symbolByCanonical)))
		}

		for _, svc := range repo.CandidateServices {
			for _, entrypoint := range svc.Entrypoints {
				fileNodeID := fileNodeIDs[filepath.ToSlash(entrypoint)]
				serviceNodeID := serviceNodeIDs[string(repo.ID)][string(svc.ID)]
				if fileNodeID == "" || serviceNodeID == "" {
					continue
				}
				edges = append(edges, newEdge(snapshotID, graph.EdgeEntrypointTo, serviceNodeID, fileNodeID, "scanner", graph.ConfidenceConfirmed, 1.0))
			}
		}
	}

	for _, repoExtraction := range extraction.Repositories {
		for _, fileResult := range repoExtraction.Files {
			sourceByID := map[symbol.ID]symbol.Symbol{}
			for _, sym := range fileResult.Symbols {
				sourceByID[sym.ID] = sym
			}
			for _, relation := range fileResult.Relations {
				source, ok := sourceByID[relation.SourceSymbolID]
				if !ok {
					continue
				}
				resolved, confidence, ok := resolveTarget(relation, symbolByCanonical, symbolByID, symbolsByFileAndName, exportCanonicalByFile, methodsBySuffix)
				if !ok {
					issueCounts.UnresolvedImports++
					continue
				}
				fromNode := symbolNodeByID[source.ID]
				toNode := symbolNodeByID[resolved.ID]
				edge := graph.Edge{
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
				}
				edges = append(edges, edge)
				if source.Kind == symbol.KindTestFunction {
					edges = append(edges, newEdge(snapshotID, graph.EdgeTestedBy, toNode.ID, fromNode.ID, relation.ExtractionMethod, graph.ConfidenceInferred, 0.85))
				}
			}

			// Add semantic hint edges
			for _, hint := range fileResult.Hints {
				source, ok := sourceByID[symbol.ID(hint.SourceSymbolID)]
				if !ok {
					continue
				}
				fromNode, ok := symbolNodeByID[source.ID]
				if !ok {
					continue
				}

				targetSym, targetNodeID, resolved := resolveHintTarget(hint, symbolByCanonical, symbolByID, symbolNodeByID)
				if !resolved {
					continue
				}

				edgeKind := mapHintToEdgeKind(hint.Kind)
				properties := map[string]string{
					"order_index":   strconv.Itoa(hint.OrderIndex),
					"semantic_kind": string(hint.Kind),
				}
				if targetSym.Properties != nil && targetSym.Properties["synthetic"] == "true" {
					properties["synthetic_target"] = "true"
				} else {
					properties["synthetic_target"] = "false"
				}
				confidence := graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1.0}
				if hint.TargetSymbolID == "" && hint.TargetSymbol != "" {
					confidence = graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.85}
				}
				edge := graph.Edge{
					ID:   ids.Stable("edge", "semantic", fromNode.ID, targetNodeID, string(edgeKind), strconv.Itoa(hint.OrderIndex), hint.Evidence),
					Kind: edgeKind,
					From: fromNode.ID,
					To:   targetNodeID,
					Evidence: graph.Evidence{
						Type:             "semantic",
						Source:           hint.Evidence,
						ExtractionMethod: extractionMethod(fileResult.Language),
						Details:          string(hint.Kind),
					},
					Confidence: confidence,
					Properties: properties,
					SnapshotID: snapshotID,
				}
				edges = append(edges, edge)
			}
		}
	}

	// Persist upstream framework boundary roots
	for _, br := range detectedRoots {
		rootID := br.ID
		if rootID == "" {
			rootID = boundaryroot.StableID(br)
		}
		rootNode := graph.Node{
			ID:            rootID,
			Kind:          graph.NodeEndpoint, // or custom NodeBoundary
			CanonicalName: br.CanonicalName,
			RepositoryID:  br.RepositoryID,
			FilePath:      br.SourceFile,
			Properties: map[string]string{
				"framework":         br.Framework,
				"boundary_kind":     string(br.Kind),
				"method":            br.Method,
				"path":              br.Path,
				"handler_target":    br.HandlerTarget,
				"source_start_byte": strconv.FormatUint(uint64(br.SourceStartByte), 10),
				"source_end_byte":   strconv.FormatUint(uint64(br.SourceEndByte), 10),
			},
			SnapshotID: snapshotID,
		}
		nodes = append(nodes, rootNode)

		// If handler target is resolved to a symbol:
		if br.HandlerTarget != "" {
			targetSym, _, resolved := resolveTarget(symbol.RelationCandidate{TargetCanonicalName: br.HandlerTarget}, symbolByCanonical, symbolByID, symbolsByFileAndName, exportCanonicalByFile, methodsBySuffix)
			targetNodeID := "unresolved_" + br.HandlerTarget
			if resolved {
				targetNodeID = symbolNodeByID[targetSym.ID].ID
			}
			edge := graph.Edge{
				ID:   ids.Stable("edge", rootNode.ID, targetNodeID, string(graph.EdgeRegistersBoundary)),
				Kind: graph.EdgeRegistersBoundary,
				From: rootNode.ID,
				To:   targetNodeID,
				Evidence: graph.Evidence{
					Type:             "static",
					Source:           br.SourceExpr,
					ExtractionMethod: br.Framework,
					Details:          "boundary_detector",
				},
				Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 1.0},
				SnapshotID: snapshotID,
			}
			edges = append(edges, edge)
		}
	}

	issueCounts.DeferredBoundaryStitching = boundaryHintCount(inventory.Repositories)
	dedupedNodes := dedupeNodes(nodes)
	dedupedEdges := dedupeEdges(edges)
	s.reporter.FinishStage("nodes=" + strconv.Itoa(len(dedupedNodes)) + " edges=" + strconv.Itoa(len(dedupedEdges)))
	return BuildResult{
		Snapshot: graph.GraphSnapshot{
			ID:          snapshotID,
			WorkspaceID: workspaceID,
			CreatedAt:   time.Now().UTC(),
			Nodes:       dedupedNodes,
			Edges:       dedupedEdges,
			Metadata: graph.SnapshotMetadata{
				IgnoreSignature: inventory.IgnoreSignature,
				RepositoryCount: len(inventory.Repositories),
				FileCount:       fileCount,
				SymbolCount:     len(symbolByCanonical),
				EdgeCount:       len(dedupedEdges),
				IssueCounts:     issueCounts,
			},
		},
		IssueCounts: issueCounts,
	}
}

func resolveTarget(relation symbol.RelationCandidate, symbolByCanonical map[string]symbol.Symbol, symbolByID map[symbol.ID]symbol.Symbol, symbolsByFileAndName map[string]map[string][]symbol.Symbol, exportCanonicalByFile map[string]map[string]string, methodsBySuffix map[string][]symbol.Symbol) (symbol.Symbol, graph.Confidence, bool) {
	if relation.TargetCanonicalName != "" {
		// First try exact ID matching (e.g. for already resolved closure IDs)
		if resolved, ok := symbolByID[symbol.ID(relation.TargetCanonicalName)]; ok {
			return resolved, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.99}, true
		}
		// Then try canonical matching
		if resolved, ok := symbolByCanonical[relation.TargetCanonicalName]; ok {
			return resolved, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.95}, true
		}
	}
	if relation.TargetFilePath != "" && relation.TargetExportName != "" {
		if exports := exportCanonicalByFile[relation.TargetFilePath]; exports != nil {
			if canonical := exports[relation.TargetExportName]; canonical != "" {
				if resolved, ok := symbolByCanonical[canonical]; ok {
					return resolved, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.93}, true
				}
			}
		}
		if fileSymbols := symbolsByFileAndName[relation.TargetFilePath]; fileSymbols != nil {
			candidates := fileSymbols[relation.TargetExportName]
			if len(candidates) == 1 {
				return candidates[0], graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.9}, true
			}
		}
	}
	if relation.TargetCanonicalName != "" {
		idx := strings.LastIndex(relation.TargetCanonicalName, ".")
		if idx >= 0 {
			suffix := relation.TargetCanonicalName[idx:]
			candidates := methodsBySuffix[suffix]
			if len(candidates) == 1 {
				return candidates[0], graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.6}, true
			}
		}
	}
	return symbol.Symbol{}, graph.Confidence{}, false
}

func resolveHintTarget(hint executionhint.Hint, symbolByCanonical map[string]symbol.Symbol, symbolByID map[symbol.ID]symbol.Symbol, symbolNodeByID map[symbol.ID]graph.Node) (symbol.Symbol, string, bool) {
	if hint.TargetSymbolID != "" {
		targetSym, ok := symbolByID[symbol.ID(hint.TargetSymbolID)]
		if !ok {
			return symbol.Symbol{}, "", false
		}
		targetNode, ok := symbolNodeByID[targetSym.ID]
		if !ok {
			return symbol.Symbol{}, "", false
		}
		return targetSym, targetNode.ID, true
	}
	if hint.TargetSymbol == "" {
		return symbol.Symbol{}, "", false
	}
	targetSym, ok := symbolByCanonical[hint.TargetSymbol]
	if !ok {
		return symbol.Symbol{}, "", false
	}
	targetNode, ok := symbolNodeByID[targetSym.ID]
	if !ok {
		return symbol.Symbol{}, "", false
	}
	return targetSym, targetNode.ID, true
}

func determineFileOwners(repo repository.Manifest, files []symbol.FileExtractionResult) (map[string]map[string]struct{}, int) {
	owners := map[string]map[string]struct{}{}
	pathOwned := map[string]bool{}
	importedBy := map[string]map[string]struct{}{}
	for _, file := range files {
		pathOwners, ownedByPath := ownersFromServiceRoots(repo, file.FilePath)
		if len(pathOwners) > 0 {
			owners[file.FilePath] = pathOwners
			pathOwned[file.FilePath] = ownedByPath
		}
		for _, binding := range file.ImportBindings {
			if !binding.IsLocal || binding.ResolvedPath == "" {
				continue
			}
			if _, ok := importedBy[binding.ResolvedPath]; !ok {
				importedBy[binding.ResolvedPath] = map[string]struct{}{}
			}
			importedBy[binding.ResolvedPath][file.FilePath] = struct{}{}
		}
	}

	changed := true
	for changed {
		changed = false
		for _, file := range files {
			if pathOwned[file.FilePath] {
				continue
			}
			importers := importedBy[file.FilePath]
			if len(importers) == 0 {
				continue
			}
			nextOwners := copyOwnerSet(owners[file.FilePath])
			for importer := range importers {
				for owner := range owners[importer] {
					nextOwners[owner] = struct{}{}
				}
			}
			if len(nextOwners) > len(owners[file.FilePath]) {
				owners[file.FilePath] = nextOwners
				changed = true
			}
		}
	}

	ambiguities := 0
	if len(repo.CandidateServices) > 1 {
		for _, file := range files {
			if pathOwned[file.FilePath] {
				continue
			}
			if len(owners[file.FilePath]) == 0 {
				ambiguities++
			}
		}
	}
	return owners, ambiguities
}

func ownersFromServiceRoots(repo repository.Manifest, filePath string) (map[string]struct{}, bool) {
	fileAbs := filepath.Join(repo.RootPath, filepath.FromSlash(filePath))
	deepest := -1
	owners := map[string]struct{}{}
	for _, svc := range repo.CandidateServices {
		root := filepath.Clean(svc.RootPath)
		fileClean := filepath.Clean(fileAbs)
		if !isWithinRoot(fileClean, root) {
			continue
		}
		depth := strings.Count(filepath.ToSlash(strings.TrimPrefix(root, filepath.Clean(repo.RootPath))), "/")
		switch {
		case depth > deepest:
			deepest = depth
			owners = map[string]struct{}{string(svc.ID): {}}
		case depth == deepest:
			owners[string(svc.ID)] = struct{}{}
		}
	}
	return owners, len(owners) > 0
}

func isWithinRoot(filePath, root string) bool {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "")
}

func sortedOwnerIDs(owners map[string]struct{}) []string {
	result := make([]string, 0, len(owners))
	for owner := range owners {
		result = append(result, owner)
	}
	sort.Strings(result)
	return result
}

func copyOwnerSet(src map[string]struct{}) map[string]struct{} {
	dst := map[string]struct{}{}
	for key := range src {
		dst[key] = struct{}{}
	}
	return dst
}

func incrementIssueCount(counts analysis.IssueCounts, category string) analysis.IssueCounts {
	switch category {
	case "unresolved_import":
		counts.UnresolvedImports++
	case "ambiguous_relation":
		counts.AmbiguousRelations++
	case "unsupported_construct":
		counts.UnsupportedConstructs++
	}
	return counts
}

func boundaryHintCount(repositories []repository.Manifest) int {
	total := 0
	for _, repo := range repositories {
		total += len(repo.BoundaryHints)
	}
	return total
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
		return "tree-sitter-python"
	case string(repository.LanguageJS), string(repository.LanguageTS):
		return "tree-sitter-js"
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

func mapHintToEdgeKind(kind executionhint.HintKind) graph.EdgeKind {
	switch kind {
	case executionhint.HintReturnHandler:
		return graph.EdgeReturnsHandler
	case executionhint.HintSpawn:
		return graph.EdgeSpawns
	case executionhint.HintDefer:
		return graph.EdgeDefers
	case executionhint.HintWait:
		return graph.EdgeWaitsOn
	case executionhint.HintBranch:
		return graph.EdgeBranches
	default:
		return graph.EdgeCalls
	}
}

package reviewgraph_import

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	legacygraph "analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewgraph"
	servicedomain "analysis-module/internal/domain/service"
	"analysis-module/internal/domain/workspace"
	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/services/reviewgraph_paths"
)

type Request struct {
	WorkspaceID         string `json:"workspace_id"`
	SnapshotID          string `json:"snapshot_id"`
	NodesPath           string `json:"nodes_path,omitempty"`
	EdgesPath           string `json:"edges_path,omitempty"`
	RepoManifestPath    string `json:"repo_manifest_path,omitempty"`
	ServiceManifestPath string `json:"service_manifest_path,omitempty"`
	QualityReportPath   string `json:"quality_report_path,omitempty"`
	IgnoreFilePath      string `json:"ignore_file_path,omitempty"`
	OutDBPath           string `json:"out_db_path,omitempty"`
}

type Result struct {
	WorkspaceID string                   `json:"workspace_id"`
	SnapshotID  string                   `json:"snapshot_id"`
	DBPath      string                   `json:"db_path"`
	NodeCount   int                      `json:"node_count"`
	EdgeCount   int                      `json:"edge_count"`
	Manifest    reviewgraph.ImportManifest `json:"manifest"`
}

type Service struct {
	paths reviewgraph_paths.Service
}

func New(paths reviewgraph_paths.Service) Service {
	return Service{paths: paths}
}

type serviceInfo struct {
	Manifest    servicedomain.Manifest
	RepoName    string
	RelRootPath string
	NodeID      string
}

type legacyNodeMeta struct {
	LegacyID      string            `json:"legacy_id"`
	LegacyKind    string            `json:"legacy_kind"`
	CanonicalName string            `json:"canonical_name"`
	Properties    map[string]string `json:"properties,omitempty"`
	Roles         []string          `json:"roles,omitempty"`
	SharedServices []string         `json:"shared_services,omitempty"`
}

type builder struct {
	snapshotID      string
	repositories    []repository.Manifest
	repoByID        map[string]repository.Manifest
	servicesByRepo  map[string][]serviceInfo
	serviceByName   map[string]serviceInfo
	qualityReport   quality.AnalysisQualityReport
	ignoreRules     reviewgraph.IgnoreRules
	inputPaths      map[string]string
	diagnostics     []reviewgraph.ImportDiagnostic
	counts          reviewgraph.ImportCounts
	legacyNodes     []legacygraph.Node
	legacyEdges     []legacygraph.Edge
	legacyNodeByID  map[string]legacygraph.Node
	oldToNewID      map[string]string
	dropReasonByOld map[string]string
	nodesByID       map[string]reviewgraph.Node
	edgesByID       map[string]reviewgraph.Edge
	serviceOwners   map[string][]string
	serviceRootMap  map[string]serviceInfo
}

func (s Service) Import(req Request) (Result, error) {
	resolved, err := s.paths.Resolve(req.WorkspaceID, req.SnapshotID)
	if err != nil {
		return Result{}, err
	}
	inputs := map[string]string{
		"workspace_manifest":  resolved.WorkspaceManifestPath,
		"repository_manifests": firstNonEmpty(req.RepoManifestPath, resolved.RepositoryManifestPath),
		"service_manifests":    firstNonEmpty(req.ServiceManifestPath, resolved.ServiceManifestPath),
		"quality_report":       firstNonEmpty(req.QualityReportPath, resolved.QualityReportPath),
		"graph_nodes":          firstNonEmpty(req.NodesPath, resolved.NodesPath),
		"graph_edges":          firstNonEmpty(req.EdgesPath, resolved.EdgesPath),
	}
	outDBPath := firstNonEmpty(req.OutDBPath, resolved.ReviewGraphDBPath)

	workspaceManifest, _ := loadJSONIfExists[workspace.Manifest](inputs["workspace_manifest"])
	repositories, err := loadJSONFile[[]repository.Manifest](inputs["repository_manifests"])
	if err != nil {
		return Result{}, err
	}
	services, err := loadJSONFile[[]servicedomain.Manifest](inputs["service_manifests"])
	if err != nil {
		return Result{}, err
	}
	qualityReport, _ := loadJSONIfExists[quality.AnalysisQualityReport](inputs["quality_report"])
	legacyNodes, err := readJSONL[legacygraph.Node](inputs["graph_nodes"])
	if err != nil {
		return Result{}, err
	}
	legacyEdges, err := readJSONL[legacygraph.Edge](inputs["graph_edges"])
	if err != nil {
		return Result{}, err
	}

	ignoreCandidates := []string{}
	if workspaceManifest.RootPath != "" {
		ignoreCandidates = append(ignoreCandidates, filepath.Join(string(workspaceManifest.RootPath), ".text-review-ignore"))
	}
	if req.IgnoreFilePath != "" {
		ignoreCandidates = append(ignoreCandidates, req.IgnoreFilePath)
	}
	ignoreRules, loadedIgnoreFiles, err := reviewgraph.LoadIgnoreRules(ignoreCandidates...)
	if err != nil {
		return Result{}, err
	}

	b := builder{
		snapshotID:      req.SnapshotID,
		repositories:    repositories,
		repoByID:        map[string]repository.Manifest{},
		servicesByRepo:  map[string][]serviceInfo{},
		serviceByName:   map[string]serviceInfo{},
		qualityReport:   qualityReport,
		ignoreRules:     ignoreRules,
		inputPaths:      inputs,
		legacyNodes:     legacyNodes,
		legacyEdges:     legacyEdges,
		legacyNodeByID:  map[string]legacygraph.Node{},
		oldToNewID:      map[string]string{},
		dropReasonByOld: map[string]string{},
		nodesByID:       map[string]reviewgraph.Node{},
		edgesByID:       map[string]reviewgraph.Edge{},
		serviceOwners:   map[string][]string{},
		serviceRootMap:  map[string]serviceInfo{},
	}
	for _, repo := range repositories {
		b.repoByID[string(repo.ID)] = repo
	}
	for _, svc := range services {
		repo := b.repoByID[svc.RepositoryID]
		relRoot := relativePath(repo.RootPath, svc.RootPath)
		info := serviceInfo{
			Manifest:    svc,
			RepoName:    repo.Name,
			RelRootPath: relRoot,
			NodeID:      reviewgraph.ServiceNodeID(svc.Name),
		}
		b.servicesByRepo[svc.RepositoryID] = append(b.servicesByRepo[svc.RepositoryID], info)
		b.serviceByName[svc.Name] = info
		b.serviceRootMap[svc.RepositoryID+"::"+relRoot] = info
	}
	for repoID := range b.servicesByRepo {
		sort.Slice(b.servicesByRepo[repoID], func(i, j int) bool {
			return b.servicesByRepo[repoID][i].RelRootPath < b.servicesByRepo[repoID][j].RelRootPath
		})
	}

	nodes, edges, err := b.build()
	if err != nil {
		return Result{}, err
	}
	manifest := reviewgraph.ImportManifest{
		WorkspaceID:     req.WorkspaceID,
		SnapshotID:      req.SnapshotID,
		ImporterVersion: reviewgraph.ImporterVersion,
		GeneratedAt:     time.Now().UTC(),
		InputPaths:      inputs,
		IgnoreFiles:     loadedIgnoreFiles,
		IgnoreRules:     ignoreRules.AllPatterns(),
		Counts:          b.counts,
		Diagnostics:     b.diagnostics,
		AsyncVersion:    reviewgraph.AsyncHeuristicVersion,
		Metadata: map[string]any{
			"quality_report_issue_counts": qualityReport.IssueCounts,
			"quality_report_gaps":         qualityReport.Gaps,
		},
	}

	store, err := reviewsqlite.New(outDBPath)
	if err != nil {
		return Result{}, err
	}
	defer store.Close()

	artifacts := []reviewgraph.Artifact{
		{
			ID:           reviewgraph.ArtifactID(reviewgraph.ArtifactImportManifest, "", ""),
			SnapshotID:   req.SnapshotID,
			ArtifactType: reviewgraph.ArtifactImportManifest,
			Path:         "",
			MetadataJSON: reviewsqlite.EncodeJSON(manifest),
		},
	}
	if err := store.ReplaceSnapshot(req.SnapshotID, nodes, edges, artifacts); err != nil {
		return Result{}, err
	}

	return Result{
		WorkspaceID: req.WorkspaceID,
		SnapshotID:  req.SnapshotID,
		DBPath:      outDBPath,
		NodeCount:   len(nodes),
		EdgeCount:   len(edges),
		Manifest:    manifest,
	}, nil
}

func (b *builder) build() ([]reviewgraph.Node, []reviewgraph.Edge, error) {
	b.counts.LegacyNodeCount = len(b.legacyNodes)
	b.counts.LegacyEdgeCount = len(b.legacyEdges)
	for _, legacyNode := range b.legacyNodes {
		b.legacyNodeByID[legacyNode.ID] = legacyNode
	}
	b.seedServiceNodes()
	b.collectServiceOwnership()
	for _, legacyNode := range b.legacyNodes {
		b.importNode(legacyNode)
	}
	for _, legacyEdge := range b.legacyEdges {
		b.importEdge(legacyEdge)
	}
	if err := b.addAsyncHeuristics(); err != nil {
		return nil, nil, err
	}
	nodes := make([]reviewgraph.Node, 0, len(b.nodesByID))
	for _, node := range b.nodesByID {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	edges := make([]reviewgraph.Edge, 0, len(b.edgesByID))
	for _, edge := range b.edgesByID {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	b.counts.ImportedNodeCount = len(nodes)
	b.counts.ImportedEdgeCount = len(edges)
	return nodes, edges, nil
}

func (b *builder) seedServiceNodes() {
	for _, repoServices := range b.servicesByRepo {
		for _, info := range repoServices {
			node := reviewgraph.Node{
				ID:           info.NodeID,
				SnapshotID:   b.snapshotID,
				Repo:         info.Manifest.RepositoryID,
				Service:      info.Manifest.Name,
				Language:     "unknown",
				Kind:         reviewgraph.NodeService,
				Symbol:       info.Manifest.Name,
				FilePath:     "",
				NodeRole:     roleFromSet([]reviewgraph.NodeRole{reviewgraph.RoleBoundary}),
				MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"service_manifest": info.Manifest}),
			}
			b.nodesByID[node.ID] = node
		}
	}
	for _, legacyNode := range b.legacyNodes {
		if legacyNode.Kind != legacygraph.NodeService {
			continue
		}
		info, ok := b.serviceByName[legacyNode.CanonicalName]
		if ok {
			b.oldToNewID[legacyNode.ID] = info.NodeID
			continue
		}
		nodeID := reviewgraph.ServiceNodeID(legacyNode.CanonicalName)
		b.oldToNewID[legacyNode.ID] = nodeID
		b.nodesByID[nodeID] = reviewgraph.Node{
			ID:           nodeID,
			SnapshotID:   b.snapshotID,
			Repo:         legacyNode.RepositoryID,
			Service:      legacyNode.CanonicalName,
			Language:     "unknown",
			Kind:         reviewgraph.NodeService,
			Symbol:       legacyNode.CanonicalName,
			FilePath:     "",
			NodeRole:     reviewgraph.RoleBoundary,
			MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"legacy_id": legacyNode.ID, "legacy_kind": string(legacyNode.Kind)}),
		}
	}
}

func (b *builder) collectServiceOwnership() {
	serviceNamesByLegacyNodeID := map[string]string{}
	for _, legacyNode := range b.legacyNodes {
		if legacyNode.Kind == legacygraph.NodeService {
			serviceNamesByLegacyNodeID[legacyNode.ID] = legacyNode.CanonicalName
		}
	}
	for _, edge := range b.legacyEdges {
		if edge.Kind != legacygraph.EdgeBelongsToService {
			continue
		}
		serviceName := serviceNamesByLegacyNodeID[edge.To]
		if serviceName == "" {
			continue
		}
		b.serviceOwners[edge.From] = appendIfMissing(b.serviceOwners[edge.From], serviceName)
	}
	for key, owners := range b.serviceOwners {
		sort.Strings(owners)
		b.serviceOwners[key] = owners
	}
}

func (b *builder) importNode(legacyNode legacygraph.Node) {
	if _, mapped := b.oldToNewID[legacyNode.ID]; mapped {
		return
	}
	if reason, _ := b.shouldDropNode(legacyNode); reason != "" {
		b.dropReasonByOld[legacyNode.ID] = reason
		return
	}
	node, ok := b.mapLegacyNode(legacyNode)
	if !ok {
		return
	}
	b.oldToNewID[legacyNode.ID] = node.ID
	b.nodesByID[node.ID] = node
}

func (b *builder) importEdge(legacyEdge legacygraph.Edge) {
	if legacyEdge.Kind == legacygraph.EdgeTestedBy {
		b.counts.DroppedTestEdgeCount++
		return
	}
	srcReason := b.dropReasonByOld[legacyEdge.From]
	dstReason := b.dropReasonByOld[legacyEdge.To]
	if srcReason != "" || dstReason != "" {
		switch {
		case srcReason == "test" || dstReason == "test":
			b.counts.DroppedTestEdgeCount++
		case srcReason == "generated" || dstReason == "generated":
			b.counts.DroppedGeneratedEdges++
		default:
			b.counts.DroppedIgnoredEdges++
		}
		return
	}
	srcID := b.oldToNewID[legacyEdge.From]
	dstID := b.oldToNewID[legacyEdge.To]
	if srcID == "" || dstID == "" {
		return
	}
	switch legacyEdge.Kind {
	case legacygraph.EdgeCalls, legacygraph.EdgeDefines, legacygraph.EdgeImports, legacygraph.EdgeBelongsToService:
	default:
		if legacyEdge.Kind == legacygraph.EdgeCallsHTTP || legacyEdge.Kind == legacygraph.EdgeCallsGRPC || legacyEdge.Kind == legacygraph.EdgeEntrypointTo {
			b.counts.ContextualBoundaryEdges++
		}
		return
	}
	edgeType, flowKind := mapLegacyEdgeKind(legacyEdge.Kind)
	evidenceFile, evidenceLine := b.edgeEvidenceLocation(legacyEdge.From)
	edge := reviewgraph.Edge{
		ID:           reviewgraph.EdgeID(srcID, edgeType, dstID),
		SnapshotID:   b.snapshotID,
		SrcID:        srcID,
		DstID:        dstID,
		EdgeType:     edgeType,
		FlowKind:     flowKind,
		Confidence:   legacyEdge.Confidence.Score,
		EvidenceFile: evidenceFile,
		EvidenceLine: evidenceLine,
		EvidenceText: firstNonEmpty(legacyEdge.Evidence.Details, legacyEdge.Evidence.Source, legacyEdge.Evidence.Type),
		MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{
			"legacy_edge_id": legacyEdge.ID,
			"legacy_kind":    legacyEdge.Kind,
			"evidence":       legacyEdge.Evidence,
			"confidence":     legacyEdge.Confidence,
			"properties":     legacyEdge.Properties,
		}),
	}
	b.addEdge(edge)
}

func (b *builder) mapLegacyNode(legacyNode legacygraph.Node) (reviewgraph.Node, bool) {
	legacyKind := legacyNode.Kind
	meta := legacyNodeMeta{
		LegacyID:      legacyNode.ID,
		LegacyKind:    string(legacyNode.Kind),
		CanonicalName: legacyNode.CanonicalName,
		Properties:    legacyNode.Properties,
	}
	owners := append([]string{}, b.serviceOwners[legacyNode.ID]...)
	if len(owners) == 0 {
		owners = b.inferServicesFromPath(legacyNode.RepositoryID, legacyNode.FilePath)
	}
	meta.SharedServices = owners
	serviceName := ""
	if len(owners) > 0 {
		serviceName = owners[0]
	}

	node := reviewgraph.Node{
		SnapshotID: b.snapshotID,
		Repo:       legacyNode.RepositoryID,
		Service:    serviceName,
		Language:   firstNonEmpty(legacyNode.Language, "unknown"),
		Symbol:     legacyNode.CanonicalName,
		FilePath:   filepath.ToSlash(legacyNode.FilePath),
	}
	switch legacyKind {
	case legacygraph.NodeFile:
		node.ID = reviewgraph.FileNodeID(legacyNode.RepositoryID, legacyNode.FilePath)
		node.Kind = reviewgraph.NodeFile
		node.Symbol = legacyNode.FilePath
	case legacygraph.NodePackage:
		node.ID = reviewgraph.ModuleNodeID(legacyNode.RepositoryID, legacyNode.CanonicalName)
		node.Kind = reviewgraph.NodeModule
	case legacygraph.NodeTopic:
		node.ID = reviewgraph.EventTopicNodeID("unknown", legacyNode.CanonicalName)
		node.Kind = reviewgraph.NodeEventTopic
	case legacygraph.NodeEndpoint:
		node.ID = reviewgraph.SymbolNodeID(node.Language, legacyNode.RepositoryID, legacyNode.FilePath, "http_endpoint", legacyNode.CanonicalName)
		node.Kind = reviewgraph.NodeHTTPEndpoint
	case legacygraph.NodeSymbol:
		symbolKind := strings.ToLower(legacyNode.Properties["kind"])
		if symbolKind == "" {
			symbolKind = "function"
		}
		node.ID = reviewgraph.SymbolNodeID(node.Language, legacyNode.RepositoryID, legacyNode.FilePath, symbolKind, legacyNode.CanonicalName)
		node.Kind = mapLegacySymbolKind(symbolKind)
		if legacyNode.Location != nil {
			node.StartLine = int(legacyNode.Location.StartLine)
			node.EndLine = int(legacyNode.Location.EndLine)
		}
		node.Signature = legacyNode.Properties["signature"]
		node.Visibility = inferVisibility(node.Language, legacyNode.Properties["name"])
	default:
		return reviewgraph.Node{}, false
	}
	roles := b.inferNodeRoles(legacyNode, node, owners)
	meta.Roles = rolesToStrings(roles)
	node.NodeRole = roleFromSet(roles)
	node.MetadataJSON = reviewsqlite.EncodeJSON(meta)
	return node, true
}

func (b *builder) inferNodeRoles(legacyNode legacygraph.Node, node reviewgraph.Node, owners []string) []reviewgraph.NodeRole {
	roles := []reviewgraph.NodeRole{reviewgraph.RoleNormal}
	if len(owners) > 1 {
		roles = append(roles, reviewgraph.RoleSharedInfra)
	}
	repo := b.repoByID[legacyNode.RepositoryID]
	if repo.Role == repository.RoleSharedLib || repo.Role == repository.RoleInfra {
		roles = append(roles, reviewgraph.RoleSharedInfra)
	}
	if node.Visibility == "public" {
		roles = append(roles, reviewgraph.RolePublicAPI)
	}
	for _, info := range b.servicesByRepo[legacyNode.RepositoryID] {
		for _, entrypoint := range info.Manifest.Entrypoints {
			if filepath.ToSlash(entrypoint) == filepath.ToSlash(legacyNode.FilePath) {
				roles = append(roles, reviewgraph.RoleEntrypoint)
				break
			}
		}
	}
	for _, hint := range repo.BoundaryHints {
		if filepath.ToSlash(hint.Path) == filepath.ToSlash(legacyNode.FilePath) {
			roles = append(roles, reviewgraph.RoleBoundary)
			break
		}
	}
	switch node.Kind {
	case reviewgraph.NodeHTTPEndpoint, reviewgraph.NodeGRPCMethod, reviewgraph.NodeEventTopic, reviewgraph.NodePubSubChannel, reviewgraph.NodeQueue, reviewgraph.NodeSchedulerJob:
		roles = append(roles, reviewgraph.RoleBoundary)
	}
	return dedupeRoles(roles)
}

func (b *builder) shouldDropNode(legacyNode legacygraph.Node) (string, bool) {
	if legacyNode.Kind == legacygraph.NodeTest || strings.EqualFold(legacyNode.Properties["kind"], "test_function") {
		b.counts.DroppedTestNodeCount++
		return "test", true
	}
	matched, generated := b.ignoreRules.Match(legacyNode.FilePath)
	if !matched {
		return "", false
	}
	if generated {
		b.counts.DroppedGeneratedNodes++
		return "generated", true
	}
	b.counts.DroppedIgnoredNodes++
	return "ignored", true
}

func (b *builder) inferServicesFromPath(repoID, filePath string) []string {
	filePath = filepath.ToSlash(filePath)
	result := []string{}
	for _, info := range b.servicesByRepo[repoID] {
		relRoot := filepath.ToSlash(strings.Trim(info.RelRootPath, "/"))
		if relRoot == "" || relRoot == "." {
			result = appendIfMissing(result, info.Manifest.Name)
			continue
		}
		if filePath == relRoot || strings.HasPrefix(filePath, relRoot+"/") {
			result = appendIfMissing(result, info.Manifest.Name)
		}
	}
	sort.Strings(result)
	return result
}

func (b *builder) edgeEvidenceLocation(oldNodeID string) (string, int) {
	legacyNode := b.legacyNodeByID[oldNodeID]
	if legacyNode.FilePath == "" {
		return "", 0
	}
	line := 0
	if legacyNode.Location != nil {
		line = int(legacyNode.Location.StartLine)
	}
	return legacyNode.FilePath, line
}

func mapLegacySymbolKind(kind string) reviewgraph.NodeKind {
	switch kind {
	case "method":
		return reviewgraph.NodeMethod
	case "class":
		return reviewgraph.NodeClass
	case "struct", "interface", "type":
		return reviewgraph.NodeType
	case "route_handler":
		return reviewgraph.NodeHTTPEndpoint
	case "grpc_handler":
		return reviewgraph.NodeGRPCMethod
	default:
		return reviewgraph.NodeFunction
	}
}

func mapLegacyEdgeKind(kind legacygraph.EdgeKind) (reviewgraph.EdgeType, reviewgraph.FlowKind) {
	switch kind {
	case legacygraph.EdgeDefines:
		return reviewgraph.EdgeDefines, reviewgraph.FlowSync
	case legacygraph.EdgeImports:
		return reviewgraph.EdgeImports, reviewgraph.FlowSync
	case legacygraph.EdgeBelongsToService:
		return reviewgraph.EdgeBelongsToService, reviewgraph.FlowSync
	default:
		return reviewgraph.EdgeCalls, reviewgraph.FlowSync
	}
}

var publicJSName = regexp.MustCompile(`^[A-Z]`)

func inferVisibility(language, name string) string {
	if name == "" {
		return ""
	}
	switch language {
	case "go":
		first := rune(name[0])
		if first >= 'A' && first <= 'Z' {
			return "public"
		}
	case "javascript", "typescript":
		if publicJSName.MatchString(name) {
			return "public"
		}
	}
	return "private"
}

func loadJSONFile[T any](path string) (T, error) {
	var payload T
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func loadJSONIfExists[T any](path string) (T, error) {
	var payload T
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return payload, err
	}
	if len(data) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	result := []T{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		result = append(result, item)
	}
	return result, scanner.Err()
}

func (b *builder) addEdge(edge reviewgraph.Edge) {
	if existing, ok := b.edgesByID[edge.ID]; ok {
		if existing.SrcID == edge.SrcID && existing.DstID == edge.DstID && existing.EdgeType == edge.EdgeType {
			return
		}
		edge.ID = reviewgraph.EdgeIDWithEvidence(edge.SrcID, edge.EdgeType, edge.DstID, edge.EvidenceFile, fmt.Sprintf("%d", edge.EvidenceLine), edge.EvidenceText)
	}
	b.edgesByID[edge.ID] = edge
}

func relativePath(root, target string) string {
	if root == "" || target == "" {
		return ""
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func appendIfMissing(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func dedupeRoles(values []reviewgraph.NodeRole) []reviewgraph.NodeRole {
	seen := map[reviewgraph.NodeRole]struct{}{}
	result := make([]reviewgraph.NodeRole, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func rolesToStrings(values []reviewgraph.NodeRole) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	sort.Strings(result)
	return result
}

func roleFromSet(values []reviewgraph.NodeRole) reviewgraph.NodeRole {
	priorities := []reviewgraph.NodeRole{
		reviewgraph.RoleAsyncProducer,
		reviewgraph.RoleAsyncConsumer,
		reviewgraph.RoleScheduler,
		reviewgraph.RoleEntrypoint,
		reviewgraph.RoleBoundary,
		reviewgraph.RolePublicAPI,
		reviewgraph.RoleSharedInfra,
		reviewgraph.RoleNormal,
	}
	for _, priority := range priorities {
		for _, value := range values {
			if value == priority {
				return value
			}
		}
	}
	return reviewgraph.RoleNormal
}

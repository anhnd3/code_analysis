package chain_reduce

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/services/participant_classify"
)

// Request configures the reduction pass.
type Request struct {
	MaxDepth     int    `json:"max_depth,omitempty"`
	MaxBranches  int    `json:"max_branches,omitempty"`
	CollapseMode string `json:"collapse_mode,omitempty"` // "default", "none", "aggressive"
}

// Service reduces stitched flows + linked boundaries into readable chart chains.
type Service struct {
	classifier participant_classify.Service
}

// New creates a chain reducer.
func New() Service {
	return Service{
		classifier: participant_classify.New(),
	}
}

// Reduce takes stitched flows and boundary links, then produces a reduced chain
// suitable for rendering. It applies:
//   - root selection
//   - deterministic walk
//   - helper collapse
//   - cycle protection
//   - branch shaping (alt, par, loop)
//   - safe cross-project crossing rules
func (s Service) Reduce(snapshot graph.GraphSnapshot, flows flow.Bundle, links boundary.Bundle, req Request) (reduced.Chain, error) {
	if len(flows.Chains) == 0 {
		return reduced.Chain{}, nil
	}

	return s.ReduceChain(snapshot, flows.Chains[0], flows.BoundaryMarkers, links, req)
}

// ReduceChain reduces one pre-selected stitched chain while still using the
// aggregate boundary markers and link bundle for cross-boundary shaping.
func (s Service) ReduceChain(snapshot graph.GraphSnapshot, chain flow.Chain, markers []flow.BoundaryMarker, links boundary.Bundle, req Request) (reduced.Chain, error) {
	if chain.RootNodeID == "" {
		return reduced.Chain{}, nil
	}

	cfg := normalizeConfig(req)
	idx := newReduceIndex(snapshot, flow.Bundle{
		Chains:          []flow.Chain{chain},
		BoundaryMarkers: markers,
	}, links)

	// Build reduced nodes and edges
	nodes, edges, blocks, notes := s.walkAndReduce(chain, idx, cfg, snapshot)

	// Sort for deterministic output
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromID != edges[j].FromID {
			return edges[i].FromID < edges[j].FromID
		}
		return edges[i].ToID < edges[j].ToID
	})
	sort.Slice(notes, func(i, j int) bool { return notes[i].AtNodeID < notes[j].AtNodeID })

	return reduced.Chain{
		RootNodeID: chain.RootNodeID,
		Nodes:      nodes,
		Edges:      edges,
		Blocks:     blocks,
		Notes:      notes,
	}, nil
}

type reduceConfig struct {
	maxDepth     int
	maxBranches  int
	collapseMode string
}

func normalizeConfig(req Request) reduceConfig {
	cfg := reduceConfig{
		maxDepth:     req.MaxDepth,
		maxBranches:  req.MaxBranches,
		collapseMode: req.CollapseMode,
	}
	if cfg.maxDepth <= 0 {
		cfg.maxDepth = 30
	}
	if cfg.maxBranches <= 0 {
		cfg.maxBranches = 10
	}
	if cfg.collapseMode == "" {
		cfg.collapseMode = "default"
	}
	return cfg
}

func (s Service) walkAndReduce(chain flow.Chain, idx *reduceIndex, cfg reduceConfig, snapshot graph.GraphSnapshot) ([]reduced.Node, []reduced.Edge, []reduced.Block, []reduced.Note) {
	var nodes []reduced.Node
	var edges []reduced.Edge
	var blocks []reduced.Block
	var notes []reduced.Note

	nodeSet := map[string]bool{}
	visited := map[string]bool{}
	var deferredEdges []reduced.Edge

	orderCounter := 0

	// Always keep the root
	rootNode := idx.nodeByID[chain.RootNodeID]
	rn := s.toReducedNode(rootNode, reduced.RoleRoot, snapshot)
	nodes = append(nodes, rn)
	nodeSet[rn.ID] = true

	// Walk steps
	depth := 0
	branchCount := 0

	for _, step := range chain.Steps {
		if depth >= cfg.maxDepth {
			break
		}
		if visited[step.ToNodeID] {
			continue
		}
		visited[step.ToNodeID] = true

		targetGraphNode := idx.nodeByID[step.ToNodeID]
		role := s.classifyRole(step, targetGraphNode, snapshot)

		// Collapse decision
		if cfg.collapseMode != "none" && s.shouldCollapse(role, targetGraphNode, cfg) {
			// Track collapsed helpers
			if nodeSet[step.FromNodeID] {
				// Find existing from-node and update collapse count
				for i := range nodes {
					if nodes[i].ID == step.FromNodeID {
						nodes[i].CollapseCount++
						break
					}
				}
			}
			continue
		}

		// Keep this node
		rn := s.toReducedNode(targetGraphNode, role, snapshot)

		// Determine cross-repo status for the edge
		crossRepo := false
		linkStatus := ""
		fromGraphNode := idx.nodeByID[step.FromNodeID]

		if strings.HasPrefix(step.ToNodeID, "unresolved_") {
			crossRepo = true
			linkStatus = idx.linkStatusBetween(step.FromNodeID, step.ToNodeID)
			targetGraphNode = graph.Node{
				ID:            step.ToNodeID,
				CanonicalName: strings.TrimPrefix(step.ToNodeID, "unresolved_"),
			}
		} else if fromGraphNode.RepositoryID != "" && targetGraphNode.RepositoryID != "" &&
			fromGraphNode.RepositoryID != targetGraphNode.RepositoryID {
			crossRepo = true
			linkStatus = idx.linkStatusBetween(step.FromNodeID, step.ToNodeID)
		}

		if crossRepo {
			// Cross-project crossing rules
			switch boundary.LinkStatus(linkStatus) {
			case boundary.StatusConfirmed, boundary.StatusCompatibleSubset:
				// May cross
			case boundary.StatusCandidate:
				// Note only, do not confirm
				notes = append(notes, reduced.Note{
					AtNodeID: step.ToNodeID,
					Text:     "candidate cross-project link (not confirmed)",
					Kind:     "candidate",
				})
			case boundary.StatusExternalOnly:
				// Stop at external node
				rn.Role = reduced.RoleRemote
				nodes = append(nodes, rn)
				nodeSet[rn.ID] = true
				edges = append(edges, reduced.Edge{
					FromID:     step.FromNodeID,
					ToID:       step.ToNodeID,
					Label:      step.Label,
					Inferred:   step.Inferred,
					CrossRepo:  true,
					LinkStatus: linkStatus,
				})
				notes = append(notes, reduced.Note{
					AtNodeID: step.ToNodeID,
					Text:     "external-only boundary (no inbound match)",
					Kind:     "candidate",
				})
				continue
			case boundary.StatusMismatch:
				// Warning note, no confirmed cross
				notes = append(notes, reduced.Note{
					AtNodeID: step.FromNodeID,
					Text:     "cross-project link mismatch detected",
					Kind:     "candidate",
				})
				continue
			}

			if linkStatus == string(boundary.StatusCompatibleSubset) {
				notes = append(notes, reduced.Note{
					AtNodeID: step.ToNodeID,
					Text:     "subset-compatible link (client uses subset of server proto)",
					Kind:     "subset",
				})
			}
		}

		if !nodeSet[rn.ID] {
			nodes = append(nodes, rn)
			nodeSet[rn.ID] = true
		}

		// Ensure from-node exists
		if !nodeSet[step.FromNodeID] {
			fn := s.toReducedNode(fromGraphNode, s.classifyRole(flow.Step{}, fromGraphNode, snapshot), snapshot)
			nodes = append(nodes, fn)
			nodeSet[fn.ID] = true
		}

		edge := reduced.Edge{
			FromID:     step.FromNodeID,
			ToID:       step.ToNodeID,
			Label:      step.Label,
			Inferred:   step.Inferred,
			CrossRepo:  crossRepo,
			LinkStatus: linkStatus,
		}

		if step.Kind == flow.StepDefer {
			deferredEdges = append(deferredEdges, edge)
		} else if step.Kind == flow.StepAsync {
			blocks = append(blocks, reduced.Block{
				Kind:       reduced.BlockPar,
				Label:      "goroutine",
				OrderIndex: orderCounter,
				Branches: []reduced.Branch{
					{Edges: []reduced.Edge{edge}},
				},
			})
			orderCounter++
		} else if step.Kind == flow.StepBranch {
			isLoop := strings.HasPrefix(step.Label, "for ") || strings.HasPrefix(step.Label, "range ")
			kind := reduced.BlockAlt
			label := "branch"
			condition := step.Label
			if isLoop {
				kind = reduced.BlockLoop
				label = "for each"
				if strings.Contains(step.Label, "range ") {
					label = "for each " + strings.TrimPrefix(step.Label, "range ")
				}
				condition = ""
			}

			blocks = append(blocks, reduced.Block{
				Kind:       kind,
				Label:      label,
				OrderIndex: orderCounter,
				Branches: []reduced.Branch{
					{
						Condition: condition,
						Edges:     []reduced.Edge{edge},
					},
				},
			})
			orderCounter++
		} else {
			edge.OrderIndex = orderCounter
			edges = append(edges, edge)
			orderCounter++
		}

		if step.Inferred {
			notes = append(notes, reduced.Note{
				AtNodeID: step.ToNodeID,
				Text:     "inferred edge",
				Kind:     "inference",
			})
		}

		if role == reduced.RoleAsync {
			branchCount++
			if branchCount >= cfg.maxBranches {
				notes = append(notes, reduced.Note{
					AtNodeID: step.ToNodeID,
					Text:     "branch limit reached",
					Kind:     "collapse",
				})
				break
			}
		}

		depth++
	}

	for i := range deferredEdges {
		deferredEdges[i].OrderIndex = orderCounter
		orderCounter++
	}
	edges = append(edges, deferredEdges...)

	return nodes, edges, blocks, notes
}

// classifyRole determines what role a node plays in the reduced chain.
func (s Service) classifyRole(step flow.Step, node graph.Node, snapshot graph.GraphSnapshot) reduced.NodeRole {
	// Check step kind first
	switch step.Kind {
	case flow.StepConstruct:
		return reduced.RoleConstructor
	case flow.StepAsync:
		return reduced.RoleAsync
	case flow.StepBoundary:
		return reduced.RoleBoundary
	}

	class := s.classifier.Classify(node, snapshot)
	return class.Role
}

// shouldCollapse returns true if the node should be collapsed (hidden).
func (s Service) shouldCollapse(role reduced.NodeRole, node graph.Node, cfg reduceConfig) bool {
	if cfg.collapseMode == "none" {
		return false
	}

	// Never collapse these
	switch role {
	case reduced.RoleRoot, reduced.RoleHandler, reduced.RoleService, reduced.RoleRepository,
		reduced.RoleConstructor, reduced.RoleProcessor, reduced.RoleBoundary, reduced.RoleRemote,
		reduced.RoleAsync:
		return false
	}

	// Never collapse industry-standard remote/unresolved nodes
	if role == reduced.RoleRemote || strings.HasPrefix(node.ID, "unresolved_") {
		return false
	}

	// Default: collapse helpers
	name := node.Properties["name"]
	if name == "" {
		name = s.deriveShortName(node.CanonicalName)
	}
	if isTinyHelper(name) || isTrivialValidator(name) || isStdlibWrapper(name) {
		return true
	}

	if cfg.collapseMode == "aggressive" {
		return role == reduced.RoleHelper
	}

	return false
}

func isTinyHelper(name string) bool {
	lower := strings.ToLower(name)
	prefixes := []string{"get", "set", "is", "has", "to", "from", "with"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) && len(name) < 12 {
			return true
		}
	}
	return false
}

func isTrivialValidator(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "validate") || strings.Contains(lower, "check") || strings.Contains(lower, "verify")
}

func isStdlibWrapper(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "fmt.") || strings.HasPrefix(lower, "log.") ||
		strings.HasPrefix(lower, "strings.") || strings.HasPrefix(lower, "strconv.")
}

func (s Service) toReducedNode(gn graph.Node, role reduced.NodeRole, snapshot graph.GraphSnapshot) reduced.Node {
	class := s.classifier.Classify(gn, snapshot)

	return reduced.Node{
		ID:            gn.ID,
		CanonicalName: gn.CanonicalName,
		ShortName:     class.ShortName,
		Role:          role,
		RepositoryID:  gn.RepositoryID,
	}
}

func (s Service) deriveShortName(canonical string) string {
	idx := strings.LastIndex(canonical, ".")
	if idx >= 0 {
		return canonical[idx+1:]
	}
	return canonical
}

// --- reduce index ---

type reduceIndex struct {
	nodeByID      map[string]graph.Node
	boundaryNodes map[string]bool
	linksByPair   map[linkPair]boundary.Link
}

type linkPair struct {
	from, to string
}

func newReduceIndex(snapshot graph.GraphSnapshot, flows flow.Bundle, links boundary.Bundle) *reduceIndex {
	idx := &reduceIndex{
		nodeByID:      make(map[string]graph.Node, len(snapshot.Nodes)),
		boundaryNodes: make(map[string]bool),
		linksByPair:   make(map[linkPair]boundary.Link),
	}
	for _, n := range snapshot.Nodes {
		idx.nodeByID[n.ID] = n
	}
	for _, m := range flows.BoundaryMarkers {
		idx.boundaryNodes[m.NodeID] = true
	}
	for _, l := range links.Links {
		idx.linksByPair[linkPair{l.OutboundNodeID, l.InboundNodeID}] = l
	}
	return idx
}

func (idx *reduceIndex) isBoundaryNode(nodeID string) bool {
	return idx.boundaryNodes[nodeID]
}

func (idx *reduceIndex) linkStatusBetween(from, to string) string {
	if l, ok := idx.linksByPair[linkPair{from, to}]; ok {
		return string(l.Status)
	}
	return ""
}

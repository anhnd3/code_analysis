package chain_reduce

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
)

// Request configures the reduction pass.
type Request struct {
	MaxDepth     int    `json:"max_depth,omitempty"`
	MaxBranches  int    `json:"max_branches,omitempty"`
	CollapseMode string `json:"collapse_mode,omitempty"` // "default", "none", "aggressive"
}

// Service reduces stitched flows + linked boundaries into readable chart chains.
type Service struct{}

// New creates a chain reducer.
func New() Service {
	return Service{}
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

	cfg := normalizeConfig(req)
	idx := newReduceIndex(snapshot, flows, links)

	// Use the first flow chain as the primary chain to reduce
	primary := flows.Chains[0]

	// Build reduced nodes and edges
	nodes, edges, notes := s.walkAndReduce(primary, idx, cfg)

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
		RootNodeID: primary.RootNodeID,
		Nodes:      nodes,
		Edges:      edges,
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

func (s Service) walkAndReduce(chain flow.Chain, idx *reduceIndex, cfg reduceConfig) ([]reduced.Node, []reduced.Edge, []reduced.Note) {
	var nodes []reduced.Node
	var edges []reduced.Edge
	var notes []reduced.Note

	nodeSet := map[string]bool{}
	visited := map[string]bool{}

	// Always keep the root
	rootNode := idx.nodeByID[chain.RootNodeID]
	rn := toReducedNode(rootNode, reduced.RoleRoot, idx)
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
		role := classifyRole(step, targetGraphNode, idx)

		// Collapse decision
		if cfg.collapseMode != "none" && shouldCollapse(role, targetGraphNode, cfg) {
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
		rn := toReducedNode(targetGraphNode, role, idx)

		// Determine cross-repo status for the edge
		crossRepo := false
		linkStatus := ""
		fromGraphNode := idx.nodeByID[step.FromNodeID]
		if fromGraphNode.RepositoryID != "" && targetGraphNode.RepositoryID != "" &&
			fromGraphNode.RepositoryID != targetGraphNode.RepositoryID {
			crossRepo = true
			linkStatus = idx.linkStatusBetween(step.FromNodeID, step.ToNodeID)

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
			fn := toReducedNode(fromGraphNode, classifyRole(flow.Step{}, fromGraphNode, idx), idx)
			nodes = append(nodes, fn)
			nodeSet[fn.ID] = true
		}

		edges = append(edges, reduced.Edge{
			FromID:     step.FromNodeID,
			ToID:       step.ToNodeID,
			Label:      step.Label,
			Inferred:   step.Inferred,
			CrossRepo:  crossRepo,
			LinkStatus: linkStatus,
		})

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

	return nodes, edges, notes
}

// classifyRole determines what role a node plays in the reduced chain.
func classifyRole(step flow.Step, node graph.Node, idx *reduceIndex) reduced.NodeRole {
	kind := nodeKindProp(node)

	// Check step kind first
	switch step.Kind {
	case flow.StepConstruct:
		return reduced.RoleConstructor
	case flow.StepAsync:
		return reduced.RoleAsync
	case flow.StepBoundary:
		return reduced.RoleBoundary
	}

	// Check node properties
	switch kind {
	case "route_handler", "grpc_handler":
		return reduced.RoleHandler
	case "consumer", "producer":
		return reduced.RoleProcessor
	case "struct", "interface":
		return reduced.RoleService
	}

	// Check if it's a boundary marker
	if idx.isBoundaryNode(node.ID) {
		return reduced.RoleBoundary
	}

	// Check for constructor by name
	name := nodeShortName(node)
	if isConstructorName(name) {
		return reduced.RoleConstructor
	}

	return reduced.RoleHelper
}

// shouldCollapse returns true if the node should be collapsed (hidden).
func shouldCollapse(role reduced.NodeRole, node graph.Node, cfg reduceConfig) bool {
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

	// Default: collapse helpers
	name := nodeShortName(node)
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

func isConstructorName(name string) bool {
	return strings.HasPrefix(name, "New") || strings.HasPrefix(name, "Create") || strings.HasPrefix(name, "Init")
}

func toReducedNode(gn graph.Node, role reduced.NodeRole, idx *reduceIndex) reduced.Node {
	return reduced.Node{
		ID:            gn.ID,
		CanonicalName: gn.CanonicalName,
		ShortName:     nodeShortName(gn),
		Role:          role,
		RepositoryID:  gn.RepositoryID,
	}
}

func nodeKindProp(n graph.Node) string {
	if n.Properties == nil {
		return ""
	}
	return n.Properties["kind"]
}

func nodeShortName(n graph.Node) string {
	if n.Properties != nil {
		if name := n.Properties["name"]; name != "" {
			return name
		}
	}
	idx := strings.LastIndex(n.CanonicalName, ".")
	if idx >= 0 {
		return n.CanonicalName[idx+1:]
	}
	return n.CanonicalName
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

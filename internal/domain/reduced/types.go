package reduced

// NodeRole classifies the importance of a node in the reduced chain.
type NodeRole string

const (
	RoleRoot       NodeRole = "root"
	RoleHandler    NodeRole = "handler"
	RoleService    NodeRole = "service"
	RoleRepository NodeRole = "repository"
	RoleConstructor NodeRole = "constructor"
	RoleProcessor  NodeRole = "processor"
	RoleBoundary   NodeRole = "boundary"
	RoleRemote     NodeRole = "remote"
	RoleAsync      NodeRole = "async"
	RoleHelper     NodeRole = "helper"
)

// BlockKind classifies branching structure in a reduced chain.
type BlockKind string

const (
	BlockAlt  BlockKind = "alt"
	BlockPar  BlockKind = "par"
	BlockLoop BlockKind = "loop"
)

// Node is a participant in the reduced chain.
type Node struct {
	ID             string   `json:"id"`
	CanonicalName  string   `json:"canonical_name"`
	ShortName      string   `json:"short_name"`
	Role           NodeRole `json:"role"`
	RepositoryID   string   `json:"repository_id,omitempty"`
	ServiceID      string   `json:"service_id,omitempty"`
	Collapsed      bool     `json:"collapsed,omitempty"`
	CollapseCount  int      `json:"collapse_count,omitempty"`
}

// Edge is a directed call in the reduced chain.
type Edge struct {
	FromID     string `json:"from_id"`
	ToID       string `json:"to_id"`
	Label      string `json:"label,omitempty"`
	Inferred   bool   `json:"inferred,omitempty"`
	CrossRepo  bool   `json:"cross_repo,omitempty"`
	LinkStatus string `json:"link_status,omitempty"`
	OrderIndex int    `json:"order_index,omitempty"`
}

// Block represents a branching structure wrapping edges.
type Block struct {
	Kind      BlockKind `json:"kind"`
	Label     string    `json:"label,omitempty"`
	Branches  []Branch  `json:"branches"`
	OrderIndex int      `json:"order_index,omitempty"`
}

// Branch is one arm inside a Block.
type Branch struct {
	Condition string `json:"condition,omitempty"`
	Edges     []Edge `json:"edges"`
}

// Note is an annotation attached to the chain.
type Note struct {
	AtNodeID string `json:"at_node_id"`
	Text     string `json:"text"`
	Kind     string `json:"kind"` // "inference", "collapse", "candidate", "subset"
}

// Chain is the complete reduced output for rendering.
type Chain struct {
	RootNodeID string  `json:"root_node_id"`
	Nodes      []Node  `json:"nodes"`
	Edges      []Edge  `json:"edges"`
	Blocks     []Block `json:"blocks"`
	Notes      []Note  `json:"notes"`
}

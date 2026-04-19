package reviewflow

// MessageKind classifies the semantic intent of a reviewflow message.
type MessageKind string

const (
	MessageSync   MessageKind = "sync"
	MessageAsync  MessageKind = "async"
	MessageReturn MessageKind = "return"
)

// BlockKind classifies structural grouping blocks in the reviewflow model.
type BlockKind string

const (
	BlockAlt     BlockKind = "alt"
	BlockLoop    BlockKind = "loop"
	BlockPar     BlockKind = "par"
	BlockSummary BlockKind = "summary"
)

// Flow is the reviewer-facing abstraction built from a reduced chain plus semantic audit.
type Flow struct {
	ID             string        `json:"id"`
	RootNodeID     string        `json:"root_node_id"`
	CanonicalName  string        `json:"canonical_name"`
	Participants   []Participant `json:"participants"`
	Stages         []Stage       `json:"stages"`
	Blocks         []Block       `json:"blocks,omitempty"`
	Notes          []Note        `json:"notes,omitempty"`
	SourceRootType string        `json:"source_root_type"`
	SourceEvidence string        `json:"source_evidence,omitempty"`
	Metadata       Metadata      `json:"metadata"`
}

// Participant is a stable human-facing actor in the reviewflow.
type Participant struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Label         string   `json:"label"`
	Role          string   `json:"role,omitempty"`
	SourceNodeIDs []string `json:"source_node_ids,omitempty"`
	IsExternal    bool     `json:"is_external,omitempty"`
}

// Stage is a reviewer-facing phase grouping one or more messages.
type Stage struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Label          string    `json:"label"`
	ParticipantIDs []string  `json:"participant_ids,omitempty"`
	Messages       []Message `json:"messages,omitempty"`
	SourceEdgeIDs  []string  `json:"source_edge_ids,omitempty"`
}

// Message is a reviewer-facing interaction between abstract participants.
type Message struct {
	ID                string      `json:"id"`
	FromParticipantID string      `json:"from_participant_id"`
	ToParticipantID   string      `json:"to_participant_id"`
	Label             string      `json:"label"`
	Kind              MessageKind `json:"kind"`
	SourceEdgeIDs     []string    `json:"source_edge_ids,omitempty"`
}

// BlockSection is one arm inside a control block.
type BlockSection struct {
	Label         string    `json:"label,omitempty"`
	Messages      []Message `json:"messages,omitempty"`
	SourceEdgeIDs []string  `json:"source_edge_ids,omitempty"`
}

// Block is a summarized control structure within a stage.
type Block struct {
	ID            string         `json:"id"`
	Kind          BlockKind      `json:"kind"`
	Label         string         `json:"label"`
	StageID       string         `json:"stage_id,omitempty"`
	Sections      []BlockSection `json:"sections,omitempty"`
	SourceEdgeIDs []string       `json:"source_edge_ids,omitempty"`
}

// Note is an annotation attached to the reviewflow.
type Note struct {
	ID                string   `json:"id"`
	OverParticipantID string   `json:"over_participant_id,omitempty"`
	Text              string   `json:"text"`
	Kind              string   `json:"kind,omitempty"`
	SourceNodeIDs     []string `json:"source_node_ids,omitempty"`
	SourceEdgeIDs     []string `json:"source_edge_ids,omitempty"`
}

// Metadata records deterministic selection metadata for a flow candidate.
type Metadata struct {
	CandidateKind string `json:"candidate_kind,omitempty"`
	Signature     string `json:"signature,omitempty"`
	Score         int    `json:"score,omitempty"`
	RootFramework string `json:"root_framework,omitempty"`
}

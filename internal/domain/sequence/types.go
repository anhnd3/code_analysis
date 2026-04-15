package sequence

// Participant is a named actor in the sequence diagram.
type Participant struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	ShortName string `json:"short_name"`
	IsExternal bool  `json:"is_external,omitempty"`
}

// MessageKind classifies signal arrows in the diagram.
type MessageKind string

const (
	MessageSync   MessageKind = "sync"
	MessageAsync  MessageKind = "async"
	MessageReturn MessageKind = "return"
)

// Message is a single arrow in the sequence diagram.
type Message struct {
	FromID string      `json:"from_id"`
	ToID   string      `json:"to_id"`
	Label  string      `json:"label"`
	Kind   MessageKind `json:"kind"`
}

// BlockKind classifies structural grouping blocks.
type BlockKind string

const (
	BlockAlt  BlockKind = "alt"
	BlockPar  BlockKind = "par"
	BlockLoop BlockKind = "loop"
)

// BlockSection is one arm inside a block.
type BlockSection struct {
	Label    string    `json:"label,omitempty"`
	Messages []Message `json:"messages"`
}

// Block wraps messages into alt/par/loop groupings.
type Block struct {
	Kind     BlockKind      `json:"kind"`
	Label    string         `json:"label,omitempty"`
	Sections []BlockSection `json:"sections"`
}

// Note is a text annotation rendered alongside the diagram.
type Note struct {
	OverID string `json:"over_id"`
	Text   string `json:"text"`
}

// Element is a union type for ordered diagram content.
type Element struct {
	Message *Message `json:"message,omitempty"`
	Block   *Block   `json:"block,omitempty"`
	Note    *Note    `json:"note,omitempty"`
}

// Diagram is the complete Mermaid-ready sequence model.
type Diagram struct {
	Title        string        `json:"title,omitempty"`
	Participants []Participant `json:"participants"`
	Elements     []Element     `json:"elements"`
}

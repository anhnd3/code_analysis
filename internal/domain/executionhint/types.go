package executionhint

type HintKind string

const (
	HintReturnHandler HintKind = "return_handler"
	HintSpawn         HintKind = "spawn"
	HintDefer         HintKind = "defer"
	HintWait          HintKind = "wait"
	HintBranch        HintKind = "branch"
)

// Hint captures Go semantics discovered during AST extraction.
type Hint struct {
	SourceSymbolID string   `json:"source_symbol_id"`
	TargetSymbolID string   `json:"target_symbol_id,omitempty"`
	TargetSymbol   string   `json:"target_symbol,omitempty"`
	Kind           HintKind `json:"kind"`
	Evidence       string   `json:"evidence,omitempty"`
	OrderIndex     int      `json:"order_index,omitempty"`
}

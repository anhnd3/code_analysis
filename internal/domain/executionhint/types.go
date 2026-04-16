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
	SourceSymbolID string
	TargetSymbol   string // optionally canonical name or synthetic symbol ID
	Kind           HintKind
	Evidence       string
}

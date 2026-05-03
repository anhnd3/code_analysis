package facts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Service provides fact queries against stored snapshots.
type QueryService struct {
	artifactRoot string
}

// InspectRequest describes inputs for a single-symbol inspection query.
type InspectRequest struct {
	WorkspaceID   string
	SnapshotID    string
	Symbol        string
	ContextWindow int
}

// New creates a new Service with the given artifact root.
func NewQueryService(artifactRoot string) QueryService {
	return QueryService{artifactRoot: filepath.Clean(artifactRoot)}
}

// ResolveSymbol finds a symbol by workspace, snapshot, and selector (id or canonical name).
func (s QueryService) ResolveSymbol(workspaceID, snapshotID, selector string) (SymbolFact, error) {
	store, err := s.openStore(workspaceID, snapshotID)
	if err != nil {
		return SymbolFact{}, err
	}
	defer store.Close()
	return store.ResolveSymbol(workspaceID, snapshotID, selector)
}

// SymbolByID returns a symbol fact by its stable id.
func (s QueryService) SymbolByID(workspaceID, snapshotID, symbolID string) (SymbolFact, error) {
	store, err := s.openStore(workspaceID, snapshotID)
	if err != nil {
		return SymbolFact{}, err
	}
	defer store.Close()
	return store.GetSymbolByID(workspaceID, snapshotID, symbolID)
}

// InspectFunction builds a context packet around the given symbol.
func (s QueryService) InspectFunction(req InspectRequest) (ContextPacket, error) {
	if req.ContextWindow <= 0 {
		req.ContextWindow = 8
	}
	store, err := s.openStore(req.WorkspaceID, req.SnapshotID)
	if err != nil {
		return ContextPacket{}, err
	}
	defer store.Close()

	symbolFact, err := store.ResolveSymbol(req.WorkspaceID, req.SnapshotID, req.Symbol)
	if err != nil {
		return ContextPacket{}, fmt.Errorf("resolve symbol %q: %w", req.Symbol, err)
	}
	fileFact, err := store.GetFileByID(req.WorkspaceID, req.SnapshotID, symbolFact.FileID)
	if err != nil {
		return ContextPacket{}, fmt.Errorf("load file %q: %w", symbolFact.FileID, err)
	}
	imports, err := store.ListImportsByFile(req.WorkspaceID, req.SnapshotID, fileFact.ID)
	if err != nil {
		return ContextPacket{}, fmt.Errorf("list imports: %w", err)
	}
	outgoing, err := store.ListOutgoingCallCandidates(req.WorkspaceID, req.SnapshotID, symbolFact.ID)
	if err != nil {
		return ContextPacket{}, fmt.Errorf("list outgoing candidates: %w", err)
	}
	incoming, err := store.ListIncomingCallCandidates(req.WorkspaceID, req.SnapshotID, symbolFact.ID)
	if err != nil {
		return ContextPacket{}, fmt.Errorf("list incoming candidates: %w", err)
	}
	tests, err := store.ListTestsByFile(req.WorkspaceID, req.SnapshotID, fileFact.ID)
	if err != nil {
		return ContextPacket{}, fmt.Errorf("list tests: %w", err)
	}

	functionSource, surrounding := readContext(fileFact.AbsolutePath, symbolFact.StartLine, symbolFact.EndLine, req.ContextWindow)

	return ContextPacket{
		WorkspaceID:        req.WorkspaceID,
		SnapshotID:         req.SnapshotID,
		RootSymbol:         symbolFact,
		RootFile:           fileFact,
		FunctionSource:     functionSource,
		SurroundingContext: surrounding,
		Imports:            imports,
		OutgoingCandidates: outgoing,
		IncomingCandidates: incoming,
		NearbyTests:        tests,
	}, nil
}

func (s QueryService) openStore(workspaceID, snapshotID string) (*SQLiteStore, error) {
	if workspaceID == "" || snapshotID == "" {
		return nil, fmt.Errorf("workspace id and snapshot id are required")
	}
	path := SQLitePathFor(s.artifactRoot, workspaceID, snapshotID)
	return NewSQLiteStore(path)
}

func readContext(absPath string, startLine, endLine uint32, window int) (string, string) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", ""
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return "", ""
	}
	start := int(startLine)
	end := int(endLine)
	if start <= 0 || start > len(lines) {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = start
	}
	if end < start {
		end = start
	}

	srcStart := start - 1
	srcEnd := end
	if srcStart < 0 {
		srcStart = 0
	}
	if srcEnd > len(lines) {
		srcEnd = len(lines)
	}
	ctxStart := start - window - 1
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := end + window
	if ctxEnd > len(lines) {
		ctxEnd = len(lines)
	}
	functionSource := strings.Join(lines[srcStart:srcEnd], "\n")
	surrounding := strings.Join(lines[ctxStart:ctxEnd], "\n")
	return functionSource, surrounding
}

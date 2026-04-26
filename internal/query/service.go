package query

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"analysis-module/internal/facts"
	factsqlite "analysis-module/internal/store/sqlite"
)

type Service struct {
	artifactRoot string
}

type InspectRequest struct {
	WorkspaceID   string
	SnapshotID    string
	Symbol        string
	ContextWindow int
}

func New(artifactRoot string) Service {
	return Service{artifactRoot: filepath.Clean(artifactRoot)}
}

func (s Service) ResolveSymbol(workspaceID, snapshotID, selector string) (facts.SymbolFact, error) {
	store, err := s.openStore(workspaceID, snapshotID)
	if err != nil {
		return facts.SymbolFact{}, err
	}
	defer store.Close()
	return store.ResolveSymbol(workspaceID, snapshotID, selector)
}

func (s Service) SymbolByID(workspaceID, snapshotID, symbolID string) (facts.SymbolFact, error) {
	store, err := s.openStore(workspaceID, snapshotID)
	if err != nil {
		return facts.SymbolFact{}, err
	}
	defer store.Close()
	return store.GetSymbolByID(workspaceID, snapshotID, symbolID)
}

func (s Service) InspectFunction(req InspectRequest) (facts.ContextPacket, error) {
	if req.ContextWindow <= 0 {
		req.ContextWindow = 8
	}
	store, err := s.openStore(req.WorkspaceID, req.SnapshotID)
	if err != nil {
		return facts.ContextPacket{}, err
	}
	defer store.Close()

	symbolFact, err := store.ResolveSymbol(req.WorkspaceID, req.SnapshotID, req.Symbol)
	if err != nil {
		return facts.ContextPacket{}, fmt.Errorf("resolve symbol %q: %w", req.Symbol, err)
	}
	fileFact, err := store.GetFileByID(req.WorkspaceID, req.SnapshotID, symbolFact.FileID)
	if err != nil {
		return facts.ContextPacket{}, fmt.Errorf("load file %q: %w", symbolFact.FileID, err)
	}
	imports, err := store.ListImportsByFile(req.WorkspaceID, req.SnapshotID, fileFact.ID)
	if err != nil {
		return facts.ContextPacket{}, fmt.Errorf("list imports: %w", err)
	}
	outgoing, err := store.ListOutgoingCallCandidates(req.WorkspaceID, req.SnapshotID, symbolFact.ID)
	if err != nil {
		return facts.ContextPacket{}, fmt.Errorf("list outgoing candidates: %w", err)
	}
	incoming, err := store.ListIncomingCallCandidates(req.WorkspaceID, req.SnapshotID, symbolFact.ID)
	if err != nil {
		return facts.ContextPacket{}, fmt.Errorf("list incoming candidates: %w", err)
	}
	tests, err := store.ListTestsByFile(req.WorkspaceID, req.SnapshotID, fileFact.ID)
	if err != nil {
		return facts.ContextPacket{}, fmt.Errorf("list tests: %w", err)
	}

	functionSource, surrounding := readContext(fileFact.AbsolutePath, symbolFact.StartLine, symbolFact.EndLine, req.ContextWindow)

	return facts.ContextPacket{
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

func (s Service) openStore(workspaceID, snapshotID string) (*factsqlite.Store, error) {
	if workspaceID == "" || snapshotID == "" {
		return nil, fmt.Errorf("workspace id and snapshot id are required")
	}
	path := factsqlite.PathFor(s.artifactRoot, workspaceID, snapshotID)
	return factsqlite.New(path)
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

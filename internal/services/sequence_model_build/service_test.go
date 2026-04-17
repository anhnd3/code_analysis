package sequence_model_build

import (
	"testing"

	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/sequence"
)

func TestBuild_BasicDiagram(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n1", ShortName: "main", Role: reduced.RoleRoot},
			{ID: "n2", ShortName: "NewServer", Role: reduced.RoleConstructor},
			{ID: "n3", ShortName: "Start", Role: reduced.RoleHandler},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n2", Label: "NewServer"},
			{FromID: "n2", ToID: "n3", Label: "Start"},
		},
	}

	diagram, err := New().Build(chain, Options{Title: "Bootstrap"})
	if err != nil {
		t.Fatal(err)
	}

	if diagram.Title != "Bootstrap" {
		t.Errorf("expected title 'Bootstrap', got %q", diagram.Title)
	}
	if len(diagram.Participants) != 3 {
		t.Errorf("expected 3 participants, got %d", len(diagram.Participants))
	}
	// Root should be first participant
	if diagram.Participants[0].ID != "n1" {
		t.Errorf("expected root n1 as first participant, got %s", diagram.Participants[0].ID)
	}
	if len(diagram.Elements) < 2 {
		t.Errorf("expected at least 2 elements, got %d", len(diagram.Elements))
	}
}

func TestBuild_ParticipantOrder(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n3", ShortName: "C"},
			{ID: "n1", ShortName: "A"},
			{ID: "n2", ShortName: "B"},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n3", Label: "callC"},
			{FromID: "n1", ToID: "n2", Label: "callB"},
		},
	}

	diagram, err := New().Build(chain, Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Root first, then edge order
	expected := []string{"n1", "n3", "n2"}
	for i, want := range expected {
		if diagram.Participants[i].ID != want {
			t.Errorf("participant[%d]: expected %s, got %s", i, want, diagram.Participants[i].ID)
		}
	}
}

func TestBuild_CrossRepoAsync(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n1", ShortName: "Client", Role: reduced.RoleRoot, RepositoryID: "repo_a"},
			{ID: "n2", ShortName: "Server", Role: reduced.RoleBoundary, RepositoryID: "repo_b"},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n2", Label: "CreateOrder", CrossRepo: true, LinkStatus: "confirmed"},
		},
	}

	diagram, err := New().Build(chain, Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Cross-repo edge should become async message
	found := false
	for _, elem := range diagram.Elements {
		if elem.Message != nil && elem.Message.Kind == sequence.MessageAsync {
			found = true
			if elem.Message.Label != "CreateOrder [confirmed]" {
				t.Errorf("expected label with link status, got %q", elem.Message.Label)
			}
		}
	}
	if !found {
		t.Errorf("expected async message for cross-repo edge")
	}
}

func TestBuild_ExternalParticipant(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n1", ShortName: "Client", Role: reduced.RoleRoot},
			{ID: "n2", ShortName: "External", Role: reduced.RoleRemote},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n2", Label: "call"},
		},
	}

	diagram, err := New().Build(chain, Options{})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range diagram.Participants {
		if p.ID == "n2" {
			if !p.IsExternal {
				t.Errorf("expected remote node to be marked as external")
			}
			return
		}
	}
	t.Errorf("participant n2 not found")
}

func TestBuild_InferredNote(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n1", ShortName: "A"},
			{ID: "n2", ShortName: "B"},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n2", Label: "call", Inferred: true},
		},
		Notes: []reduced.Note{
			{AtNodeID: "n2", Text: "inferred edge", Kind: "inference"},
		},
	}

	diagram, err := New().Build(chain, Options{})
	if err != nil {
		t.Fatal(err)
	}

	hasNote := false
	hasInferredLabel := false
	for _, elem := range diagram.Elements {
		if elem.Note != nil && elem.Note.Text == "inferred edge" {
			hasNote = true
		}
		if elem.Message != nil && elem.Message.Label == "call [inferred]" {
			hasInferredLabel = true
		}
	}
	if !hasNote {
		t.Errorf("expected note element for inferred edge")
	}
	if !hasInferredLabel {
		t.Errorf("expected message label to contain [inferred]")
	}
}

func TestBuild_EmptyChain(t *testing.T) {
	diagram, err := New().Build(reduced.Chain{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagram.Participants) != 0 {
		t.Errorf("expected 0 participants for empty chain, got %d", len(diagram.Participants))
	}
}

func TestBuild_BlockStructure(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n1", ShortName: "A"},
			{ID: "n2", ShortName: "B"},
			{ID: "n3", ShortName: "C"},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n2", Label: "call"},
		},
		Blocks: []reduced.Block{
			{
				Kind:  reduced.BlockAlt,
				Label: "condition",
				Branches: []reduced.Branch{
					{Condition: "success", Edges: []reduced.Edge{{FromID: "n1", ToID: "n2", Label: "ok"}}},
					{Condition: "failure", Edges: []reduced.Edge{{FromID: "n1", ToID: "n3", Label: "err"}}},
				},
			},
		},
	}

	diagram, err := New().Build(chain, Options{})
	if err != nil {
		t.Fatal(err)
	}

	hasBlock := false
	for _, elem := range diagram.Elements {
		if elem.Block != nil && elem.Block.Kind == sequence.BlockAlt {
			hasBlock = true
			if len(elem.Block.Sections) != 2 {
				t.Errorf("expected 2 sections in alt block, got %d", len(elem.Block.Sections))
			}
		}
	}
	if !hasBlock {
		t.Errorf("expected alt block element")
	}
}

func TestBuild_AvoidDuplicateEdges(t *testing.T) {
	chain := reduced.Chain{
		RootNodeID: "n1",
		Nodes: []reduced.Node{
			{ID: "n1", ShortName: "A"},
			{ID: "n2", ShortName: "B"},
		},
		Edges: []reduced.Edge{
			{FromID: "n1", ToID: "n2", Label: "call", OrderIndex: 0},
		},
		Blocks: []reduced.Block{
			{
				Kind:       reduced.BlockAlt,
				Label:      "condition",
				OrderIndex: 0,
				Branches: []reduced.Branch{
					{Condition: "true", Edges: []reduced.Edge{{FromID: "n1", ToID: "n2", Label: "call"}}},
				},
			},
		},
	}

	diagram, err := New().Build(chain, Options{})
	if err != nil {
		t.Fatal(err)
	}

	edgeCount := 0
	for _, elem := range diagram.Elements {
		if elem.Message != nil {
			edgeCount++
		}
		if elem.Block != nil {
			for _, sec := range elem.Block.Sections {
				edgeCount += len(sec.Messages)
			}
		}
	}

	// Should be 1 because the top-level edge is the same as the block edge (in this test case's logic)
	// and our logic should have skipped the duplicate.
	if edgeCount != 1 {
		t.Errorf("expected 1 edge total (de-duplicated), got %d", edgeCount)
	}
}

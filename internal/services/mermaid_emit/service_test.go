package mermaid_emit

import (
	"strings"
	"testing"

	"analysis-module/internal/domain/sequence"
)

func TestEmit_BootstrapChart(t *testing.T) {
	diagram := sequence.Diagram{
		Title: "Bootstrap",
		Participants: []sequence.Participant{
			{ID: "main", Label: "main", ShortName: "main"},
			{ID: "server", Label: "NewServer", ShortName: "NewServer"},
			{ID: "start", Label: "Start", ShortName: "Start"},
		},
		Elements: []sequence.Element{
			{Message: &sequence.Message{FromID: "main", ToID: "server", Label: "NewServer", Kind: sequence.MessageSync}},
			{Message: &sequence.Message{FromID: "server", ToID: "start", Label: "Start", Kind: sequence.MessageSync}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	golden := `sequenceDiagram
    title Bootstrap
    participant main as main
    participant server as NewServer
    participant start as Start

    main->>server: NewServer
    server->>start: Start
`

	if output != golden {
		t.Errorf("bootstrap chart mismatch:\n--- got ---\n%s\n--- want ---\n%s", output, golden)
	}
}

func TestEmit_HTTPEndpointChart(t *testing.T) {
	diagram := sequence.Diagram{
		Title: "GET /api/users",
		Participants: []sequence.Participant{
			{ID: "handler", Label: "GetUser", ShortName: "GetUser"},
			{ID: "service", Label: "UserService", ShortName: "UserService"},
			{ID: "repo", Label: "UserRepo", ShortName: "UserRepo"},
		},
		Elements: []sequence.Element{
			{Message: &sequence.Message{FromID: "handler", ToID: "service", Label: "FindUser", Kind: sequence.MessageSync}},
			{Message: &sequence.Message{FromID: "service", ToID: "repo", Label: "GetByID", Kind: sequence.MessageSync}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	golden := `sequenceDiagram
    title GET /api/users
    participant handler as GetUser
    participant service as UserService
    participant repo as UserRepo

    handler->>service: FindUser
    service->>repo: GetByID
`

	if output != golden {
		t.Errorf("HTTP endpoint chart mismatch:\n--- got ---\n%s\n--- want ---\n%s", output, golden)
	}
}

func TestEmit_CrossProjectGRPC(t *testing.T) {
	diagram := sequence.Diagram{
		Title: "Cross-Project gRPC",
		Participants: []sequence.Participant{
			{ID: "client", Label: "OrderClient", ShortName: "OrderClient"},
			{ID: "server", Label: "OrderServer", ShortName: "OrderServer"},
		},
		Elements: []sequence.Element{
			{Note: &sequence.Note{OverID: "client", Text: "subset-compatible link"}},
			{Message: &sequence.Message{FromID: "client", ToID: "server", Label: "CreateOrder [compatible_subset]", Kind: sequence.MessageAsync}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	golden := `sequenceDiagram
    title Cross-Project gRPC
    participant client as OrderClient
    participant server as OrderServer

    note over client: subset-compatible link
    client->>+server: CreateOrder [compatible_subset]
`

	if output != golden {
		t.Errorf("cross-project chart mismatch:\n--- got ---\n%s\n--- want ---\n%s", output, golden)
	}
}

func TestEmit_AsyncFanout(t *testing.T) {
	diagram := sequence.Diagram{
		Participants: []sequence.Participant{
			{ID: "producer", Label: "EventProducer", ShortName: "EventProducer"},
			{ID: "consumer1", Label: "Consumer1", ShortName: "Consumer1"},
			{ID: "consumer2", Label: "Consumer2", ShortName: "Consumer2"},
		},
		Elements: []sequence.Element{
			{Message: &sequence.Message{FromID: "producer", ToID: "consumer1", Label: "OrderCreated", Kind: sequence.MessageAsync}},
			{Message: &sequence.Message{FromID: "producer", ToID: "consumer2", Label: "OrderCreated", Kind: sequence.MessageAsync}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, output, "sequenceDiagram")
	assertContains(t, output, "producer->>+consumer1: OrderCreated")
	assertContains(t, output, "producer->>+consumer2: OrderCreated")
}

func TestEmit_ExternalParticipant(t *testing.T) {
	diagram := sequence.Diagram{
		Participants: []sequence.Participant{
			{ID: "client", Label: "Client", ShortName: "Client"},
			{ID: "external", Label: "PaymentGateway", ShortName: "PaymentGateway", IsExternal: true},
		},
		Elements: []sequence.Element{
			{Message: &sequence.Message{FromID: "client", ToID: "external", Label: "Charge", Kind: sequence.MessageAsync}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, output, "participant external as PaymentGateway [external]")
}

func TestEmit_AltBlock(t *testing.T) {
	diagram := sequence.Diagram{
		Participants: []sequence.Participant{
			{ID: "a", Label: "A", ShortName: "A"},
			{ID: "b", Label: "B", ShortName: "B"},
			{ID: "c", Label: "C", ShortName: "C"},
		},
		Elements: []sequence.Element{
			{Block: &sequence.Block{
				Kind:  sequence.BlockAlt,
				Label: "condition check",
				Sections: []sequence.BlockSection{
					{Label: "success", Messages: []sequence.Message{
						{FromID: "a", ToID: "b", Label: "ok", Kind: sequence.MessageSync},
					}},
					{Label: "failure", Messages: []sequence.Message{
						{FromID: "a", ToID: "c", Label: "err", Kind: sequence.MessageSync},
					}},
				},
			}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, output, "alt condition check")
	assertContains(t, output, "a->>b: ok")
	assertContains(t, output, "else failure")
	assertContains(t, output, "a->>c: err")
	assertContains(t, output, "end")
}

func TestEmit_ReturnMessage(t *testing.T) {
	diagram := sequence.Diagram{
		Participants: []sequence.Participant{
			{ID: "a", Label: "A", ShortName: "A"},
			{ID: "b", Label: "B", ShortName: "B"},
		},
		Elements: []sequence.Element{
			{Message: &sequence.Message{FromID: "a", ToID: "b", Label: "call", Kind: sequence.MessageSync}},
			{Message: &sequence.Message{FromID: "b", ToID: "a", Label: "response", Kind: sequence.MessageReturn}},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, output, "a->>b: call")
	assertContains(t, output, "b-->>-a: response")
}

func TestEmit_SanitizeSpecialChars(t *testing.T) {
	diagram := sequence.Diagram{
		Title: "Test; with # special\nchars",
		Participants: []sequence.Participant{
			{ID: "a.b/c", Label: "A.B", ShortName: "AB"},
		},
	}

	output, err := New().Emit(diagram)
	if err != nil {
		t.Fatal(err)
	}

	// Semicolons become commas, # gets escaped, newlines removed
	assertContains(t, output, "title Test, with")
	// ID should be sanitized
	assertContains(t, output, "participant a_b_c as A.B")
}

func TestEmit_EmptyDiagram(t *testing.T) {
	output, err := New().Emit(sequence.Diagram{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(output, "sequenceDiagram") {
		t.Errorf("expected sequenceDiagram header")
	}
}

func TestEmit_Deterministic(t *testing.T) {
	diagram := sequence.Diagram{
		Title: "Test",
		Participants: []sequence.Participant{
			{ID: "a", Label: "A", ShortName: "A"},
			{ID: "b", Label: "B", ShortName: "B"},
		},
		Elements: []sequence.Element{
			{Message: &sequence.Message{FromID: "a", ToID: "b", Label: "call", Kind: sequence.MessageSync}},
		},
	}

	out1, _ := New().Emit(diagram)
	out2, _ := New().Emit(diagram)

	if out1 != out2 {
		t.Errorf("non-deterministic output:\n%s\nvs\n%s", out1, out2)
	}
}

func TestSanitizeID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"a.b.c", "a_b_c"},
		{"path/to/thing", "path_to_thing"},
		{"with-dash", "with_dash"},
		{"123start", "n_123start"},
		{"", "unknown"},
	}
	for _, tc := range cases {
		got := sanitizeID(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeID(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- helpers ---

func assertContains(t *testing.T, output, substr string) {
	t.Helper()
	if !strings.Contains(output, substr) {
		t.Errorf("output missing %q:\n%s", substr, output)
	}
}

package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"analysis-module/internal/facts"
)

func TestDecodeReviewResponseAcceptsStructuredStatuses(t *testing.T) {
	resp, err := decodeReviewResponse(`{
		"decisions": [
			{"target_symbol_id":"sym-1","status":"accepted","rationale":"evidence-backed"},
			{"target_canonical_name":"demo.Child","status":"ambiguous","rationale":"needs more evidence"},
			{"target_canonical_name":"demo.Test","status":"rejected","rationale":"test edge"}
		],
		"notes":["cycle guarded"]
	}`)
	if err != nil {
		t.Fatalf("decodeReviewResponse: %v", err)
	}
	if len(resp.Decisions) != 3 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Decisions[0].Status != facts.StepAccepted || resp.Decisions[1].Status != facts.StepAmbiguous || resp.Decisions[2].Status != facts.StepRejected {
		t.Fatalf("unexpected statuses: %+v", resp.Decisions)
	}
}

func TestDecodeReviewResponseRejectsUnknownFields(t *testing.T) {
	_, err := decodeReviewResponse(`{
		"decisions": [
			{"target_symbol_id":"sym-1","status":"accepted","unexpected":"field"}
		]
	}`)
	if err == nil {
		t.Fatal("expected unknown field validation to fail")
	}
}

func TestDecodeReviewResponseRejectsMissingTargets(t *testing.T) {
	_, err := decodeReviewResponse(`{
		"decisions": [
			{"status":"ambiguous","rationale":"missing target"}
		]
	}`)
	if err == nil {
		t.Fatal("expected missing target validation to fail")
	}
}

func TestOpenAIClientRetriesInvalidJSON(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch calls.Add(1) {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": "not json"}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"decisions":[{"target_symbol_id":"sym-2","status":"accepted","rationale":"ok"}],"notes":["fine"]}`}},
				},
			})
		}
	}))
	defer server.Close()

	client := OpenAIClient{
		BaseURL:    server.URL,
		Model:      "demo",
		Timeout:    time.Second,
		MaxRetries: 1,
	}
	resp, err := client.Review(ReviewRequest{
		RootSymbol: facts.SymbolFact{ID: "sym-1", CanonicalName: "demo.Root"},
		Packet: facts.ContextPacket{
			OutgoingCandidates: []facts.CallCandidate{
				{TargetSymbolID: "sym-2", TargetCanonicalName: "demo.Child"},
			},
		},
		Depth: 1,
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected retry after invalid json, got %d calls", calls.Load())
	}
	if len(resp.Decisions) != 1 || resp.Decisions[0].TargetSymbolID != "sym-2" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestOpenAIClientReportsStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, `upstream unavailable`)
	}))
	defer server.Close()

	client := OpenAIClient{
		BaseURL:    server.URL,
		Model:      "demo",
		Timeout:    time.Second,
		MaxRetries: 0,
	}
	if _, err := client.Review(ReviewRequest{Packet: facts.ContextPacket{OutgoingCandidates: []facts.CallCandidate{{TargetCanonicalName: "demo.Child"}}}}); err == nil {
		t.Fatal("expected status errors to fail")
	}
}

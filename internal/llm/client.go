package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"analysis-module/internal/facts"
)

type HopDecision struct {
	TargetSymbolID      string           `json:"target_symbol_id,omitempty"`
	TargetCanonicalName string           `json:"target_canonical_name,omitempty"`
	Status              facts.StepStatus `json:"status"`
	Rationale           string           `json:"rationale,omitempty"`
}

type ReviewRequest struct {
	RootSymbol facts.SymbolFact    `json:"root_symbol"`
	Packet     facts.ContextPacket `json:"packet"`
	Depth      int                 `json:"depth"`
}

type ReviewResponse struct {
	Decisions []HopDecision `json:"decisions"`
	Notes     []string      `json:"notes,omitempty"`
}

type Client interface {
	Review(req ReviewRequest) (ReviewResponse, error)
}

type NoopClient struct{}

func (NoopClient) Review(req ReviewRequest) (ReviewResponse, error) {
	decisions := make([]HopDecision, 0, len(req.Packet.OutgoingCandidates))
	for _, candidate := range req.Packet.OutgoingCandidates {
		status := facts.StepAmbiguous
		reason := "candidate is unresolved and needs verification"
		if candidate.TargetSymbolID != "" {
			status = facts.StepAccepted
			reason = "target symbol is present in indexed facts"
		}
		if strings.EqualFold(candidate.Relationship, "test") {
			status = facts.StepRejected
			reason = "test edge excluded from runtime flow"
		}
		decisions = append(decisions, HopDecision{
			TargetSymbolID:      candidate.TargetSymbolID,
			TargetCanonicalName: candidate.TargetCanonicalName,
			Status:              status,
			Rationale:           reason,
		})
	}
	return ReviewResponse{Decisions: decisions}, nil
}

type OpenAIClient struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

func (c OpenAIClient) Review(req ReviewRequest) (ReviewResponse, error) {
	if c.BaseURL == "" || c.Model == "" {
		return NoopClient{}.Review(req)
	}
	if c.Timeout <= 0 {
		c.Timeout = 15 * time.Second
	}

	promptBody, _ := json.Marshal(req.Packet)
	requestBody := map[string]any{
		"model": c.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "Classify call candidates into accepted, ambiguous, or rejected. Return strict JSON: {\"decisions\": [...], \"notes\": [...]}.",
			},
			{
				"role":    "user",
				"content": fmt.Sprintf("Depth=%d\nPacket=%s", req.Depth, string(promptBody)),
			},
		},
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return ReviewResponse{}, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ReviewResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ReviewResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ReviewResponse{}, fmt.Errorf("llm status %d", resp.StatusCode)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return ReviewResponse{}, err
	}
	if len(completion.Choices) == 0 {
		return ReviewResponse{}, fmt.Errorf("llm response contained no choices")
	}
	content := completion.Choices[0].Message.Content
	var out ReviewResponse
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return NoopClient{}.Review(req)
	}
	return out, nil
}

package sequence_model_build

import (
	"testing"

	"analysis-module/internal/domain/reviewflow"
)

func TestBuildFromReviewFlow_EmitsStageNotesAndReturnMessages(t *testing.T) {
	flow := reviewflow.Flow{
		ID:            "flow_1",
		RootNodeID:    "root_1",
		CanonicalName: "POST /process",
		Participants: []reviewflow.Participant{
			{ID: "boundary", Label: "POST /process"},
			{ID: "handler", Label: "Handler"},
		},
		Stages: []reviewflow.Stage{
			{
				ID:             "stage_entry",
				Kind:           "boundary_entry",
				Label:          "Boundary Entry",
				ParticipantIDs: []string{"boundary", "handler"},
				Messages: []reviewflow.Message{
					{FromParticipantID: "boundary", ToParticipantID: "handler", Label: "dispatch request", Kind: reviewflow.MessageSync},
				},
			},
			{
				ID:             "stage_response",
				Kind:           "response",
				Label:          "Response",
				ParticipantIDs: []string{"handler", "boundary"},
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "boundary", Label: "return response", Kind: reviewflow.MessageReturn},
				},
			},
		},
	}

	diagram, err := New().BuildFromReviewFlow(flow, Options{Title: "Review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagram.Participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(diagram.Participants))
	}
	if len(diagram.Elements) < 4 {
		t.Fatalf("expected stage notes and messages, got %+v", diagram.Elements)
	}
	if diagram.Elements[0].Note == nil || diagram.Elements[0].Note.Text != "Stage: Boundary Entry" {
		t.Fatalf("expected first element to be boundary entry note, got %+v", diagram.Elements[0])
	}
	foundReturn := false
	for _, element := range diagram.Elements {
		if element.Message != nil && element.Message.Kind == "return" {
			foundReturn = true
			break
		}
	}
	if !foundReturn {
		t.Fatalf("expected return message in diagram, got %+v", diagram.Elements)
	}
}

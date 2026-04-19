package sequence_model_build

import (
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/sequence"
)

// BuildFromReviewFlow converts a reviewflow candidate into a Mermaid-ready sequence diagram.
func (s Service) BuildFromReviewFlow(flow reviewflow.Flow, opts Options) (sequence.Diagram, error) {
	if flow.RootNodeID == "" {
		return sequence.Diagram{}, nil
	}

	participants := make([]sequence.Participant, 0, len(flow.Participants))
	for _, participant := range flow.Participants {
		participants = append(participants, sequence.Participant{
			ID:         participant.ID,
			Label:      participant.Label,
			ShortName:  participant.Label,
			IsExternal: participant.IsExternal,
		})
	}

	var elements []sequence.Element
	for _, stage := range flow.Stages {
		if len(stage.ParticipantIDs) > 0 {
			elements = append(elements, sequence.Element{
				Note: &sequence.Note{
					OverID: stage.ParticipantIDs[0],
					Text:   "Stage: " + stage.Label,
				},
			})
		}
		for _, message := range stage.Messages {
			elements = append(elements, sequence.Element{
				Message: &sequence.Message{
					FromID: message.FromParticipantID,
					ToID:   message.ToParticipantID,
					Label:  message.Label,
					Kind:   mapReviewMessageKind(message.Kind),
				},
			})
		}
		for _, block := range flow.Blocks {
			if block.StageID != "" && block.StageID != stage.ID && block.StageID != stage.Kind {
				continue
			}
			if block.Kind == reviewflow.BlockSummary {
				overID := ""
				if len(stage.ParticipantIDs) > 0 {
					overID = stage.ParticipantIDs[0]
				}
				if overID != "" {
					elements = append(elements, sequence.Element{
						Note: &sequence.Note{
							OverID: overID,
							Text:   block.Label,
						},
					})
				}
				continue
			}
			seqBlock := sequence.Block{
				Kind:  sequence.BlockKind(block.Kind),
				Label: block.Label,
			}
			for _, section := range block.Sections {
				seqSection := sequence.BlockSection{Label: section.Label}
				for _, message := range section.Messages {
					seqSection.Messages = append(seqSection.Messages, sequence.Message{
						FromID: message.FromParticipantID,
						ToID:   message.ToParticipantID,
						Label:  message.Label,
						Kind:   mapReviewMessageKind(message.Kind),
					})
				}
				if len(seqSection.Messages) > 0 {
					seqBlock.Sections = append(seqBlock.Sections, seqSection)
				}
			}
			if len(seqBlock.Sections) > 0 {
				elements = append(elements, sequence.Element{Block: &seqBlock})
			}
		}
	}

	for _, note := range flow.Notes {
		if note.OverParticipantID == "" {
			continue
		}
		elements = append(elements, sequence.Element{
			Note: &sequence.Note{
				OverID: note.OverParticipantID,
				Text:   note.Text,
			},
		})
	}

	return sequence.Diagram{
		Title:        opts.Title,
		Participants: participants,
		Elements:     elements,
	}, nil
}

func mapReviewMessageKind(kind reviewflow.MessageKind) sequence.MessageKind {
	switch kind {
	case reviewflow.MessageAsync:
		return sequence.MessageAsync
	case reviewflow.MessageReturn:
		return sequence.MessageReturn
	default:
		return sequence.MessageSync
	}
}

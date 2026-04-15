package sequence_model_build

import (
	"sort"

	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/sequence"
)

// Options configures the sequence model builder.
type Options struct {
	Title            string `json:"title,omitempty"`
	ServiceShortName string `json:"service_short_name,omitempty"`
}

// Service converts reduced chains into Mermaid-ready sequence models.
type Service struct{}

// New creates a sequence model builder.
func New() Service {
	return Service{}
}

// Build converts a reduced chain into a sequence Diagram by:
//   - mapping unique nodes to stable participants
//   - mapping edges to messages
//   - converting blocks to alt/par/loop structures
//   - converting notes to note elements
func (s Service) Build(chain reduced.Chain, opts Options) (sequence.Diagram, error) {
	if chain.RootNodeID == "" {
		return sequence.Diagram{}, nil
	}

	participants := buildParticipants(chain, opts)
	elements := buildElements(chain, participants)

	return sequence.Diagram{
		Title:        opts.Title,
		Participants: participants,
		Elements:     elements,
	}, nil
}

func buildParticipants(chain reduced.Chain, opts Options) []sequence.Participant {
	// Collect unique participants in edge order for stable layout
	seen := map[string]bool{}
	var ordered []reduced.Node

	// Root first
	for _, n := range chain.Nodes {
		if n.ID == chain.RootNodeID {
			ordered = append(ordered, n)
			seen[n.ID] = true
			break
		}
	}

	// Then in edge order
	for _, e := range chain.Edges {
		for _, id := range []string{e.FromID, e.ToID} {
			if seen[id] {
				continue
			}
			for _, n := range chain.Nodes {
				if n.ID == id {
					ordered = append(ordered, n)
					seen[id] = true
					break
				}
			}
		}
	}

	// Any remaining nodes not referenced in edges
	for _, n := range chain.Nodes {
		if !seen[n.ID] {
			ordered = append(ordered, n)
			seen[n.ID] = true
		}
	}

	var participants []sequence.Participant
	for _, n := range ordered {
		label := participantLabel(n, opts)
		participants = append(participants, sequence.Participant{
			ID:         n.ID,
			Label:      label,
			ShortName:  n.ShortName,
			IsExternal: n.Role == reduced.RoleRemote,
		})
	}

	return participants
}

func participantLabel(n reduced.Node, opts Options) string {
	if n.Collapsed && n.CollapseCount > 0 {
		return n.ShortName + " (collapsed)"
	}
	// If we have a service short name and the node has a different repo, qualify it
	if opts.ServiceShortName != "" && n.RepositoryID != "" {
		return n.ShortName
	}
	return n.ShortName
}

func buildElements(chain reduced.Chain, participants []sequence.Participant) []sequence.Element {
	var elements []sequence.Element
	participantSet := map[string]bool{}
	for _, p := range participants {
		participantSet[p.ID] = true
	}

	// Notes that should appear before their associated edge
	notesByNode := map[string][]reduced.Note{}
	for _, note := range chain.Notes {
		notesByNode[note.AtNodeID] = append(notesByNode[note.AtNodeID], note)
	}

	// Track which notes have been emitted
	emittedNotes := map[int]bool{}

	for _, edge := range chain.Edges {
		// Skip edges referencing unknown participants
		if !participantSet[edge.FromID] || !participantSet[edge.ToID] {
			continue
		}

		// Emit notes for the target node before the message
		for i, note := range chain.Notes {
			if emittedNotes[i] {
				continue
			}
			if note.AtNodeID == edge.ToID || note.AtNodeID == edge.FromID {
				if participantSet[note.AtNodeID] {
					elements = append(elements, sequence.Element{
						Note: &sequence.Note{
							OverID: note.AtNodeID,
							Text:   note.Text,
						},
					})
					emittedNotes[i] = true
				}
			}
		}

		// Determine message kind
		kind := sequence.MessageSync
		if edge.CrossRepo {
			kind = sequence.MessageAsync
		}

		label := edge.Label
		if label == "" {
			label = "call"
		}
		if edge.Inferred {
			label += " [inferred]"
		}
		if edge.CrossRepo && edge.LinkStatus != "" {
			label += " [" + edge.LinkStatus + "]"
		}

		elements = append(elements, sequence.Element{
			Message: &sequence.Message{
				FromID: edge.FromID,
				ToID:   edge.ToID,
				Label:  label,
				Kind:   kind,
			},
		})
	}

	// Emit any remaining notes
	remaining := remainingNotes(chain.Notes, emittedNotes, participantSet)
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].AtNodeID < remaining[j].AtNodeID
	})
	for _, note := range remaining {
		elements = append(elements, sequence.Element{
			Note: &sequence.Note{
				OverID: note.AtNodeID,
				Text:   note.Text,
			},
		})
	}

	// Add block structures
	for _, block := range chain.Blocks {
		seqBlock := sequence.Block{
			Kind:  sequence.BlockKind(block.Kind),
			Label: block.Label,
		}
		for _, branch := range block.Branches {
			section := sequence.BlockSection{
				Label: branch.Condition,
			}
			for _, edge := range branch.Edges {
				if !participantSet[edge.FromID] || !participantSet[edge.ToID] {
					continue
				}
				section.Messages = append(section.Messages, sequence.Message{
					FromID: edge.FromID,
					ToID:   edge.ToID,
					Label:  edge.Label,
					Kind:   sequence.MessageSync,
				})
			}
			seqBlock.Sections = append(seqBlock.Sections, section)
		}
		if len(seqBlock.Sections) > 0 {
			elements = append(elements, sequence.Element{Block: &seqBlock})
		}
	}

	return elements
}

func remainingNotes(notes []reduced.Note, emitted map[int]bool, participantSet map[string]bool) []reduced.Note {
	var out []reduced.Note
	for i, note := range notes {
		if emitted[i] || !participantSet[note.AtNodeID] {
			continue
		}
		out = append(out, note)
	}
	return out
}

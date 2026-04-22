package mermaid_emit

import (
	"fmt"
	"strings"

	"analysis-module/internal/domain/sequence"
)

// Service emits Mermaid sequence diagram code from a sequence model.
type Service struct{}

// New creates a Mermaid emitter.
func New() Service {
	return Service{}
}

// Emit converts a sequence.Diagram into deterministic Mermaid syntax.
// Output is a valid `.mmd` file body.
func (s Service) Emit(diagram sequence.Diagram) (string, error) {
	var b strings.Builder

	// Header
	b.WriteString("sequenceDiagram\n")
	if diagram.Title != "" {
		b.WriteString("    title " + sanitize(diagram.Title) + "\n")
	}

	// Participants
	for _, p := range diagram.Participants {
		alias := sanitizeID(p.ID)
		label := sanitize(p.Label)
		if p.IsExternal {
			b.WriteString(fmt.Sprintf("    participant %s as %s [external]\n", alias, label))
		} else {
			b.WriteString(fmt.Sprintf("    participant %s as %s\n", alias, label))
		}
	}

	if len(diagram.Participants) > 0 {
		b.WriteString("\n")
	}

	// Elements in order
	for _, elem := range diagram.Elements {
		switch {
		case elem.Message != nil:
			writeMessage(&b, *elem.Message, 4)
		case elem.Note != nil:
			writeNote(&b, *elem.Note)
		case elem.Block != nil:
			writeBlock(&b, *elem.Block)
		}
	}

	return b.String(), nil
}

func writeMessage(b *strings.Builder, msg sequence.Message, indent int) {
	padding := strings.Repeat(" ", indent)
	from := sanitizeID(msg.FromID)
	to := sanitizeID(msg.ToID)
	label := sanitize(msg.Label)

	switch msg.Kind {
	case sequence.MessageAsync:
		b.WriteString(fmt.Sprintf("%s%s-)%s: %s\n", padding, from, to, label))
	case sequence.MessageReturn:
		b.WriteString(fmt.Sprintf("%s%s-->>%s: %s\n", padding, from, to, label))
	default: // sync
		b.WriteString(fmt.Sprintf("%s%s->>%s: %s\n", padding, from, to, label))
	}
}

func writeNote(b *strings.Builder, note sequence.Note) {
	over := sanitizeID(note.OverID)
	text := sanitize(note.Text)
	b.WriteString(fmt.Sprintf("    note over %s: %s\n", over, text))
}

func writeBlock(b *strings.Builder, block sequence.Block) {
	keyword := string(block.Kind)
	label := sanitize(block.Label)

	b.WriteString(fmt.Sprintf("    %s %s\n", keyword, label))
	for i, section := range block.Sections {
		if i > 0 {
			sectionLabel := sanitize(section.Label)
			if sectionLabel == "" {
				sectionLabel = "else"
			}
			b.WriteString(fmt.Sprintf("    else %s\n", sectionLabel))
		}
		for _, msg := range section.Messages {
			writeMessage(b, msg, 8)
		}
	}
	b.WriteString("    end\n")
}

// sanitize removes characters that would break Mermaid syntax.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, ";", ",")
	// Mermaid uses # for special chars; escape it
	s = strings.ReplaceAll(s, "#", "＃")
	return strings.TrimSpace(s)
}

// sanitizeID creates a valid Mermaid participant alias.
func sanitizeID(id string) string {
	// Replace characters that are invalid in Mermaid identifiers
	r := strings.NewReplacer(
		"/", "_",
		".", "_",
		"-", "_",
		" ", "_",
		"(", "",
		")", "",
		":", "",
		"@", "_",
	)
	result := r.Replace(id)
	// Ensure it doesn't start with a digit
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "n_" + result
	}
	if result == "" {
		result = "unknown"
	}
	return result
}

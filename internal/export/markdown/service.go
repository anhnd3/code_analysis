package markdown

import (
	"fmt"
	"strings"

	"analysis-module/internal/facts"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Render(flow facts.ReviewFlow) string {
	var out strings.Builder
	out.WriteString("# Flow Review\n\n")
	out.WriteString(fmt.Sprintf("- root: `%s`\n", flow.RootCanonicalName))
	out.WriteString(fmt.Sprintf("- review_id: `%s`\n", flow.ID))
	out.WriteString(fmt.Sprintf("- accepted: `%d`\n", len(flow.Accepted)))
	out.WriteString(fmt.Sprintf("- ambiguous: `%d`\n", len(flow.Ambiguous)))
	out.WriteString(fmt.Sprintf("- rejected: `%d`\n\n", len(flow.Rejected)))

	out.WriteString("## Accepted Steps\n\n")
	if len(flow.Accepted) == 0 {
		out.WriteString("- none\n\n")
	} else {
		for _, step := range flow.Accepted {
			out.WriteString(fmt.Sprintf("- `%s` -> `%s`: %s\n", step.FromCanonicalName, step.ToCanonicalName, safeReason(step.Rationale)))
		}
		out.WriteString("\n")
	}

	out.WriteString("## Ambiguous Steps\n\n")
	if len(flow.Ambiguous) == 0 {
		out.WriteString("- none\n\n")
	} else {
		for _, step := range flow.Ambiguous {
			out.WriteString(fmt.Sprintf("- `%s` -> `%s`: %s\n", step.FromCanonicalName, step.ToCanonicalName, safeReason(step.Rationale)))
		}
		out.WriteString("\n")
	}

	out.WriteString("## Rejected Steps\n\n")
	if len(flow.Rejected) == 0 {
		out.WriteString("- none\n\n")
	} else {
		for _, step := range flow.Rejected {
			out.WriteString(fmt.Sprintf("- `%s` -> `%s`: %s\n", step.FromCanonicalName, step.ToCanonicalName, safeReason(step.Rationale)))
		}
		out.WriteString("\n")
	}

	if len(flow.UncertaintyNotes) > 0 {
		out.WriteString("## Uncertainty Notes\n\n")
		for _, note := range flow.UncertaintyNotes {
			out.WriteString("- " + note + "\n")
		}
		out.WriteString("\n")
	}
	return out.String()
}

func safeReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "no rationale provided"
	}
	return trimmed
}

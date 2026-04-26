package mermaid

import (
	"fmt"
	"sort"
	"strings"

	"analysis-module/internal/facts"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Render(flow facts.ReviewFlow) string {
	var out strings.Builder
	out.WriteString("sequenceDiagram\n")
	participantLabels := map[string]string{}
	participants := map[string]string{}

	addParticipant := func(key, canonical string) {
		if key == "" || canonical == "" {
			return
		}
		label := participantLabel(canonical)
		if existing, ok := participantLabels[label]; ok && existing != canonical {
			label = label + "_" + shortSuffix(key)
		}
		participantLabels[label] = canonical
		participants[key] = label
	}

	addParticipant(flow.RootSymbolID, flow.RootCanonicalName)
	for _, step := range flow.Accepted {
		addParticipant(step.FromSymbolID, step.FromCanonicalName)
		addParticipant(step.ToSymbolID, step.ToCanonicalName)
	}

	labels := make([]string, 0, len(participantLabels))
	for label := range participantLabels {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		out.WriteString(fmt.Sprintf("    participant %s as %s\n", label, quote(participantLabels[label])))
	}
	for _, step := range flow.Accepted {
		from := participants[step.FromSymbolID]
		to := participants[step.ToSymbolID]
		if from == "" {
			from = participantLabel(step.FromCanonicalName)
		}
		if to == "" {
			to = participantLabel(step.ToCanonicalName)
		}
		if from == "" || to == "" {
			continue
		}
		label := "calls"
		if strings.TrimSpace(step.Rationale) != "" {
			label = strings.TrimSpace(step.Rationale)
		}
		out.WriteString(fmt.Sprintf("    %s->>%s: %s\n", from, to, quote(label)))
	}
	return out.String()
}

func participantLabel(canonical string) string {
	base := canonical
	if idx := strings.LastIndex(base, "."); idx >= 0 && idx+1 < len(base) {
		base = base[idx+1:]
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	base = strings.ReplaceAll(base, "-", "_")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, ":", "_")
	base = strings.ReplaceAll(base, " ", "_")
	return base
}

func shortSuffix(value string) string {
	if len(value) <= 6 {
		return value
	}
	return value[len(value)-6:]
}

func quote(value string) string {
	if strings.ContainsAny(value, " \t:>") {
		return "\"" + strings.ReplaceAll(value, "\"", "'") + "\""
	}
	return value
}

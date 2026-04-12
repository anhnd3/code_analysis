package reviewgraph

import (
	"fmt"
	"strings"

	"analysis-module/pkg/ids"
)

func FileNodeID(repo, filePath string) string {
	return fmt.Sprintf("file:%s:%s", safeIDPart(repo), safeIDPart(filePath))
}

func ServiceNodeID(name string) string {
	return fmt.Sprintf("service:%s", safeIDPart(name))
}

func SymbolNodeID(language, repo, filePath, symbolKind, canonical string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", safeIDPart(language), safeIDPart(repo), safeIDPart(filePath), safeIDPart(symbolKind), safeIDPart(canonical))
}

func ModuleNodeID(repo, symbol string) string {
	return fmt.Sprintf("module:%s:%s", safeIDPart(repo), safeIDPart(symbol))
}

func EventTopicNodeID(transport, topic string) string {
	return fmt.Sprintf("event_topic:%s:%s", safeIDPart(transport), safeIDPart(topic))
}

func PubSubChannelNodeID(transport, channel string) string {
	return fmt.Sprintf("pubsub_channel:%s:%s", safeIDPart(transport), safeIDPart(channel))
}

func QueueNodeID(transport, queue string) string {
	return fmt.Sprintf("queue:%s:%s", safeIDPart(transport), safeIDPart(queue))
}

func SchedulerJobNodeID(scope, job string) string {
	return fmt.Sprintf("scheduler_job:%s:%s", safeIDPart(scope), safeIDPart(job))
}

func EdgeID(srcID string, edgeType EdgeType, dstID string) string {
	return fmt.Sprintf("%s->%s->%s", srcID, edgeType, dstID)
}

func EdgeIDWithEvidence(srcID string, edgeType EdgeType, dstID string, discriminator ...string) string {
	base := EdgeID(srcID, edgeType, dstID)
	cleaned := make([]string, 0, len(discriminator))
	for _, item := range discriminator {
		if strings.TrimSpace(item) != "" {
			cleaned = append(cleaned, strings.TrimSpace(item))
		}
	}
	if len(cleaned) == 0 {
		return base
	}
	return fmt.Sprintf("%s:%s", base, ids.StableSuffix(cleaned...))
}

func ArtifactID(artifactType ArtifactType, targetNodeID, path string) string {
	return fmt.Sprintf("artifact:%s:%s", artifactType, ids.StableSuffix(targetNodeID, path))
}

func FlowSlug(raw string) string {
	return ids.Slug(raw)
}

func FlowSlugWithCollision(raw string, discriminator string) string {
	slug := FlowSlug(raw)
	if discriminator == "" {
		return slug
	}
	return fmt.Sprintf("%s_%s", slug, ids.StableSuffix(discriminator))
}

func safeIDPart(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ReplaceAll(raw, ":", "_")
	if raw == "" {
		return "unknown"
	}
	return raw
}

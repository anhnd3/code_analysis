package cross_boundary_link

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
)

// Service links repos/services across gRPC / REST / Kafka boundaries.
type Service struct{}

// New creates a cross-boundary linker.
func New() Service {
	return Service{}
}

// Build scans boundary markers and graph edges for outbound/inbound pairs,
// then classifies each pair using protocol-specific matching rules.
func (s Service) Build(snapshot graph.GraphSnapshot, inventory repository.Inventory, flows flow.Bundle) (boundary.Bundle, error) {
	idx := newLinkIndex(snapshot, flows)

	var links []boundary.Link

	// gRPC linking
	links = append(links, s.matchGRPC(idx)...)

	// REST linking (Milestone 3 full implementation, basic stub here)
	links = append(links, s.matchREST(idx)...)

	// Kafka linking (Milestone 3 full implementation, basic stub here)
	links = append(links, s.matchKafka(idx)...)

	// Unresolved handlers become external links
	links = append(links, s.matchUnresolvedBoundaries(idx)...)

	links = deduplicateLinks(links)
	sort.Slice(links, func(i, j int) bool {
		if links[i].OutboundNodeID != links[j].OutboundNodeID {
			return links[i].OutboundNodeID < links[j].OutboundNodeID
		}
		return links[i].InboundNodeID < links[j].InboundNodeID
	})

	return boundary.Bundle{Links: links}, nil
}

// matchREST performs basic REST linking by matching HTTP method + normalized path.
// Full implementation deferred to Milestone 3.
func (s Service) matchREST(idx *linkIndex) []boundary.Link {
	var links []boundary.Link

	clients := idx.markersByProtocolRole("http", "client")
	servers := idx.markersByProtocolRole("http", "server")

	for _, client := range clients {
		clientPath := normalizeRESTPath(client.Detail)
		if clientPath == "" {
			continue
		}
		for _, server := range servers {
			serverNode, ok := idx.nodeByID[server.NodeID]
			if !ok {
				continue
			}
			// Same repo? skip, that's local
			clientNode, ok := idx.nodeByID[client.NodeID]
			if !ok {
				continue
			}
			if clientNode.RepositoryID == serverNode.RepositoryID {
				continue
			}

			serverPath := normalizeRESTPath(serverNode.CanonicalName)
			if serverPath == "" {
				continue
			}

			if strings.Contains(clientPath, serverPath) || strings.Contains(serverPath, clientPath) {
				links = append(links, boundary.Link{
					OutboundNodeID: client.NodeID,
					InboundNodeID:  server.NodeID,
					Protocol:       boundary.ProtocolREST,
					Status:         boundary.StatusCandidate,
					OutboundRepoID: clientNode.RepositoryID,
					InboundRepoID:  serverNode.RepositoryID,
					Evidence:       "REST path similarity: " + clientPath + " ~ " + serverPath,
				})
			}
		}
	}

	// External-only: clients with no matching servers
	for _, client := range clients {
		hasMatch := false
		for _, link := range links {
			if link.OutboundNodeID == client.NodeID {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			clientNode, ok := idx.nodeByID[client.NodeID]
			if !ok {
				continue
			}
			links = append(links, boundary.Link{
				OutboundNodeID: client.NodeID,
				Protocol:       boundary.ProtocolREST,
				Status:         boundary.StatusExternalOnly,
				OutboundRepoID: clientNode.RepositoryID,
				Evidence:       "no matching REST server in workspace",
			})
		}
	}

	return links
}

// matchKafka performs topic-name matching for Kafka producers/consumers.
// Full schema matching deferred to Milestone 3.
func (s Service) matchKafka(idx *linkIndex) []boundary.Link {
	var links []boundary.Link

	producers := idx.markersByProtocolRole("kafka", "producer")
	consumers := idx.markersByProtocolRole("kafka", "consumer")

	for _, producer := range producers {
		topic := normalizeTopicName(producer.Detail)
		if topic == "" {
			continue
		}
		producerNode, ok := idx.nodeByID[producer.NodeID]
		if !ok {
			continue
		}
		matched := false
		for _, consumer := range consumers {
			consumerTopic := normalizeTopicName(consumer.Detail)
			consumerNode, ok := idx.nodeByID[consumer.NodeID]
			if !ok {
				continue
			}
			if producerNode.RepositoryID == consumerNode.RepositoryID {
				continue
			}
			if topic == consumerTopic {
				links = append(links, boundary.Link{
					OutboundNodeID: producer.NodeID,
					InboundNodeID:  consumer.NodeID,
					Protocol:       boundary.ProtocolKafka,
					Status:         boundary.StatusConfirmed,
					OutboundRepoID: producerNode.RepositoryID,
					InboundRepoID:  consumerNode.RepositoryID,
					Evidence:       "exact topic match: " + topic,
				})
				matched = true
			} else if isSimilarTopic(topic, consumerTopic) {
				links = append(links, boundary.Link{
					OutboundNodeID: producer.NodeID,
					InboundNodeID:  consumer.NodeID,
					Protocol:       boundary.ProtocolKafka,
					Status:         boundary.StatusCandidate,
					OutboundRepoID: producerNode.RepositoryID,
					InboundRepoID:  consumerNode.RepositoryID,
					Evidence:       "similar topic names: " + topic + " ~ " + consumerTopic,
				})
				matched = true
			}
		}
		if !matched {
			links = append(links, boundary.Link{
				OutboundNodeID: producer.NodeID,
				Protocol:       boundary.ProtocolKafka,
				Status:         boundary.StatusExternalOnly,
				OutboundRepoID: producerNode.RepositoryID,
				Evidence:       "no matching Kafka consumer in workspace",
			})
		}
	}

	return links
}

// matchUnresolvedBoundaries marks any EdgeRegistersBoundary targeting an unresolved node as StatusExternalOnly
// so downstream chain reduction converts it into a RoleRemote Mermaid participant.
func (s Service) matchUnresolvedBoundaries(idx *linkIndex) []boundary.Link {
	var links []boundary.Link
	for _, edges := range idx.outgoing {
		for _, e := range edges {
			if e.Kind == graph.EdgeRegistersBoundary {
				if strings.HasPrefix(e.To, "unresolved_") {
					fromNode := idx.nodeByID[e.From]
					links = append(links, boundary.Link{
						OutboundNodeID: e.From,
						InboundNodeID:  e.To,
						Protocol:       boundary.ProtocolREST, // Generic protocol for unresolved boundaries
						Status:         boundary.StatusExternalOnly,
						OutboundRepoID: fromNode.RepositoryID,
						Evidence:       "unresolved boundary handler target",
					})
				}
			}
		}
	}
	return links
}

// --- helpers ---

func normalizeRESTPath(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip method prefix if present (e.g., "GET /api/users")
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) == 2 {
		raw = parts[1]
	}
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return ""
	}
	return strings.ToLower(raw)
}

func normalizeTopicName(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func isSimilarTopic(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func deduplicateLinks(links []boundary.Link) []boundary.Link {
	type key struct {
		outbound string
		inbound  string
		protocol boundary.Protocol
	}
	seen := map[key]bool{}
	var out []boundary.Link
	for _, l := range links {
		k := key{l.OutboundNodeID, l.InboundNodeID, l.Protocol}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, l)
	}
	return out
}

// --- link index ---

type linkIndex struct {
	nodeByID map[string]graph.Node
	outgoing map[string][]graph.Edge
	markers  []flow.BoundaryMarker
}

func newLinkIndex(snapshot graph.GraphSnapshot, flows flow.Bundle) *linkIndex {
	idx := &linkIndex{
		nodeByID: make(map[string]graph.Node, len(snapshot.Nodes)),
		outgoing: make(map[string][]graph.Edge),
		markers:  flows.BoundaryMarkers,
	}
	for _, n := range snapshot.Nodes {
		idx.nodeByID[n.ID] = n
	}
	for _, e := range snapshot.Edges {
		idx.outgoing[e.From] = append(idx.outgoing[e.From], e)
	}
	return idx
}

func (idx *linkIndex) markersByProtocolRole(protocol, role string) []flow.BoundaryMarker {
	var out []flow.BoundaryMarker
	for _, m := range idx.markers {
		if m.Protocol == protocol && m.Role == role {
			out = append(out, m)
		}
	}
	return out
}

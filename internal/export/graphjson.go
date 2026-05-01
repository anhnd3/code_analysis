package export

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"analysis-module/internal/facts"
)

// GraphJSONService renders ReviewFlow as graph JSON with nodes and edges.
type GraphJSONService struct{}

// NewGraphJSONService creates a new GraphJSONService instance.
func NewGraphJSONService() GraphJSONService {
	return GraphJSONService{}
}

// Render renders the flow review as graph JSON containing nodes and edges.
func (s GraphJSONService) Render(flow facts.ReviewFlow) ([]byte, error) {
	nodes := make(map[string]GraphNode)
	edges := []GraphEdge{}

	nodeFor := func(symbolID, canonical string) {
		key := symbolID
		if key == "" || len(key) > 64 {
			key = canonical + "_" + nodeKeyFallback(canonical)
		}
		if _, exists := nodes[key]; !exists {
			nodes[key] = GraphNode{
				Id:       key,
				Label:    participantLabelShort(canonical),
				Fullname: canonical,
			}
		}
	}

	nodeFor(flow.RootSymbolID, flow.RootCanonicalName)
	for _, step := range flow.Accepted {
		nodeFor(step.FromSymbolID, step.FromCanonicalName)
		nodeFor(step.ToSymbolID, step.ToCanonicalName)
	}

	for _, step := range flow.Accepted {
		fromKey := step.FromSymbolID
		if fromKey == "" || len(fromKey) > 64 {
			fromKey = step.FromCanonicalName + "_" + nodeKeyFallback(step.FromCanonicalName)
		}
		toKey := step.ToSymbolID
		if toKey == "" || len(toKey) > 64 {
			toKey = step.ToCanonicalName + "_" + nodeKeyFallback(step.ToCanonicalName)
		}
		edges = append(edges, GraphEdge{
			From: fromKey,
			To:   toKey,
		})
	}

	nodeKeys := make([]string, 0, len(nodes))
	for k := range nodes {
		nodeKeys = append(nodeKeys, k)
	}
	sort.Strings(nodeKeys)

	resultNodeSlice := []GraphNode{}
	for _, key := range nodeKeys {
		resultNodeSlice = append(resultNodeSlice, nodes[key])
	}

	return json.MarshalIndent(GraphJSON{Nodes: resultNodeSlice, Edges: edges}, "", "  ")
}

func nodeKeyFallback(canonical string) string {
	base := canonical
	if idx := strings.LastIndex(base, "/"); idx >= 0 && idx+1 < len(base) {
		base = base[idx+1:]
	}
	return simpleHashString(base)
}

func participantLabelShort(canonical string) string {
	label := canonical
	if idx := strings.LastIndex(canonical, "/"); idx >= 0 && idx+1 < len(canonical) {
		label = canonical[idx+1:]
	}
	return label
}

func simpleHashString(s string) string {
	h := uint32(5381)
	for i := 0; i < len(s); i++ {
		h = h<<5 ^ h ^ uint32(s[i])
	}
	return fmt.Sprintf("%x", h%0xFFFFFFFF)
}

// GraphJSON represents the graph JSON structure.
type GraphJSON struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode represents a node in the graph.
type GraphNode struct {
	Id       string `json:"id"`
	Label    string `json:"label"`
	Fullname string `json:"fullname,omitempty"`
}

// GraphEdge represents an edge in the graph.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

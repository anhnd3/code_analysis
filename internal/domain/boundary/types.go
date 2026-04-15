package boundary

// LinkStatus classifies the cross-project link quality.
type LinkStatus string

const (
	StatusConfirmed        LinkStatus = "confirmed"
	StatusCompatibleSubset LinkStatus = "compatible_subset"
	StatusCandidate        LinkStatus = "candidate"
	StatusMismatch         LinkStatus = "mismatch"
	StatusExternalOnly     LinkStatus = "external_only"
)

// Protocol identifies the inter-service communication mechanism.
type Protocol string

const (
	ProtocolGRPC  Protocol = "grpc"
	ProtocolREST  Protocol = "rest"
	ProtocolKafka Protocol = "kafka"
)

// Identity normalizes cross-project endpoint identity for matching.
type Identity struct {
	Protocol    Protocol `json:"protocol"`
	ServiceName string   `json:"service_name"`
	Endpoint    string   `json:"endpoint"`
	Detail      string   `json:"detail,omitempty"`
}

// ContractField describes a single field in a proto/schema for subset matching.
type ContractField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  int    `json:"tag,omitempty"`
}

// Contract represents the schema shape of a boundary endpoint.
type Contract struct {
	Package        string          `json:"package,omitempty"`
	ServiceName    string          `json:"service_name"`
	RPCName        string          `json:"rpc_name,omitempty"`
	RequestType    string          `json:"request_type,omitempty"`
	ResponseType   string          `json:"response_type,omitempty"`
	RequestFields  []ContractField `json:"request_fields,omitempty"`
	ResponseFields []ContractField `json:"response_fields,omitempty"`
	Method         string          `json:"method,omitempty"`
	Path           string          `json:"path,omitempty"`
	TopicName      string          `json:"topic_name,omitempty"`
	EventType      string          `json:"event_type,omitempty"`
}

// Link is a single matched cross-project connection.
type Link struct {
	OutboundNodeID string     `json:"outbound_node_id"`
	InboundNodeID  string     `json:"inbound_node_id"`
	Protocol       Protocol   `json:"protocol"`
	Status         LinkStatus `json:"status"`
	OutboundRepoID string     `json:"outbound_repo_id"`
	InboundRepoID  string     `json:"inbound_repo_id"`
	Evidence       string     `json:"evidence,omitempty"`
}

// Bundle holds all cross-project links for a snapshot.
type Bundle struct {
	Links []Link `json:"links"`
}

package packet

type Type string

type Item struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type Packet struct {
	Type  Type   `json:"type"`
	Items []Item `json:"items"`
}

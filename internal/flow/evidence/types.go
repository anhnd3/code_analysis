package evidence

type EvidenceItem struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Source   string                 `json:"source"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

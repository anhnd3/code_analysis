package harness

type RoleName string

type SubAgentRole struct {
	Name        RoleName    `json:"name"`
	Description string      `json:"description"`
	InputTypes  []string    `json:"input_types,omitempty"`
	OutputTypes []string    `json:"output_types,omitempty"`
	TokenBudget TokenBudget `json:"token_budget"`
	RetryPolicy RetryPolicy `json:"retry_policy"`
}

type SubAgentTask struct {
	ID             string        `json:"id"`
	Role           RoleName      `json:"role"`
	WorkspaceID    string        `json:"workspace_id,omitempty"`
	SnapshotID     string        `json:"snapshot_id,omitempty"`
	InputArtifacts []ArtifactRef `json:"input_artifacts,omitempty"`
	OutputDir      string        `json:"output_dir"`
}

type ArtifactRef struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type TokenBudget struct {
	MaxInputTokens  int `json:"max_input_tokens"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

type RetryPolicy struct {
	MaxAttempts        int  `json:"max_attempts"`
	RetryOnJSONError   bool `json:"retry_on_json_error"`
	RetryOnSchemaError bool `json:"retry_on_schema_error"`
}

type ValidationReport struct {
	Accepted bool              `json:"accepted"`
	Issues   []ValidationIssue `json:"issues,omitempty"`
	Warnings []ValidationIssue `json:"warnings,omitempty"`
}

type ValidationIssue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Source   string `json:"source,omitempty"`
}

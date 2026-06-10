package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSubAgentRoleJSONRoundtrip(t *testing.T) {
	r := SubAgentRole{
		Name:        "example-role",
		Description: "Example description of the role.",
		InputTypes:  []string{"workspace_scan", "artifact_list"},
		OutputTypes: []string{"architecture_roles", "call_graph"},
		TokenBudget: TokenBudget{MaxInputTokens: 2500, MaxOutputTokens: 1500},
		RetryPolicy: RetryPolicy{MaxAttempts: 3, RetryOnJSONError: true, RetryOnSchemaError: false},
	}

	out, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		t.Fatal(err)
	}

	var r2 SubAgentRole
	if err := json.Unmarshal([]byte(out), &r2); err != nil {
		t.Fatalf("unmarshal failed: %s", err)
	}

	if r2.Name != "example-role" || !strings.Contains(r2.Description, "Example description of the role.") {
		t.Fatal("roundtrip values mismatch")
	}

	if r2.TokenBudget.MaxInputTokens != 2500 || r2.TokenBudget.MaxOutputTokens != 1500 {
		t.Fatal("TokenBudget roundtrip values mismatch")
	}

	if r2.RetryPolicy.MaxAttempts != 3 || !r2.RetryPolicy.RetryOnJSONError {
		t.Fatalf("RetryPolicy roundtrip values mismatch")
	}

	if len(r2.InputTypes) < 1 || len(r2.OutputTypes) < 1 {
		t.Fatal("Input/Output Types underfilled")
	}
}

func TestRoleNameTypeExists(t *testing.T) {
	var _ RoleName = "test-role-name"
}

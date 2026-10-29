package gateway

import (
	"strings"
	"testing"

	wire "opencode2api/internal/protocol"
)

func TestReuseOfficialToolReplacesSameNamedClientDefinition(t *testing.T) {
	payload := map[string]any{"tools": []any{
		map[string]any{"type": "function", "function": map[string]any{
			"name": "grep", "description": "client grep description",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
		}},
	}}
	renameAndCompleteTools(payload, wire.Chat, false, true)
	tools := payload["tools"].([]any)
	grepCount := 0
	for _, raw := range tools {
		tool := raw.(map[string]any)
		fn := tool["function"].(map[string]any)
		if fn["name"] == "grep" {
			grepCount++
			description := fn["description"].(string)
			if strings.Contains(description, "client grep") || !strings.Contains(description, "ripgrep's regular expression syntax") {
				t.Fatalf("client description survived: %q", description)
			}
		}
	}
	if grepCount != 1 {
		t.Fatalf("grep definition count = %d, want 1", grepCount)
	}
}

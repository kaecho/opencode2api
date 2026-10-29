package gateway

import (
	"testing"

	wire "opencode2api/internal/protocol"
)

func chatTool(name string) map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{"name": name}}
}

func TestReuseOfficialToolsReplacesAliases(t *testing.T) {
	payload := map[string]any{"tools": []any{
		chatTool("bash"), chatTool("edit"), chatTool("web_fetch"), chatTool("custom"),
	}}
	renames, _ := renameAndCompleteTools(payload, wire.Chat, false, true)
	tools := payload["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		names = append(names, raw.(map[string]any)["function"].(map[string]any)["name"].(string))
	}
	for _, want := range []string{"shell", "edit", "webfetch", "question"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing official %q in %v", want, names)
		}
	}
	for _, unwanted := range []string{"bash", "web_fetch"} {
		for _, name := range names {
			if name == unwanted {
				t.Fatalf("client alias %q survived: %v", unwanted, names)
			}
		}
	}
	if got, ok := renames.Client("shell"); !ok || got != "bash" {
		t.Fatalf("shell restore = %q, %v; want bash", got, ok)
	}
	if got, ok := renames.Client("webfetch"); !ok || got != "web_fetch" {
		t.Fatalf("webfetch restore = %q, %v; want web_fetch", got, ok)
	}
}

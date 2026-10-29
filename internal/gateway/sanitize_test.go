package gateway

import (
	"strings"
	"testing"
)

func TestTrimClientTools(t *testing.T) {
	tools := make([]any, 25)
	for i := range tools {
		tools[i] = map[string]any{"type": "function", "function": map[string]any{"name": string(rune('a' + i))}}
	}
	payload := map[string]any{"tools": tools}
	if !trimClientTools(payload, 17) {
		t.Fatal("tool list was not trimmed")
	}
	got := payload["tools"].([]any)
	if len(got) != 17 {
		t.Fatalf("tool count = %d, want 17", len(got))
	}
	if name := got[16].(map[string]any)["function"].(map[string]any)["name"]; name != "q" {
		t.Fatalf("last retained tool = %v, want q", name)
	}
	if trimClientTools(payload, 0) {
		t.Fatal("zero limit must disable trimming")
	}
}

func TestApplyZeroWidthKeywords(t *testing.T) {
	payload := map[string]any{
		"messages": []any{
			map[string]any{"role": "developer", "content": "You are an AI agent powered by DeepSeek Harness."},
			map[string]any{"role": "user", "content": "run bash now"},
		},
		"tools": []any{
			map[string]any{"type": "function", "function": map[string]any{
				"name": "bash", "description": "Use bash to inspect files",
			}},
		},
	}
	if !applyZeroWidthKeywords(payload, []string{"AI agent", "bash"}) {
		t.Fatal("keywords were not replaced")
	}
	content := payload["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(strings.ToLower(content), "ai agent") {
		t.Fatalf("literal keyword remains: %q", content)
	}
	if !strings.Contains(content, zeroWidthSpace) {
		t.Fatalf("zero-width marker missing: %q", content)
	}
	tool := payload["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "bash" {
		t.Fatalf("tool name changed: %v", tool["name"])
	}
	description := tool["description"].(string)
	if strings.Contains(strings.ToLower(description), "bash") || !strings.Contains(description, zeroWidthSpace) {
		t.Fatalf("tool description not obfuscated: %q", description)
	}
	args := payload["messages"].([]any)[1].(map[string]any)["content"].(string)
	if !strings.Contains(args, zeroWidthSpace) {
		t.Fatalf("message content not obfuscated: %q", args)
	}
}

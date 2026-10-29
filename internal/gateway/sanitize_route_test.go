package gateway

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"opencode2api/internal/config"
	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
)

func TestPrepareRouteBodiesAppliesSanitizers(t *testing.T) {
	tools := make([]any, 20)
	for i := range tools {
		tools[i] = map[string]any{"type": "function", "function": map[string]any{
			"name": string(rune('a' + i)), "description": "AI agent tool",
		}}
	}
	g := &Gateway{
		cfg: config.Config{
			Upstream: config.UpstreamConfig{Zen: "https://example.invalid/zen"},
			Performance: config.PerformanceConfig{
				MaxClientTools:    17,
				ZeroWidthKeywords: []string{"AI agent"},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	route := models.Route{ID: "free", Tier: config.TierZen, Protocol: wire.Chat}
	input := map[string]any{
		"model": "free",
		"messages": []any{
			map[string]any{"role": "user", "content": "You are an AI agent."},
		},
		"tools": tools,
	}
	bodies, err := g.prepareRouteBodies(wire.Chat, route, input)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bodies[config.TierZen], &payload); err != nil {
		t.Fatal(err)
	}
	if got := len(payload["tools"].([]any)); got != 17 {
		t.Fatalf("tools = %d, want 17", got)
	}
	content := payload["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(strings.ToLower(content), "ai agent") || !strings.Contains(content, zeroWidthSpace) {
		t.Fatalf("message not sanitized: %q", content)
	}
	tool := payload["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "a" {
		t.Fatalf("tool name changed: %v", tool["name"])
	}
}

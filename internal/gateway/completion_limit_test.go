package gateway

import (
	"encoding/json"
	"testing"

	wire "opencode2api/internal/protocol"
)

func TestEnsureV2CompletionLimitDefaults(t *testing.T) {
	payload := map[string]any{}
	if !ensureV2CompletionLimit(payload) {
		t.Fatal("missing ceiling did not change the payload")
	}
	if got := payload["max_completion_tokens"]; got != v2CompletionLimit {
		t.Fatalf("default ceiling = %v, want %d", got, v2CompletionLimit)
	}
}

func TestEnsureV2CompletionLimitHonorsMaxCompletionTokens(t *testing.T) {
	payload := map[string]any{"max_completion_tokens": 456}
	if ensureV2CompletionLimit(payload) {
		t.Fatal("explicit max_completion_tokens changed the payload")
	}
	if got := payload["max_completion_tokens"]; got != 456 {
		t.Fatalf("max_completion_tokens = %v, want 456", got)
	}
}

func TestEnsureV2CompletionLimitMovesMaxTokens(t *testing.T) {
	payload := map[string]any{"max_tokens": 512}
	if !ensureV2CompletionLimit(payload) {
		t.Fatal("max_tokens did not change the payload")
	}
	if got := payload["max_completion_tokens"]; got != 512 {
		t.Fatalf("max_completion_tokens = %v, want 512", got)
	}
	if _, ok := payload["max_tokens"]; ok {
		t.Fatal("max_tokens was not removed")
	}
}

func TestEnsureV2CompletionLimitClientMaxTokensOverridesDefault(t *testing.T) {
	payload := map[string]any{"max_tokens": 512, "max_completion_tokens": v2CompletionLimit}
	if !ensureV2CompletionLimit(payload) {
		t.Fatal("client max_tokens did not change the payload")
	}
	if got := payload["max_completion_tokens"]; got != 512 {
		t.Fatalf("max_completion_tokens = %v, want client value 512", got)
	}
	if _, ok := payload["max_tokens"]; ok {
		t.Fatal("max_tokens was not removed")
	}
}

func TestPrepareAnonymousBodyClientMaxTokensOverrides(t *testing.T) {
	body := []byte(`{"model":"m","messages":[],"max_tokens":512,"max_completion_tokens":32000}`)
	shaped, _ := prepareAnonymousBody(body, wire.Chat, false)
	var payload map[string]any
	if err := json.Unmarshal(shaped, &payload); err != nil {
		t.Fatalf("shaped body is not JSON: %v", err)
	}
	if got := payload["max_completion_tokens"]; got != float64(512) {
		t.Fatalf("max_completion_tokens = %v, want 512", got)
	}
	if _, ok := payload["max_tokens"]; ok {
		t.Fatal("max_tokens survived shaping")
	}
}

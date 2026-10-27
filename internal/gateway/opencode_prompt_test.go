package gateway

import (
	"strings"
	"testing"
)

func TestV2PromptMatches21Shape(t *testing.T) {
	prompt := opencodeSystemPrompt("deepseek-v4.1-flash-free", false)
	for _, want := range []string{
		"You are an AI agent running in OpenCode",
		"Skills provide specialized instructions",
		"Today's date:",
		"Prefer /tmp/opencode",
		"tools.opencode.session_move",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("v2 prompt misses %q", want)
		}
	}
	if strings.Contains(prompt, "You are powered by the model named") {
		t.Fatal("v2 prompt still carries the 1.x model line")
	}
	if strings.Index(prompt, "Skills provide specialized instructions") > strings.Index(prompt, "Today's date:") {
		t.Fatal("v2 prompt places the environment before the skill listing")
	}
}

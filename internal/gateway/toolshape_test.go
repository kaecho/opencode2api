package gateway

import (
	"testing"

	wire "opencode2api/internal/protocol"
)

func chatTool(name string) map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{"name": name}}
}

func toolNames(payload map[string]any) []string {
	tools, _ := payload["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		if tool, ok := raw.(map[string]any); ok {
			names = append(names, tool["function"].(map[string]any)["name"].(string))
		}
	}
	return names
}

func TestRenameAndCompleteToolsSkipsOverlappingOfficialTools(t *testing.T) {
	payload := map[string]any{"tools": []any{
		chatTool("bash"), chatTool("edit"), chatTool("web_fetch"), chatTool("custom"),
	}}
	if _, changed := renameAndCompleteTools(payload, wire.Chat, false, 0); !changed {
		t.Fatal("tool shaping reported no change")
	}
	names := toolNames(payload)
	for _, official := range []string{"shell", "edit", "webfetch"} {
		for _, name := range names {
			if name == official {
				t.Fatalf("overlapping official tool %q was appended: %v", official, names)
			}
		}
	}
	for _, wanted := range []string{"clint_bash", "clint_edit", "clint_web_fetch", "question", "execute"} {
		found := false
		for _, name := range names {
			if name == wanted {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %v", wanted, names)
		}
	}
}

func TestRenameAndCompleteToolsRespectsTotalBudget(t *testing.T) {
	names := []string{
		"bash", "create_goal", "edit", "exit_plan_mode", "get_goal", "glob", "grep",
		"interrupt_agent", "job_kill", "job_list", "job_output", "list_agents", "read",
		"read_image", "send_message", "skill", "subagent", "subagent_fork", "todo_write",
		"update_goal", "web_fetch", "web_search", "workflow", "working_directory", "write",
	}
	tools := make([]any, 0, len(names))
	for _, name := range names {
		tools = append(tools, chatTool(name))
	}
	payload := map[string]any{"tools": tools}
	renameAndCompleteTools(payload, wire.Chat, false, 29)
	if got := len(toolNames(payload)); got != 27 {
		t.Fatalf("total tools = %d, want 27 (25 client + 2 unique official)", got)
	}
}

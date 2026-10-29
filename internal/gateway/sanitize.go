package gateway

import "strings"

const zeroWidthSpace = "\u200b"

// trimClientTools caps client-supplied tools before the official tool set is
// appended. A non-positive limit disables the cap.
func trimClientTools(payload map[string]any, max int) bool {
	if payload == nil || max <= 0 {
		return false
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) <= max {
		return false
	}
	payload["tools"] = tools[:max]
	return true
}

// applyZeroWidthKeywords inserts U+200B inside configured keywords in message
// text, system/instructions, and tool descriptions. Tool names, roles, types,
// ids, and tool-call arguments are left untouched.
func applyZeroWidthKeywords(payload map[string]any, keywords []string) bool {
	if payload == nil || len(keywords) == 0 {
		return false
	}
	changed := false
	for _, field := range []string{"system", "instructions"} {
		if value, ok := payload[field]; ok {
			if next, ok := sanitizeKeywordValue(value, keywords); ok {
				payload[field] = next
				changed = true
			}
		}
	}
	if messages, ok := payload["messages"].([]any); ok {
		for _, raw := range messages {
			message, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := message["content"]; ok {
				if next, ok := sanitizeKeywordValue(content, keywords); ok {
					message["content"] = next
					changed = true
				}
			}
		}
	}
	if input, ok := payload["input"].([]any); ok {
		for _, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := item["content"]; ok {
				if next, ok := sanitizeKeywordValue(content, keywords); ok {
					item["content"] = next
					changed = true
				}
			}
		}
	}
	if tools, ok := payload["tools"].([]any); ok {
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if fn, ok := tool["function"].(map[string]any); ok {
				if next, ok := sanitizeKeywordString(fn["description"], keywords); ok {
					fn["description"] = next
					changed = true
				}
				continue
			}
			if next, ok := sanitizeKeywordString(tool["description"], keywords); ok {
				tool["description"] = next
				changed = true
			}
		}
	}
	return changed
}

func sanitizeKeywordValue(value any, keywords []string) (any, bool) {
	switch current := value.(type) {
	case string:
		return sanitizeKeywordString(current, keywords)
	case []any:
		out := make([]any, len(current))
		changed := false
		for i, item := range current {
			out[i] = item
			if next, ok := sanitizeKeywordValue(item, keywords); ok {
				out[i] = next
				changed = true
			}
		}
		return out, changed
	case map[string]any:
		out := make(map[string]any, len(current))
		changed := false
		for key, item := range current {
			out[key] = item
			if key == "type" || key == "role" || key == "name" || key == "id" {
				continue
			}
			if next, ok := sanitizeKeywordValue(item, keywords); ok {
				out[key] = next
				changed = true
			}
		}
		return out, changed
	default:
		return value, false
	}
}

func sanitizeKeywordString(value any, keywords []string) (string, bool) {
	text, ok := value.(string)
	if !ok || text == "" {
		return text, false
	}
	changed := false
	for _, raw := range keywords {
		keyword := strings.TrimSpace(raw)
		if keyword == "" {
			continue
		}
		lowerText := strings.ToLower(text)
		lowerKeyword := strings.ToLower(keyword)
		for {
			index := strings.Index(lowerText, lowerKeyword)
			if index < 0 {
				break
			}
			end := index + len(keyword)
			text = text[:index] + insertZeroWidth(text[index:end]) + text[end:]
			lowerText = lowerText[:index] + insertZeroWidth(lowerText[index:end]) + lowerText[end:]
			changed = true
		}
	}
	return text, changed
}

func insertZeroWidth(text string) string {
	runes := []rune(text)
	if len(runes) < 2 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + len(runes) - 1)
	for i, r := range runes {
		if i > 0 {
			out.WriteString(zeroWidthSpace)
		}
		out.WriteRune(r)
	}
	return out.String()
}

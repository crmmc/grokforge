package msgutil

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
)

// BuildToolPrompt generates a system prompt with tool definitions.
// Returns empty string if tools is empty or toolChoice is "none".
func BuildToolPrompt(tools []upstream.Tool, toolChoice any, parallelCalls bool) string {
	if len(tools) == 0 {
		return ""
	}

	choiceStr, ok := toolChoice.(string)
	if ok && choiceStr == "none" {
		return ""
	}

	var sb strings.Builder
	appendToolPromptHeader(&sb)
	appendToolDefinitions(&sb, tools)
	appendToolPolicy(&sb, toolChoice, choiceStr, parallelCalls)
	return sb.String()
}

func appendToolPromptHeader(sb *strings.Builder) {
	sb.WriteString("You are a function calling AI assistant. You are provided with function signatures within <tools></tools> XML tags. ")
	sb.WriteString("You may call one or more functions to assist with the user query. Don't make assumptions about what values to plug into functions.\n\n")
	sb.WriteString("For each function call return a JSON object with function name and arguments within <tool_call></tool_call> XML tags:\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString("{\"name\": \"<function_name>\", \"arguments\": {\"<arg_name>\": <value>}}\n")
	sb.WriteString("</tool_call>\n\n")
}

func appendToolDefinitions(sb *strings.Builder, tools []upstream.Tool) {
	sb.WriteString("<tools>\n")
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		toolJSON, err := json.Marshal(tool.Function)
		if err != nil {
			continue
		}
		sb.WriteString(string(toolJSON))
		sb.WriteByte('\n')
	}
	sb.WriteString("</tools>\n\n")
}

func appendToolPolicy(sb *strings.Builder, toolChoice any, choiceStr string, parallelCalls bool) {
	if !parallelCalls {
		sb.WriteString("You may only make one tool call at a time.\n")
	}

	if choiceStr == "required" {
		sb.WriteString("You MUST call at least one tool in your response. Do not respond with only text.\n")
	} else if choiceMap, ok := toolChoice.(map[string]any); ok {
		if fn, ok := choiceMap["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				sb.WriteString(fmt.Sprintf("You MUST call the \"%s\" function in your response.\n", name))
			}
		}
	}
}

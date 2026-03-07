package flow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Tool represents an OpenAI-compatible tool definition.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function represents a function definition within a tool.
type Function struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall represents a tool call in the response.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
	Index    *int         `json:"index,omitempty"`
}

// FunctionCall represents the function invocation details.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

var toolCallRegex = regexp.MustCompile(`<tool_call>\s*([\s\S]*?)\s*</tool_call>`)

// BuildToolPrompt generates a system prompt with tool definitions.
// Returns empty string if tools is empty or toolChoice is "none".
func BuildToolPrompt(tools []Tool, toolChoice any, parallelCalls bool) string {
	if len(tools) == 0 {
		return ""
	}

	// Handle tool_choice
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

func appendToolDefinitions(sb *strings.Builder, tools []Tool) {
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

// genToolCallID generates a tool call ID matching Python grok2api format:
// "call_" + first 24 hex characters of a UUID.
func genToolCallID() string {
	return "call_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
}

// ParseToolCalls extracts tool calls from response content.
// Returns the remaining text and parsed tool calls.
// When tools is non-nil, only tool calls with names matching the provided tools are accepted.
func ParseToolCalls(content string, tools ...[]Tool) (string, []ToolCall) {
	matchIndexes := toolCallRegex.FindAllStringSubmatchIndex(content, -1)
	if len(matchIndexes) == 0 {
		return content, nil
	}

	// Build valid names set from tools if provided
	var validNames map[string]struct{}
	if len(tools) > 0 && len(tools[0]) > 0 {
		validNames = make(map[string]struct{}, len(tools[0]))
		for _, t := range tools[0] {
			if t.Function.Name != "" {
				validNames[t.Function.Name] = struct{}{}
			}
		}
	}

	var calls []ToolCall
	var textBuilder strings.Builder
	cursor := 0
	for _, idx := range matchIndexes {
		start, end := idx[0], idx[1]
		payloadStart, payloadEnd := idx[2], idx[3]
		textBuilder.WriteString(content[cursor:start])

		rawBlock := content[start:end]
		rawPayload := content[payloadStart:payloadEnd]
		if call := parseToolCallPayload(rawPayload, validNames, nil); call != nil {
			calls = append(calls, *call)
		} else {
			textBuilder.WriteString(rawBlock)
		}
		cursor = end
	}
	textBuilder.WriteString(content[cursor:])

	if len(calls) == 0 {
		return content, nil
	}

	text := strings.TrimSpace(textBuilder.String())
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleaned = append(cleaned, line)
		}
	}
	text = strings.Join(cleaned, "\n")

	return text, calls
}

// ParseToolCallBlock parses a single <tool_call> block payload into a ToolCall.
// Returns nil if the payload is invalid or the tool name is not allowed.
func ParseToolCallBlock(raw string, tools []Tool, index int) *ToolCall {
	validNames := allowedToolNames(tools)
	idx := index
	return parseToolCallPayload(raw, validNames, &idx)
}

func allowedToolNames(tools []Tool) map[string]struct{} {
	if len(tools) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(tools))
	if len(tools) > 0 {
		for _, t := range tools {
			if t.Function.Name != "" {
				allowed[t.Function.Name] = struct{}{}
			}
		}
	}
	return allowed
}

func parseToolCallPayload(raw string, validNames map[string]struct{}, index *int) *ToolCall {
	jsonStr := strings.TrimSpace(raw)
	if jsonStr == "" {
		return nil
	}
	jsonStr = repairJSON(jsonStr)

	var parsed struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil || parsed.Name == "" {
		return nil
	}
	if validNames != nil {
		if _, ok := validNames[parsed.Name]; !ok {
			return nil
		}
	}

	call := &ToolCall{
		ID:   genToolCallID(),
		Type: "function",
		Function: FunctionCall{
			Name:      parsed.Name,
			Arguments: normalizeArguments(parsed.Arguments),
		},
	}
	if index != nil {
		idx := *index
		call.Index = &idx
	}
	return call
}

// normalizeArguments converts arguments to a JSON string.
// Handles both string and object/dict arguments.
func normalizeArguments(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}

	// Try to unmarshal as string first (double-encoded JSON)
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return repairJSON(s)
	}

	// Already a JSON object/value — return as-is (repaired)
	return repairJSON(string(raw))
}

var (
	codeFenceStart = regexp.MustCompile("^```[a-zA-Z0-9_-]*\\s*")
	codeFenceEnd   = regexp.MustCompile("\\s*```$")
	trailingComma  = regexp.MustCompile(`,\s*([}\]])`)
)

// repairJSON attempts to fix common JSON issues from LLM output.
func repairJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Step 1: Strip code fences (```json ... ```)
	s = stripCodeFences(s)

	// Step 2: Extract JSON object ({...})
	s = extractJSONObject(s)

	// Step 3: Normalize whitespace
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Step 4: Remove trailing commas before } or ]
	s = trailingComma.ReplaceAllString(s, "$1")

	// Step 5: Balance brackets
	s = balanceBrackets(s)

	return s
}

// stripCodeFences removes markdown code fence delimiters.
func stripCodeFences(s string) string {
	cleaned := strings.TrimSpace(s)
	if !strings.HasPrefix(cleaned, "```") {
		return s
	}
	cleaned = codeFenceStart.ReplaceAllString(cleaned, "")
	cleaned = codeFenceEnd.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

// extractJSONObject extracts content between first '{' and last '}'.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}
	end := strings.LastIndex(s, "}")
	if end == -1 || end < start {
		return s[start:]
	}
	return s[start : end+1]
}

// balanceBrackets closes unclosed brackets.
func balanceBrackets(s string) string {
	var stack []rune
	inString := false
	escape := false

	for _, r := range s {
		if escape {
			escape = false
			continue
		}
		if r == '\\' && inString {
			escape = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}

		switch r {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == r {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Close unclosed brackets in reverse order
	for i := len(stack) - 1; i >= 0; i-- {
		s += string(stack[i])
	}

	return s
}

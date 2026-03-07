package openai

import (
	"encoding/base64"
	"strings"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
)

const (
	errTypeInvalidRequest  = "invalid_request_error"
	defaultStreamDisabled  = false
	minBase64LikeLength    = 32
	defaultChatTemperature = 0.8
	defaultChatTopP        = 0.95
	minTemperature         = 0.0
	maxTemperature         = 2.0
	minTopP                = 0.0
	maxTopP                = 1.0
)

var (
	validRoles = map[string]struct{}{
		"developer": {},
		"system":    {},
		"user":      {},
		"assistant": {},
		"tool":      {},
	}
	userContentTypes = map[string]struct{}{
		"text":        {},
		"image_url":   {},
		"input_audio": {},
		"file":        {},
	}
	allowedReasoningEffort = map[string]struct{}{
		"none":    {},
		"minimal": {},
		"low":     {},
		"medium":  {},
		"high":    {},
		"xhigh":   {},
	}
)

type chatValidationError struct {
	status  int
	errType string
	code    string
	message string
}

func normalizeChatRequest(req *ChatRequest, cfg *config.Config) (*ChatRequest, *chatValidationError) {
	if req == nil {
		return nil, invalidRequest("invalid_request", "request is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, invalidRequest("missing_model", "model is required")
	}

	out := *req
	if err := validateMessages(out.Messages); err != nil {
		return nil, err
	}
	if err := validateTools(out.Tools); err != nil {
		return nil, err
	}
	if err := validateToolChoice(out.ToolChoice); err != nil {
		return nil, err
	}
	if choiceStr, ok := out.ToolChoice.(string); ok {
		out.ToolChoice = strings.ToLower(strings.TrimSpace(choiceStr))
	}

	if out.ReasoningEffort != "" {
		normalized := strings.ToLower(strings.TrimSpace(out.ReasoningEffort))
		if _, ok := allowedReasoningEffort[normalized]; !ok {
			return nil, invalidRequest("invalid_reasoning_effort", "reasoning_effort must be one of none, minimal, low, medium, high, xhigh")
		}
		out.ReasoningEffort = normalized
	}

	if tempErr := normalizeTemperature(&out); tempErr != nil {
		return nil, tempErr
	}
	if topPErr := normalizeTopP(&out); topPErr != nil {
		return nil, topPErr
	}

	streamDefault := defaultStreamDisabled
	if cfg != nil {
		streamDefault = cfg.App.Stream
	}
	if out.Stream == nil {
		streamDefaultCopy := streamDefault
		out.Stream = &streamDefaultCopy
	}

	if out.ParallelToolCalls == nil {
		enabled := true
		out.ParallelToolCalls = &enabled
	}

	return &out, nil
}

func validateTools(tools []flow.Tool) *chatValidationError {
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "function" {
			return invalidRequest("invalid_tool_type", "each tool must have type='function'")
		}
		if strings.TrimSpace(tool.Function.Name) == "" {
			return invalidRequest("missing_function_name", "each tool function must have a name")
		}
	}
	return nil
}

func validateToolChoice(choice any) *chatValidationError {
	if choice == nil {
		return nil
	}
	if choiceStr, ok := choice.(string); ok {
		normalized := strings.ToLower(strings.TrimSpace(choiceStr))
		if normalized == "auto" || normalized == "required" || normalized == "none" {
			return nil
		}
		return invalidRequest("invalid_tool_choice", "tool_choice must be 'auto', 'required', 'none', or a function object")
	}
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return invalidRequest("invalid_tool_choice", "tool_choice must be 'auto', 'required', 'none', or a function object")
	}
	choiceType, _ := choiceMap["type"].(string)
	if strings.TrimSpace(choiceType) != "function" {
		return invalidRequest("invalid_tool_choice", "tool_choice object must have type='function' and function.name")
	}
	fnMap, _ := choiceMap["function"].(map[string]any)
	name, _ := fnMap["name"].(string)
	if strings.TrimSpace(name) == "" {
		return invalidRequest("invalid_tool_choice", "tool_choice object must have type='function' and function.name")
	}
	return nil
}

func validateMessages(messages []ChatMessage) *chatValidationError {
	if len(messages) == 0 {
		return invalidRequest("invalid_messages", "messages is required and must not be empty")
	}
	for i := range messages {
		if err := validateMessage(messages[i], i); err != nil {
			return err
		}
	}
	return nil
}

func validateMessage(msg ChatMessage, index int) *chatValidationError {
	role := strings.TrimSpace(msg.Role)
	if role == "" {
		return invalidRequest("invalid_role", "role must be one of developer, system, user, assistant, tool")
	}
	if _, ok := validRoles[role]; !ok {
		return invalidRequest("invalid_role", "role must be one of developer, system, user, assistant, tool")
	}

	if role == "tool" {
		if strings.TrimSpace(msg.ToolCallID) == "" {
			return invalidRequest("missing_tool_call_id", "tool messages must have a tool_call_id")
		}
		return nil
	}

	if role == "assistant" && len(msg.ToolCalls) > 0 && msg.Content == nil {
		return nil
	}

	if msg.Content == nil {
		return invalidRequest("empty_content", "message content cannot be null")
	}

	switch content := msg.Content.(type) {
	case string:
		if strings.TrimSpace(content) == "" {
			return invalidRequest("empty_content", "message content cannot be empty")
		}
		return nil
	case map[string]any:
		return validateSingleContentObject(content, role)
	case []any:
		if len(content) == 0 {
			return invalidRequest("empty_content", "message content cannot be an empty array")
		}
		for blockIdx, raw := range content {
			block, ok := raw.(map[string]any)
			if !ok {
				return invalidRequest("invalid_block", "content block must be an object")
			}
			if err := validateContentBlock(block, role, blockIdx); err != nil {
				return err
			}
		}
		return nil
	default:
		return invalidRequest("invalid_content", "message content must be a string or array")
	}
}

func validateSingleContentObject(content map[string]any, role string) *chatValidationError {
	if content == nil {
		return invalidRequest("invalid_content_item", "message content items must be objects")
	}
	blockType, ok := content["type"].(string)
	if !ok || strings.TrimSpace(blockType) == "" {
		return invalidRequest("invalid_content_type", "when content is an object, type must be 'text'")
	}
	if blockType != "text" {
		return invalidRequest("invalid_content_type", "when content is an object, type must be 'text'")
	}
	text, _ := content["text"].(string)
	if strings.TrimSpace(text) == "" {
		return invalidRequest("empty_content", "text content cannot be empty")
	}
	return nil
}

func validateContentBlock(block map[string]any, role string, _ int) *chatValidationError {
	if len(block) == 0 {
		return invalidRequest("empty_block", "content block cannot be empty")
	}
	rawType, ok := block["type"]
	if !ok {
		return invalidRequest("missing_type", "content block must have a type field")
	}
	blockType, ok := rawType.(string)
	if !ok || strings.TrimSpace(blockType) == "" {
		return invalidRequest("empty_type", "content block type cannot be empty")
	}

	if role == "user" {
		if _, ok := userContentTypes[blockType]; !ok {
			return invalidRequest("invalid_type", "invalid content block type")
		}
	} else if blockType != "text" {
		return invalidRequest("invalid_type", "only text content is supported for this role")
	}

	switch blockType {
	case "text":
		text, _ := block["text"].(string)
		if strings.TrimSpace(text) == "" {
			return invalidRequest("empty_text", "text content cannot be empty")
		}
	case "image_url":
		media, ok := block["image_url"].(map[string]any)
		if !ok {
			return invalidRequest("missing_url", "image_url must have a url field")
		}
		url, _ := media["url"].(string)
		if err := validateMediaInput(url); err != nil {
			return err
		}
	case "input_audio":
		media, ok := block["input_audio"].(map[string]any)
		if !ok {
			return invalidRequest("missing_audio", "input_audio must have a data field")
		}
		url, _ := media["data"].(string)
		if err := validateMediaInput(url); err != nil {
			return err
		}
	case "file":
		media, ok := block["file"].(map[string]any)
		if !ok {
			return invalidRequest("missing_file", "file must have a file_data field")
		}
		url, _ := media["file_data"].(string)
		if err := validateMediaInput(url); err != nil {
			return err
		}
	}

	return nil
}

func validateMediaInput(value string) *chatValidationError {
	if value == "" {
		return invalidRequest("invalid_media", "media input cannot be empty")
	}
	if strings.HasPrefix(value, "data:") {
		return nil
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return nil
	}
	if looksLikeBase64(value) {
		return invalidRequest("invalid_media", "base64 media must be provided as a data URI (data:<mime>;base64,...)")
	}
	return invalidRequest("invalid_media", "media must be a URL or data URI")
}

func looksLikeBase64(value string) bool {
	candidate := strings.Join(strings.Fields(value), "")
	if len(candidate) < minBase64LikeLength || len(candidate)%4 != 0 {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(candidate)
	return err == nil
}

func invalidRequest(code, message string) *chatValidationError {
	return &chatValidationError{
		status:  400,
		errType: errTypeInvalidRequest,
		code:    code,
		message: message,
	}
}

func isStreamEnabled(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func shouldShowThinking(req *ChatRequest, cfg *config.Config) bool {
	if req != nil && strings.TrimSpace(req.ReasoningEffort) != "" {
		return strings.ToLower(strings.TrimSpace(req.ReasoningEffort)) != "none"
	}
	if cfg != nil {
		return cfg.App.Thinking
	}
	return false
}

func toolCallsEnabled(req *ChatRequest) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	if choice, ok := req.ToolChoice.(string); ok && choice == "none" {
		return false
	}
	return true
}

func normalizeTemperature(req *ChatRequest) *chatValidationError {
	if req.Temperature == nil {
		val := defaultChatTemperature
		req.Temperature = &val
		return nil
	}
	if *req.Temperature < minTemperature || *req.Temperature > maxTemperature {
		return invalidRequest("invalid_temperature", "temperature must be between 0 and 2")
	}
	return nil
}

func normalizeTopP(req *ChatRequest) *chatValidationError {
	if req.TopP == nil {
		val := defaultChatTopP
		req.TopP = &val
		return nil
	}
	if *req.TopP < minTopP || *req.TopP > maxTopP {
		return invalidRequest("invalid_top_p", "top_p must be between 0 and 1")
	}
	return nil
}

package grok

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/upstream/msgutil"
)

func (g *GrokUpstream) buildBody(req *upstream.ChatRequest, fileAttachments []string) ([]byte, error) {
	if req == nil {
		return nil, errors.New("chat request is required")
	}
	if strings.TrimSpace(req.UpstreamMode) == "" {
		return nil, fmt.Errorf("upstream mode is required")
	}

	messages, err := buildMessages(req)
	if err != nil {
		return nil, err
	}
	if fileAttachments == nil {
		fileAttachments = []string{}
	}

	responseMeta := map[string]any{}
	modelConfigOverride := map[string]any{}
	if req.Temperature != nil {
		modelConfigOverride["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		modelConfigOverride["topP"] = *req.TopP
	}
	if req.ReasoningEffort != "" {
		modelConfigOverride["reasoningEffort"] = req.ReasoningEffort
	}
	if len(modelConfigOverride) > 0 {
		responseMeta["modelConfigOverride"] = modelConfigOverride
	}

	payload := map[string]any{
		"temporary":                 req.Temporary,
		"message":                   flattenMessages(messages),
		"fileAttachments":           fileAttachments,
		"imageAttachments":          []any{},
		"disableSearch":             false,
		"disableMemory":             req.DisableMemory,
		"enableImageGeneration":     true,
		"returnImageBytes":          false,
		"returnRawGrokInXaiRequest": false,
		"enableImageStreaming":      true,
		"imageGenerationCount":      2,
		"forceConcise":              false,
		"toolOverrides":             map[string]any{},
		"enableSideBySide":          true,
		"sendFinalMetadata":         true,
		"collectionIds":             []any{},
		"connectors":                []any{},
		"searchAllConnectors":       false,
		"responseMetadata":          responseMeta,
		"deviceEnvInfo": map[string]any{
			"darkModeEnabled":  false,
			"devicePixelRatio": 2,
			"screenWidth":      2056,
			"screenHeight":     1329,
			"viewportWidth":    2056,
			"viewportHeight":   1083,
		},
		"disableSelfHarmShortCircuit": false,
		"disableTextFollowUps":        false,
		"forceSideBySide":             false,
		"isAsyncChat":                 false,
		"modeId":                      req.UpstreamMode,
	}

	if strings.TrimSpace(req.CustomInstruction) != "" {
		payload["customPersonality"] = req.CustomInstruction
	}
	if req.DeepSearch != "" {
		payload["deepsearchPreset"] = req.DeepSearch
	}

	return json.Marshal(payload)
}

func buildMessages(req *upstream.ChatRequest) ([]upstream.Message, error) {
	formattedMessages := msgutil.FormatToolHistory(req.Messages)
	toolPrompt := msgutil.BuildToolPrompt(req.Tools, req.ToolChoice, req.ParallelToolCalls)

	messages := make([]upstream.Message, 0, len(formattedMessages))
	for i, m := range formattedMessages {
		textContent, err := messageContentText(m)
		if err != nil {
			return nil, err
		}
		if i == 0 && m.Role == "system" && toolPrompt != "" {
			textContent = toolPrompt + "\n\n" + textContent
		}
		messages = append(messages, upstream.Message{
			Role:       m.Role,
			Content:    textContent,
			ToolCalls:  m.ToolCalls,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		})
	}

	if toolPrompt != "" && (len(messages) == 0 || messages[0].Role != "system") {
		messages = append([]upstream.Message{{Role: "system", Content: toolPrompt}}, messages...)
	}
	if allMessagesEmpty(messages) {
		return nil, errors.New("all messages have empty content")
	}
	return messages, nil
}

func messageContentText(m upstream.Message) (string, error) {
	switch c := m.Content.(type) {
	case string:
		return c, nil
	case map[string]any:
		if content, ok := msgutil.FormatStructuredMessage(m.Role, c); ok {
			return content, nil
		}
		raw, err := json.Marshal(c)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return msgutil.ContentToString(c), nil
	}
}

// flattenMessages converts chat messages into Grok's single message string.
func flattenMessages(messages []upstream.Message) string {
	if len(messages) == 0 {
		return ""
	}
	if len(messages) == 1 {
		return msgutil.ContentToString(messages[0].Content)
	}

	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	var b strings.Builder
	for i, msg := range messages {
		if i > 0 {
			b.WriteString("\n\n")
		}
		content := msgutil.ContentToString(msg.Content)
		if i == lastUserIdx {
			b.WriteString(content)
		} else {
			b.WriteString(msg.Role)
			b.WriteString(": ")
			b.WriteString(content)
		}
	}
	return b.String()
}

func allMessagesEmpty(messages []upstream.Message) bool {
	for _, m := range messages {
		if strings.TrimSpace(msgutil.ContentToString(m.Content)) != "" {
			return false
		}
	}
	return true
}

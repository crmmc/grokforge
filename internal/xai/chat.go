package xai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

const (
	grokAPIURL = "https://grok.com/rest/app-chat/conversations/new"
)

// Chat sends a chat request and returns a channel of stream events.
func (c *client) Chat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStreamClosed
	}
	c.mu.Unlock()

	// Build request body
	body, err := buildChatBody(req)
	if err != nil {
		return nil, err
	}

	slog.Debug("xai: chat request built",
		"model", req.Model,
		"grok_model", mapModel(req.Model),
		"model_mode", modelModeForModel(req.Model),
		"msg_count", len(req.Messages),
		"body_len", len(body),
		"temporary", req.Temporary,
		"disable_memory", req.DisableMemory)

	events := make(chan StreamEvent, 16)

	go c.streamChat(ctx, body, events)

	return events, nil
}

// buildChatBody constructs the Grok API request body.
// Grok's web API accepts a single "message" string, so all conversation
// messages are flattened into one text block. The last user message is
// sent without a role prefix; all others are prefixed with "role: ".
func buildChatBody(req *ChatRequest) ([]byte, error) {
	message := flattenMessages(req.Messages)
	grokModel := mapModel(req.Model)
	fileAttachments := req.FileAttachments
	if fileAttachments == nil {
		fileAttachments = []string{}
	}

	// Build responseMetadata
	responseMeta := map[string]any{
		"requestModelDetails": map[string]any{
			"modelId": grokModel,
		},
	}
	modelConfigOverride := cloneMap(req.ModelConfig)
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
		"modelName":                 grokModel,
		"message":                   message,
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
		"toolOverrides":             cloneMap(req.ToolOverrides),
		"enableSideBySide":          true,
		"sendFinalMetadata":         true,
		"isReasoning":               false,
		"modelMode":                 modelModeForModel(req.Model),
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
	}

	// Only include customPersonality when non-empty after trimming
	if strings.TrimSpace(req.CustomInstruction) != "" {
		payload["customPersonality"] = req.CustomInstruction
	}

	return json.Marshal(payload)
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		nested, ok := value.(map[string]any)
		if ok {
			cloned[key] = cloneMap(nested)
			continue
		}
		cloned[key] = value
	}
	return cloned
}

// flattenMessages converts an array of chat messages into a single string
// suitable for the Grok API's "message" field.
// The last user message is sent without a role prefix; all preceding
// messages are prefixed with their role (e.g. "system: ...", "assistant: ...").
func flattenMessages(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	if len(messages) == 1 {
		return messages[0].Content
	}

	// Find the index of the last user message
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
		if i == lastUserIdx {
			b.WriteString(msg.Content)
		} else {
			b.WriteString(msg.Role)
			b.WriteString(": ")
			b.WriteString(msg.Content)
		}
	}
	return b.String()
}

// modelMapping holds the grok_model and modelMode for a given model_id.
type modelMapping struct {
	GrokModel string
	ModelMode string
}

// modelMappings maps OpenAI-style model_id to Grok API grok_model + modelMode.
// Sourced from Python reference: grok2api ModelService.MODELS.
var modelMappings = map[string]modelMapping{
	"grok-3":                 {"grok-3", "MODEL_MODE_GROK_3"},
	"grok-3-mini":            {"grok-3", "MODEL_MODE_GROK_3_MINI_THINKING"},
	"grok-3-thinking":        {"grok-3", "MODEL_MODE_GROK_3_THINKING"},
	"grok-4":                 {"grok-4", "MODEL_MODE_GROK_4"},
	"grok-4-mini":            {"grok-4-mini", "MODEL_MODE_GROK_4_MINI_THINKING"},
	"grok-4-thinking":        {"grok-4", "MODEL_MODE_GROK_4_THINKING"},
	"grok-4-heavy":           {"grok-4", "MODEL_MODE_HEAVY"},
	"grok-4.1-mini":          {"grok-4-1-thinking-1129", "MODEL_MODE_GROK_4_1_MINI_THINKING"},
	"grok-4.1-fast":          {"grok-4-1-thinking-1129", "MODEL_MODE_FAST"},
	"grok-4.1-expert":        {"grok-4-1-thinking-1129", "MODEL_MODE_EXPERT"},
	"grok-4.1-thinking":      {"grok-4-1-thinking-1129", "MODEL_MODE_GROK_4_1_THINKING"},
	"grok-4.20-beta":         {"grok-420", "MODEL_MODE_GROK_420"},
	"grok-imagine-1.0":       {"grok-3", "MODEL_MODE_FAST"},
	"grok-imagine-1.0-fast":  {"grok-3", "MODEL_MODE_FAST"},
	"grok-imagine-1.0-edit":  {"imagine-image-edit", "MODEL_MODE_FAST"},
	"grok-imagine-1.0-video": {"grok-3", "MODEL_MODE_FAST"},
}

// mapModel converts OpenAI model names to Grok API model names.
// Unknown grok-prefixed names pass through; empty or non-grok names default to "grok-3".
func mapModel(model string) string {
	// Look up in mapping table
	if m, ok := modelMappings[model]; ok {
		return m.GrokModel
	}

	// Passthrough any grok-prefixed model name
	if strings.HasPrefix(model, "grok-") {
		return model
	}

	// Default fallback
	return "grok-3"
}

// modelModeForModel returns the Grok API modelMode for the given model_id.
// Returns empty string for unknown models.
func modelModeForModel(model string) string {
	if m, ok := modelMappings[model]; ok {
		return m.ModelMode
	}
	return ""
}

// streamChat handles the streaming response in a goroutine.
func (c *client) streamChat(ctx context.Context, body []byte, events chan<- StreamEvent) {
	defer close(events)

	var lastErr error
	for attempt := 0; attempt <= c.opts.MaxRetry; attempt++ {
		if attempt > 0 {
			slog.Debug("xai: retrying chat request",
				"attempt", attempt, "max_retry", c.opts.MaxRetry,
				"last_error", lastErr)

			timer := time.NewTimer(c.opts.RetryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				events <- StreamEvent{Error: ctx.Err()}
				return
			case <-timer.C:
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}

		err := c.doStreamRequest(ctx, body, events)
		if err == nil {
			return
		}

		lastErr = err
		slog.Debug("xai: stream request failed", "attempt", attempt, "error", err)

		// Don't retry on context cancellation
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			events <- StreamEvent{Error: err}
			return
		}

		// Don't retry on 403 (need session reset or token swap at flow layer)
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrCFChallenge) {
			slog.Debug("xai: 403, not retrying (handled at flow layer)")
			events <- StreamEvent{Error: err}
			return
		}
	}

	events <- StreamEvent{Error: fmt.Errorf("max retries exceeded: %w", lastErr)}
}

// doStreamRequest performs a single streaming request.
func (c *client) doStreamRequest(ctx context.Context, body []byte, events chan<- StreamEvent) error {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", grokAPIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	reqStart := time.Now()
	resp, err := c.doRequest(httpReq)
	if err != nil {
		urlErr := &url.Error{}
		if errors.As(err, &urlErr) {
			slog.Debug("xai: network error on request",
				"error", err, "elapsed_ms", time.Since(reqStart).Milliseconds())
			return fmt.Errorf("%w: %v", ErrNetwork, err)
		}
		return err
	}
	defer resp.Body.Close()

	// Log response details — this is THE critical anti-bot indicator
	cfRay := resp.Header.Get("Cf-Ray")
	contentType := resp.Header.Get("Content-Type")
	slog.Debug("xai: response received",
		"status", resp.StatusCode,
		"content_type", contentType,
		"cf_ray", cfRay,
		"elapsed_ms", time.Since(reqStart).Milliseconds())

	// Handle error status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Continue to parse stream
	case http.StatusTooManyRequests:
		slog.Debug("xai: rate limited (429)")
		return ErrRateLimited
	case http.StatusForbidden:
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		body := string(bodyBytes)
		if isCFChallenge(contentType, body) {
			slog.Debug("xai: cloudflare challenge (403)")
			return ErrCFChallenge
		}
		slog.Debug("xai: forbidden (403) — token-level rejection", "body_prefix", truncateBody(body, 200))
		return ErrForbidden
	case http.StatusUnauthorized:
		slog.Debug("xai: unauthorized (401) — token invalid/expired")
		return ErrInvalidToken
	default:
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
		slog.Debug("xai: unexpected status",
			"status", resp.StatusCode, "body", string(bodyBytes))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return c.parseSSEStream(resp.Body, events)
}

// parseSSEStream reads SSE events from the response body.
func (c *client) parseSSEStream(body io.Reader, events chan<- StreamEvent) error {
	scanner := bufio.NewScanner(body)
	// Increase buffer size for large responses
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE data line
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			if data == "[DONE]" {
				return nil
			}

			events <- StreamEvent{Data: json.RawMessage(data)}
		} else if strings.HasPrefix(line, "{") {
			// Raw NDJSON line (bare JSON without SSE prefix)
			events <- StreamEvent{Data: json.RawMessage(line)}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}

	return nil
}

// isCFChallenge detects Cloudflare challenge pages in 403 responses.
func isCFChallenge(contentType, body string) bool {
	if strings.Contains(contentType, "text/html") {
		return true
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "cf-") ||
		strings.Contains(lower, "cloudflare") ||
		strings.Contains(lower, "challenge-platform") ||
		strings.Contains(lower, "just a moment")
}

// truncateBody returns the first n bytes of s for logging.
func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

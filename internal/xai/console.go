package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const consoleResponsesAPIURL = "https://console.x.ai/v1/responses"

// ConsoleResponses sends a request to the xAI Console Responses API.
func (c *client) ConsoleResponses(ctx context.Context, req *ConsoleRequest) (<-chan StreamEvent, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStreamClosed
	}
	c.mu.Unlock()

	body, err := buildConsoleBody(req)
	if err != nil {
		return nil, err
	}

	slog.Debug("xai: console request built",
		"model", req.Model,
		"input_count", len(req.Input),
		"body_len", len(body),
		"web_search", req.WebSearch)

	events := make(chan StreamEvent, 16)
	safeGo("xai_stream_console", func() {
		c.streamConsole(ctx, body, events)
	})
	return events, nil
}

func buildConsoleBody(req *ConsoleRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("console request is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("console model is required")
	}

	payload := map[string]any{
		"model":  req.Model,
		"input":  req.Input,
		"stream": true,
	}
	if strings.TrimSpace(req.Instructions) != "" {
		payload["instructions"] = req.Instructions
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		payload["max_output_tokens"] = *req.MaxTokens
	}
	if effort := consoleReasoningEffort(req.ReasoningEffort, req.SupportsReasoningEffort); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	if req.WebSearch {
		payload["tools"] = []map[string]string{{"type": "web_search"}}
	}
	return json.Marshal(payload)
}

func consoleReasoningEffort(effort string, supported bool) string {
	if !supported {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "", "none":
		return ""
	case "xhigh":
		return "high"
	default:
		return strings.ToLower(strings.TrimSpace(effort))
	}
}

func (c *client) streamConsole(ctx context.Context, body []byte, events chan<- StreamEvent) {
	defer close(events)

	var lastErr error
	for attempt := 0; attempt <= c.opts.MaxRetry; attempt++ {
		if attempt > 0 {
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

		err := c.doConsoleStreamRequest(ctx, body, events)
		if err == nil {
			return
		}

		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			events <- StreamEvent{Error: err}
			return
		}
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrCFChallenge) {
			events <- StreamEvent{Error: err}
			return
		}
	}

	events <- StreamEvent{Error: fmt.Errorf("max retries exceeded: %w", lastErr)}
}

func (c *client) doConsoleStreamRequest(ctx context.Context, body []byte, events chan<- StreamEvent) error {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", consoleResponsesAPIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	reqStart := time.Now()
	resp, err := c.doConsoleRequest(httpReq)
	if err != nil {
		urlErr := &url.Error{}
		if errors.As(err, &urlErr) {
			slog.Debug("xai: console network error on request",
				"error", err, "elapsed_ms", time.Since(reqStart).Milliseconds())
			return fmt.Errorf("%w: %v", ErrNetwork, err)
		}
		return err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	slog.Debug("xai: console response received",
		"status", resp.StatusCode,
		"content_type", contentType,
		"cf_ray", resp.Header.Get("Cf-Ray"),
		"elapsed_ms", time.Since(reqStart).Milliseconds())

	switch resp.StatusCode {
	case http.StatusOK:
		return c.parseConsoleSSEStream(resp.Body, events)
	case http.StatusUnauthorized:
		return ErrInvalidToken
	case http.StatusPaymentRequired:
		return ErrConsoleCreditExhausted
	case http.StatusForbidden:
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		body := string(bodyBytes)
		if isCFChallenge(contentType, body) {
			return ErrCFChallenge
		}
		return ErrForbidden
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}
}

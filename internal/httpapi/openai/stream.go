package openai

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/google/uuid"
)

const (
	chatChunkObject          = "chat.completion.chunk"
	chatObject               = "chat.completion"
	defaultChoiceIndex       = 0
	defaultToolCallIndexBase = 0
	heartbeatInterval        = 15 * time.Second
)

// streamResponse handles SSE streaming response for chat completions.
func (h *Handler) streamResponse(w http.ResponseWriter, r *http.Request, eventCh <-chan flow.StreamEvent, req *ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpapi.WriteError(w, http.StatusInternalServerError, "server_error", "streaming_unsupported", "Streaming not supported")
		return
	}

	writer := httpapi.NewSSEWriter(w)
	w.WriteHeader(http.StatusOK)

	cfg := h.currentConfig()
	state := newStreamResponseState(streamResponseOptions{
		h:       h,
		r:       r,
		writer:  writer,
		flusher: flusher,
		req:     req,
		cfg:     cfg,
	})
	if err := writer.WriteSSE(state.adapter.RoleChunk()); err != nil {
		state.writeError(err)
		return
	}
	flusher.Flush()

	// Send initial padding to force proxy/CDN buffer flush
	w.Write([]byte(": heartbeat stream connected\n" + strings.Repeat(" ", 2048) + "\n\n"))
	flusher.Flush()

	timer := time.NewTimer(heartbeatInterval)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				goto done
			}
			timer.Reset(heartbeatInterval)

			if event.Error != nil {
				state.writeError(event.Error)
				return
			}

			if err := state.handleEvent(event); err != nil {
				state.writeError(err)
				return
			}
		case <-timer.C:
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
			timer.Reset(heartbeatInterval)
		case <-r.Context().Done():
			return
		}
	}
done:
	if err := state.finish(); err != nil {
		state.writeError(err)
		return
	}
}

func writeStreamChunks(writer *httpapi.SSEWriter, flusher http.Flusher, chunks []chatStreamChunk) {
	for _, chunk := range chunks {
		writer.WriteSSE(chunk)
		flusher.Flush()
	}
}

// blockingResponse collects all events and returns a single response.
func (h *Handler) blockingResponse(w http.ResponseWriter, r *http.Request, eventCh <-chan flow.StreamEvent, req *ChatRequest) {
	collector := newChatResponseCollector(req, h.currentConfig())
	var rewriter *mediaRewriter

	for event := range eventCh {
		if event.Error != nil {
			status, apiErr := httpapi.MapXAIError(event.Error)
			httpapi.WriteJSON(w, status, apiErr)
			return
		}
		if err := rewriteEventContent(r.Context(), &event, &rewriter); err != nil {
			apiErr := mediaRewriteAPIError(err)
			httpapi.WriteJSON(w, apiErr.Status, apiErr)
			return
		}
		collector.AddEvent(event)
	}

	resp := collector.Build()
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) writeStreamingOrJSONError(w http.ResponseWriter, stream *bool, err error) {
	if !isStreamEnabled(stream) {
		status, apiErr := httpapi.MapXAIError(err)
		httpapi.WriteJSON(w, status, apiErr)
		return
	}

	_, apiErr := httpapi.MapXAIError(err)
	writeStreamingErrorResponse(w, apiErr)
}

func writeStreamingErrorResponse(w http.ResponseWriter, apiErr *httpapi.APIError) {
	writer := httpapi.NewSSEWriter(w)
	w.WriteHeader(http.StatusOK)
	writer.WriteSSEError(apiErr)
}

func writeMediaProxyError(w http.ResponseWriter, stream *bool, err error) {
	apiErr := mediaRewriteAPIError(err)
	if isStreamEnabled(stream) {
		writeStreamingErrorResponse(w, apiErr)
		return
	}
	httpapi.WriteJSON(w, apiErr.Status, apiErr)
}

func rewriteEventContent(
	ctx context.Context,
	event *flow.StreamEvent,
	rewriter **mediaRewriter,
) error {
	if event.Downloader != nil {
		*rewriter = newMediaRewriter(event.Downloader)
	}
	var err error
	event.Content, err = rewriteContent(*rewriter, ctx, event.Content)
	return err
}

func mediaRewriteAPIError(err error) *httpapi.APIError {
	slog.Warn("chat media rewrite failed", "error", err)
	return httpapi.NewAPIError(
		http.StatusBadGateway,
		"server_error",
		"media_proxy_failed",
		"Failed to proxy upstream media",
	)
}

func generateChatID() string {
	return "chatcmpl-" + uuid.NewString()
}

func filterToolCalls(calls []flow.ToolCall, tools []flow.Tool) []flow.ToolCall {
	if len(tools) == 0 {
		return calls
	}
	allowed := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool.Function.Name != "" {
			allowed[tool.Function.Name] = struct{}{}
		}
	}
	out := make([]flow.ToolCall, 0, len(calls))
	for _, call := range calls {
		if _, ok := allowed[call.Function.Name]; ok {
			out = append(out, call)
		}
	}
	return out
}

func formatToolCallsAsText(calls []flow.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, call := range calls {
		if call.Function.Name == "" {
			continue
		}
		if call.Function.Arguments == "" {
			call.Function.Arguments = "{}"
		}
		b.WriteString(toolCallStartTag)
		b.WriteString("{\"name\":\"")
		b.WriteString(call.Function.Name)
		b.WriteString("\",\"arguments\":")
		b.WriteString(call.Function.Arguments)
		b.WriteString("}")
		b.WriteString(toolCallEndTag)
	}
	return b.String()
}

package openai

import (
	"net/http"
	"strings"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/google/uuid"
)

const (
	chatChunkObject          = "chat.completion.chunk"
	chatObject               = "chat.completion"
	defaultChoiceIndex       = 0
	defaultToolCallIndexBase = 0
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
	adapter := newChatStreamAdapter(req, cfg)
	writer.WriteSSE(adapter.RoleChunk())
	flusher.Flush()

	var rewriter *mediaRewriter

	for event := range eventCh {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		if event.Error != nil {
			_, apiErr := httpapi.MapXAIError(event.Error)
			writer.WriteSSEError(apiErr)
			return
		}

		// Lazily create rewriter on first event with a downloader
		if rewriter == nil && event.Downloader != nil && cfg != nil && cfg.App.MediaGenerationEnabled {
			rewriter = newMediaRewriter(event.Downloader)
		}
		event.Content = rewriteContent(rewriter, r.Context(), event.Content)

		chunks := adapter.HandleEvent(event)
		for _, chunk := range chunks {
			writer.WriteSSE(chunk)
			flusher.Flush()
		}
	}

	for _, chunk := range adapter.FinishChunks() {
		writer.WriteSSE(chunk)
		flusher.Flush()
	}
	writer.WriteSSEDone()
}

// blockingResponse collects all events and returns a single response.
func (h *Handler) blockingResponse(w http.ResponseWriter, r *http.Request, eventCh <-chan flow.StreamEvent, req *ChatRequest) {
	cfg := h.currentConfig()
	collector := newChatResponseCollector(req, cfg)
	var dl flow.DownloadFunc

	for event := range eventCh {
		if event.Error != nil {
			status, apiErr := httpapi.MapXAIError(event.Error)
			httpapi.WriteJSON(w, status, apiErr)
			return
		}
		if dl == nil && event.Downloader != nil {
			dl = event.Downloader
		}
		collector.AddEvent(event)
	}

	resp := collector.Build()

	// Rewrite image URLs in the final assembled content
	if dl != nil && len(resp.Choices) > 0 && cfg != nil && cfg.App.MediaGenerationEnabled {
		rewriter := newMediaRewriter(dl)
		resp.Choices[0].Message.Content = rewriteContent(rewriter, r.Context(), resp.Choices[0].Message.Content)
	}

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

package openai

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
)

// SetupRoutes registers OpenAI-compatible API endpoints on the given router.
func (h *Handler) SetupRoutes(r chi.Router) {
	if h.ModelRegistry != nil {
		r.Get("/models", HandleModelsFromRegistry(h.ModelRegistry))
	}
	r.Post("/chat/completions", h.handleChat)
}

func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeChatRequest(w, r)
	if !ok {
		return
	}
	normalized, valErr := normalizeChatRequest(req, h.currentConfig())
	if valErr != nil {
		httpapi.WriteError(w, valErr.status, valErr.errType, valErr.code, valErr.message)
		return
	}
	if apiErr := h.validateModel(r, normalized.Model); apiErr != nil {
		httpapi.WriteJSON(w, apiErr.Status, apiErr)
		return
	}
	if h.handleMediaRoutes(w, r, normalized) {
		return
	}
	h.handleChatCompletion(w, r, normalized)
}

func (h *Handler) decodeChatRequest(w http.ResponseWriter, r *http.Request) (*ChatRequest, bool) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json",
			"Invalid JSON in request body")
		return nil, false
	}
	return &req, true
}

func (h *Handler) validateModel(r *http.Request, model string) *httpapi.APIError {
	if h.ModelRegistry == nil {
		return httpapi.NewAPIError(http.StatusInternalServerError, "server_error", "model_registry_unavailable",
			"Model registry is not configured")
	}
	if _, ok := h.ModelRegistry.Resolve(model); !ok {
		return httpapi.NewAPIError(http.StatusNotFound, "not_found", "model_not_found",
			"The model `"+model+"` does not exist")
	}
	if !httpapi.CheckModelWhitelist(r.Context(), model) {
		return httpapi.NewAPIError(http.StatusForbidden, "forbidden", "model_not_allowed",
			"Model not allowed for this API key: "+model)
	}
	return nil
}

func (h *Handler) handleMediaRoutes(w http.ResponseWriter, r *http.Request, req *ChatRequest) bool {
	if cfg := h.currentConfig(); cfg != nil && !cfg.App.MediaGenerationEnabled && h.isMediaModel(req.Model) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "media_generation_disabled",
			"Image and video generation is disabled by the administrator")
		return true
	}
	if h.isImageEditModel(req.Model) {
		h.handleChatImageEdit(w, r, req)
		return true
	}
	if h.isImageModel(req.Model) {
		h.handleChatImage(w, r, req)
		return true
	}
	if h.isImageWSModel(req.Model) {
		h.handleChatImageWSGeneration(w, r, req)
		return true
	}
	if h.isVideoModel(req.Model) {
		h.handleChatVideo(w, r, req)
		return true
	}
	return false
}

func (h *Handler) handleChatCompletion(w http.ResponseWriter, r *http.Request, req *ChatRequest) {
	if h.ChatFlow == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented",
			"Chat completions not yet configured")
		return
	}
	flowReq := h.toFlowRequest(req)
	ctx := httpapi.BridgeFlowContext(r.Context())
	eventCh, err := h.ChatFlow.Complete(ctx, flowReq)
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}
	if isStreamEnabled(req.Stream) {
		h.streamResponse(w, r, eventCh, req)
		return
	}
	h.blockingResponse(w, r, eventCh, req)
}

// toFlowRequest converts ChatRequest to flow.ChatRequest.
// Resolves UpstreamModel/UpstreamMode from the model registry.
func (h *Handler) toFlowRequest(req *ChatRequest) *flow.ChatRequest {
	messages := make([]flow.Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = flow.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  m.ToolCalls,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
	}

	flowReq := &flow.ChatRequest{
		Model:           req.Model,
		Messages:        messages,
		Stream:          true,
		ReasoningEffort: req.ReasoningEffort,
		Tools:           req.Tools,
		ToolChoice:      req.ToolChoice,
	}
	if req.ParallelToolCalls != nil {
		flowReq.ParallelToolCalls = *req.ParallelToolCalls
	} else {
		flowReq.ParallelToolCalls = true
	}

	if req.Temperature != nil {
		flowReq.Temperature = req.Temperature
	}
	if req.TopP != nil {
		flowReq.TopP = req.TopP
	}
	if req.MaxTokens != nil {
		flowReq.MaxTokens = req.MaxTokens
	}

	// Resolve upstream model/mode from registry
	if h.ModelRegistry != nil {
		if rm, ok := h.ModelRegistry.Resolve(req.Model); ok {
			flowReq.UpstreamModel = rm.UpstreamModel
			flowReq.UpstreamMode = rm.UpstreamMode
			flowReq.ForceThinking = rm.ForceThinking
			flowReq.QuotaMode = rm.QuotaMode
		}
	}

	// Pass through deepsearch preset (only valid values)
	if req.DeepSearch == "default" || req.DeepSearch == "deeper" {
		flowReq.DeepSearch = req.DeepSearch
	}

	return flowReq
}

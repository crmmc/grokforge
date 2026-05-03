package openai

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/registry"
)

// buildFileURL constructs a full URL for a cached file based on the request.
func buildFileURL(r *http.Request, mediaType, filename string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/files/%s/%s", scheme, r.Host, mediaType, filename)
}

const maxImageEditInputs = 3
const maxVideoReferenceInputs = 7

// resolveModelType resolves the model type from the registry.
// Returns empty string if registry is nil or model not found.
func (h *Handler) resolveModelType(model string) string {
	rm, ok := h.resolveModel(model)
	if !ok {
		return ""
	}
	return rm.Type
}

func (h *Handler) resolveModel(model string) (*registry.ResolvedModel, bool) {
	if h.ModelRegistry == nil {
		return nil, false
	}
	rm, ok := h.ModelRegistry.Resolve(model)
	if !ok {
		return nil, false
	}
	return rm, true
}

func (h *Handler) isImageWSModel(model string) bool {
	return h.resolveModelType(model) == "image_ws"
}

func (h *Handler) isImageModel(model string) bool {
	return h.resolveModelType(model) == "image_lite"
}

func (h *Handler) isImageEditModel(model string) bool {
	return h.resolveModelType(model) == "image_edit"
}

func (h *Handler) isVideoModel(model string) bool {
	return h.resolveModelType(model) == "video"
}

func (h *Handler) isMediaModel(model string) bool {
	t := h.resolveModelType(model)
	return t == "image_ws" || t == "image_lite" || t == "image_edit" || t == "video"
}

func (h *Handler) resolveUpstream(model string) (string, string) {
	rm, ok := h.resolveModel(model)
	if !ok {
		return "", ""
	}
	return rm.UpstreamModel, rm.UpstreamMode
}

func (h *Handler) handleChatImage(w http.ResponseWriter, r *http.Request, req *ChatRequest) {
	if h.ImageFlow == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented", "image flow not configured")
		return
	}

	prompt, _, err := extractChatPromptAndImages(r.Context(), req.Messages)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_messages", err.Error())
		return
	}
	if strings.TrimSpace(prompt) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt is required")
		return
	}

	imageCfg, err := h.resolveChatLiteImageConfig(req)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_image_config", err.Error())
		return
	}
	rm, _ := h.resolveModel(req.Model)
	_, upstreamMode := h.resolveUpstream(req.Model)

	result, err := h.ImageFlow.GenerateLite(httpapi.BridgeFlowContext(r.Context()), &flow.ImageLiteRequest{
		Model:          req.Model,
		Prompt:         prompt,
		N:              imageCfg.n,
		Mode:           modeValue(rm),
		UpstreamMode:   upstreamMode,
		ResponseFormat: imageCfg.responseFormat,
	})
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	content, err := h.renderImagesForChat(r, result)
	if err != nil {
		writeMediaProxyError(w, req.Stream, err)
		return
	}
	eventCh := singleMessageEventCh(content)
	if isStreamEnabled(req.Stream) {
		h.streamResponse(w, r, eventCh, req)
		return
	}
	h.blockingResponse(w, r, eventCh, req)
}

func (h *Handler) handleChatImageWSGeneration(w http.ResponseWriter, r *http.Request, req *ChatRequest) {
	if h.ImageFlow == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented", "image flow not configured")
		return
	}

	prompt, _, err := extractChatPromptAndImages(r.Context(), req.Messages)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_messages", err.Error())
		return
	}
	if strings.TrimSpace(prompt) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt is required")
		return
	}

	imageCfg, err := h.resolveChatImageConfig(req)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_image_config", err.Error())
		return
	}
	rm, _ := h.resolveModel(req.Model)
	result, err := h.ImageFlow.Generate(httpapi.BridgeFlowContext(r.Context()), &flow.ImageRequest{
		Model:           req.Model,
		Prompt:          prompt,
		N:               imageCfg.n,
		Size:            imageCfg.size,
		ResponseFormat:  imageCfg.responseFormat,
		EnableNSFW:      imageCfg.enableNSFW,
		CooldownSeconds: cooldownValue(rm),
	})
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	content, err := h.renderImagesForChat(r, result)
	if err != nil {
		writeMediaProxyError(w, req.Stream, err)
		return
	}
	eventCh := singleMessageEventCh(content)
	if isStreamEnabled(req.Stream) {
		h.streamResponse(w, r, eventCh, req)
		return
	}
	h.blockingResponse(w, r, eventCh, req)
}

func (h *Handler) handleChatImageEdit(w http.ResponseWriter, r *http.Request, req *ChatRequest) {
	if h.ImageFlow == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented", "image flow not configured")
		return
	}

	prompt, images, err := extractChatPromptAndImages(r.Context(), req.Messages)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_messages", err.Error())
		return
	}
	if len(images) == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_image", "image_url is required for image edits")
		return
	}
	if strings.TrimSpace(prompt) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt is required")
		return
	}

	imageCfg, err := h.resolveChatImageConfig(req)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_image_config", err.Error())
		return
	}
	editReq := h.buildChatImageEditFlowRequest(req, chatImageEditFlowInput{
		prompt: prompt,
		images: images,
		cfg:    imageCfg,
	})
	result, err := h.ImageFlow.Edit(httpapi.BridgeFlowContext(r.Context()), editReq)
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	content, err := h.renderImagesForChat(r, result)
	if err != nil {
		writeMediaProxyError(w, req.Stream, err)
		return
	}
	eventCh := singleMessageEventCh(content)
	if isStreamEnabled(req.Stream) {
		h.streamResponse(w, r, eventCh, req)
		return
	}
	h.blockingResponse(w, r, eventCh, req)
}

func (h *Handler) handleChatVideo(w http.ResponseWriter, r *http.Request, req *ChatRequest) {
	if h.VideoFlow == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented", "video flow not configured")
		return
	}

	prompt, images, err := extractChatPromptAndImages(r.Context(), req.Messages)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_messages", err.Error())
		return
	}
	if strings.TrimSpace(prompt) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt is required")
		return
	}

	videoCfg, err := h.resolveChatVideoConfig(req.VideoConfig)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_video_config", err.Error())
		return
	}
	videoReq := h.buildChatVideoFlowRequest(req, chatVideoFlowInput{
		prompt: prompt,
		images: images,
		cfg:    videoCfg,
	})
	videoURL, err := h.VideoFlow.GenerateSync(httpapi.BridgeFlowContext(r.Context()), videoReq)
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	videoURL, err = h.renderVideoForChat(r, videoURL)
	if err != nil {
		writeMediaProxyError(w, req.Stream, err)
		return
	}
	content := fmt.Sprintf("[video](%s)", videoURL)
	eventCh := singleMessageEventCh(content)
	if isStreamEnabled(req.Stream) {
		h.streamResponse(w, r, eventCh, req)
		return
	}
	h.blockingResponse(w, r, eventCh, req)
}

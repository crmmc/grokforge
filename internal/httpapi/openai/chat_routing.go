package openai

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
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

	n := 1
	responseFormat := "b64_json"
	if req.ImageConfig != nil {
		if req.ImageConfig.N > 0 {
			n = req.ImageConfig.N
		}
		if req.ImageConfig.ResponseFormat != "" {
			responseFormat = req.ImageConfig.ResponseFormat
		}
	}
	rm, _ := h.resolveModel(req.Model)
	_, upstreamMode := h.resolveUpstream(req.Model)

	result, err := h.ImageFlow.GenerateLite(httpapi.BridgeFlowContext(r.Context()), &flow.ImageLiteRequest{
		Model:          req.Model,
		Prompt:         prompt,
		N:              n,
		Mode:           modeValue(rm),
		UpstreamMode:   upstreamMode,
		ResponseFormat: responseFormat,
	})
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	content := h.renderImagesForChat(result)
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

	content := h.renderImagesForChat(result)
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
	if len(images) > maxImageEditInputs {
		images = images[len(images)-maxImageEditInputs:]
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
	upstreamModel, upstreamMode := h.resolveUpstream(req.Model)

	result, err := h.ImageFlow.Edit(httpapi.BridgeFlowContext(r.Context()), &flow.ImageEditRequest{
		Model:          req.Model,
		UpstreamModel:  upstreamModel,
		UpstreamMode:   upstreamMode,
		Mode:           modeValue(rm),
		Prompt:         prompt,
		OriginalImages: images,
		N:              imageCfg.n,
		Size:           imageCfg.size,
		ResponseFormat: imageCfg.responseFormat,
		EnableNSFW:     imageCfg.enableNSFW,
	})
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	content := h.renderImagesForChat(result)
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
	rm, _ := h.resolveModel(req.Model)
	upstreamModel, upstreamMode := h.resolveUpstream(req.Model)

	videoReq := &flow.VideoRequest{
		Prompt:        prompt,
		Model:         req.Model,
		UpstreamModel: upstreamModel,
		UpstreamMode:  upstreamMode,
		Mode:          modeValue(rm),
		Size:          videoCfg.size,
		AspectRatio:   videoCfg.aspectRatio,
		Seconds:       videoCfg.seconds,
		Quality:       videoCfg.quality,
		Preset:        videoCfg.preset,
	}
	if len(images) > 0 {
		videoReq.ReferenceImage = images[0]
	}

	videoURL, err := h.VideoFlow.GenerateSync(httpapi.BridgeFlowContext(r.Context()), videoReq)
	if err != nil {
		h.writeStreamingOrJSONError(w, req.Stream, err)
		return
	}

	// Normalize relative cache path to full URL (same as image handling).
	if strings.HasPrefix(videoURL, "/api/files/") {
		filename := strings.TrimPrefix(videoURL, "/api/files/video/")
		videoURL = buildFileURL(r, "video", filename)
	}
	content := fmt.Sprintf("[video](%s)", videoURL)
	eventCh := singleMessageEventCh(content)
	if isStreamEnabled(req.Stream) {
		h.streamResponse(w, r, eventCh, req)
		return
	}
	h.blockingResponse(w, r, eventCh, req)
}

func modeValue(rm *registry.ResolvedModel) string {
	if rm == nil {
		return ""
	}
	return rm.Mode
}

func cooldownValue(rm *registry.ResolvedModel) int {
	if rm == nil {
		return 0
	}
	return rm.CooldownSeconds
}

func decodeImageDataURI(dataURI string) ([]byte, error) {
	parts := strings.SplitN(dataURI, ",", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid image data uri")
	}
	b64 := strings.TrimSpace(parts[1])
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode image data: %w", err)
	}
	return decoded, nil
}

func (h *Handler) renderImagesForChat(result *flow.ImageResponse) string {
	parts := make([]string, 0, len(result.Data))
	for _, img := range result.Data {
		if img.B64JSON != "" {
			parts = append(parts, fmt.Sprintf("![image](data:image/png;base64,%s)", img.B64JSON))
			continue
		}
		if img.URL != "" {
			parts = append(parts, fmt.Sprintf("![image](%s)", img.URL))
		}
	}
	return strings.Join(parts, "\n")
}

func singleMessageEventCh(content string) <-chan flow.StreamEvent {
	ch := make(chan flow.StreamEvent, 2)
	stop := "stop"
	ch <- flow.StreamEvent{Content: content}
	ch <- flow.StreamEvent{
		FinishReason: &stop,
		Usage: &flow.Usage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}
	close(ch)
	return ch
}

func normalizeAspectRatio(v string) string {
	switch v {
	case "1280x720", "16:9":
		return "16:9"
	case "720x1280", "9:16":
		return "9:16"
	case "1792x1024", "3:2":
		return "3:2"
	case "1024x1792", "2:3":
		return "2:3"
	case "1024x1024", "1:1":
		return "1:1"
	default:
		return ""
	}
}

func parseAspectRatioPair(v string) (int, int, error) {
	parts := strings.Split(v, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid aspect ratio")
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil || w <= 0 {
		return 0, 0, fmt.Errorf("invalid aspect ratio")
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil || h <= 0 {
		return 0, 0, fmt.Errorf("invalid aspect ratio")
	}
	return w, h, nil
}

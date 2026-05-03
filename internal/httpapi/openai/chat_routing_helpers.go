package openai

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/registry"
)

type chatImageEditFlowInput struct {
	prompt string
	images [][]byte
	cfg    *resolvedChatImageConfig
}

type chatVideoFlowInput struct {
	prompt string
	images [][]byte
	cfg    *resolvedChatVideoConfig
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

func (h *Handler) renderImagesForChat(result *flow.ImageResponse) (string, error) {
	if result == nil {
		return "", fmt.Errorf("image response is nil")
	}
	parts := make([]string, 0, len(result.Data))
	for _, img := range result.Data {
		if img.B64JSON != "" {
			parts = append(parts, fmt.Sprintf("![image](data:image/png;base64,%s)", img.B64JSON))
			continue
		}
		if img.URL != "" {
			if _, ok := mediaDownloadURL(img.URL); ok {
				return "", fmt.Errorf("image response contains upstream media URL")
			}
			parts = append(parts, fmt.Sprintf("![image](%s)", img.URL))
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (h *Handler) renderVideoForChat(r *http.Request, videoURL string) (string, error) {
	if !strings.HasPrefix(videoURL, "/api/files/video/") {
		return "", fmt.Errorf("video response is not cached locally")
	}
	filename := strings.TrimPrefix(videoURL, "/api/files/video/")
	if filename == "" || strings.ContainsAny(filename, `/\?#`) {
		return "", fmt.Errorf("video cache filename is invalid")
	}
	return buildFileURL(r, "video", filename), nil
}

func (h *Handler) buildChatImageEditFlowRequest(req *ChatRequest, input chatImageEditFlowInput) *flow.ImageEditRequest {
	rm, _ := h.resolveModel(req.Model)
	upstreamModel, upstreamMode := h.resolveUpstream(req.Model)
	return &flow.ImageEditRequest{
		Model:          req.Model,
		UpstreamModel:  upstreamModel,
		UpstreamMode:   upstreamMode,
		Mode:           modeValue(rm),
		Prompt:         input.prompt,
		OriginalImages: lastImageEditInputs(input.images),
		N:              input.cfg.n,
		Size:           input.cfg.size,
		ResponseFormat: input.cfg.responseFormat,
		EnableNSFW:     input.cfg.enableNSFW,
	}
}

func lastImageEditInputs(images [][]byte) [][]byte {
	if len(images) <= maxImageEditInputs {
		return images
	}
	return images[len(images)-maxImageEditInputs:]
}

func (h *Handler) buildChatVideoFlowRequest(req *ChatRequest, input chatVideoFlowInput) *flow.VideoRequest {
	rm, _ := h.resolveModel(req.Model)
	upstreamModel, upstreamMode := h.resolveUpstream(req.Model)
	videoReq := &flow.VideoRequest{
		Prompt:        input.prompt,
		Model:         req.Model,
		UpstreamModel: upstreamModel,
		UpstreamMode:  upstreamMode,
		Mode:          modeValue(rm),
		Size:          input.cfg.size,
		AspectRatio:   input.cfg.aspectRatio,
		Seconds:       input.cfg.seconds,
		Quality:       input.cfg.quality,
		Preset:        input.cfg.preset,
	}
	if refs := firstVideoReferenceInputs(input.images); len(refs) > 0 {
		videoReq.ReferenceImages = refs
	}
	return videoReq
}

func firstVideoReferenceInputs(images [][]byte) [][]byte {
	if len(images) <= maxVideoReferenceInputs {
		return images
	}
	return images[:maxVideoReferenceInputs]
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

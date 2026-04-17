package openai

import (
	"fmt"
	"strings"
)

const (
	defaultChatImageCount   = 1
	defaultChatImageSize    = "1024x1024"
	defaultChatVideoSeconds = 6
	defaultChatVideoPreset  = "custom"
)

var allowedChatImageSizes = map[string]struct{}{
	"1280x720":  {},
	"720x1280":  {},
	"1792x1024": {},
	"1024x1792": {},
	"1024x1024": {},
}

var allowedVideoPresets = map[string]struct{}{
	"fun":    {},
	"normal": {},
	"spicy":  {},
	"custom": {},
}

type resolvedChatImageConfig struct {
	n              int
	size           string
	responseFormat string
	enableNSFW     *bool
}

type resolvedChatVideoConfig struct {
	size        string
	aspectRatio string
	seconds     int
	quality     string
	preset      string
}

func (h *Handler) resolveChatImageConfig(req *ChatRequest) (*resolvedChatImageConfig, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request is nil")
	}

	cfg := &resolvedChatImageConfig{
		n:              defaultChatImageCount,
		size:           defaultChatImageSize,
		responseFormat: h.defaultChatImageFormat(),
	}

	if req.ImageConfig != nil {
		if req.ImageConfig.N > 0 {
			cfg.n = req.ImageConfig.N
		}
		if req.ImageConfig.Size != "" {
			cfg.size = req.ImageConfig.Size
		}
		if req.ImageConfig.ResponseFormat != "" {
			cfg.responseFormat = req.ImageConfig.ResponseFormat
		}
		cfg.enableNSFW = req.ImageConfig.EnableNSFW
	}

	responseFormat, err := normalizeChatImageResponseFormat(cfg.responseFormat)
	if err != nil {
		return nil, err
	}
	cfg.responseFormat = responseFormat

	if cfg.n < 1 || cfg.n > 10 {
		return nil, fmt.Errorf("image_config.n must be between 1 and 10")
	}
	if isStreamEnabled(req.Stream) && cfg.n != 1 && cfg.n != 2 {
		return nil, fmt.Errorf("streaming is only supported when image_config.n is 1 or 2")
	}
	if _, ok := allowedChatImageSizes[cfg.size]; !ok {
		return nil, fmt.Errorf("image_config.size must be one of 1024x1024, 1024x1792, 1280x720, 1792x1024, 720x1280")
	}

	return cfg, nil
}

func (h *Handler) resolveChatVideoConfig(cfg *VideoConfig) (*resolvedChatVideoConfig, error) {
	aspectRatio := "3:2"
	seconds := defaultChatVideoSeconds
	resolution := "480p"
	preset := defaultChatVideoPreset

	if cfg != nil {
		if cfg.AspectRatio != "" {
			aspectRatio = normalizeAspectRatio(cfg.AspectRatio)
			if aspectRatio == "" {
				return nil, fmt.Errorf("aspect_ratio must be one of 1280x720, 720x1280, 1792x1024, 1024x1792, 1024x1024, 16:9, 9:16, 3:2, 2:3, 1:1")
			}
		}
		if cfg.VideoLength > 0 {
			seconds = cfg.VideoLength
		}
		if cfg.ResolutionName != "" {
			resolution = strings.ToLower(strings.TrimSpace(cfg.ResolutionName))
		}
		if cfg.Preset != "" {
			preset = strings.ToLower(strings.TrimSpace(cfg.Preset))
		}
	}

	if seconds < 6 || seconds > 30 {
		return nil, fmt.Errorf("video_length must be between 6 and 30 seconds")
	}
	if resolution != "480p" && resolution != "720p" {
		return nil, fmt.Errorf("resolution_name must be one of 480p, 720p")
	}
	if _, ok := allowedVideoPresets[preset]; !ok {
		return nil, fmt.Errorf("preset must be one of custom, fun, normal, spicy")
	}

	height := 480
	quality := "standard"
	if resolution == "720p" {
		height = 720
		quality = "high"
	}
	arW, arH, err := parseAspectRatioPair(aspectRatio)
	if err != nil {
		return nil, err
	}

	return &resolvedChatVideoConfig{
		size:        fmt.Sprintf("%dx%d", height*arW/arH, height),
		aspectRatio: aspectRatio,
		seconds:     seconds,
		quality:     quality,
		preset:      preset,
	}, nil
}

func normalizeChatImageResponseFormat(value string) (string, error) {
	return "b64_json", nil // 统一 base64，忽略输入
}

func (h *Handler) defaultChatImageFormat() string {
	return "b64_json"
}

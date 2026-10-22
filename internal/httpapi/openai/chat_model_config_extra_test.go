package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveChatImageConfig(t *testing.T) {
	tests := []struct {
		name        string
		req         *ChatRequest
		wantErrCode string
		check       func(t *testing.T, cfg *resolvedChatImageConfig)
	}{
		{
			name:        "nil request",
			req:         nil,
			wantErrCode: "chat request is nil",
		},
		{
			name: "defaults",
			req:  &ChatRequest{Model: "m"},
			check: func(t *testing.T, cfg *resolvedChatImageConfig) {
				assert.Equal(t, defaultChatImageCount, cfg.n)
				assert.Equal(t, defaultChatImageSize, cfg.size)
				assert.Equal(t, "b64_json", cfg.responseFormat)
				assert.Nil(t, cfg.enableNSFW)
			},
		},
		{
			name: "overrides applied",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{
					N:              2,
					Size:           "1024x1792",
					ResponseFormat: "url",
					EnableNSFW:     boolPtr(true),
				},
			},
			check: func(t *testing.T, cfg *resolvedChatImageConfig) {
				assert.Equal(t, 2, cfg.n)
				assert.Equal(t, "1024x1792", cfg.size)
				assert.Equal(t, "b64_json", cfg.responseFormat)
				require.NotNil(t, cfg.enableNSFW)
				assert.True(t, *cfg.enableNSFW)
			},
		},
		{
			name: "invalid response format",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{ResponseFormat: "bogus"},
			},
			wantErrCode: "image_config.response_format must be one of url, b64_json, base64",
		},
		{
			name: "n out of range high",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{N: 11},
			},
			wantErrCode: "image_config.n must be between 1 and 10",
		},
		{
			name: "n zero keeps default",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{N: 0},
			},
			check: func(t *testing.T, cfg *resolvedChatImageConfig) {
				assert.Equal(t, defaultChatImageCount, cfg.n)
			},
		},
		{
			name: "streaming rejects n of 3",
			req: &ChatRequest{
				Stream:      boolPtr(true),
				ImageConfig: &ImageConfig{N: 3},
			},
			wantErrCode: "streaming is only supported when image_config.n is 1 or 2",
		},
		{
			name: "streaming allows n of 2",
			req: &ChatRequest{
				Stream:      boolPtr(true),
				ImageConfig: &ImageConfig{N: 2},
			},
			check: func(t *testing.T, cfg *resolvedChatImageConfig) {
				assert.Equal(t, 2, cfg.n)
			},
		},
		{
			name: "invalid size",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{Size: "55x55"},
			},
			wantErrCode: "image_config.size must be one of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{}
			cfg, err := h.resolveChatImageConfig(tc.req)
			if tc.wantErrCode != "" {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.wantErrCode)
				return
			}
			require.Nil(t, err)
			require.NotNil(t, cfg)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestResolveChatLiteImageConfig(t *testing.T) {
	tests := []struct {
		name        string
		req         *ChatRequest
		wantErrCode string
		check       func(t *testing.T, cfg *resolvedChatImageConfig)
	}{
		{
			name:        "nil request",
			req:         nil,
			wantErrCode: "chat request is nil",
		},
		{
			name: "defaults",
			req:  &ChatRequest{Model: "m"},
			check: func(t *testing.T, cfg *resolvedChatImageConfig) {
				assert.Equal(t, defaultChatImageCount, cfg.n)
				assert.Equal(t, "b64_json", cfg.responseFormat)
			},
		},
		{
			name: "n and format override",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{N: 4, ResponseFormat: "base64"},
			},
			check: func(t *testing.T, cfg *resolvedChatImageConfig) {
				assert.Equal(t, 4, cfg.n)
				assert.Equal(t, "b64_json", cfg.responseFormat)
			},
		},
		{
			name: "n above 4 rejected",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{N: 5},
			},
			wantErrCode: "image_config.n must be between 1 and 4",
		},
		{
			name: "invalid response format",
			req: &ChatRequest{
				ImageConfig: &ImageConfig{ResponseFormat: "nope"},
			},
			wantErrCode: "image_config.response_format must be one of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{}
			cfg, err := h.resolveChatLiteImageConfig(tc.req)
			if tc.wantErrCode != "" {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.wantErrCode)
				return
			}
			require.Nil(t, err)
			require.NotNil(t, cfg)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestResolveChatVideoConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *VideoConfig
		wantErrCode string
		check       func(t *testing.T, cfg *resolvedChatVideoConfig)
	}{
		{
			name: "nil config defaults",
			check: func(t *testing.T, cfg *resolvedChatVideoConfig) {
				assert.Equal(t, "720x480", cfg.size)
				assert.Equal(t, "3:2", cfg.aspectRatio)
				assert.Equal(t, defaultChatVideoSeconds, cfg.seconds)
				assert.Equal(t, "standard", cfg.quality)
				assert.Equal(t, defaultChatVideoPreset, cfg.preset)
			},
		},
		{
			name: "720p landscape",
			cfg:  &VideoConfig{ResolutionName: "720P", VideoLength: 12},
			check: func(t *testing.T, cfg *resolvedChatVideoConfig) {
				assert.Equal(t, "1080x720", cfg.size) // 720 * 3 / 2 with default 3:2 ratio
				assert.Equal(t, "high", cfg.quality)
				assert.Equal(t, 12, cfg.seconds)
			},
		},
		{
			name: "portrait 9:16",
			cfg:  &VideoConfig{AspectRatio: "720x1280"},
			check: func(t *testing.T, cfg *resolvedChatVideoConfig) {
				assert.Equal(t, "9:16", cfg.aspectRatio)
				assert.Equal(t, "270x480", cfg.size) // 480 * 9 / 16
			},
		},
		{
			name: "preset normalized",
			cfg:  &VideoConfig{Preset: "  FUN "},
			check: func(t *testing.T, cfg *resolvedChatVideoConfig) {
				assert.Equal(t, "fun", cfg.preset)
			},
		},
		{
			name:        "invalid aspect ratio",
			cfg:         &VideoConfig{AspectRatio: "4:3"},
			wantErrCode: "aspect_ratio must be one of",
		},
		{
			name:        "video length too small",
			cfg:         &VideoConfig{VideoLength: 5},
			wantErrCode: "video_length must be between 6 and 30 seconds",
		},
		{
			name:        "video length too large",
			cfg:         &VideoConfig{VideoLength: 31},
			wantErrCode: "video_length must be between 6 and 30 seconds",
		},
		{
			name:        "invalid resolution",
			cfg:         &VideoConfig{ResolutionName: "1080p"},
			wantErrCode: "resolution_name must be one of 480p, 720p",
		},
		{
			name:        "invalid preset",
			cfg:         &VideoConfig{Preset: "epic"},
			wantErrCode: "preset must be one of custom, fun, normal, spicy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{}
			cfg, err := h.resolveChatVideoConfig(tc.cfg)
			if tc.wantErrCode != "" {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.wantErrCode)
				return
			}
			require.Nil(t, err)
			require.NotNil(t, cfg)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestNormalizeAspectRatio(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "1280x720", value: "1280x720", want: "16:9"},
		{name: "16:9", value: "16:9", want: "16:9"},
		{name: "720x1280", value: "720x1280", want: "9:16"},
		{name: "9:16", value: "9:16", want: "9:16"},
		{name: "1792x1024", value: "1792x1024", want: "3:2"},
		{name: "3:2", value: "3:2", want: "3:2"},
		{name: "1024x1792", value: "1024x1792", want: "2:3"},
		{name: "2:3", value: "2:3", want: "2:3"},
		{name: "1024x1024", value: "1024x1024", want: "1:1"},
		{name: "1:1", value: "1:1", want: "1:1"},
		{name: "unknown", value: "4:3", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeAspectRatio(tc.value))
		})
	}
}

func TestParseAspectRatioPair(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantW   int
		wantH   int
		wantErr bool
	}{
		{name: "valid", value: "16:9", wantW: 16, wantH: 9},
		{name: "missing separator", value: "16x9", wantErr: true},
		{name: "non numeric width", value: "a:9", wantErr: true},
		{name: "non numeric height", value: "16:b", wantErr: true},
		{name: "zero width", value: "0:9", wantErr: true},
		{name: "negative height", value: "16:-2", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, h, err := parseAspectRatioPair(tc.value)
			if tc.wantErr {
				require.NotNil(t, err)
				assert.Equal(t, "invalid aspect ratio", err.Error())
				return
			}
			assert.Nil(t, err)
			assert.Equal(t, tc.wantW, w)
			assert.Equal(t, tc.wantH, h)
		})
	}
}

func TestNormalizeChatImageResponseFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		want    string
		wantErr bool
	}{
		{name: "empty", format: "", want: "b64_json"},
		{name: "url", format: "URL", want: "b64_json"},
		{name: "b64_json", format: "b64_json", want: "b64_json"},
		{name: "base64 padded", format: "  Base64  ", want: "b64_json"},
		{name: "invalid", format: "png", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeChatImageResponseFormat(tc.format)
			if tc.wantErr {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDefaultChatImageFormat(t *testing.T) {
	h := &Handler{}
	assert.Equal(t, "b64_json", h.defaultChatImageFormat())
}

package openai

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/registry"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRoutingHelpers_NilRegistry(t *testing.T) {
	h := &Handler{}

	assert.Equal(t, "", h.resolveModelType("grok-3"))
	rm, ok := h.resolveModel("grok-3")
	assert.Nil(t, rm)
	assert.False(t, ok)
	assert.False(t, h.isMediaModel("grok-3"))

	upstreamModel, upstreamMode := h.resolveUpstream("grok-3")
	assert.Equal(t, "", upstreamModel)
	assert.Equal(t, "", upstreamMode)
}

func TestIsMediaModel(t *testing.T) {
	h := &Handler{ModelRegistry: testMediaRegistry()}
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "image ws model", model: "grok-imagine-image", want: true},
		{name: "image lite model", model: "grok-imagine-image-lite", want: true},
		{name: "image edit model", model: "grok-imagine-image-edit", want: true},
		{name: "video model", model: "grok-imagine-video", want: true},
		{name: "chat model", model: "grok-3", want: false},
		{name: "unknown model", model: "nope", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, h.isMediaModel(tc.model))
			assert.Equal(t, h.resolveModelType(tc.model) == "image_ws", h.isImageWSModel(tc.model))
			assert.Equal(t, h.resolveModelType(tc.model) == "image_lite", h.isImageModel(tc.model))
			assert.Equal(t, h.resolveModelType(tc.model) == "image_edit", h.isImageEditModel(tc.model))
			assert.Equal(t, h.resolveModelType(tc.model) == "video", h.isVideoModel(tc.model))
		})
	}
}

func TestResolveModelTypeAndUpstream(t *testing.T) {
	h := &Handler{ModelRegistry: testMediaRegistry()}
	assert.Equal(t, "image_lite", h.resolveModelType("grok-imagine-image-lite"))
	assert.Equal(t, "", h.resolveModelType("missing"))
	assert.Equal(t, "", h.resolveModelType("grok-3"))

	upstreamModel, upstreamMode := h.resolveUpstream("grok-imagine-image-edit")
	assert.Equal(t, "imagine-image-edit", upstreamModel)
	assert.Equal(t, "MODEL_MODE_FAST", upstreamMode)
}

func TestHandleChatImage_NotConfigured(t *testing.T) {
	h := &Handler{}
	req := &ChatRequest{Model: "grok-imagine-image-lite", Messages: baseValidMessages()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatImage(w, r, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "not_implemented", resp.Error.Code)
}

func TestHandleChatMediaHandlers_MissingPrompt(t *testing.T) {
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator { return nil })
	tests := []struct {
		name   string
		h      *Handler
		req    *ChatRequest
		target func(h *Handler, w http.ResponseWriter, r *http.Request, req *ChatRequest)
	}{
		{
			name:   "image lite",
			h:      &Handler{ImageFlow: imageFlow},
			req:    &ChatRequest{Model: "grok-imagine-image-lite", Messages: []ChatMessage{{Role: "user", Content: "   "}}},
			target: (*Handler).handleChatImage,
		},
		{
			name:   "image ws",
			h:      &Handler{ImageFlow: imageFlow},
			req:    &ChatRequest{Model: "grok-imagine-image", Messages: []ChatMessage{{Role: "user", Content: "   "}}},
			target: (*Handler).handleChatImageWSGeneration,
		},
		{
			// image edit validates images before the prompt, so keep an image present
			// and make the text blank to reach the missing_prompt branch.
			name: "image edit",
			h:    &Handler{ImageFlow: imageFlow},
			req: &ChatRequest{Model: "grok-imagine-image-edit", Messages: []ChatMessage{{
				Role: "user",
				Content: []any{
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNGBytes())}},
					map[string]any{"type": "text", "text": "   "},
				},
			}}},
			target: (*Handler).handleChatImageEdit,
		},
		{
			name:   "video",
			h:      &Handler{VideoFlow: newStubVideoFlow()},
			req:    &ChatRequest{Model: "grok-imagine-video", Messages: []ChatMessage{{Role: "user", Content: "   "}}},
			target: (*Handler).handleChatVideo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			tc.target(tc.h, w, r, tc.req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			var resp httpapi.APIError
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "missing_prompt", resp.Error.Code)
		})
	}
}

func TestHandleChatMediaHandlers_NotConfigured(t *testing.T) {
	tests := []struct {
		name   string
		h      *Handler
		req    *ChatRequest
		target func(h *Handler, w http.ResponseWriter, r *http.Request, req *ChatRequest)
	}{
		{
			name:   "image ws",
			h:      &Handler{},
			req:    &ChatRequest{Model: "grok-imagine-image", Messages: baseValidMessages()},
			target: (*Handler).handleChatImageWSGeneration,
		},
		{
			name:   "image edit",
			h:      &Handler{},
			req:    &ChatRequest{Model: "grok-imagine-image-edit", Messages: baseValidMessages()},
			target: (*Handler).handleChatImageEdit,
		},
		{
			name:   "video",
			h:      &Handler{},
			req:    &ChatRequest{Model: "grok-imagine-video", Messages: baseValidMessages()},
			target: (*Handler).handleChatVideo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			tc.target(tc.h, w, r, tc.req)
			assert.Equal(t, http.StatusNotImplemented, w.Code)
		})
	}
}

func TestHandleChatMediaHandlers_InvalidMessages(t *testing.T) {
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator { return nil })
	badMessages := []ChatMessage{{
		Role: "user",
		Content: []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,!!!not-base64!!!"}},
		},
	}}
	tests := []struct {
		name   string
		h      *Handler
		target func(h *Handler, w http.ResponseWriter, r *http.Request, req *ChatRequest)
	}{
		{name: "image lite", h: &Handler{ImageFlow: imageFlow}, target: (*Handler).handleChatImage},
		{name: "image ws", h: &Handler{ImageFlow: imageFlow}, target: (*Handler).handleChatImageWSGeneration},
		{name: "image edit", h: &Handler{ImageFlow: imageFlow}, target: (*Handler).handleChatImageEdit},
		{name: "video", h: &Handler{VideoFlow: newStubVideoFlow()}, target: (*Handler).handleChatVideo},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &ChatRequest{Model: "m", Messages: badMessages}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			tc.target(tc.h, w, r, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			var resp httpapi.APIError
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "invalid_messages", resp.Error.Code)
		})
	}
}

func TestHandleChatMediaHandlers_InvalidImageConfig(t *testing.T) {
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator { return nil })
	tests := []struct {
		name   string
		h      *Handler
		req    *ChatRequest
		target func(h *Handler, w http.ResponseWriter, r *http.Request, req *ChatRequest)
	}{
		{
			name:   "image lite",
			h:      &Handler{ImageFlow: imageFlow},
			req:    &ChatRequest{Model: "m", Messages: baseValidMessages(), ImageConfig: &ImageConfig{N: 5}},
			target: (*Handler).handleChatImage,
		},
		{
			name:   "image ws",
			h:      &Handler{ImageFlow: imageFlow},
			req:    &ChatRequest{Model: "m", Messages: baseValidMessages(), ImageConfig: &ImageConfig{N: 99}},
			target: (*Handler).handleChatImageWSGeneration,
		},
		{
			// image edit requires at least one image before config resolution.
			name: "image edit",
			h:    &Handler{ImageFlow: imageFlow},
			req: &ChatRequest{
				Model: "m",
				Messages: []ChatMessage{{
					Role: "user",
					Content: []any{
						map[string]any{"type": "text", "text": "edit"},
						map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNGBytes())}},
					},
				}},
				ImageConfig: &ImageConfig{N: 99},
			},
			target: (*Handler).handleChatImageEdit,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			tc.target(tc.h, w, r, tc.req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			var resp httpapi.APIError
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "invalid_image_config", resp.Error.Code)
		})
	}
}

// newStubVideoFlow builds a VideoFlow whose client is never invoked because the
// tested paths bail out before generation.
func newStubVideoFlow() *flow.VideoFlow {
	return flow.NewVideoFlow(
		&chatMockTokenSvc{},
		func(token string) flow.VideoClient { return &chatVideoClientMock{} },
		&flow.VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: flowTestModelResolver()},
	)
}

func TestHandleChatVideo_InvalidVideoConfig(t *testing.T) {
	h := &Handler{VideoFlow: newStubVideoFlow()}
	req := &ChatRequest{
		Model:       "grok-imagine-video",
		Messages:    baseValidMessages(),
		VideoConfig: &VideoConfig{AspectRatio: "4:3"},
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatVideo(w, r, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "invalid_video_config", resp.Error.Code)
}

func TestHandleChatImageEdit_MissingImage(t *testing.T) {
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator { return nil })
	h := &Handler{ImageFlow: imageFlow}
	req := &ChatRequest{Model: "grok-imagine-image-edit", Messages: baseValidMessages()}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	h.handleChatImageEdit(w, r, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "missing_image", resp.Error.Code)
}

func TestHandleChatImage_StreamingSuccess(t *testing.T) {
	client := &chatImageLiteClientMock{
		imageURL:     "https://assets.grok.com/users/u/generated/id/image.png",
		downloadBody: testPNGBytes(),
	}
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator { return nil })
	imageFlow.SetEditClientFactory(func(token string) flow.ImageEditClient { return client })
	imageFlow.SetModelResolver(flowTestModelResolver())

	h := &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()}
	req := &ChatRequest{
		Model:    "grok-imagine-image-lite",
		Messages: baseValidMessages(),
		Stream:   boolPtr(true),
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatImage(w, r, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "data: [DONE]")
	assert.Contains(t, body, "![image]")
	assert.NotContains(t, body, "assets.grok.com")
}

func TestHandleChatImageWSGeneration_StreamingSuccess(t *testing.T) {
	mock := &mockImagineClient{events: []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "abc123"}}}
	imageFlow := newTestImageFlow(mock)
	h := &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()}
	req := &ChatRequest{
		Model:       "grok-imagine-image",
		Messages:    baseValidMessages(),
		Stream:      boolPtr(true),
		ImageConfig: &ImageConfig{N: 1},
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatImageWSGeneration(w, r, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "data: [DONE]")
	assert.Contains(t, body, "![image](data:image/png;base64,abc123)")
}

func TestHandleChatImageEdit_StreamingSuccess(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString(testPNGBytes())
	client := &chatImageLiteClientMock{
		imageURL:     "https://assets.grok.com/users/u/generated/id/image.png",
		downloadBody: testPNGBytes(),
	}
	imageFlow := flow.NewImageFlow(&chatMockTokenSvc{}, func(token string) flow.ImagineGenerator { return nil })
	imageFlow.SetEditClientFactory(func(token string) flow.ImageEditClient { return client })
	imageFlow.SetModelResolver(flowTestModelResolver())

	h := &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()}
	req := &ChatRequest{
		Model: "grok-imagine-image-edit",
		Messages: []ChatMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "make it blue"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + pngB64}},
			},
		}},
		Stream:      boolPtr(true),
		ImageConfig: &ImageConfig{N: 1},
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatImageEdit(w, r, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "data: [DONE]")
	assert.Contains(t, body, "![image]")
}

func TestRenderImagesForChat(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Host = "api.example.test"

	tests := []struct {
		name    string
		result  *flow.ImageResponse
		want    string
		wantErr bool
	}{
		{
			name:    "nil result",
			result:  nil,
			wantErr: true,
		},
		{
			name:   "b64 image",
			result: &flow.ImageResponse{Data: []flow.ImageData{{B64JSON: "abc123"}}},
			want:   "![image](data:image/png;base64,abc123)",
		},
		{
			name:   "plain external url kept",
			result: &flow.ImageResponse{Data: []flow.ImageData{{URL: "https://cdn.example.com/a.png"}}},
			want:   "![image](https://cdn.example.com/a.png)",
		},
		{
			name:   "local api files url expanded",
			result: &flow.ImageResponse{Data: []flow.ImageData{{URL: "/api/files/image/img-1.png"}}},
			want:   "![image](http://api.example.test/api/files/image/img-1.png)",
		},
		{
			name:    "grok url rejected",
			result:  &flow.ImageResponse{Data: []flow.ImageData{{URL: "https://assets.grok.com/users/u/generated/a.png"}}},
			wantErr: true,
		},
		{
			name: "multiple images joined",
			result: &flow.ImageResponse{Data: []flow.ImageData{
				{B64JSON: "aaa"},
				{B64JSON: "bbb"},
			}},
			want: "![image](data:image/png;base64,aaa)\n![image](data:image/png;base64,bbb)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := h.renderImagesForChat(r, tc.result)
			if tc.wantErr {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLastImageEditInputs(t *testing.T) {
	two := [][]byte{{1}, {2}}
	assert.Equal(t, two, lastImageEditInputs(two))

	five := [][]byte{{1}, {2}, {3}, {4}, {5}}
	got := lastImageEditInputs(five)
	require.Len(t, got, maxImageEditInputs)
	assert.Equal(t, []byte{3}, got[0])
	assert.Equal(t, []byte{5}, got[2])
}

func TestFirstVideoReferenceInputs(t *testing.T) {
	two := [][]byte{{1}, {2}}
	assert.Equal(t, two, firstVideoReferenceInputs(two))

	many := make([][]byte, maxVideoReferenceInputs+3)
	for i := range many {
		many[i] = []byte{byte(i)}
	}
	got := firstVideoReferenceInputs(many)
	require.Len(t, got, maxVideoReferenceInputs)
	assert.Equal(t, []byte{0}, got[0])
}

func TestModeValueAndCooldownValue(t *testing.T) {
	assert.Equal(t, "", modeValue(nil))
	assert.Equal(t, 0, cooldownValue(nil))

	rm := &registry.ResolvedModel{Mode: "auto", CooldownSeconds: 42}
	assert.Equal(t, "auto", modeValue(rm))
	assert.Equal(t, 42, cooldownValue(rm))
}

func TestDecodeImageDataURI(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)

	got, err := decodeImageDataURI(uri)
	require.Nil(t, err)
	assert.Equal(t, data, got)

	_, err = decodeImageDataURI("no-comma-here")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid image data uri")

	_, err = decodeImageDataURI("data:image/png;base64,!!!")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "decode image data")
}

func TestBuildChatVideoFlowRequest(t *testing.T) {
	h := &Handler{ModelRegistry: testMediaRegistry()}

	videoCfg := &resolvedChatVideoConfig{
		size:        "720x480",
		aspectRatio: "3:2",
		seconds:     8,
		quality:     "standard",
		preset:      "custom",
	}

	t.Run("without reference images", func(t *testing.T) {
		videoReq := h.buildChatVideoFlowRequest(&ChatRequest{Model: "grok-imagine-video"}, chatVideoFlowInput{
			prompt: "clip",
			images: nil,
			cfg:    videoCfg,
		})
		assert.Equal(t, "grok-imagine-video", videoReq.Model)
		assert.Equal(t, "grok-3", videoReq.UpstreamModel)
		assert.Equal(t, "MODEL_MODE_FAST", videoReq.UpstreamMode)
		assert.Equal(t, "auto", videoReq.Mode)
		assert.Equal(t, videoCfg.size, videoReq.Size)
		assert.Equal(t, 8, videoReq.Seconds)
		assert.Empty(t, videoReq.ReferenceImages)
	})

	t.Run("reference images capped", func(t *testing.T) {
		images := make([][]byte, maxVideoReferenceInputs+1)
		videoReq := h.buildChatVideoFlowRequest(&ChatRequest{Model: "grok-imagine-video"}, chatVideoFlowInput{
			prompt: "clip",
			images: images,
			cfg:    videoCfg,
		})
		require.Len(t, videoReq.ReferenceImages, maxVideoReferenceInputs)
	})
}

func TestVideoFlowWithoutCacheReturnsError(t *testing.T) {
	videoFlow := flow.NewVideoFlow(
		&chatMockTokenSvc{},
		func(token string) flow.VideoClient { return &chatVideoClientMock{} },
		&flow.VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: flowTestModelResolver()},
	)
	h := &Handler{VideoFlow: videoFlow, ModelRegistry: testMediaRegistry()}
	req := &ChatRequest{Model: "grok-imagine-video", Messages: baseValidMessages()}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	h.handleChatVideo(w, r, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHandlerCurrentConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Image.Format = config.ImageFormatLocalURL

	var nilHandler *Handler
	assert.Nil(t, nilHandler.currentConfig())
	assert.Equal(t, config.ImageFormatBase64, nilHandler.imageOutputFormat())

	t.Run("cfg fallback", func(t *testing.T) {
		h := &Handler{Cfg: cfg}
		assert.Equal(t, cfg, h.currentConfig())
		assert.Equal(t, config.ImageFormatLocalURL, h.imageOutputFormat())
	})

	t.Run("runtime wins", func(t *testing.T) {
		runtimeCfg := config.DefaultConfig()
		h := &Handler{Cfg: cfg, Runtime: config.NewRuntime(runtimeCfg)}
		assert.Equal(t, runtimeCfg, h.currentConfig())
		assert.Equal(t, config.ImageFormatBase64, h.imageOutputFormat())
	})

	t.Run("nil config defaults to base64", func(t *testing.T) {
		h := &Handler{}
		assert.Equal(t, config.ImageFormatBase64, h.imageOutputFormat())
	})
}

func TestHandleChatImage_NoTokenAvailable(t *testing.T) {
	imageFlow := flow.NewImageFlow(&chatUnavailableTokenSvc{err: tkn.ErrNoTokenAvailable}, func(token string) flow.ImagineGenerator {
		return &mockImagineClient{}
	})
	imageFlow.SetEditClientFactory(func(token string) flow.ImageEditClient { return &chatImageLiteClientMock{} })
	imageFlow.SetModelResolver(flowTestModelResolver())

	h := &Handler{ImageFlow: imageFlow, ModelRegistry: testMediaRegistry()}
	req := &ChatRequest{Model: "grok-imagine-image-lite", Messages: baseValidMessages()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatImage(w, r, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "no_token_available", resp.Error.Code)
}

func TestHandleChatVideo_StreamingSuccess(t *testing.T) {
	videoFlow := flow.NewVideoFlow(
		&chatMockTokenSvc{},
		func(token string) flow.VideoClient { return &chatVideoClientMock{} },
		&flow.VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: flowTestModelResolver()},
	)
	videoFlow.SetCacheService(cache.NewService(t.TempDir(), nil))

	h := &Handler{VideoFlow: videoFlow, ModelRegistry: testMediaRegistry()}
	req := &ChatRequest{
		Model:    "grok-imagine-video",
		Messages: baseValidMessages(),
		Stream:   boolPtr(true),
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleChatVideo(w, r, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "[video](")
	assert.Contains(t, body, "/api/files/video/")
	assert.NotContains(t, body, "example.com/video.mp4")
	assert.Contains(t, body, "data: [DONE]")
}

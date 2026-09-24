package flow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubVideoClient is a configurable VideoClient fake with per-call behavior.
type stubVideoClient struct {
	mu               sync.Mutex
	chatErr          error
	chatEvents       []xai.StreamEvent
	uploadErr        error
	imagePostErr     error
	createPostErr    error
	pollErr          error
	pollURL          string
	downloadToErr    error
	ignoreWriteErr   bool
	uploadURI        string
	postID           string
	chatRequests     []*xai.ChatRequest
	chatCalls        int
	uploadCalls      int
	imagePostCalls   int
	videoPostCalls   int
	pollCalls        int
	downloadToURLs   []string
	downloadToCalled bool
}

func (s *stubVideoClient) Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	s.mu.Lock()
	s.chatCalls++
	s.chatRequests = append(s.chatRequests, req)
	chatErr := s.chatErr
	events := s.chatEvents
	s.mu.Unlock()

	if chatErr != nil {
		return nil, chatErr
	}
	ch := make(chan xai.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (s *stubVideoClient) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	s.mu.Lock()
	s.imagePostCalls++
	err := s.imagePostErr
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	return s.postID, nil
}

func (s *stubVideoClient) CreateVideoPost(ctx context.Context, prompt string) (string, error) {
	s.mu.Lock()
	s.videoPostCalls++
	err := s.createPostErr
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	return s.postID, nil
}

func (s *stubVideoClient) PollUpscale(ctx context.Context, videoID string, interval time.Duration) (string, error) {
	s.mu.Lock()
	s.pollCalls++
	pollErr := s.pollErr
	pollURL := s.pollURL
	s.mu.Unlock()
	if pollErr != nil {
		return "", pollErr
	}
	return pollURL, nil
}

func (s *stubVideoClient) DownloadURL(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}

func (s *stubVideoClient) DownloadTo(ctx context.Context, url string, w io.Writer) error {
	s.mu.Lock()
	s.downloadToCalled = true
	s.downloadToURLs = append(s.downloadToURLs, url)
	err := s.downloadToErr
	s.mu.Unlock()
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("video-bytes"))
	if s.ignoreWriteErr {
		return nil // simulate an uploader that does not surface pipe errors
	}
	return err
}

func (s *stubVideoClient) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	s.mu.Lock()
	s.uploadCalls++
	err := s.uploadErr
	s.mu.Unlock()
	if err != nil {
		return "", "", err
	}
	uri := s.uploadURI
	if uri == "" {
		uri = "generated/ref-" + fileName
	}
	return "file-id", uri, nil
}

func newVideoFlowWithStub(t *testing.T, svc *mockTokenService, client *stubVideoClient) *VideoFlow {
	t.Helper()
	vf := NewVideoFlow(svc, func(string) VideoClient { return client }, &VideoFlowConfig{
		TimeoutSeconds:      5,
		PollIntervalSeconds: 1,
		ModelResolver:       testModelResolver(),
	})
	setTestVideoCache(t, vf)
	return vf
}

func videoStreamPayload(t *testing.T, inner map[string]any) xai.StreamEvent {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"result": map[string]any{"response": inner}})
	require.NoError(t, err)
	return xai.StreamEvent{Data: payload}
}

func TestNewVideoFlow_DefaultsOnNilConfig(t *testing.T) {
	vf := NewVideoFlow(&mockTokenService{}, nil, nil)
	require.NotNil(t, vf.cfg)
	assert.Equal(t, 300, vf.cfg.TimeoutSeconds)
	assert.Equal(t, 5, vf.cfg.PollIntervalSeconds)
}

func TestVideoFlow_GenerateSync_ChatStartError(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{chatErr: errors.New("connection refused")})
	recorder := &imageUsageRecorder{}
	vf.SetUsageRecorder(recorder)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start video generation")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 500, recorder.records[0].Status)
	assert.Equal(t, "video", recorder.records[0].Endpoint)
}

func TestVideoFlow_GenerateSync_StreamErrorEvent(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{
		chatEvents: []xai.StreamEvent{{Error: errors.New("stream broke")}},
	})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "video stream")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)
}

func TestVideoFlow_GenerateSync_Moderated(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{"moderated": true})},
	})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "moderated")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)
}

func TestVideoFlow_GenerateSync_MissingFinalURL(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{})},
	})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing final url")
}

func TestVideoFlow_GenerateSync_InvalidJSONEvent(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{
		chatEvents: []xai.StreamEvent{{Data: json.RawMessage("{not-json")}},
	})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode video stream")
}

func TestVideoFlow_GenerateSync_ReferenceUploadError(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{uploadErr: errors.New("upload failed")})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:          "Test",
		Model:           "grok-imagine-video",
		ReferenceImages: [][]byte{createTestPNG()},
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload video reference 0")
}

func TestVideoFlow_GenerateSync_ReferenceImagePostError(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{imagePostErr: errors.New("post failed")})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:          "Test",
		Model:           "grok-imagine-video",
		ReferenceImages: [][]byte{createTestPNG()},
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create video reference post 0")
}

func TestVideoFlow_GenerateSync_UpscaleMissingID(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: tkn.PoolBasic}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{
			"streamingVideoGenerationResponse": map[string]any{"videoUrl": "https://example.com/video.mp4"},
		})},
	})

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:  "Test",
		Model:   "grok-imagine-video",
		Quality: "high",
	}))
	assert.Empty(t, url)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVideoPostProcess)
	assert.Contains(t, err.Error(), "missing generated video id")
	assert.Equal(t, []uint{1}, tokenSvc.successCalls, "post-process failure without token error keeps success")
}

func TestVideoFlow_GenerateSync_UpscaleEmptyURL(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: tkn.PoolBasic}}}
	vf := newVideoFlowWithStub(t, tokenSvc, &stubVideoClient{
		pollURL: "   ",
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{
			"streamingVideoGenerationResponse": map[string]any{
				"videoUrl": "https://assets.grok.com/users/u/generated/123e4567-e89b-12d3-a456-426614174000/video.mp4",
			},
		})},
	})

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:  "Test",
		Model:   "grok-imagine-video",
		Quality: "high",
	}))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVideoPostProcess)
	assert.Contains(t, err.Error(), "empty url")
}

func TestVideoFlow_GenerateSync_ResolutionRequestShape(t *testing.T) {
	// High quality on a basic pool token downgrades the generation resolution
	// to 480p and upscales afterwards.
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: tkn.PoolBasic}}}
	client := &stubVideoClient{
		pollURL: "https://assets.grok.com/users/u/generated/123e4567-e89b-12d3-a456-426614174000/upscaled.mp4",
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{
			"streamingVideoGenerationResponse": map[string]any{
				"videoUrl": "https://assets.grok.com/users/u/generated/123e4567-e89b-12d3-a456-426614174000/video.mp4",
			},
		})},
	}
	vf := newVideoFlowWithStub(t, tokenSvc, client)

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:  "Test",
		Model:   "grok-imagine-video",
		Quality: "high",
	}))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(url, "/api/files/video/"))

	require.Len(t, client.chatRequests, 1)
	mc := client.chatRequests[0].ModelConfig["modelMap"].(map[string]any)
	vc := mc["videoGenModelConfig"].(map[string]any)
	assert.Equal(t, "480p", vc["resolutionName"])
}

func TestVideoFlow_GenerateSync_AppConfigApplied(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	client := &stubVideoClient{
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{
			"streamingVideoGenerationResponse": map[string]any{"videoUrl": "https://example.com/video.mp4"},
		})},
	}
	vf := newVideoFlowWithStub(t, tokenSvc, client)

	t.Run("static config", func(t *testing.T) {
		vf.SetAppConfig(&config.AppConfig{Temporary: true, DisableMemory: true, CustomInstruction: "ci"})
		_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
			Prompt: "Test",
			Model:  "grok-imagine-video",
		}))
		require.NoError(t, err)
		require.Len(t, client.chatRequests, 1)
		assert.True(t, client.chatRequests[0].Temporary)
		assert.True(t, client.chatRequests[0].DisableMemory)
		assert.Equal(t, "ci", client.chatRequests[0].CustomInstruction)
	})

	t.Run("config provider", func(t *testing.T) {
		tokenSvc.mu.Lock()
		tokenSvc.pickIndex = 0
		tokenSvc.mu.Unlock()
		vf.SetAppConfigProvider(func() *config.AppConfig {
			return &config.AppConfig{Temporary: true}
		})
		_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
			Prompt: "Test",
			Model:  "grok-imagine-video",
		}))
		require.NoError(t, err)
		assert.True(t, client.chatRequests[len(client.chatRequests)-1].Temporary)
	})
}

func TestVideoFlow_PickTokenForModelMatrix(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		vf := &VideoFlow{tokenSvc: &mockTokenService{}}
		_, err := vf.pickTokenForModel("m", "auto")
		assert.ErrorIs(t, err, tkn.ErrModelNotFound)
	})

	t.Run("nil resolver", func(t *testing.T) {
		vf := &VideoFlow{tokenSvc: &mockTokenService{}, cfg: &VideoFlowConfig{}}
		_, err := vf.pickTokenForModel("m", "auto")
		assert.ErrorIs(t, err, tkn.ErrModelNotFound)
	})

	t.Run("unknown model", func(t *testing.T) {
		vf := &VideoFlow{
			tokenSvc: &mockTokenService{},
			cfg:      &VideoFlowConfig{ModelResolver: testModelResolver()},
		}
		_, err := vf.pickTokenForModel("unknown", "auto")
		assert.ErrorIs(t, err, tkn.ErrModelNotFound)
	})

	t.Run("all pools exhausted", func(t *testing.T) {
		vf := &VideoFlow{
			tokenSvc: &mockTokenService{},
			cfg:      &VideoFlowConfig{ModelResolver: testModelResolver()},
		}
		_, err := vf.pickTokenForModel("grok-imagine-video", "auto")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no tokens available")
	})
}

func TestVideoHelpers(t *testing.T) {
	t.Run("videoModeFlag", func(t *testing.T) {
		assert.Equal(t, "--mode=extremely-crazy", videoModeFlag("fun"))
		assert.Equal(t, "--mode=normal", videoModeFlag("Normal"))
		assert.Equal(t, "--mode=extremely-spicy-or-crazy", videoModeFlag(" spicy "))
		assert.Equal(t, "--mode=custom", videoModeFlag("other"))
	})

	t.Run("buildVideoModePrompt trims", func(t *testing.T) {
		assert.Equal(t, "prompt  --mode=normal", buildVideoModePrompt("  prompt ", "normal"))
	})

	t.Run("videoResolutionFromQuality", func(t *testing.T) {
		assert.Equal(t, "720p", videoResolutionFromQuality("HIGH"))
		assert.Equal(t, "480p", videoResolutionFromQuality(""))
		assert.Equal(t, "480p", videoResolutionFromQuality("standard"))
	})

	t.Run("shouldUpscaleVideo", func(t *testing.T) {
		assert.True(t, shouldUpscaleVideo(tkn.PoolBasic, "720p"))
		assert.False(t, shouldUpscaleVideo(tkn.PoolBasic, "480p"))
		assert.False(t, shouldUpscaleVideo(tkn.PoolSuper, "720p"))
	})

	t.Run("videoGenerationResolution", func(t *testing.T) {
		assert.Equal(t, "480p", videoGenerationResolution(tkn.PoolBasic, "high"))
		assert.Equal(t, "480p", videoGenerationResolution(tkn.PoolBasic, ""))
		assert.Equal(t, "720p", videoGenerationResolution(tkn.PoolSuper, "high"))
	})

	t.Run("extractGeneratedVideoID", func(t *testing.T) {
		assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000",
			extractGeneratedVideoID("https://x/users/u/generated/123e4567-e89b-12d3-a456-426614174000/v.mp4"))
		assert.Empty(t, extractGeneratedVideoID("https://example.com/video.mp4"))
		assert.Empty(t, extractGeneratedVideoID(""))
	})

	t.Run("resolveVideoAspectRatio", func(t *testing.T) {
		assert.Equal(t, "3:2", resolveVideoAspectRatio(" 3:2 ", ""))
		assert.Equal(t, "16:9", resolveVideoAspectRatio("", "1280x720"))
		assert.Equal(t, "2:3", resolveVideoAspectRatio("", "unknown"))
	})

	t.Run("extractVideoResponse", func(t *testing.T) {
		_, ok := extractVideoResponse(map[string]any{})
		assert.False(t, ok)
		_, ok = extractVideoResponse(map[string]any{"result": "str"})
		assert.False(t, ok)
		response, ok := extractVideoResponse(map[string]any{"result": map[string]any{"response": map[string]any{"k": "v"}}})
		assert.True(t, ok)
		assert.Equal(t, "v", response["k"])
	})
}

func TestUpdateVideoStreamStateTable(t *testing.T) {
	t.Run("invalid json errors", func(t *testing.T) {
		state := &videoStreamState{}
		err := updateVideoStreamState(state, json.RawMessage("{bad"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode video stream")
	})

	t.Run("missing response ignored", func(t *testing.T) {
		state := &videoStreamState{}
		require.NoError(t, updateVideoStreamState(state, json.RawMessage(`{"result":"text"}`)))
		assert.False(t, state.moderated)
		assert.Empty(t, state.videoURL)
	})

	t.Run("moderated and url captured with trimming", func(t *testing.T) {
		state := &videoStreamState{}
		payload, err := json.Marshal(map[string]any{
			"result": map[string]any{
				"response": map[string]any{
					"moderated": true,
					"streamingVideoGenerationResponse": map[string]any{
						"videoUrl": "  https://x/v.mp4  ",
					},
				},
			},
		})
		require.NoError(t, err)
		require.NoError(t, updateVideoStreamState(state, payload))
		assert.True(t, state.moderated)
		assert.Equal(t, "https://x/v.mp4", state.videoURL)
	})

	t.Run("moderated false ignored", func(t *testing.T) {
		state := &videoStreamState{}
		payload, err := json.Marshal(map[string]any{
			"result": map[string]any{"response": map[string]any{"moderated": false}},
		})
		require.NoError(t, err)
		require.NoError(t, updateVideoStreamState(state, payload))
		assert.False(t, state.moderated)
	})
}

func TestCollectVideoStreamStateTable(t *testing.T) {
	t.Run("error event", func(t *testing.T) {
		ch := make(chan xai.StreamEvent, 1)
		ch <- xai.StreamEvent{Error: errors.New("bad stream")}
		close(ch)
		_, err := collectVideoStreamState(ch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "video stream")
	})

	t.Run("invalid json event", func(t *testing.T) {
		ch := make(chan xai.StreamEvent, 1)
		ch <- xai.StreamEvent{Data: json.RawMessage("{bad")}
		close(ch)
		_, err := collectVideoStreamState(ch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode video stream")
	})

	t.Run("valid events", func(t *testing.T) {
		ch := make(chan xai.StreamEvent, 1)
		ch <- videoStreamPayload(t, map[string]any{
			"streamingVideoGenerationResponse": map[string]any{"videoUrl": "https://x/v.mp4"},
		})
		close(ch)
		state, err := collectVideoStreamState(ch)
		require.NoError(t, err)
		assert.Equal(t, "https://x/v.mp4", state.videoURL)
	})
}

func TestVideoFlow_CacheVideoSaveFailsAfterDownload(t *testing.T) {
	// The cache root is a regular file so SaveStream fails immediately while
	// the download itself completes; the save error must win.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	client := &stubVideoClient{
		ignoreWriteErr: true,
		chatEvents: []xai.StreamEvent{videoStreamPayload(t, map[string]any{
			"streamingVideoGenerationResponse": map[string]any{"videoUrl": "https://example.com/video.mp4"},
		})},
	}
	vf := NewVideoFlow(tokenSvc, func(string) VideoClient { return client }, &VideoFlowConfig{
		TimeoutSeconds:      5,
		PollIntervalSeconds: 1,
		ModelResolver:       testModelResolver(),
	})
	vf.SetCacheService(cache.NewService(blocker, nil))

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	assert.Empty(t, url)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVideoCache)
	assert.Contains(t, err.Error(), "save video")
	assert.True(t, client.downloadToCalled)
}

package flow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// proCaptureClient records enableNSFW/enablePro flags and returns canned events.
type proCaptureClient struct {
	mu        sync.Mutex
	events    []xai.ImageEvent
	err       error
	nsfwFlags []bool
	proFlags  []bool
}

func (c *proCaptureClient) Generate(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
	c.mu.Lock()
	c.nsfwFlags = append(c.nsfwFlags, enableNSFW)
	c.proFlags = append(c.proFlags, enablePro)
	events := c.events
	err := c.err
	c.mu.Unlock()

	if err != nil {
		return nil, err
	}
	ch := make(chan xai.ImageEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func TestImageRequestValidateMatrix(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(r *ImageRequest)
		wantErr    string
		wantFormat string
	}{
		{"valid defaults", func(r *ImageRequest) { r.Prompt = "p" }, "", "b64_json"},
		{"empty prompt", func(r *ImageRequest) {}, "prompt is required", ""},
		{"n over limit", func(r *ImageRequest) { r.Prompt = "p"; r.N = 11 }, "n must be between 1 and 10", ""},
		{"bad response format", func(r *ImageRequest) { r.Prompt = "p"; r.ResponseFormat = "gif" }, "response_format must be url, b64_json, or base64", ""},
		{"quality unsupported", func(r *ImageRequest) { r.Prompt = "p"; r.Quality = "hd" }, "quality is not supported", ""},
		{"style unsupported", func(r *ImageRequest) { r.Prompt = "p"; r.Style = "vivid" }, "style is not supported", ""},
		{"url format accepted", func(r *ImageRequest) { r.Prompt = "p"; r.ResponseFormat = "url" }, "", "url"},
		{"base64 format accepted", func(r *ImageRequest) { r.Prompt = "p"; r.ResponseFormat = "base64" }, "", "base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ImageRequest{}
			tt.mutate(req)
			err := req.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, 1, req.N)
				assert.Equal(t, "1024x1024", req.Size)
				assert.Equal(t, tt.wantFormat, req.ResponseFormat)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestImageEditRequestValidateMatrix(t *testing.T) {
	png := createTestPNG()
	tests := []struct {
		name    string
		req     *ImageEditRequest
		wantErr string
	}{
		{"valid", &ImageEditRequest{Prompt: "p", OriginalImages: [][]byte{png}}, ""},
		{"empty prompt", &ImageEditRequest{OriginalImages: [][]byte{png}}, "prompt is required"},
		{"no images", &ImageEditRequest{Prompt: "p"}, "at least one original image is required"},
		{"n over limit", &ImageEditRequest{Prompt: "p", OriginalImages: [][]byte{png}, N: 11}, "n must be between 1 and 10"},
		{"bad format", &ImageEditRequest{Prompt: "p", OriginalImages: [][]byte{png}, ResponseFormat: "gif"}, "response_format must be url, b64_json, or base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestImageFlow_Generate_InvalidRequest(t *testing.T) {
	flow := newTestImageFlow(&mockImagineClient{})
	_, err := flow.Generate(context.Background(), &ImageRequest{Model: "grok-imagine-image"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestImageFlow_Generate_NilClient(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, func(token string) ImagineGenerator { return nil })
	flow.SetModelResolver(testModelResolver())

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image client is nil")
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
}

func TestImageFlow_Generate_ClientGenerateError(t *testing.T) {
	mock := &mockImagineClient{err: errors.New("ws down")}
	flow := newTestImageFlow(mock)
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow.tokenSvc = tokenSvc

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start generation")
}

func TestImageFlow_Generate_UnknownErrorEvent(t *testing.T) {
	mock := &mockImagineClient{events: []xai.ImageEvent{{Type: xai.ImageEventError}}}
	flow := newTestImageFlow(mock)

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown generation error")
}

func TestImageFlow_Generate_NoFinalImage(t *testing.T) {
	mock := &mockImagineClient{events: []xai.ImageEvent{{Type: xai.ImageEventPreview, ImageData: "p"}}}
	flow := newTestImageFlow(mock)

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no final image received")
}

func TestImageFlow_Generate_EnableProResolver(t *testing.T) {
	client := &proCaptureClient{events: []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "img"}}}
	flow := newTestImageFlow(client)
	flow.SetEnableProResolver(func(model string) bool { return true })

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "pro",
		Size:   "1024x1024",
	})
	require.NoError(t, err)
	require.Len(t, client.proFlags, 1)
	assert.True(t, client.proFlags[0])
}

func TestImageFlow_Generate_UnsupportedOutputFormat(t *testing.T) {
	mock := &mockImagineClient{events: []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "img"}}}
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, func(string) ImagineGenerator { return mock })
	flow.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	flow.SetUsageRecorder(recorder)
	flow.SetImageConfigProvider(func() *config.ImageConfig {
		return &config.ImageConfig{Format: "floppy_disk"}
	})

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 500, recorder.records[0].Status)
}

func TestImageFlow_Generate_BlockedRecoveryDisabled(t *testing.T) {
	disabled := false
	mock := &mockImagineClient{events: []xai.ImageEvent{{Type: xai.ImageEventBlocked}}}
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, func(string) ImagineGenerator { return mock })
	flow.SetModelResolver(testModelResolver())
	flow.SetImageConfig(&config.ImageConfig{BlockedParallelEnabled: &disabled})

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "blocked",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	assert.Equal(t, []uint{1, 1}, tokenSvc.releaseCalls)
	assert.Empty(t, tokenSvc.successCalls)
}

func TestImageFlow_Generate_BlockedRecoveryNoTokens(t *testing.T) {
	enabled := true
	mock := &mockImagineClient{events: []xai.ImageEvent{{Type: xai.ImageEventBlocked}}}
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, func(string) ImagineGenerator { return mock })
	flow.SetModelResolver(testModelResolver())
	flow.SetImageConfig(&config.ImageConfig{BlockedParallelEnabled: &enabled})

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "blocked",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errImageGenerationBlocked)
	assert.Empty(t, tokenSvc.successCalls)
}

func TestImageFlow_Generate_BlockedRecoveryAllErrors(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok-1", Pool: "basic"},
		{ID: 2, Token: "tok-2", Pool: "basic"},
		{ID: 3, Token: "tok-3", Pool: "basic"},
	}}
	flow := NewImageFlow(tokenSvc, func(token string) ImagineGenerator {
		return imagineGeneratorFunc(func(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
			ch := make(chan xai.ImageEvent, 1)
			if token == "tok-1" {
				ch <- xai.ImageEvent{Type: xai.ImageEventBlocked}
			} else {
				ch <- xai.ImageEvent{Type: xai.ImageEventError, Error: errors.New("gpu exploded")}
			}
			close(ch)
			return ch, nil
		})
	})
	flow.SetModelResolver(testModelResolver())
	enabled := true
	flow.SetImageConfig(&config.ImageConfig{BlockedParallelAttempts: 2, BlockedParallelEnabled: &enabled})

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "blocked",
		Size:   "1024x1024",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gpu exploded")
	assert.Empty(t, tokenSvc.successCalls)
	released := map[uint]bool{}
	for _, id := range tokenSvc.releaseCalls {
		released[id] = true
	}
	assert.True(t, released[1], "initial token should be released")
	assert.True(t, released[2] && released[3], "failed recovery tokens should be released")
}

func TestImageFlow_Generate_BlockedRecoveryWinnerAndLoser(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok-1", Pool: "basic"},
		{ID: 2, Token: "tok-2", Pool: "basic"},
		{ID: 3, Token: "tok-3", Pool: "basic"},
	}}
	flow := NewImageFlow(tokenSvc, func(token string) ImagineGenerator {
		return imagineGeneratorFunc(func(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
			ch := make(chan xai.ImageEvent, 1)
			if token == "tok-1" {
				ch <- xai.ImageEvent{Type: xai.ImageEventBlocked}
			} else {
				ch <- xai.ImageEvent{Type: xai.ImageEventFinal, ImageData: "img-" + token}
			}
			close(ch)
			return ch, nil
		})
	})
	flow.SetModelResolver(testModelResolver())
	enabled := true
	flow.SetImageConfig(&config.ImageConfig{BlockedParallelAttempts: 2, BlockedParallelEnabled: &enabled})

	resp, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "blocked",
		Size:   "1024x1024",
	})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)

	winner := tokenSvc.successCalls[0]
	require.Len(t, tokenSvc.successCalls, 1)
	assert.Contains(t, []uint{2, 3}, winner)

	loser := uint(2)
	if winner == 2 {
		loser = 3
	}
	released := map[uint]bool{}
	for _, id := range tokenSvc.releaseCalls {
		released[id] = true
	}
	assert.True(t, released[1], "initial token should be released")
	assert.True(t, released[loser], "successful loser should release inflight")
}

func TestSelectImageRecoveryResultMatrix(t *testing.T) {
	newTok := func(id uint) *store.Token { return &store.Token{ID: id, Token: "tok", Pool: "basic"} }

	t.Run("mode tracked: success wins, errors reported, blocked released", func(t *testing.T) {
		svc := &mockTokenService{}
		flow := &ImageFlow{tokenSvc: svc}
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan imageAttemptResult, 3)
		ch <- imageAttemptResult{token: newTok(1), err: errors.New("500 boom")}
		ch <- imageAttemptResult{token: newTok(2), err: errImageGenerationBlocked}
		ch <- imageAttemptResult{token: newTok(3), b64JSON: "img"}
		close(ch)

		result, err := flow.selectImageRecoveryResult("m", ch, cancel, "auto", 60)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "img", result.b64JSON)
		assert.Equal(t, []uint{1}, svc.errorCalls)
		assert.Equal(t, []uint{2}, svc.releaseCalls)
		assert.Equal(t, []uint{3}, svc.successCalls)
	})

	t.Run("no winner returns first non-blocked error", func(t *testing.T) {
		svc := &mockTokenService{}
		flow := &ImageFlow{tokenSvc: svc}
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan imageAttemptResult, 2)
		ch <- imageAttemptResult{token: newTok(1), err: errImageGenerationBlocked}
		ch <- imageAttemptResult{token: newTok(2), err: errors.New("boom")}
		close(ch)

		_, err := flow.selectImageRecoveryResult("m", ch, cancel, "auto", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assert.Equal(t, []uint{1}, svc.releaseCalls)
		assert.Equal(t, []uint{2}, svc.errorCalls)
	})

	t.Run("only blocked errors returns blocked sentinel", func(t *testing.T) {
		svc := &mockTokenService{}
		flow := &ImageFlow{tokenSvc: svc}
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan imageAttemptResult, 1)
		ch <- imageAttemptResult{token: newTok(1), err: errImageGenerationBlocked}
		close(ch)

		_, err := flow.selectImageRecoveryResult("m", ch, cancel, "auto", 0)
		assert.ErrorIs(t, err, errImageGenerationBlocked)
		assert.Equal(t, []uint{1}, svc.releaseCalls)
	})
}

func TestResolveEnableNSFWMatrix(t *testing.T) {
	yes := true
	no := false
	cfg := &config.ImageConfig{NSFW: true}
	assert.False(t, resolveEnableNSFW(nil, nil))
	assert.True(t, resolveEnableNSFW(nil, cfg))
	assert.True(t, resolveEnableNSFW(&yes, nil))
	assert.False(t, resolveEnableNSFW(&no, cfg))
}

func TestBlockedParallelAttemptsMatrix(t *testing.T) {
	assert.Equal(t, defaultBlockedParallelAttempts, blockedParallelAttempts(nil))
	assert.Equal(t, defaultBlockedParallelAttempts, blockedParallelAttempts(&config.ImageConfig{BlockedParallelAttempts: 0}))
	assert.Equal(t, defaultBlockedParallelAttempts, blockedParallelAttempts(&config.ImageConfig{BlockedParallelAttempts: -3}))
	assert.Equal(t, 3, blockedParallelAttempts(&config.ImageConfig{BlockedParallelAttempts: 3}))
	assert.Equal(t, maxBlockedParallelAttempts, blockedParallelAttempts(&config.ImageConfig{BlockedParallelAttempts: 99}))
}

func TestImageOutputBase64NoData(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig {
		return &config.ImageConfig{Format: "base64"}
	}}
	_, err := f.resolveImageOutput(context.Background(), imageOutputInput{Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data to encode")
}

func TestImageOutputLocalURLErrors(t *testing.T) {
	f := func(imageConfigFn func() *config.ImageConfig) *ImageFlow {
		return &ImageFlow{
			imageConfigFn: imageConfigFn,
			cacheSvc:      cache.NewService(t.TempDir(), nil),
		}
	}
	localURL := func() *config.ImageConfig { return &config.ImageConfig{Format: "local_url"} }

	t.Run("bad base64", func(t *testing.T) {
		_, err := f(localURL).resolveImageOutput(context.Background(), imageOutputInput{B64JSON: "!!not-base64!!"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode base64")
	})
	t.Run("nil download", func(t *testing.T) {
		_, err := f(localURL).resolveImageOutput(context.Background(), imageOutputInput{RawURL: "https://x/y.png"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download not available")
	})
	t.Run("no data", func(t *testing.T) {
		_, err := f(localURL).resolveImageOutput(context.Background(), imageOutputInput{Prompt: "p"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no data to cache")
	})
	t.Run("cache save failure", func(t *testing.T) {
		// Use a regular file as the cache root so MkdirAll fails.
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
		flow := &ImageFlow{
			imageConfigFn: localURL,
			cacheSvc:      cache.NewService(blocker, nil),
		}
		_, err := flow.resolveImageOutput(context.Background(), imageOutputInput{B64JSON: "aGVsbG8="})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cache save")
	})
}

func TestDetectImageExtTable(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	gif := []byte("GIF89a....")
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", jpeg, ".jpg"},
		{"gif", gif, ".gif"},
		{"webp", webp, ".webp"},
		{"default png", []byte("plain text"), ".png"},
		{"empty", nil, ".png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectImageExt(tt.data))
		})
	}
}

func TestValidateMediaUpstreamTable(t *testing.T) {
	tests := []struct {
		name          string
		upstreamModel string
		upstreamMode  string
		wantErr       string
	}{
		{"ok", "imagine", "fast", ""},
		{"missing model", "  ", "fast", "upstream model is required"},
		{"missing mode", "imagine", "", "upstream mode is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMediaUpstream(tt.upstreamModel, tt.upstreamMode)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestImageFlow_ReportWSImageErrorMatrix(t *testing.T) {
	newFlow := func() (*ImageFlow, *mockTokenService) {
		svc := &mockTokenService{}
		return &ImageFlow{tokenSvc: svc, cooldownUntil: make(map[string]time.Time)}, svc
	}

	t.Run("invalid token marks expired", func(t *testing.T) {
		flow, svc := newFlow()
		flow.reportWSImageError("m", 1, xai.ErrInvalidToken, 60)
		assert.Equal(t, []uint{1}, svc.expiredCalls)
	})

	t.Run("rate limited sets cooldown and releases", func(t *testing.T) {
		flow, svc := newFlow()
		flow.reportWSImageError("m", 1, xai.ErrRateLimited, 60)
		assert.Equal(t, []uint{1}, svc.releaseCalls)
		assert.True(t, flow.wsCooldownActive("m", 1, time.Now()))
	})

	t.Run("busy error sets cooldown and releases", func(t *testing.T) {
		flow, svc := newFlow()
		flow.reportWSImageError("m", 1, errors.New("upstream busy"), 60)
		assert.Equal(t, []uint{1}, svc.releaseCalls)
		assert.True(t, flow.wsCooldownActive("m", 1, time.Now()))
	})

	t.Run("plain error only releases", func(t *testing.T) {
		flow, svc := newFlow()
		flow.reportWSImageError("m", 1, errors.New("boom"), 60)
		assert.Equal(t, []uint{1}, svc.releaseCalls)
		assert.False(t, flow.wsCooldownActive("m", 1, time.Now()))
	})

	t.Run("zero cooldown seconds skips cooldown", func(t *testing.T) {
		flow, svc := newFlow()
		flow.reportWSImageError("m", 1, xai.ErrRateLimited, 0)
		assert.Equal(t, []uint{1}, svc.releaseCalls)
		assert.False(t, flow.wsCooldownActive("m", 1, time.Now()))
	})
}

func TestImageFlow_WSCooldownLifecycle(t *testing.T) {
	flow := &ImageFlow{cooldownUntil: make(map[string]time.Time)}

	assert.False(t, flow.wsCooldownActive("m", 1, time.Now()), "no entry means inactive")

	flow.setWSCooldown("m", 1, 60)
	assert.True(t, flow.wsCooldownActive("m", 1, time.Now()))

	// Expired entries are removed on check.
	assert.False(t, flow.wsCooldownActive("m", 1, time.Now().Add(2*time.Minute)))
	assert.False(t, flow.wsCooldownActive("m", 1, time.Now()), "entry should be deleted after expiry")

	flow.setWSCooldown("m", 1, 0)
	assert.False(t, flow.wsCooldownActive("m", 1, time.Now()))

	assert.Equal(t, "m:7", wsCooldownKey("m", 7))
}

func TestIsWSBusyErrorTable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate limit", errors.New("Rate Limit exceeded"), true},
		{"too many requests", errors.New("429 too many requests"), true},
		{"busy", errors.New("server is BUSY"), true},
		{"other", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isWSBusyError(tt.err))
		})
	}
}

func TestImageFlow_PickTokenForModelExcludingMatrix(t *testing.T) {
	t.Run("nil resolver", func(t *testing.T) {
		flow := &ImageFlow{tokenSvc: &mockTokenService{}}
		_, err := flow.pickTokenForModelExcluding("m", "auto", nil)
		assert.ErrorIs(t, err, tkn.ErrModelNotFound)
	})

	t.Run("unknown model", func(t *testing.T) {
		flow := &ImageFlow{tokenSvc: &mockTokenService{}}
		flow.SetModelResolver(testModelResolver())
		_, err := flow.pickTokenForModelExcluding("unknown", "auto", nil)
		assert.ErrorIs(t, err, tkn.ErrModelNotFound)
	})

	t.Run("empty mode uses any-pool picking", func(t *testing.T) {
		svc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "t", Pool: "basic"}}}
		flow := &ImageFlow{tokenSvc: svc}
		flow.SetModelResolver(testModelResolver())
		tok, err := flow.pickTokenForModelExcluding("grok-imagine-image", "", nil)
		require.NoError(t, err)
		require.NotNil(t, tok)
		assert.Equal(t, []string{""}, svc.pickModes)
	})

	t.Run("mode picking", func(t *testing.T) {
		svc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "t", Pool: "basic"}}}
		flow := &ImageFlow{tokenSvc: svc}
		flow.SetModelResolver(testModelResolver())
		tok, err := flow.pickTokenForModelExcluding("grok-imagine-image", "auto", nil)
		require.NoError(t, err)
		require.NotNil(t, tok)
		assert.Equal(t, []string{"auto"}, svc.pickModes)
	})

	t.Run("all pools exhausted returns last error", func(t *testing.T) {
		flow := &ImageFlow{tokenSvc: &mockTokenService{}}
		flow.SetModelResolver(testModelResolver())
		_, err := flow.pickTokenForModelExcluding("grok-imagine-image", "auto", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no tokens available")
	})
}

func TestImageFlow_PickWSTokenWithoutResolver(t *testing.T) {
	flow := &ImageFlow{tokenSvc: &mockTokenService{}}
	_, err := flow.pickWSTokenForModel("grok-imagine-image")
	assert.ErrorIs(t, err, tkn.ErrModelNotFound)
}

func TestImageFlow_SelectRecoveryTokensWithMode(t *testing.T) {
	svc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "t1", Pool: "basic"},
		{ID: 2, Token: "t2", Pool: "basic"},
		{ID: 3, Token: "t3", Pool: "basic"},
	}}
	flow := &ImageFlow{tokenSvc: svc}
	flow.SetModelResolver(testModelResolver())

	tokens := flow.selectRecoveryTokens("grok-imagine-image", "auto", 1, 2)
	require.Len(t, tokens, 2)
	assert.Equal(t, []uint{2, 3}, svc.pickCalls, "initial token excluded from recovery picks")
}

func TestImageOutputLocalURLDownloadError(t *testing.T) {
	flow := &ImageFlow{
		imageConfigFn: func() *config.ImageConfig { return &config.ImageConfig{Format: "local_url"} },
		cacheSvc:      cache.NewService(t.TempDir(), nil),
	}
	_, err := flow.resolveImageOutput(context.Background(), imageOutputInput{
		RawURL: "https://assets.grok.com/x.png",
		Download: func(ctx context.Context, url string) ([]byte, error) {
			return nil, errors.New("cdn down")
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download")
}

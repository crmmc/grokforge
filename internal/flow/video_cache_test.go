package flow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

const largeVideoBytes = 2 * 1024 * 1024

func TestVideoFlow_GenerateSync_CacheServiceRequired(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}
	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if !errors.Is(err, ErrVideoCache) {
		t.Fatalf("expected video cache error, got url=%q err=%v", url, err)
	}
	if url != "" {
		t.Fatalf("GenerateSync() URL = %q, want empty on cache error", url)
	}
}

func TestVideoFlow_GenerateSync_DownloadFailureReturnsError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		downloadErr: errors.New("download failed"),
		videoURL:    "https://example.com/video.mp4",
	}
	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	setTestVideoCache(t, vf)

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if !errors.Is(err, ErrVideoCache) {
		t.Fatalf("expected video cache error, got url=%q err=%v", url, err)
	}
	if url != "" {
		t.Fatalf("GenerateSync() URL = %q, want empty on download error", url)
	}
}

func TestVideoFlow_GenerateSync_DownloadPanicReturnsError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		downloadPanic: "download panic",
		videoURL:      "https://example.com/video.mp4",
	}
	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	setTestVideoCache(t, vf)

	type result struct {
		url string
		err error
	}
	done := make(chan result, 1)
	go func() {
		url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
			Prompt: "Test",
			Model:  "grok-imagine-video",
		}))
		done <- result{url: url, err: err}
	}()

	select {
	case got := <-done:
		if !errors.Is(got.err, ErrVideoCache) {
			t.Fatalf("expected video cache error, got url=%q err=%v", got.url, got.err)
		}
		if got.url != "" {
			t.Fatalf("GenerateSync() URL = %q, want empty on download panic", got.url)
		}
	case <-time.After(time.Second):
		t.Fatal("GenerateSync() hung after download panic")
	}
}

func TestVideoFlow_GenerateSync_SaveStreamFailureReturnsError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		downloadBytes: largeVideoBytes,
		videoURL:      "https://example.com/video.mp4",
	}
	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	setLimitedTestVideoCache(t, vf, 1)

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if !errors.Is(err, ErrVideoCache) {
		t.Fatalf("expected video cache error, got url=%q err=%v", url, err)
	}
	if url != "" {
		t.Fatalf("GenerateSync() URL = %q, want empty on save error", url)
	}
}

func setTestVideoCache(t *testing.T, vf *VideoFlow) {
	t.Helper()
	vf.SetCacheService(cache.NewService(t.TempDir(), nil))
}

func setLimitedTestVideoCache(t *testing.T, vf *VideoFlow, videoMaxMB int) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Cache.VideoMaxMB = videoMaxMB
	vf.SetCacheService(cache.NewService(t.TempDir(), config.NewRuntime(cfg)))
}

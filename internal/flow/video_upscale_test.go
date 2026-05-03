package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

func TestVideoFlow_GenerateSync_UpscaleFailureReturnsError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: tkn.PoolBasic}},
	}
	client := &mockVideoClient{
		pollErr:  errors.New("upscale failed"),
		videoURL: "https://assets.grok.com/users/u/generated/123e4567-e89b-12d3-a456-426614174000/video.mp4",
	}
	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	setTestVideoCache(t, vf)

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:  "Test",
		Model:   "grok-imagine-video",
		Quality: "high",
	}))
	if !errors.Is(err, ErrVideoPostProcess) {
		t.Fatalf("expected video postprocess error, got url=%q err=%v", url, err)
	}
	if url != "" {
		t.Fatalf("GenerateSync() URL = %q, want empty on upscale error", url)
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.successCalls) != 1 || tokenSvc.successCalls[0] != 1 {
		t.Fatalf("expected token success without penalty, got successes=%v", tokenSvc.successCalls)
	}
	if len(tokenSvc.errorCalls) != 0 || len(tokenSvc.rateLimitCalls) != 0 || len(tokenSvc.expiredCalls) != 0 {
		t.Fatalf("expected no token penalty, errors=%v rate_limits=%v expired=%v",
			tokenSvc.errorCalls, tokenSvc.rateLimitCalls, tokenSvc.expiredCalls)
	}
}

func TestVideoFlow_GenerateSync_UpscaleTokenErrorPenalizesToken(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: tkn.PoolBasic}},
	}
	client := &mockVideoClient{
		pollErr:  xai.ErrInvalidToken,
		videoURL: "https://assets.grok.com/users/u/generated/123e4567-e89b-12d3-a456-426614174000/video.mp4",
	}
	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	setTestVideoCache(t, vf)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:  "Test",
		Model:   "grok-imagine-video",
		Quality: "high",
	}))
	if !errors.Is(err, xai.ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.successCalls) != 0 {
		t.Fatalf("expected no token success, got %v", tokenSvc.successCalls)
	}
	if len(tokenSvc.expiredCalls) != 1 || tokenSvc.expiredCalls[0] != 1 {
		t.Fatalf("expected token expiry, got %v", tokenSvc.expiredCalls)
	}
}

package flow

import (
	"context"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
)

func TestVideoFlow_GenerateSync_MultipleReferences(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	vf.SetModeResolver(testModeResolver())
	setTestVideoCache(t, vf)

	images := [][]byte{
		[]byte("fake-png-1"),
		[]byte("fake-png-2"),
		[]byte("fake-png-3"),
	}

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:          "animate these",
		Model:           "grok-imagine-video",
		Size:            "1280x720",
		ReferenceImages: images,
	}))
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.uploadCalls != 3 {
		t.Errorf("uploadCalls = %d, want 3", client.uploadCalls)
	}
	if client.imagePostCalls != 3 {
		t.Errorf("imagePostCalls = %d, want 3", client.imagePostCalls)
	}

	if client.lastChatReq == nil {
		t.Fatal("expected captured chat request")
	}
	mc, ok := client.lastChatReq.ModelConfig["modelMap"].(map[string]any)
	if !ok {
		t.Fatal("missing modelMap in ModelConfig")
	}
	vc, ok := mc["videoGenModelConfig"].(map[string]any)
	if !ok {
		t.Fatal("missing videoGenModelConfig")
	}

	refs, ok := vc["imageReferences"].([]string)
	if !ok {
		t.Fatal("imageReferences missing or wrong type")
	}
	if len(refs) != 3 {
		t.Errorf("imageReferences len = %d, want 3", len(refs))
	}
	if v, _ := vc["isReferenceToVideo"].(bool); !v {
		t.Error("isReferenceToVideo should be true")
	}
	if v, ok := vc["isVideoEdit"].(bool); !ok || v {
		t.Error("isVideoEdit should be false")
	}
}

func TestVideoFlow_GenerateSync_SingleReference(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	vf.SetModeResolver(testModeResolver())
	setTestVideoCache(t, vf)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt:          "animate this",
		Model:           "grok-imagine-video",
		Size:            "1280x720",
		ReferenceImages: [][]byte{[]byte("fake-png")},
	}))
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.uploadCalls != 1 {
		t.Errorf("uploadCalls = %d, want 1", client.uploadCalls)
	}
	if client.imagePostCalls != 1 {
		t.Errorf("imagePostCalls = %d, want 1", client.imagePostCalls)
	}

	mc := client.lastChatReq.ModelConfig["modelMap"].(map[string]any)
	vc := mc["videoGenModelConfig"].(map[string]any)

	refs, ok := vc["imageReferences"].([]string)
	if !ok || len(refs) != 1 {
		t.Errorf("imageReferences = %v, want 1 URL", refs)
	}
	if v, _ := vc["isReferenceToVideo"].(bool); !v {
		t.Error("isReferenceToVideo should be true")
	}
	if v, ok := vc["isVideoEdit"].(bool); !ok || v {
		t.Error("isVideoEdit should be false")
	}
}

func TestVideoFlow_GenerateSync_NoReference_NoImageFields(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	vf.SetModeResolver(testModeResolver())
	setTestVideoCache(t, vf)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "generate a sunset",
		Model:  "grok-imagine-video",
		Size:   "1280x720",
	}))
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.uploadCalls != 0 {
		t.Errorf("uploadCalls = %d, want 0", client.uploadCalls)
	}
	if client.imagePostCalls != 0 {
		t.Errorf("imagePostCalls = %d, want 0", client.imagePostCalls)
	}

	mc := client.lastChatReq.ModelConfig["modelMap"].(map[string]any)
	vc := mc["videoGenModelConfig"].(map[string]any)

	if _, exists := vc["imageReferences"]; exists {
		t.Error("imageReferences should not be present for no-reference request")
	}
	if _, exists := vc["isReferenceToVideo"]; exists {
		t.Error("isReferenceToVideo should not be present for no-reference request")
	}
	if _, exists := vc["isVideoEdit"]; exists {
		t.Error("isVideoEdit should not be present for no-reference request")
	}
}

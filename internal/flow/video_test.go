package flow

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

// mockVideoClient simulates xai video API calls.
type mockVideoClient struct {
	mu          sync.Mutex
	chatErr     error
	chatDelay   time.Duration
	videoURL    string
	pollCalls   int
	lastChatReq *xai.ChatRequest
}

func (m *mockVideoClient) Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	m.mu.Lock()
	chatErr := m.chatErr
	chatDelay := m.chatDelay
	videoURL := m.videoURL
	m.lastChatReq = req
	m.mu.Unlock()
	if chatErr != nil {
		return nil, chatErr
	}

	if chatDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(chatDelay):
		}
	}

	// Simulate a chat response containing video URL
	payload := []byte(`{"result":{"response":{"post":{"id":"post-1"},"streamingVideoGenerationResponse":{"videoUrl":"` + videoURL + `"}}}}`)
	ch := make(chan xai.StreamEvent, 1)
	ch <- xai.StreamEvent{Data: payload}
	close(ch)
	return ch, nil
}

func (m *mockVideoClient) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	return "post-1", nil
}

func (m *mockVideoClient) CreateVideoPost(ctx context.Context, prompt string) (string, error) {
	return "post-1", nil
}

func (m *mockVideoClient) PollUpscale(ctx context.Context, jobID string, interval time.Duration) (string, error) {
	m.mu.Lock()
	m.pollCalls++
	videoURL := m.videoURL
	m.mu.Unlock()
	return videoURL, nil
}

func (m *mockVideoClient) DownloadURL(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}

func (m *mockVideoClient) DownloadTo(ctx context.Context, url string, w io.Writer) error {
	_, err := io.WriteString(w, "video-data")
	return err
}

func (m *mockVideoClient) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	return "file-1", "generated/ref-image", nil
}

func withVideoUpstream(req *VideoRequest) *VideoRequest {
	if req.UpstreamModel == "" {
		req.UpstreamModel = "grok-3"
	}
	if req.UpstreamMode == "" {
		req.UpstreamMode = "MODEL_MODE_FAST"
	}
	return req
}

func TestVideoFlow_GenerateSync_Success(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)
	vf.SetModeResolver(testModeResolver())

	url, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "A sunset over mountains",
		Model:  "grok-imagine-video",
		Size:   "1280x720",
	}))
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}
	if url == "" {
		t.Error("GenerateSync() returned empty URL")
	}
	if len(tokenSvc.firstUseModes) != 1 || tokenSvc.firstUseModes[0] != "auto" {
		t.Fatalf("expected first use mode auto, got %+v", tokenSvc.firstUseModes)
	}
}

func TestVideoFlow_GenerateSync_ChatError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		chatErr: errors.New("connection refused"),
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if err == nil {
		t.Fatal("GenerateSync() expected error")
	}
}

func TestVideoFlow_GenerateSync_PresetAppended(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "ssoBasic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/generated/video.mp4",
	}

	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, &VideoFlowConfig{
		TimeoutSeconds:      5,
		PollIntervalSeconds: 1,
		ModelResolver:       testModelResolver(),
	})

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt:        "make it move",
		Model:         "grok-imagine-video",
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODEL_MODE_FAST",
		Size:          "1280x720",
		Preset:        "normal",
	})
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.lastChatReq == nil {
		t.Fatal("expected captured chat request")
	}
	got := client.lastChatReq.Messages[0].Content
	if got != "make it move --mode=normal" {
		t.Errorf("chat prompt = %q, want %q", got, "make it move --mode=normal")
	}
	if client.lastChatReq.UpstreamModel != "grok-3" {
		t.Errorf("upstream model = %q, want %q", client.lastChatReq.UpstreamModel, "grok-3")
	}
	if client.lastChatReq.UpstreamMode != "MODEL_MODE_FAST" {
		t.Errorf("upstream mode = %q, want %q", client.lastChatReq.UpstreamMode, "MODEL_MODE_FAST")
	}
}

func TestVideoFlow_GenerateSync_AspectRatioDirect(t *testing.T) {
	tests := []struct {
		name        string
		aspectRatio string
		size        string
		wantRatio   string
	}{
		{"explicit 3:2", "3:2", "720x480", "3:2"},
		{"explicit 16:9", "16:9", "853x480", "16:9"},
		{"explicit 1:1", "1:1", "480x480", "1:1"},
		{"explicit 9:16", "9:16", "270x480", "9:16"},
		{"fallback to ParseAspectRatio", "", "1280x720", "16:9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenSvc := &mockTokenService{
				tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
			}
			client := &mockVideoClient{
				videoURL: "https://example.com/video.mp4",
			}

			vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, &VideoFlowConfig{
				TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver(),
			})

			_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
				Prompt:      "test",
				Model:       "grok-imagine-video",
				Size:        tt.size,
				AspectRatio: tt.aspectRatio,
			}))
			if err != nil {
				t.Fatalf("GenerateSync() error = %v", err)
			}

			client.mu.Lock()
			defer client.mu.Unlock()
			if client.lastChatReq == nil {
				t.Fatal("expected captured chat request")
			}

			// Extract aspectRatio from modelConfig
			mc, ok := client.lastChatReq.ModelConfig["modelMap"].(map[string]any)
			if !ok {
				t.Fatal("missing modelMap in ModelConfig")
			}
			vc, ok := mc["videoGenModelConfig"].(map[string]any)
			if !ok {
				t.Fatal("missing videoGenModelConfig")
			}
			gotRatio, _ := vc["aspectRatio"].(string)
			if gotRatio != tt.wantRatio {
				t.Errorf("aspectRatio = %q, want %q", gotRatio, tt.wantRatio)
			}
		})
	}
}

func TestVideoFlow_GenerateSync_Timeout(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		chatDelay: 10 * time.Second, // longer than timeout
	}

	cfg := &VideoFlowConfig{
		TimeoutSeconds:      1,
		PollIntervalSeconds: 1,
		ModelResolver:       testModelResolver(),
	}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if err == nil {
		t.Fatal("GenerateSync() expected timeout error")
	}
}

func TestVideoFlow_GenerateSync_UnknownModelWithResolverFails(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}
	cfg := &VideoFlowConfig{
		TimeoutSeconds:      5,
		PollIntervalSeconds: 1,
		ModelResolver:       testModelResolver(),
	}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "unknown-video-model",
	}))
	if !errors.Is(err, tkn.ErrModelNotFound) {
		t.Fatalf("expected ErrModelNotFound, got %v", err)
	}
}

func TestVideoFlow_GenerateSync_NoConsumeOnFailure(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		chatErr: errors.New("generation failed"),
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if err == nil {
		t.Fatal("expected error")
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	// Consume removed: quota pre-deducted in Pick
}

func TestVideoFlow_GenerateSync_ConsumeOnSuccess(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), withVideoUpstream(&VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.successCalls) != 1 {
		t.Errorf("ReportSuccess should be called once on success, got %d calls", len(tokenSvc.successCalls))
	}
}

func TestVideoFlow_GenerateSync_MissingUpstreamMapping(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1, ModelResolver: testModelResolver()}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-video",
	})
	if err == nil || err.Error() != "upstream model is required" {
		t.Fatalf("expected upstream model is required, got %v", err)
	}
}

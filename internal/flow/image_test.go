package flow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockImagineClient implements ImagineGenerator for testing.
type mockImagineClient struct {
	mu         sync.Mutex
	events     []xai.ImageEvent
	callEvents [][]xai.ImageEvent
	err        error
	callCount  int
	nsfwCalls  []bool
}

func (m *mockImagineClient) Generate(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
	m.mu.Lock()
	m.nsfwCalls = append(m.nsfwCalls, enableNSFW)
	events := m.events
	if m.callCount < len(m.callEvents) {
		events = m.callEvents[m.callCount]
	}
	m.callCount++
	err := m.err
	m.mu.Unlock()

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

func (m *mockImagineClient) Edit(
	ctx context.Context,
	prompt, aspectRatio, originalImageB64 string,
	enableNSFW bool,
) (<-chan xai.ImageEvent, error) {
	return m.Generate(ctx, prompt, aspectRatio, enableNSFW, false)
}

type imagineGeneratorFunc func(context.Context, string, string, bool, bool) (<-chan xai.ImageEvent, error)

func (fn imagineGeneratorFunc) Generate(
	ctx context.Context,
	prompt, aspectRatio string,
	enableNSFW, enablePro bool,
) (<-chan xai.ImageEvent, error) {
	return fn(ctx, prompt, aspectRatio, enableNSFW, enablePro)
}

func (fn imagineGeneratorFunc) Edit(
	ctx context.Context,
	prompt, aspectRatio, originalImageB64 string,
	enableNSFW bool,
) (<-chan xai.ImageEvent, error) {
	return fn(ctx, prompt, aspectRatio, enableNSFW, false)
}

// newTestImageFlow creates an ImageFlow with a mock token service for testing.
func newTestImageFlow(mock ImagineGenerator) *ImageFlow {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "test-token", Pool: "basic"}},
	}
	flow := NewImageFlow(tokenSvc, func(token string) ImagineGenerator {
		return mock
	})
	flow.SetModelResolver(testModelResolver())
	return flow
}

func newTestImageFlowWithEditor(mock ImagineGenerator, editor ImageEditClient) *ImageFlow {
	flow := newTestImageFlow(mock)
	flow.SetEditClientFactory(func(token string) ImageEditClient {
		return editor
	})
	return flow
}

type mockImageEditClient struct {
	mu            sync.Mutex
	chatErr       error
	uploadErr     error
	createPostErr error
	downloadErr   error
	postID        string
	imageURLs     []string
	downloadBody  []byte
	chatRequests  []*xai.ChatRequest
	uploadedURLs  []string
}

func (m *mockImageEditClient) Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	m.mu.Lock()
	m.chatRequests = append(m.chatRequests, req)
	payload, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"response": map[string]any{
				"modelResponse": map[string]any{
					"generatedImageUrls": m.imageURLs,
				},
			},
		},
	})
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ch := make(chan xai.StreamEvent, 1)
	ch <- xai.StreamEvent{Data: payload}
	close(ch)
	return ch, nil
}

func (m *mockImageEditClient) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	if m.uploadErr != nil {
		return "", "", m.uploadErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploadedURLs = append(m.uploadedURLs, fileMimeType)
	return "file-id", "generated/test-image", nil
}

func (m *mockImageEditClient) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	if m.createPostErr != nil {
		return "", m.createPostErr
	}
	return m.postID, nil
}

func (m *mockImageEditClient) DownloadURL(ctx context.Context, url string) ([]byte, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return m.downloadBody, nil
}

func TestImageFlow_Generate_Success(t *testing.T) {
	finalData := "base64-final-image"
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventPreview, ImageData: "preview"},
			{Type: xai.ImageEventMedium, ImageData: "medium"},
			{Type: xai.ImageEventFinal, ImageData: finalData, RequestID: "req-1"},
		},
	}

	flow := newTestImageFlow(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "a beautiful sunset",
		Size:   "1024x1024",
		N:      1,
	}

	resp, err := flow.Generate(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, finalData, resp.Data[0].B64JSON)
}

func TestImageFlow_Generate_Blocked(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventMedium, ImageData: "medium"},
			{Type: xai.ImageEventBlocked, RequestID: "req-1"},
		},
	}

	flow := newTestImageFlow(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "blocked content",
		Size:   "1024x1024",
		N:      1,
	}

	resp, err := flow.Generate(ctx, req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "blocked")
}

func TestImageFlow_Generate_BlockedRecoverySuccess(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{
			{ID: 1, Token: "tok-1", Pool: "basic"},
			{ID: 2, Token: "tok-2", Pool: "basic"},
		},
	}
	var mu sync.Mutex
	usedTokens := make([]string, 0, 2)
	flow := NewImageFlow(tokenSvc, func(token string) ImagineGenerator {
		return imagineGeneratorFunc(func(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
			mu.Lock()
			usedTokens = append(usedTokens, token)
			mu.Unlock()

			events := []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "recovered-image", RequestID: "req-2"}}
			if token == "tok-1" {
				events = []xai.ImageEvent{{Type: xai.ImageEventBlocked, RequestID: "req-1"}}
			}
			ch := make(chan xai.ImageEvent, len(events))
			for _, event := range events {
				ch <- event
			}
			close(ch)
			return ch, nil
		})
	})
	flow.SetModelResolver(testModelResolver())
	enabled := true
	flow.SetImageConfig(&config.ImageConfig{
		BlockedParallelAttempts: 1,
		BlockedParallelEnabled:  &enabled,
	})

	resp, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "recover me",
		Size:   "1024x1024",
		N:      1,
	})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "recovered-image", resp.Data[0].B64JSON)
	assert.Equal(t, []string{"tok-1", "tok-2"}, usedTokens)
}

func TestImageFlow_Generate_UsesConfigNSFWDefault(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventFinal, ImageData: "img"},
		},
	}
	flow := newTestImageFlow(mock)
	flow.SetImageConfig(&config.ImageConfig{NSFW: true})

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "nsfw default",
		Size:   "1024x1024",
		N:      1,
	})
	require.NoError(t, err)
	require.Len(t, mock.nsfwCalls, 1)
	assert.True(t, mock.nsfwCalls[0])
}

func TestImageFlow_Generate_UnknownModelWithResolverFails(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventFinal, ImageData: "final", RequestID: "req-1"},
		},
	}
	flow := newTestImageFlow(mock)
	flow.SetModelResolver(testModelResolver())

	_, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "unknown-image-model",
		Prompt: "draw something",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, tkn.ErrModelNotFound)
}

func TestImageFlow_Generate_MultipleImages(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventFinal, ImageData: "img1", RequestID: "req-1"},
		},
	}

	flow := newTestImageFlow(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
		N:      3, // Request 3 images
	}

	resp, err := flow.Generate(ctx, req)
	require.NoError(t, err)
	// Should have 3 images (mock returns same for each call)
	assert.Len(t, resp.Data, 3)
}

func TestImageFlow_Generate_AspectRatioMapping(t *testing.T) {
	tests := []struct {
		size          string
		expectedRatio string
	}{
		{"1024x1024", "1:1"},
		{"1792x1024", "3:2"},
		{"1024x1792", "2:3"},
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			var capturedRatio string
			mock := &mockImagineClient{
				events: []xai.ImageEvent{
					{Type: xai.ImageEventFinal, ImageData: "img"},
				},
			}

			// Use a capturing mock
			capturingMock := &capturingImagineClient{
				inner:         mock,
				capturedRatio: &capturedRatio,
			}

			flow := newTestImageFlow(capturingMock)

			ctx := context.Background()
			req := &ImageRequest{
				Model:  "grok-imagine-image",
				Prompt: "test",
				Size:   tt.size,
				N:      1,
			}

			_, _ = flow.Generate(ctx, req)
			assert.Equal(t, tt.expectedRatio, capturedRatio)
		})
	}
}

type capturingImagineClient struct {
	inner         *mockImagineClient
	capturedRatio *string
}

func (c *capturingImagineClient) Generate(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
	*c.capturedRatio = aspectRatio
	return c.inner.Generate(ctx, prompt, aspectRatio, enableNSFW, enablePro)
}

func (c *capturingImagineClient) Edit(
	ctx context.Context,
	prompt, aspectRatio, originalImageB64 string,
	enableNSFW bool,
) (<-chan xai.ImageEvent, error) {
	*c.capturedRatio = aspectRatio
	return c.inner.Edit(ctx, prompt, aspectRatio, originalImageB64, enableNSFW)
}

func TestImageRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *ImageRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     &ImageRequest{Prompt: "test", Size: "1024x1024", N: 1},
			wantErr: false,
		},
		{
			name:    "empty prompt",
			req:     &ImageRequest{Prompt: "", Size: "1024x1024", N: 1},
			wantErr: true,
		},
		{
			name:    "n too large",
			req:     &ImageRequest{Prompt: "test", Size: "1024x1024", N: 11},
			wantErr: true,
		},
		{
			name:    "n zero defaults to 1",
			req:     &ImageRequest{Prompt: "test", Size: "1024x1024", N: 0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestImageEditRequest_Validate(t *testing.T) {
	// Create a minimal valid PNG (1x1 pixel)
	validImage := createTestPNG()

	tests := []struct {
		name    string
		req     *ImageEditRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &ImageEditRequest{
				Prompt:         "add a hat",
				OriginalImages: [][]byte{validImage},
				Size:           "1024x1024",
			},
			wantErr: false,
		},
		{
			name: "empty prompt",
			req: &ImageEditRequest{
				Prompt:         "",
				OriginalImages: [][]byte{validImage},
			},
			wantErr: true,
		},
		{
			name: "missing image",
			req: &ImageEditRequest{
				Prompt:         "add a hat",
				OriginalImages: nil,
			},
			wantErr: true,
		},
		{
			name: "size defaults to 1024x1024",
			req: &ImageEditRequest{
				Prompt:         "test",
				OriginalImages: [][]byte{validImage},
				Size:           "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.req.Size == "" {
					// Validate should set default
					assert.Equal(t, "1024x1024", tt.req.Size)
				}
			}
		})
	}
}

func TestImageFlow_Edit_Success(t *testing.T) {
	editBytes := []byte("edited-image")
	editor := &mockImageEditClient{
		postID:       "post-123",
		imageURLs:    []string{"https://example.com/generated.png"},
		downloadBody: editBytes,
	}

	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &ImageEditRequest{
		Model:          "grok-imagine-image-edit",
		UpstreamModel:  "imagine-image-edit",
		UpstreamMode:   "MODEL_MODE_FAST",
		Prompt:         "add sunglasses",
		OriginalImages: [][]byte{createTestPNG()},
		Size:           "1024x1024",
	}

	resp, err := flow.Edit(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, base64.StdEncoding.EncodeToString(editBytes), resp.Data[0].B64JSON)
	require.Len(t, editor.chatRequests, 1)
	assert.Equal(t, true, editor.chatRequests[0].ToolOverrides["imageGen"])
	assert.Equal(t, "imagine-image-edit", editor.chatRequests[0].UpstreamModel)
	assert.Equal(t, "MODEL_MODE_FAST", editor.chatRequests[0].UpstreamMode)
	modelMap, ok := editor.chatRequests[0].ModelConfig["modelMap"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "imagine", modelMap["imageEditModel"])
}

func TestImageFlow_Edit_Blocked(t *testing.T) {
	editor := &mockImageEditClient{
		chatErr: errors.New("content blocked by safety filter"),
	}

	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &ImageEditRequest{
		Model:          "grok-imagine-image-edit",
		UpstreamModel:  "imagine-image-edit",
		UpstreamMode:   "MODEL_MODE_FAST",
		Prompt:         "inappropriate edit",
		OriginalImages: [][]byte{createTestPNG()},
		Size:           "1024x1024",
	}

	resp, err := flow.Edit(ctx, req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "blocked")
}

func TestImageFlow_Edit_InvalidRequest(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &mockImageEditClient{})
	ctx := context.Background()

	// Missing prompt
	req := &ImageEditRequest{
		Prompt:         "",
		OriginalImages: [][]byte{createTestPNG()},
	}

	resp, err := flow.Edit(ctx, req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestImageFlow_Edit_MissingUpstreamMapping(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &mockImageEditClient{})

	resp, err := flow.Edit(context.Background(), &ImageEditRequest{
		Model:          "grok-imagine-image-edit",
		Prompt:         "add a hat",
		OriginalImages: [][]byte{createTestPNG()},
	})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "upstream model is required")
}

// createTestPNG creates a minimal valid PNG image for testing.
func createTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// imageUsageRecorder captures usage logs for testing.
type imageUsageRecorder struct {
	mu      sync.Mutex
	records []*store.UsageLog
}

func (m *imageUsageRecorder) Record(ctx context.Context, log *store.UsageLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, log)
	return nil
}

func TestImageFlow_RecordUsage_TokenID(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventFinal, ImageData: "base64-final"},
		},
	}

	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 42, Token: "tok-42", Pool: "basic"}},
	}
	f := NewImageFlow(tokenSvc, func(token string) ImagineGenerator { return mock })
	f.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	f.SetUsageRecorder(recorder)

	ctx := context.Background()
	req := &ImageRequest{Model: "grok-imagine-image", Prompt: "test", Size: "1024x1024", N: 1}

	_, err := f.Generate(ctx, req)
	require.NoError(t, err)

	// Wait briefly for async recording goroutine
	time.Sleep(50 * time.Millisecond)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, uint(42), recorder.records[0].TokenID)
}

func TestImageFlow_RecordUsage_DefaultTokenID(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventFinal, ImageData: "base64-final"},
		},
	}

	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok-1", Pool: "basic"}},
	}
	f := NewImageFlow(tokenSvc, func(token string) ImagineGenerator { return mock })
	f.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	f.SetUsageRecorder(recorder)

	ctx := context.Background()
	req := &ImageRequest{Model: "grok-imagine-image", Prompt: "test", Size: "1024x1024", N: 1}

	_, err := f.Generate(ctx, req)
	require.NoError(t, err)

	// Wait briefly for async recording goroutine
	time.Sleep(50 * time.Millisecond)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, uint(1), recorder.records[0].TokenID)
}

func TestImageFlow_Generate_NoConsumeOnFailure(t *testing.T) {
	mock := &mockImagineClient{
		err: errors.New("generation failed"),
	}

	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok-1", Pool: "basic"}},
	}
	f := NewImageFlow(tokenSvc, func(token string) ImagineGenerator { return mock })
	f.SetModelResolver(testModelResolver())

	_, err := f.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
		N:      1,
	})
	require.Error(t, err)

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	assert.Empty(t, tokenSvc.consumeCalls, "Consume should not be called on failure")
}

func TestImageFlow_Generate_ConsumeOnSuccess(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{
			{Type: xai.ImageEventFinal, ImageData: "img", RequestID: "req-1"},
		},
	}

	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 42, Token: "tok-42", Pool: "basic"}},
	}
	f := NewImageFlow(tokenSvc, func(token string) ImagineGenerator { return mock })
	f.SetModelResolver(testModelResolver())

	_, err := f.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "test",
		Size:   "1024x1024",
		N:      2,
	})
	require.NoError(t, err)

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	assert.Len(t, tokenSvc.consumeCalls, 2, "Consume should be called once per successful image")
	for _, id := range tokenSvc.consumeCalls {
		assert.Equal(t, uint(42), id)
	}
}

func TestImageFlow_BlockedRecovery_ConsumeOnlyOnSuccess(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{
			{ID: 1, Token: "tok-1", Pool: "basic"},
			{ID: 2, Token: "tok-2", Pool: "basic"},
		},
	}
	flow := NewImageFlow(tokenSvc, func(token string) ImagineGenerator {
		return imagineGeneratorFunc(func(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
			events := []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "recovered", RequestID: "req-2"}}
			if token == "tok-1" {
				events = []xai.ImageEvent{{Type: xai.ImageEventBlocked, RequestID: "req-1"}}
			}
			ch := make(chan xai.ImageEvent, len(events))
			for _, e := range events {
				ch <- e
			}
			close(ch)
			return ch, nil
		})
	})
	flow.SetModelResolver(testModelResolver())
	enabled := true
	flow.SetImageConfig(&config.ImageConfig{
		BlockedParallelAttempts: 1,
		BlockedParallelEnabled:  &enabled,
	})

	resp, err := flow.Generate(context.Background(), &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "recover me",
		Size:   "1024x1024",
		N:      1,
	})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)

	// Wait for goroutines to finish
	time.Sleep(100 * time.Millisecond)

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	// Only the successful recovery token (tok-2) should have Consume called
	assert.Len(t, tokenSvc.consumeCalls, 1, "Consume should only be called for the successful recovery token")
	assert.Equal(t, uint(2), tokenSvc.consumeCalls[0])
}

func TestImageFlow_Generate_Timeout(t *testing.T) {
	originalTimeout := imageGenerationTimeout
	imageGenerationTimeout = 20 * time.Millisecond
	defer func() {
		imageGenerationTimeout = originalTimeout
	}()

	flow := newTestImageFlow(imagineGeneratorFunc(func(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}))

	req := &ImageRequest{
		Model:  "grok-imagine-image",
		Prompt: "slow image",
		Size:   "1024x1024",
		N:      1,
	}

	start := time.Now()
	_, err := flow.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Fatalf("timeout returned too early, likely not using internal timeout")
	}
}

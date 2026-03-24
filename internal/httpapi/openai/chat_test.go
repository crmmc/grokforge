package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleChat_MissingMessages(t *testing.T) {
	s := httpapi.NewServer(&httpapi.ServerConfig{ChatProvider: &Handler{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"grok-3"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
	var resp httpapi.APIError
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_request_error", resp.Error.Type)
	assert.Contains(t, resp.Error.Message, "messages")
}

func TestHandleChat_EmptyMessages(t *testing.T) {
	s := httpapi.NewServer(&httpapi.ServerConfig{ChatProvider: &Handler{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"grok-3","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
	var resp httpapi.APIError
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_request_error", resp.Error.Type)
}

func TestHandleChat_InvalidModel(t *testing.T) {
	cfg := config.DefaultConfig()
	s := httpapi.NewServer(&httpapi.ServerConfig{Config: cfg, ChatProvider: &Handler{Cfg: cfg}})
	body := `{"model":"invalid-model","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)
	var resp httpapi.APIError
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not_found", resp.Error.Type)
	assert.Equal(t, "model_not_found", resp.Error.Code)
}

func TestHandleChat_InvalidJSON(t *testing.T) {
	s := httpapi.NewServer(&httpapi.ServerConfig{ChatProvider: &Handler{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestToFlowRequest_PropagatesSamplingParams(t *testing.T) {
	temp := 0.0
	topP := 0.25
	maxTokens := 128
	req := &ChatRequest{
		Model: "grok-3",
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
	}

	flowReq := toFlowRequest(req)
	require.NotNil(t, flowReq.Temperature)
	require.NotNil(t, flowReq.TopP)
	require.NotNil(t, flowReq.MaxTokens)
	assert.Equal(t, temp, *flowReq.Temperature)
	assert.Equal(t, topP, *flowReq.TopP)
	assert.Equal(t, maxTokens, *flowReq.MaxTokens)
}

func TestNormalizeChatRequestDefaults(t *testing.T) {
	req := &ChatRequest{
		Model: "grok-3",
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
	}
	normalized, err := normalizeChatRequest(req, nil)
	require.Nil(t, err)
	require.NotNil(t, normalized.Temperature)
	require.NotNil(t, normalized.TopP)
	assert.Equal(t, defaultChatTemperature, *normalized.Temperature)
	assert.Equal(t, defaultChatTopP, *normalized.TopP)
	assert.False(t, isStreamEnabled(normalized.Stream))
	require.NotNil(t, normalized.ParallelToolCalls)
	assert.True(t, *normalized.ParallelToolCalls)
}

func TestNormalizeChatRequest_InvalidToolChoice(t *testing.T) {
	req := &ChatRequest{
		Model: "grok-3",
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
		ToolChoice: "invalid",
	}

	_, err := normalizeChatRequest(req, nil)
	require.NotNil(t, err)
	assert.Equal(t, "invalid_tool_choice", err.code)
}

func TestNormalizeChatRequest_InvalidToolObject(t *testing.T) {
	req := &ChatRequest{
		Model: "grok-3",
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
		ToolChoice: map[string]any{
			"type": "not_function",
		},
	}

	_, err := normalizeChatRequest(req, nil)
	require.NotNil(t, err)
	assert.Equal(t, "invalid_tool_choice", err.code)
}

func TestNormalizeChatRequest_InvalidToolDefinition(t *testing.T) {
	req := &ChatRequest{
		Model: "grok-3",
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
		Tools: []flow.Tool{
			{Type: "invalid_type", Function: flow.Function{Name: "search"}},
		},
	}

	_, err := normalizeChatRequest(req, nil)
	require.NotNil(t, err)
	assert.Equal(t, "invalid_tool_type", err.code)
}

// --- Image mock ---

type mockImagineClient struct {
	events []xai.ImageEvent
}

func (m *mockImagineClient) Generate(_ context.Context, _, _ string, _ bool) (<-chan xai.ImageEvent, error) {
	ch := make(chan xai.ImageEvent, len(m.events))
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func newTestImageFlow(mock *mockImagineClient) *flow.ImageFlow {
	tokenSvc := &chatMockTokenSvc{}
	return flow.NewImageFlow(tokenSvc, func(token string) flow.ImagineGenerator { return mock })
}

// chatMockTokenSvc is a minimal TokenServicer for httpapi chat tests.
type chatMockTokenSvc struct{}

func (m *chatMockTokenSvc) Pick(pool string, _ tkn.QuotaCategory) (*store.Token, error) {
	return &store.Token{ID: 1, Token: "tok-test", Pool: pool}, nil
}
func (m *chatMockTokenSvc) PickExcluding(pool string, _ tkn.QuotaCategory, _ map[uint]struct{}) (*store.Token, error) {
	return m.Pick(pool, tkn.CategoryChat)
}
func (m *chatMockTokenSvc) Consume(tokenID uint, _ tkn.QuotaCategory, _ int) (int, error) {
	return 99, nil
}
func (m *chatMockTokenSvc) ReportSuccess(id uint)                  {}
func (m *chatMockTokenSvc) ReportRateLimit(id uint, reason string) {}
func (m *chatMockTokenSvc) ReportError(id uint, reason string)     {}
func (m *chatMockTokenSvc) MarkExpired(id uint, reason string)     {}

type chatMockAPIKeyStore struct{}

func (m *chatMockAPIKeyStore) List(context.Context, int, int, string) ([]*store.APIKey, int64, error) {
	return nil, 0, nil
}

func (m *chatMockAPIKeyStore) GetByID(context.Context, uint) (*store.APIKey, error) {
	return nil, store.ErrNotFound
}

func (m *chatMockAPIKeyStore) GetByKey(_ context.Context, key string) (*store.APIKey, error) {
	if key != "test-api-key" {
		return nil, store.ErrNotFound
	}
	return &store.APIKey{ID: 42, Key: key, Name: "test", Status: "active"}, nil
}

func (m *chatMockAPIKeyStore) Create(context.Context, *store.APIKey) error { return nil }
func (m *chatMockAPIKeyStore) Update(context.Context, *store.APIKey) error { return nil }
func (m *chatMockAPIKeyStore) Delete(context.Context, uint) error          { return nil }
func (m *chatMockAPIKeyStore) Regenerate(context.Context, uint) (string, error) {
	return "", nil
}

func (m *chatMockAPIKeyStore) CountByStatus(context.Context) (int, int, int, int, int, error) {
	return 0, 0, 0, 0, 0, nil
}

func (m *chatMockAPIKeyStore) IncrementUsage(context.Context, uint) error { return nil }
func (m *chatMockAPIKeyStore) ResetDailyUsage(context.Context) error      { return nil }

// --- Video mock ---

type chatVideoClientMock struct{}

func (m *chatVideoClientMock) Chat(_ context.Context, _ *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	payload := []byte(`{"result":{"response":{"post":{"id":"post-1"},"streamingVideoGenerationResponse":{"videoUrl":"https://example.com/video.mp4"}}}}`)
	ch := make(chan xai.StreamEvent, 1)
	ch <- xai.StreamEvent{Data: payload}
	close(ch)
	return ch, nil
}
func (m *chatVideoClientMock) CreateImagePost(_ context.Context, _ string) (string, error) {
	return "post-1", nil
}
func (m *chatVideoClientMock) CreateVideoPost(_ context.Context, _ string) (string, error) {
	return "post-1", nil
}
func (m *chatVideoClientMock) PollUpscale(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "https://example.com/video.mp4", nil
}
func (m *chatVideoClientMock) DownloadURL(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (m *chatVideoClientMock) DownloadTo(_ context.Context, _ string, w io.Writer) error {
	_, err := io.WriteString(w, "video-data")
	return err
}
func (m *chatVideoClientMock) UploadFile(_ context.Context, _, _, _ string) (string, string, error) {
	return "file-1", "generated/ref", nil
}

// --- Usage recorder ---

type chatUsageRecorder struct {
	mu      sync.Mutex
	records []*store.UsageLog
}

func (r *chatUsageRecorder) Record(ctx context.Context, log *store.UsageLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, log)
	return nil
}

// --- Tests ---

func TestHandleChat_ImageModelRoute(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "abc123"}},
	}
	imageFlow := newTestImageFlow(mock)
	s := httpapi.NewServer(&httpapi.ServerConfig{ChatProvider: &Handler{ImageFlow: imageFlow}})

	body := `{"model":"grok-imagine-1.0","messages":[{"role":"user","content":"draw a cat"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "![image](")
}

func TestHandleChat_ImageModelRoute_BridgesAPIKeyID(t *testing.T) {
	mock := &mockImagineClient{
		events: []xai.ImageEvent{{Type: xai.ImageEventFinal, ImageData: "abc123"}},
	}
	imageFlow := newTestImageFlow(mock)
	recorder := &chatUsageRecorder{}
	imageFlow.SetUsageRecorder(recorder)
	s := httpapi.NewServer(&httpapi.ServerConfig{
		ChatProvider: &Handler{ImageFlow: imageFlow},
		APIKeyStore:  &chatMockAPIKeyStore{},
	})

	body := `{"model":"grok-imagine-1.0","messages":[{"role":"user","content":"draw a cat"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, uint(42), recorder.records[0].APIKeyID)
}

func TestHandleChat_VideoModelRoute(t *testing.T) {
	videoFlow := flow.NewVideoFlow(
		&chatMockTokenSvc{},
		func(token string) flow.VideoClient { return &chatVideoClientMock{} },
		&flow.VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1},
	)
	s := httpapi.NewServer(&httpapi.ServerConfig{ChatProvider: &Handler{VideoFlow: videoFlow}})

	body := `{"model":"grok-imagine-1.0-video","messages":[{"role":"user","content":"make a short clip"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "[video](")
}

package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

// testModelResolver returns a mock ModelResolver for flow tests.
// Maps models to pool floors: basic models → "basic", super models → "super".
type testResolver struct{}

func (r *testResolver) ResolvePoolFloor(requestName string) (floor string, ok bool) {
	basicModels := map[string]bool{
		"grok-2": true, "grok-2-mini": true, "grok-2-imageGen": true, "grok-2-vision": true,
		"grok-imagine-image-lite": true,
	}
	superModels := map[string]bool{
		"grok-3": true, "grok-3-mini": true, "grok-3-reasoning": true, "grok-3-deepsearch": true, "grok-4": true,
		"grok-imagine-image": true, "grok-imagine-image-edit": true, "grok-imagine-video": true,
	}
	if basicModels[requestName] {
		return "basic", true
	}
	if superModels[requestName] {
		return "super", true
	}
	return "", false
}

func (r *testResolver) ResolveMode(requestName string) (mode string, ok bool) {
	switch requestName {
	case "grok-2", "grok-2-mini", "grok-2-imageGen", "grok-2-vision", "grok-imagine-image-lite", "grok-imagine-video":
		return "auto", true
	case "grok-imagine-image", "grok-imagine-image-edit":
		return "", false
	case "grok-3", "grok-3-mini", "grok-3-reasoning", "grok-3-deepsearch", "grok-4":
		return "auto", true
	default:
		return "", false
	}
}

func testModelResolver() tkn.ModelResolver {
	return &testResolver{}
}

func testModeResolver() ModeResolver {
	return &testResolver{}
}

// mockTokenService implements TokenServicer for testing.
type mockTokenService struct {
	mu             sync.Mutex
	tokens         []*store.Token
	pickIndex      int
	pickErr        error
	successCalls   []uint
	firstUseCalls  []uint
	firstUseModes  []string
	rateLimitCalls []uint
	errorCalls     []uint
	expiredCalls   []uint
}

func (m *mockTokenService) Pick(pool string, mode string) (*store.Token, error) {
	return m.PickExcluding(pool, mode, nil)
}

func (m *mockTokenService) PickExcluding(pool string, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pickErr != nil {
		return nil, m.pickErr
	}
	for m.pickIndex < len(m.tokens) {
		t := m.tokens[m.pickIndex]
		m.pickIndex++
		if _, skipped := exclude[t.ID]; skipped {
			continue
		}
		return t, nil
	}
	return nil, errors.New("no tokens available")
}

func (m *mockTokenService) PickAnyExcluding(pool string, exclude map[uint]struct{}) (*store.Token, error) {
	return m.PickExcluding(pool, "", exclude)
}

func (m *mockTokenService) RecordFirstUse(id uint, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.firstUseCalls = append(m.firstUseCalls, id)
	m.firstUseModes = append(m.firstUseModes, mode)
}

func (m *mockTokenService) RefundQuota(id uint, mode string) {}

func (m *mockTokenService) ReportSuccess(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.successCalls = append(m.successCalls, id)
}

func (m *mockTokenService) ReportRateLimit(id uint, mode string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimitCalls = append(m.rateLimitCalls, id)
}

func (m *mockTokenService) ReportError(id uint, mode string, recoverable bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorCalls = append(m.errorCalls, id)
}

func (m *mockTokenService) MarkExpired(id uint, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expiredCalls = append(m.expiredCalls, id)
}

// mockXAIClient implements xai.Client for testing.
type mockXAIClient struct {
	mu        sync.Mutex
	events    []xai.StreamEvent
	chatErr   error
	callCount int
	lastReq   *xai.ChatRequest
}

func (m *mockXAIClient) Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	m.mu.Lock()
	m.callCount++
	m.lastReq = req
	events := m.events
	chatErr := m.chatErr
	m.mu.Unlock()

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

func (m *mockXAIClient) ResetSession() error { return nil }
func (m *mockXAIClient) Close() error        { return nil }

// Video methods (not used in chat tests, but required by interface)
func (m *mockXAIClient) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	return "", nil
}
func (m *mockXAIClient) CreateVideoPost(ctx context.Context, prompt string) (string, error) {
	return "", nil
}
func (m *mockXAIClient) PollUpscale(ctx context.Context, videoID string, interval time.Duration) (string, error) {
	return "", nil
}
func (m *mockXAIClient) DownloadURL(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}
func (m *mockXAIClient) DownloadTo(ctx context.Context, url string, w io.Writer) error {
	_, err := io.WriteString(w, "mock-download")
	return err
}
func (m *mockXAIClient) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	return "file-1", "https://assets.grok.com/file-1", nil
}

func TestMapReasoningEffort(t *testing.T) {
	tests := []struct {
		input       string
		wantThink   string
		wantEnabled bool
	}{
		{"", "", false},
		{"none", "", false},
		{"low", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"unknown", "medium", true}, // default to medium
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			think, enabled := MapReasoningEffort(tt.input)
			if think != tt.wantThink || enabled != tt.wantEnabled {
				t.Errorf("MapReasoningEffort(%q) = (%q, %v), want (%q, %v)",
					tt.input, think, enabled, tt.wantThink, tt.wantEnabled)
			}
		})
	}
}

func TestChatFlow_Success(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	// Simulate successful response with content
	respData := `{"result":{"response":{"token":"Hello world","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ctx := context.Background()
	ch, err := flow.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Check success was reported
	if len(tokenSvc.successCalls) != 1 || tokenSvc.successCalls[0] != 1 {
		t.Errorf("expected success reported for token 1, got %v", tokenSvc.successCalls)
	}
}

func TestChatFlow_RetryOnRateLimit(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{
			{ID: 1, Token: "tok1", Pool: "basic"},
			{ID: 2, Token: "tok2", Pool: "basic"},
		},
	}

	callCount := 0
	respData := `{"result":{"response":{"token":"Success","isThinking":false}}}`

	clientFactory := func(token string) xai.Client {
		return &mockXAIClient{
			events: []xai.StreamEvent{{Data: json.RawMessage(respData)}},
			chatErr: func() error {
				callCount++
				if callCount <= 2 {
					return xai.ErrRateLimited
				}
				return nil
			}(),
		}
	}

	cfg := &ChatFlowConfig{RetryConfig: &RetryConfig{
		MaxTokens:       6,
		PerTokenRetries: 2,
		BaseDelay:       time.Millisecond, // fast for tests
		MaxDelay:        10 * time.Millisecond,
		JitterFactor:    0,
	}, ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, clientFactory, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ctx := context.Background()
	ch, err := flow.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	for range ch {
		// drain channel
	}

	// Should have rate limit reports
	if len(tokenSvc.rateLimitCalls) < 2 {
		t.Errorf("expected at least 2 rate limit reports, got %v", tokenSvc.rateLimitCalls)
	}
}

func TestChatFlow_TokenRotation(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{
			{ID: 1, Token: "tok1", Pool: "basic"},
			{ID: 2, Token: "tok2", Pool: "basic"},
			{ID: 3, Token: "tok3", Pool: "basic"},
		},
	}

	// Track which tokens were used
	var usedTokens []string
	var mu sync.Mutex

	clientFactory := func(token string) xai.Client {
		mu.Lock()
		usedTokens = append(usedTokens, token)
		mu.Unlock()

		// First two tokens fail, third succeeds
		if token == "tok1" || token == "tok2" {
			return &mockXAIClient{chatErr: xai.ErrRateLimited}
		}
		respData := `{"result":{"response":{"token":"Success","isThinking":false}}}`
		return &mockXAIClient{
			events: []xai.StreamEvent{{Data: json.RawMessage(respData)}},
		}
	}

	cfg := &ChatFlowConfig{RetryConfig: &RetryConfig{
		MaxTokens:       6,
		PerTokenRetries: 2,
		BaseDelay:       time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		JitterFactor:    0,
	}, ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, clientFactory, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ctx := context.Background()
	ch, _ := flow.Complete(ctx, req)
	for range ch {
	}

	// Should have rotated through tokens
	if len(usedTokens) < 3 {
		t.Errorf("expected at least 3 token uses, got %v", usedTokens)
	}
}

func TestChatFlow_NonRetryableError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	badReqErr := errors.New("400 Bad Request: invalid model")
	client := &mockXAIClient{chatErr: badReqErr}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ctx := context.Background()
	ch, _ := flow.Complete(ctx, req)

	var lastEvent StreamEvent
	for e := range ch {
		lastEvent = e
	}

	// Should get error without retrying (400 is non-recoverable)
	if lastEvent.Error == nil || !strings.Contains(lastEvent.Error.Error(), "400") {
		t.Errorf("expected 400 error, got %v", lastEvent.Error)
	}
}

func TestChatFlow_HandleError_CFChallenge_NoTokenPenalty(t *testing.T) {
	tokenSvc := &mockTokenService{}
	flow := &ChatFlow{tokenSvc: tokenSvc}
	cfg := DefaultRetryConfig()

	flow.handleError(1, "auto", xai.ErrCFChallenge, cfg)

	if len(tokenSvc.rateLimitCalls) != 0 {
		t.Errorf("CF challenge should not rate limit, got %v", tokenSvc.rateLimitCalls)
	}
	if len(tokenSvc.expiredCalls) != 0 {
		t.Errorf("CF challenge should not expire token, got %v", tokenSvc.expiredCalls)
	}
	if len(tokenSvc.errorCalls) != 0 {
		t.Errorf("CF challenge should not report error, got %v", tokenSvc.errorCalls)
	}
}

func TestChatFlow_HandleError_Forbidden_NoTokenPenalty(t *testing.T) {
	tokenSvc := &mockTokenService{}
	flow := &ChatFlow{tokenSvc: tokenSvc}
	cfg := DefaultRetryConfig()

	flow.handleError(1, "auto", xai.ErrForbidden, cfg)

	if len(tokenSvc.expiredCalls) != 0 {
		t.Errorf("403 should not expire token, got %v", tokenSvc.expiredCalls)
	}
	if len(tokenSvc.rateLimitCalls) != 0 {
		t.Errorf("403 should not rate limit, got %v", tokenSvc.rateLimitCalls)
	}
	if len(tokenSvc.errorCalls) != 0 {
		t.Errorf("403 should not report error, got %v", tokenSvc.errorCalls)
	}
}

func TestChatFlow_HandleError_TransportSkipsPenalty(t *testing.T) {
	tokenSvc := &mockTokenService{}
	flow := &ChatFlow{tokenSvc: tokenSvc}
	cfg := DefaultRetryConfig()

	flow.handleError(1, "auto", xai.ErrNetwork, cfg)
	flow.handleError(1, "auto", errors.New("503 Service Unavailable"), cfg)

	if len(tokenSvc.rateLimitCalls) != 0 {
		t.Errorf("expected no rate limit calls, got %v", tokenSvc.rateLimitCalls)
	}
	// Transport errors now report as recoverable errors (quota refunded)
	if len(tokenSvc.errorCalls) != 2 {
		t.Errorf("expected 2 error calls (transport), got %v", tokenSvc.errorCalls)
	}
}

// TestGetPoolForModel is tested in token/picker_test.go.
// Removed duplicate test after consolidating GetPoolForModel to token package.

// Integration tests for combined tool calling + multimodal scenarios

func TestChatFlow_WithTools(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	// Response with tool call in XML format
	respData := `{"result":{"response":{"token":"I'll check the weather.\n<tool_call>\n{\"name\":\"get_weather\",\"arguments\":\"{\\\"location\\\":\\\"Tokyo\\\"}\"}\n</tool_call>","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "What's the weather in Tokyo?"}},
		Model:    "grok-2",
		Tools: []Tool{
			{
				Type: "function",
				Function: Function{
					Name:        "get_weather",
					Description: "Get weather for a location",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
						},
						"required": []string{"location"},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	ctx := context.Background()
	ch, err := flow.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Check tool calls were parsed (may be on any event, not necessarily the last)
	var foundToolCalls []ToolCall
	for _, e := range events {
		if len(e.ToolCalls) > 0 {
			foundToolCalls = e.ToolCalls
		}
	}
	if len(foundToolCalls) != 1 {
		t.Errorf("expected 1 tool call across events, got %d", len(foundToolCalls))
	}
	if len(foundToolCalls) > 0 && foundToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %q", foundToolCalls[0].Function.Name)
	}
}

func TestChatFlow_WithMultimodalContent(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	respData := `{"result":{"response":{"token":"I see a cat in the image.","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	// Multimodal content with text and image
	req := &ChatRequest{
		Messages: []Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "What's in this image?"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
				},
			},
		},
		Model: "grok-2-vision",
	}

	ctx := context.Background()
	ch, err := flow.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		if e.Error != nil {
			t.Fatalf("unexpected error: %v", e.Error)
		}
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Check content was processed
	found := false
	for _, e := range events {
		if e.Content != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected content in response")
	}
}

func TestChatFlow_ToolsWithMultimodal(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	// Response with tool call after analyzing image
	respData := `{"result":{"response":{"token":"Based on the image, I'll search for this product.\n<tool_call>\n{\"name\":\"search_product\",\"arguments\":\"{\\\"query\\\":\\\"red sneakers\\\"}\"}\n</tool_call>","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	req := &ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a shopping assistant."},
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Find this product for me"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
				},
			},
		},
		Model: "grok-2-vision",
		Tools: []Tool{
			{
				Type: "function",
				Function: Function{
					Name:        "search_product",
					Description: "Search for a product",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	ctx := context.Background()
	ch, err := flow.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		if e.Error != nil {
			t.Fatalf("unexpected error: %v", e.Error)
		}
		events = append(events, e)
	}

	// Verify both multimodal processing and tool call parsing worked
	var foundToolCalls []ToolCall
	for _, e := range events {
		if len(e.ToolCalls) > 0 {
			foundToolCalls = e.ToolCalls
		}
	}
	if len(foundToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(foundToolCalls))
	}
	if len(foundToolCalls) > 0 && foundToolCalls[0].Function.Name != "search_product" {
		t.Errorf("expected tool name 'search_product', got %q", foundToolCalls[0].Function.Name)
	}
}

// mockUsageRecorder implements UsageRecorder for testing.
type mockUsageRecorder struct {
	mu      sync.Mutex
	records []*store.UsageLog
}

func (m *mockUsageRecorder) Record(ctx context.Context, log *store.UsageLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, log)
	return nil
}

func TestChatFlow_HotReload(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{
			{ID: 1, Token: "tok1", Pool: "basic"},
			{ID: 2, Token: "tok2", Pool: "basic"},
		},
	}

	callCount := 0
	// Client always fails with retryable error
	clientFactory := func(token string) xai.Client {
		callCount++
		return &mockXAIClient{chatErr: xai.ErrRateLimited}
	}

	// Start with MaxTokens=1, PerTokenRetries=1 (only 1 attempt total)
	currentMax := 1
	currentPerToken := 1
	cfg := &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       6, // fallback, should not be used
			PerTokenRetries: 2,
			BaseDelay:       time.Millisecond,
			MaxDelay:        10 * time.Millisecond,
			JitterFactor:    0,
		},
		RetryConfigProvider: func() *RetryConfig {
			return &RetryConfig{
				MaxTokens:       currentMax,
				PerTokenRetries: currentPerToken,
				BaseDelay:       time.Millisecond,
				MaxDelay:        10 * time.Millisecond,
				JitterFactor:    0,
			}
		},
		ModelResolver: testModelResolver(),
	}
	f := NewChatFlow(tokenSvc, clientFactory, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ctx := context.Background()
	ch, _ := f.Complete(ctx, req)
	for range ch {
	}

	// With MaxTokens=1 from provider, should have only 1 attempt
	if callCount != 1 {
		t.Errorf("expected 1 attempt from hot-reload provider (MaxTokens=1), got %d", callCount)
	}
}

func TestChatFlow_RecordUsageAPIKeyID(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	respData := `{"result":{"response":{"token":"Hello","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	f := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	recorder := &mockUsageRecorder{}
	f.SetUsageRecorder(recorder)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	// Set FlowAPIKeyIDKey in context
	ctx := context.WithValue(context.Background(), FlowAPIKeyIDKey, uint(42))
	ch, err := f.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for range ch {
	}

	// Wait briefly for async recording
	time.Sleep(50 * time.Millisecond)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(recorder.records))
	}
	if recorder.records[0].APIKeyID != 42 {
		t.Errorf("expected APIKeyID=42, got %d", recorder.records[0].APIKeyID)
	}
}

func TestChatFlow_ParseEvent_NoUsage(t *testing.T) {
	f := &ChatFlow{cfg: &ChatFlowConfig{RetryConfig: DefaultRetryConfig()}}
	event := xai.StreamEvent{
		Data: json.RawMessage(`{"result":{"response":{"token":"hi","isThinking":false},"usage":{"input_tokens":10,"output_tokens":2}}}`),
	}

	got := f.parseEvent(event)
	// Upstream usage is no longer extracted — always nil from parseEvent.
	// Usage is estimated at stream-end in streamEvents instead.
	if got.Usage != nil {
		t.Errorf("expected Usage to be nil, got %+v", got.Usage)
	}
	if got.Content != "hi" {
		t.Errorf("Content = %q, want %q", got.Content, "hi")
	}
}

func TestChatFlow_ParseEvent_TokenField(t *testing.T) {
	f := &ChatFlow{cfg: &ChatFlowConfig{RetryConfig: DefaultRetryConfig()}}

	// Normal content: isThinking=false → goes to Content
	event := xai.StreamEvent{
		Data: json.RawMessage(`{"result":{"response":{"token":"Hello world","isThinking":false}}}`),
	}
	got := f.parseEvent(event)
	if got.Content != "Hello world" {
		t.Errorf("Content = %q, want %q", got.Content, "Hello world")
	}
	if got.ReasoningContent != "" {
		t.Errorf("ReasoningContent = %q, want empty", got.ReasoningContent)
	}
	if got.IsThinking {
		t.Error("IsThinking = true, want false")
	}
}

func TestChatFlow_ParseEvent_ThinkingFlag(t *testing.T) {
	f := &ChatFlow{cfg: &ChatFlowConfig{RetryConfig: DefaultRetryConfig()}}

	// Thinking content: isThinking=true → goes to ReasoningContent
	event := xai.StreamEvent{
		Data: json.RawMessage(`{"result":{"response":{"token":"Let me think...","isThinking":true}}}`),
	}
	got := f.parseEvent(event)
	if got.Content != "" {
		t.Errorf("Content = %q, want empty", got.Content)
	}
	if got.ReasoningContent != "Let me think..." {
		t.Errorf("ReasoningContent = %q, want %q", got.ReasoningContent, "Let me think...")
	}
	if !got.IsThinking {
		t.Error("IsThinking = false, want true")
	}
}

func TestChatFlow_ParseEvent_ModelResponse(t *testing.T) {
	f := &ChatFlow{cfg: &ChatFlowConfig{RetryConfig: DefaultRetryConfig()}}

	event := xai.StreamEvent{
		Data: json.RawMessage(`{"result":{"response":{"token":"","isThinking":false,"modelResponse":{"message":"done","generatedImageUrls":["https://grok.com/img/abc/1.png"]}}}}`),
	}
	got := f.parseEvent(event)
	if !strings.Contains(got.Content, "![abc](https://grok.com/img/abc/1.png)") {
		t.Errorf("Content missing image markdown, got: %q", got.Content)
	}
}

func TestChatFlow_ParseEvent_CardAttachment(t *testing.T) {
	f := &ChatFlow{cfg: &ChatFlowConfig{RetryConfig: DefaultRetryConfig()}}

	cardJSON := `{"image":{"original":"https://example.com/photo.jpg","title":"A photo"}}`
	data := fmt.Sprintf(`{"result":{"response":{"token":"","isThinking":false,"cardAttachment":{"jsonData":%s}}}}`, strconv.Quote(cardJSON))
	event := xai.StreamEvent{Data: json.RawMessage(data)}
	got := f.parseEvent(event)
	if !strings.Contains(got.Content, "![A photo](https://example.com/photo.jpg)") {
		t.Errorf("Content missing card image, got: %q", got.Content)
	}
}

func TestChatFlow_ParseEvent_FilterTags(t *testing.T) {
	f := &ChatFlow{cfg: &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		FilterTags:  []string{"xaiartifact", "grok:render"},
	}}

	tests := []struct {
		name  string
		token string
	}{
		{"normal", "hello"},
		{"xaiartifact open", "<xaiartifact>code</xaiartifact>"},
		{"grok:render", "<grok:render>stuff"},
		{"clean token", "no tags here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := fmt.Sprintf(`{"result":{"response":{"token":%s,"isThinking":false}}}`, strconv.Quote(tt.token))
			got := f.parseEvent(xai.StreamEvent{Data: json.RawMessage(data)})
			if got.Content != tt.token {
				t.Errorf("Content = %q, want raw token %q", got.Content, tt.token)
			}
		})
	}
}

func TestChatFlow_ParallelToolCalls(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	// Response with multiple parallel tool calls (separate tool_call blocks)
	respData := `{"result":{"response":{"token":"I'll check both locations.\n<tool_call>\n{\"name\":\"get_weather\",\"arguments\":\"{\\\"location\\\":\\\"Tokyo\\\"}\"}\n</tool_call>\n<tool_call>\n{\"name\":\"get_weather\",\"arguments\":\"{\\\"location\\\":\\\"London\\\"}\"}\n</tool_call>","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Weather in Tokyo and London?"}},
		Model:    "grok-2",
		Tools: []Tool{
			{
				Type: "function",
				Function: Function{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
		ParallelToolCalls: true,
	}

	ctx := context.Background()
	ch, err := flow.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	var foundToolCalls []ToolCall
	for _, e := range events {
		if len(e.ToolCalls) > 0 {
			foundToolCalls = e.ToolCalls
		}
	}
	if len(foundToolCalls) != 2 {
		t.Errorf("expected 2 parallel tool calls, got %d", len(foundToolCalls))
	}
}

func TestChatFlow_MultimodalUploadsAsAttachments(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	respData := `{"result":{"response":{"token":"ok","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{{Data: json.RawMessage(respData)}},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	chatFlow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	req := &ChatRequest{
		Messages: []Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "describe this image"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
				},
			},
		},
		Model: "grok-2-vision",
	}

	ch, err := chatFlow.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for range ch {
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.lastReq == nil {
		t.Fatal("expected last request to be captured")
	}
	if len(client.lastReq.FileAttachments) == 0 {
		t.Fatal("expected uploaded file attachments for multimodal request")
	}
}

func TestChatFlow_EstimatedTrue_WhenNoUsageFromUpstream(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	// Response WITHOUT usage data — tokens will be estimated
	respData := `{"result":{"response":{"token":"Hello world response","isThinking":false}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	f := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	recorder := &mockUsageRecorder{}
	f.SetUsageRecorder(recorder)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ch, err := f.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for range ch {
	}

	time.Sleep(50 * time.Millisecond)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(recorder.records))
	}
	if !recorder.records[0].Estimated {
		t.Error("expected Estimated=true when upstream provides no usage")
	}
}

func TestChatFlow_UsageAlwaysEstimated(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}

	// Response with upstream usage data (which we now ignore)
	respData := `{"result":{"response":{"token":"Hello","isThinking":false},"usage":{"input_tokens":10,"output_tokens":5}}}`
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			{Data: json.RawMessage(respData)},
		},
	}

	cfg := &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()}
	f := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, cfg)

	recorder := &mockUsageRecorder{}
	f.SetUsageRecorder(recorder)

	req := &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	}

	ch, err := f.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for range ch {
	}

	time.Sleep(50 * time.Millisecond)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(recorder.records))
	}
	// Upstream usage is no longer parsed — always estimated
	if !recorder.records[0].Estimated {
		t.Error("expected Estimated=true since upstream usage is no longer parsed")
	}
}

package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/upstream"
)

// testModelResolver returns a mock ModelResolver for flow tests.
// Maps models to pool floors: basic models -> "basic", super models -> "super".
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

func testModelResolver() tkn.ModelResolver { return &testResolver{} }

func testModeResolver() ModeResolver { return &testResolver{} }

// mockTokenService implements TokenServicer for testing.
type mockTokenService struct {
	mu             sync.Mutex
	tokens         []*store.Token
	pickIndex      int
	pickErr        error
	successCalls   []uint
	rateLimitCalls []uint
	errorCalls     []uint
	keepErrorCalls []uint
	expiredCalls   []uint
	releaseCalls   []uint
	pickCalls      []uint
	pickPools      []string
	pickModes      []string
	inflight       map[uint]int
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
		m.addInflightLocked(t.ID)
		m.pickCalls = append(m.pickCalls, t.ID)
		m.pickPools = append(m.pickPools, pool)
		m.pickModes = append(m.pickModes, mode)
		return t, nil
	}
	return nil, errors.New("no tokens available")
}

func (m *mockTokenService) PickAnyExcluding(pool string, exclude map[uint]struct{}) (*store.Token, error) {
	return m.PickExcluding(pool, "", exclude)
}

func (m *mockTokenService) RefundQuota(id uint, mode string) {}

func (m *mockTokenService) ReportSuccess(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.successCalls = append(m.successCalls, id)
	m.releaseInflightLocked(id)
}

func (m *mockTokenService) ReportRateLimit(id uint, mode string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimitCalls = append(m.rateLimitCalls, id)
	m.releaseInflightLocked(id)
}

func (m *mockTokenService) ReportError(id uint, mode string, recoverable bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorCalls = append(m.errorCalls, id)
	m.releaseInflightLocked(id)
}

func (m *mockTokenService) ReportErrorKeepInflight(id uint, mode string, recoverable bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keepErrorCalls = append(m.keepErrorCalls, id)
}

func (m *mockTokenService) MarkExpired(id uint, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expiredCalls = append(m.expiredCalls, id)
	m.releaseInflightLocked(id)
}

func (m *mockTokenService) ReleaseToken(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseCalls = append(m.releaseCalls, id)
	m.releaseInflightLocked(id)
}

func (m *mockTokenService) addInflightLocked(id uint) {
	if m.inflight == nil {
		m.inflight = make(map[uint]int)
	}
	m.inflight[id]++
}

func (m *mockTokenService) releaseInflightLocked(id uint) {
	if m.inflight[id] > 0 {
		m.inflight[id]--
	}
	if m.inflight[id] == 0 {
		delete(m.inflight, id)
	}
}

func (m *mockTokenService) getInflight(id uint) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inflight[id]
}

type mockUpstream struct {
	mu         sync.Mutex
	name       string
	events     []upstream.StreamEvent
	eventDelay time.Duration
	chatErr    error
	chatErrs   []error
	calls      int
	tokens     []string
	requests   []*upstream.ChatRequest
}

func (m *mockUpstream) Name() string {
	if m.name == "" {
		return "grok"
	}
	return m.name
}

func (m *mockUpstream) Chat(ctx context.Context, token string, req *upstream.ChatRequest) (<-chan upstream.StreamEvent, error) {
	m.mu.Lock()
	callIndex := m.calls
	m.calls++
	m.tokens = append(m.tokens, token)
	m.requests = append(m.requests, req)
	events := append([]upstream.StreamEvent(nil), m.events...)
	chatErr := m.chatErr
	if callIndex < len(m.chatErrs) {
		chatErr = m.chatErrs[callIndex]
	}
	delay := m.eventDelay
	m.mu.Unlock()

	if chatErr != nil {
		return nil, chatErr
	}

	ch := make(chan upstream.StreamEvent, len(events))
	if delay > 0 {
		go func() {
			time.Sleep(delay)
			for _, e := range events {
				ch <- e
			}
			close(ch)
		}()
		return ch, nil
	}
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (m *mockUpstream) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockUpstream) lastRequest() *upstream.ChatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return nil
	}
	return m.requests[len(m.requests)-1]
}

func newTestChatFlow(tokenSvc TokenServicer, grokUp *mockUpstream, cfg *ChatFlowConfig) *ChatFlow {
	if grokUp == nil {
		grokUp = &mockUpstream{name: "grok", events: []upstream.StreamEvent{{Content: "ok"}}}
	}
	upstreams := map[string]upstream.Upstream{"grok": grokUp}
	return NewChatFlow(tokenSvc, upstreams, cfg)
}

func drainChat(t *testing.T, ch <-chan StreamEvent) []StreamEvent {
	t.Helper()
	var events []StreamEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func lastError(events []StreamEvent) error {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Error != nil {
			return events[i].Error
		}
	}
	return nil
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
		{"unknown", "medium", true},
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

func TestChatFlow_SuccessfulStreaming(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grokUp := &mockUpstream{name: "grok", events: []upstream.StreamEvent{{Content: "Hello world"}}}
	flow := newTestChatFlow(tokenSvc, grokUp, &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	events := drainChat(t, ch)

	var content strings.Builder
	var finish *StreamEvent
	for _, event := range events {
		if event.Error != nil {
			t.Fatalf("unexpected stream error: %v", event.Error)
		}
		content.WriteString(event.Content)
		if event.FinishReason != nil {
			e := event
			finish = &e
		}
	}
	if content.String() != "Hello world" {
		t.Fatalf("content = %q, want Hello world", content.String())
	}
	if finish == nil || finish.Usage == nil {
		t.Fatalf("finish event = %#v, want usage", finish)
	}
	if len(tokenSvc.successCalls) != 1 || tokenSvc.successCalls[0] != 1 {
		t.Fatalf("success calls = %v, want [1]", tokenSvc.successCalls)
	}
}

func TestChatFlow_ConsoleRouteDispatch(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: tkn.PoolBasic}}}
	grokUp := &mockUpstream{name: "grok"}
	consoleUp := &mockUpstream{name: "console", events: []upstream.StreamEvent{
		{Content: "Console hello"},
		{Usage: &upstream.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}},
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grokUp, "console": consoleUp}, &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		// Console routes with an explicit PoolFloor should not need model resolver lookup.
		ModelResolver: nil,
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages:                       []Message{{Role: "user", Content: "Hi"}},
		Model:                          "public-console-model",
		UpstreamName:                   "console",
		UpstreamModel:                  "grok-4.20",
		Mode:                           "console",
		PoolFloor:                      "basic",
		ConsoleSupportsReasoningEffort: true,
		ConsoleWebSearch:               true,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var content strings.Builder
	var finish *StreamEvent
	for _, event := range drainChat(t, ch) {
		if event.Error != nil {
			t.Fatalf("unexpected stream error: %v", event.Error)
		}
		content.WriteString(event.Content)
		if event.FinishReason != nil {
			e := event
			finish = &e
		}
	}

	if grokUp.callCount() != 0 {
		t.Fatalf("grok upstream called %d times, want 0", grokUp.callCount())
	}
	if consoleUp.callCount() != 1 {
		t.Fatalf("console upstream called %d times, want 1", consoleUp.callCount())
	}
	lastReq := consoleUp.lastRequest()
	if lastReq == nil || lastReq.Model != "grok-4.20" || !lastReq.WebSearch || !lastReq.SupportsReasoningEffort {
		t.Fatalf("last console request = %#v, want grok-4.20 with console flags", lastReq)
	}
	if len(tokenSvc.pickPools) != 1 || tokenSvc.pickPools[0] != tkn.PoolBasic {
		t.Fatalf("pick pools = %v, want [%s]", tokenSvc.pickPools, tkn.PoolBasic)
	}
	if len(tokenSvc.pickModes) != 1 || tokenSvc.pickModes[0] != "console" {
		t.Fatalf("pick modes = %v, want [console]", tokenSvc.pickModes)
	}
	if content.String() != "Console hello" {
		t.Fatalf("content = %q, want Console hello", content.String())
	}
	if finish == nil || finish.Usage == nil || finish.Usage.PromptTokens != 3 || finish.Usage.CompletionTokens != 2 || finish.Usage.TotalTokens != 5 {
		t.Fatalf("finish event = %#v, want usage 3/2/5", finish)
	}
}

func TestChatFlow_RateLimitedSwapsToken(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok1", Pool: "basic"},
		{ID: 2, Token: "tok2", Pool: "basic"},
	}}
	grokUp := &mockUpstream{
		name:     "grok",
		chatErrs: []error{upstream.ErrRateLimited, nil},
		events:   []upstream.StreamEvent{{Content: "Success"}},
	}
	flow := newTestChatFlow(tokenSvc, grokUp, &ChatFlowConfig{RetryConfig: &RetryConfig{
		MaxTokens:       2,
		PerTokenRetries: 2,
		BaseDelay:       time.Millisecond,
		MaxDelay:        time.Millisecond,
		JitterFactor:    0,
	}, ModelResolver: testModelResolver()})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for _, event := range drainChat(t, ch) {
		if event.Error != nil {
			t.Fatalf("unexpected stream error: %v", event.Error)
		}
	}

	if got := grokUp.callCount(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
	if len(tokenSvc.rateLimitCalls) != 1 || tokenSvc.rateLimitCalls[0] != 1 {
		t.Fatalf("rate limit calls = %v, want [1]", tokenSvc.rateLimitCalls)
	}
	if len(tokenSvc.pickCalls) != 2 || tokenSvc.pickCalls[0] != 1 || tokenSvc.pickCalls[1] != 2 {
		t.Fatalf("pick calls = %v, want [1 2]", tokenSvc.pickCalls)
	}
}

func TestChatFlow_NetworkErrorRetriesSameToken(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grokUp := &mockUpstream{
		name:     "grok",
		chatErrs: []error{upstream.ErrNetwork, nil},
		events:   []upstream.StreamEvent{{Content: "Success"}},
	}
	flow := newTestChatFlow(tokenSvc, grokUp, &ChatFlowConfig{RetryConfig: &RetryConfig{
		MaxTokens:       1,
		PerTokenRetries: 2,
		BaseDelay:       time.Millisecond,
		MaxDelay:        time.Millisecond,
		JitterFactor:    0,
	}, ModelResolver: testModelResolver()})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for _, event := range drainChat(t, ch) {
		if event.Error != nil {
			t.Fatalf("unexpected stream error: %v", event.Error)
		}
	}

	if grokUp.callCount() != 2 {
		t.Fatalf("upstream calls = %d, want 2", grokUp.callCount())
	}
	if len(tokenSvc.pickCalls) != 1 || tokenSvc.pickCalls[0] != 1 {
		t.Fatalf("pick calls = %v, want [1]", tokenSvc.pickCalls)
	}
	if len(tokenSvc.keepErrorCalls) != 1 || tokenSvc.keepErrorCalls[0] != 1 {
		t.Fatalf("keep-inflight calls = %v, want [1]", tokenSvc.keepErrorCalls)
	}
	if got := tokenSvc.getInflight(1); got != 0 {
		t.Fatalf("expected inflight released after success, got %d", got)
	}
}

func TestChatFlow_InvalidTokenDoesNotRetry(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok1", Pool: "basic"},
		{ID: 2, Token: "tok2", Pool: "basic"},
	}}
	grokUp := &mockUpstream{name: "grok", chatErrs: []error{upstream.ErrInvalidToken, nil}}
	flow := newTestChatFlow(tokenSvc, grokUp, &ChatFlowConfig{RetryConfig: &RetryConfig{
		MaxTokens:       2,
		PerTokenRetries: 2,
		BaseDelay:       time.Millisecond,
		MaxDelay:        time.Millisecond,
		JitterFactor:    0,
	}, ModelResolver: testModelResolver()})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	events := drainChat(t, ch)

	if !errors.Is(lastError(events), upstream.ErrInvalidToken) {
		t.Fatalf("last error = %v, want ErrInvalidToken", lastError(events))
	}
	if grokUp.callCount() != 1 {
		t.Fatalf("upstream calls = %d, want 1", grokUp.callCount())
	}
	if len(tokenSvc.expiredCalls) != 1 || tokenSvc.expiredCalls[0] != 1 {
		t.Fatalf("expired calls = %v, want [1]", tokenSvc.expiredCalls)
	}
}

func TestChatFlow_RetryBudgetExceededReleasesSameTokenInflight(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grokUp := &mockUpstream{name: "grok", chatErrs: []error{upstream.ErrNetwork}}
	flow := newTestChatFlow(tokenSvc, grokUp, &ChatFlowConfig{RetryConfig: &RetryConfig{
		MaxTokens:       1,
		PerTokenRetries: 2,
		BaseDelay:       50 * time.Millisecond,
		MaxDelay:        50 * time.Millisecond,
		JitterFactor:    0,
		RetryBudget:     time.Millisecond,
	}, ModelResolver: testModelResolver()})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	events := drainChat(t, ch)

	if !errors.Is(lastError(events), ErrRetryBudgetExceeded) {
		t.Fatalf("last error = %v, want retry budget exceeded", lastError(events))
	}
	if got := tokenSvc.getInflight(1); got != 0 {
		t.Fatalf("expected inflight released after retry budget exit, got %d", got)
	}
	if len(tokenSvc.releaseCalls) != 1 || tokenSvc.releaseCalls[0] != 1 {
		t.Fatalf("release calls = %v, want [1]", tokenSvc.releaseCalls)
	}
}

func TestChatFlow_UnknownUpstreamDoesNotPickToken(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": &mockUpstream{name: "grok"}}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "Hi"}},
		Model:        "grok-2",
		UpstreamName: "missing",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	events := drainChat(t, ch)

	if lastError(events) == nil || !strings.Contains(lastError(events).Error(), "missing") {
		t.Fatalf("last error = %v, want unknown upstream", lastError(events))
	}
	if len(tokenSvc.pickCalls) != 0 {
		t.Fatalf("token picks = %v, want none", tokenSvc.pickCalls)
	}
}

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

func TestChatFlow_RealUsageNotMarkedEstimated(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grokUp := &mockUpstream{name: "grok", events: []upstream.StreamEvent{
		{Content: "Hello"},
		{Usage: &upstream.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}},
	}}
	flow := newTestChatFlow(tokenSvc, grokUp, &ChatFlowConfig{RetryConfig: DefaultRetryConfig(), ModelResolver: testModelResolver()})
	recorder := &mockUsageRecorder{}
	flow.SetUsageRecorder(recorder)

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	for _, event := range drainChat(t, ch) {
		if event.Error != nil {
			t.Fatalf("unexpected stream error: %v", event.Error)
		}
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 1 {
		t.Fatalf("usage records = %d, want 1", len(recorder.records))
	}
	if recorder.records[0].Estimated {
		t.Fatalf("usage should not be marked estimated when upstream supplied usage")
	}
}

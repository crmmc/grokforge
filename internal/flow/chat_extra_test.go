package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUpstream is a configurable upstream fake: each Chat call is delegated
// to onChat with the 1-based call index.
type fakeUpstream struct {
	name      string
	onChat    func(call int) (<-chan upstream.StreamEvent, error)
	mu        sync.Mutex
	calls     int
	requests  []*upstream.ChatRequest
	tokens    []string
	chatCalls []context.Context
}

func (f *fakeUpstream) Name() string {
	if f.name == "" {
		return "grok"
	}
	return f.name
}

func (f *fakeUpstream) Chat(ctx context.Context, token string, req *upstream.ChatRequest) (<-chan upstream.StreamEvent, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.requests = append(f.requests, req)
	f.tokens = append(f.tokens, token)
	f.chatCalls = append(f.chatCalls, ctx)
	f.mu.Unlock()

	if f.onChat == nil {
		ch := make(chan upstream.StreamEvent)
		close(ch)
		return ch, nil
	}
	return f.onChat(call)
}

func (f *fakeUpstream) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeUpstream) lastRequest() *upstream.ChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return nil
	}
	return f.requests[len(f.requests)-1]
}

func eventsChannel(events []upstream.StreamEvent) <-chan upstream.StreamEvent {
	ch := make(chan upstream.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

func TestNewChatFlow_NilConfigDefaults(t *testing.T) {
	svc := &mockTokenService{}
	flow := NewChatFlow(svc, map[string]upstream.Upstream{"grok": &fakeUpstream{}}, nil)
	require.NotNil(t, flow.cfg)

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "unknown-model",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)
	assert.ErrorIs(t, lastError(events), tkn.ErrModelNotFound)
	assert.Empty(t, svc.pickCalls)
}

func TestChatFlow_CompleteNilConfig(t *testing.T) {
	flow := &ChatFlow{}
	ch, err := flow.Complete(context.Background(), &ChatRequest{Model: "grok-2"})
	require.NoError(t, err)
	events := drainChat(t, ch)
	assert.ErrorIs(t, lastError(events), tkn.ErrModelNotFound)
}

func TestChatFlow_CompleteNilRequest(t *testing.T) {
	svc := &mockTokenService{}
	flow := NewChatFlow(svc, map[string]upstream.Upstream{"grok": &fakeUpstream{}}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})
	ch, err := flow.Complete(context.Background(), nil)
	require.NoError(t, err)
	events := drainChat(t, ch)
	assert.ErrorIs(t, lastError(events), tkn.ErrModelNotFound)
	assert.Empty(t, svc.pickCalls)
}

func TestChatFlow_ApiKeyUsageIncCallback(t *testing.T) {
	incCh := make(chan uint, 1)
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Content: "hello"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})
	flow.SetAPIKeyUsageInc(func(_ context.Context, apiKeyID uint) { incCh <- apiKeyID })

	ctx := context.WithValue(context.Background(), FlowAPIKeyIDKey, uint(9))
	ch, err := flow.Complete(ctx, &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	drainChat(t, ch)

	select {
	case got := <-incCh:
		assert.Equal(t, uint(9), got)
	case <-time.After(2 * time.Second):
		t.Fatal("API key usage callback not invoked")
	}
}

func TestChatFlow_CFRefreshTriggerOn403(t *testing.T) {
	triggered := make(chan struct{}, 4)
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(call int) (<-chan upstream.StreamEvent, error) {
		if call == 1 {
			return nil, upstream.ErrCFChallenge
		}
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       2,
			PerTokenRetries: 2,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})
	flow.SetCFRefreshTrigger(func() { triggered <- struct{}{} })

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}

	select {
	case <-triggered:
	case <-time.After(2 * time.Second):
		t.Fatal("CF refresh trigger not invoked on 403")
	}
	assert.Equal(t, []uint{1}, tokenSvc.successCalls)
	assert.Empty(t, tokenSvc.releaseCalls)
	assert.Empty(t, tokenSvc.errorCalls)
	assert.Empty(t, tokenSvc.rateLimitCalls)
	assert.Empty(t, tokenSvc.expiredCalls)
	if got := tokenSvc.getInflight(1); got != 0 {
		t.Fatalf("expected inflight released, got %d", got)
	}
}

func TestChatFlow_ForbiddenSwapsTokenAndReleases(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok1", Pool: "basic"},
		{ID: 2, Token: "tok2", Pool: "basic"},
	}}
	grok := &fakeUpstream{onChat: func(call int) (<-chan upstream.StreamEvent, error) {
		if call == 1 {
			return nil, upstream.ErrForbidden
		}
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       2,
			PerTokenRetries: 2,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}

	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
	assert.Equal(t, []uint{2}, tokenSvc.successCalls)
	assert.Empty(t, tokenSvc.expiredCalls)
	assert.Equal(t, 2, grok.callCount())
}

func TestChatFlow_StreamErrorNonRecoverable(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{
			{Content: "hi"},
			{Error: upstream.ErrInvalidToken},
		}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	assert.ErrorIs(t, lastError(events), upstream.ErrInvalidToken)
	assert.Equal(t, []uint{1}, tokenSvc.expiredCalls)
	assert.Equal(t, 1, grok.callCount())
}

func TestChatFlow_StreamErrorRecoverableSwaps(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok1", Pool: "basic"},
		{ID: 2, Token: "tok2", Pool: "basic"},
	}}
	grok := &fakeUpstream{onChat: func(call int) (<-chan upstream.StreamEvent, error) {
		if call == 1 {
			return eventsChannel([]upstream.StreamEvent{{Error: upstream.ErrRateLimited}}), nil
		}
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       2,
			PerTokenRetries: 1,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}

	assert.Equal(t, []uint{1}, tokenSvc.rateLimitCalls)
	assert.Equal(t, []uint{2}, tokenSvc.successCalls)
	assert.Equal(t, 2, grok.callCount())
}

func TestChatFlow_ContextCanceledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		cancel() // cancel while the flow is about to back off
		return nil, upstream.ErrNetwork
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       1,
			PerTokenRetries: 2,
			BaseDelay:       50 * time.Millisecond,
			MaxDelay:        50 * time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(ctx, &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	assert.ErrorIs(t, lastError(events), context.Canceled)
	assert.Equal(t, []uint{1}, tokenSvc.keepErrorCalls)
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
	if got := tokenSvc.getInflight(1); got != 0 {
		t.Fatalf("expected inflight released after cancel, got %d", got)
	}
}

func TestChatFlow_PoolExhaustedReportsPickError(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return nil, upstream.ErrNetwork
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       3,
			PerTokenRetries: 1,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	require.NotNil(t, lastError(events))
	assert.Contains(t, lastError(events).Error(), "no tokens available")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)
	assert.Equal(t, []uint{1}, tokenSvc.pickCalls)
}

func TestChatFlow_ContextCanceledBeforeLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(ctx, &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	assert.ErrorIs(t, lastError(events), context.Canceled)
	assert.Empty(t, tokenSvc.pickCalls)
	assert.Equal(t, 0, grok.callCount())
}

func TestChatFlow_RetryConfigProviderUsed(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		ModelResolver: testModelResolver(),
		RetryConfigProvider: func() *RetryConfig {
			return &RetryConfig{
				MaxTokens:       1,
				PerTokenRetries: 1,
				BaseDelay:       time.Millisecond,
				MaxDelay:        time.Millisecond,
				JitterFactor:    0,
			}
		},
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}
	assert.Equal(t, 1, grok.callCount())
	assert.Equal(t, []uint{1}, tokenSvc.successCalls)
}

func TestChatFlow_NilRetryConfigFallsBackToDefault(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}
	assert.Equal(t, []uint{1}, tokenSvc.successCalls)
}

func TestChatFlow_ProvidersForAppConfigAndFilterTags(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{
			{Content: "hi <xaiartifact>secret</xaiartifact>"},
		}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
		AppConfigProvider: func() *config.AppConfig {
			return &config.AppConfig{Temporary: true, DisableMemory: true, CustomInstruction: "be nice"}
		},
		FilterTagsProvider: func() []string { return []string{"xaiartifact"} },
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)

	var content strings.Builder
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
		content.WriteString(event.Content)
	}
	assert.Equal(t, "hi ", content.String())

	req := grok.lastRequest()
	require.NotNil(t, req)
	assert.True(t, req.Temporary)
	assert.True(t, req.DisableMemory)
	assert.Equal(t, "be nice", req.CustomInstruction)
}

func TestChatFlow_EstimatedUsageFromMessages(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Content: "hello"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})
	recorder := &mockUsageRecorder{}
	flow.SetUsageRecorder(recorder)

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)

	var finish *StreamEvent
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
		if event.FinishReason != nil {
			e := event
			finish = &e
		}
	}
	require.NotNil(t, finish)
	require.NotNil(t, finish.Usage)
	// "user"+"Hi" = 6 chars -> (6+2)/3 = 2 prompt tokens
	assert.Equal(t, 2, finish.Usage.PromptTokens)
	assert.Equal(t, 2, finish.Usage.CompletionTokens)
	assert.Equal(t, 4, finish.Usage.TotalTokens)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.True(t, recorder.records[0].Estimated)
}

func TestChatFlow_SearchSourcesDeduplicated(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{
			{SearchSources: []upstream.SearchSource{{URL: "a"}, {URL: "b"}}},
			{SearchSources: []upstream.SearchSource{{URL: "a"}, {URL: "c"}}},
		}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)

	var finish *StreamEvent
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
		if event.FinishReason != nil {
			e := event
			finish = &e
		}
	}
	require.NotNil(t, finish)
	var urls []string
	for _, src := range finish.SearchSources {
		urls = append(urls, src.URL)
	}
	assert.Equal(t, []string{"a", "b", "c"}, urls)
}

func TestChatFlow_StreamEndsWithPartialTag(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Content: "hello <xaiart"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
		FilterTags:    []string{"xaiartifact"},
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)

	var content strings.Builder
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
		content.WriteString(event.Content)
	}
	assert.Equal(t, "hello <xaiart", content.String())
}

func TestChatFlow_StreamEndsWithIncompleteToolCall(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		tools     []Tool
		wantText  string
		wantCalls int
	}{
		{
			name:      "flush emits parsed call from unterminated block",
			content:   `<tool_call>{"name":"f","arguments":{}`,
			tools:     []Tool{{Type: "function", Function: Function{Name: "f"}}},
			wantText:  "",
			wantCalls: 1,
		},
		{
			name:      "flush returns raw tag for unterminated invalid block",
			content:   `<tool_call>not-json`,
			wantText:  "<tool_call>not-json",
			wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
			grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
				return eventsChannel([]upstream.StreamEvent{{Content: tt.content}}), nil
			}}
			flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
				RetryConfig:   DefaultRetryConfig(),
				ModelResolver: testModelResolver(),
			})

			ch, err := flow.Complete(context.Background(), &ChatRequest{
				Messages: []Message{{Role: "user", Content: "Hi"}},
				Model:    "grok-2",
				Tools:    tt.tools,
			})
			require.NoError(t, err)

			var content strings.Builder
			var calls []ToolCall
			for _, event := range drainChat(t, ch) {
				assert.NoError(t, event.Error)
				content.WriteString(event.Content)
				calls = append(calls, event.ToolCalls...)
			}
			assert.Equal(t, tt.wantText, content.String())
			assert.Len(t, calls, tt.wantCalls)
		})
	}
}

func TestChatFlow_RetryBudgetExceededImmediately(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return nil, upstream.ErrNetwork
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       1,
			PerTokenRetries: 1,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
			RetryBudget:     time.Nanosecond,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	assert.ErrorIs(t, lastError(events), ErrRetryBudgetExceeded)
	assert.Empty(t, tokenSvc.successCalls)
}

func TestChatFlow_RetryBudgetExceededWithTokenInHand(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return nil, upstream.ErrNetwork
	}}
	// Budget barely exceeds the first backoff delay: whichever path exits the
	// loop (budget check at loop top, or delay-over-budget check after an
	// error), the held token must be released and the error surfaced.
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       1,
			PerTokenRetries: 5,
			BaseDelay:       20 * time.Millisecond,
			MaxDelay:        20 * time.Millisecond,
			JitterFactor:    0,
			RetryBudget:     20*time.Millisecond + 50*time.Microsecond,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	assert.ErrorIs(t, lastError(events), ErrRetryBudgetExceeded)
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
	if got := tokenSvc.getInflight(1); got != 0 {
		t.Fatalf("expected inflight released after budget exit, got %d", got)
	}
}

func TestChatFlow_ShouldRetrySameTokenEdges(t *testing.T) {
	cfg := &RetryConfig{PerTokenRetries: 2}
	assert.False(t, shouldRetrySameToken(nil, cfg, 0))
	assert.False(t, shouldRetrySameToken(upstream.ErrNetwork, nil, 0))
	assert.False(t, shouldRetrySameToken(upstream.ErrInvalidToken, cfg, 0))
	assert.False(t, shouldRetrySameToken(upstream.ErrNetwork, cfg, 2))
	assert.True(t, shouldRetrySameToken(upstream.ErrNetwork, cfg, 1))
}

func TestChatFlow_HandleErrorMatrix(t *testing.T) {
	cfg := &RetryConfig{PerTokenRetries: 3}
	tests := []struct {
		name          string
		err           error
		keepInflight  bool
		wantExpired   []uint
		wantRelease   []uint
		wantRateLimit []uint
		wantError     []uint
		wantKeepError []uint
	}{
		{
			name:        "invalid token marks expired",
			err:         upstream.ErrInvalidToken,
			wantExpired: []uint{1},
		},
		{
			name:         "invalid token marks expired even when kept in flight",
			err:          upstream.ErrInvalidToken,
			keepInflight: true,
			wantExpired:  []uint{1},
		},
		{
			name:        "forbidden releases token",
			err:         upstream.ErrForbidden,
			wantRelease: []uint{1},
		},
		{
			name:         "forbidden keeps token in flight",
			err:          upstream.ErrForbidden,
			keepInflight: true,
		},
		{
			name:        "cf challenge releases token",
			err:         upstream.ErrCFChallenge,
			wantRelease: []uint{1},
		},
		{
			name:         "cf challenge keeps token in flight",
			err:          upstream.ErrCFChallenge,
			keepInflight: true,
		},
		{
			name:      "transport error refunds quota",
			err:       errors.New("500 internal"),
			wantError: []uint{1},
		},
		{
			name:          "transport error kept in flight",
			err:           errors.New("500 internal"),
			keepInflight:  true,
			wantKeepError: []uint{1},
		},
		{
			name:          "rate limit cools token",
			err:           upstream.ErrRateLimited,
			wantRateLimit: []uint{1},
		},
		{
			name:      "client error is not recoverable",
			err:       errors.New("400 bad request"),
			wantError: []uint{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockTokenService{}
			flow := &ChatFlow{tokenSvc: svc}
			if tt.keepInflight {
				flow.handleErrorKeepInflight(1, "auto", tt.err, cfg)
			} else {
				flow.handleErrorAndRelease(1, "auto", tt.err, cfg)
			}
			assert.Equal(t, tt.wantExpired, svc.expiredCalls)
			assert.Equal(t, tt.wantRelease, svc.releaseCalls)
			assert.Equal(t, tt.wantRateLimit, svc.rateLimitCalls)
			assert.Equal(t, tt.wantError, svc.errorCalls)
			assert.Equal(t, tt.wantKeepError, svc.keepErrorCalls)
		})
	}
}

func TestChatFlow_ZeroValueConfigAccessors(t *testing.T) {
	flow := &ChatFlow{}
	assert.Nil(t, flow.appConfig())
	assert.Nil(t, flow.filterTags())
}

func TestTruncateReason(t *testing.T) {
	short := "boom"
	assert.Equal(t, short, truncateReason(short))

	long := strings.Repeat("x", 300)
	got := truncateReason(long)
	assert.Len(t, got, 256)
	assert.Equal(t, long[:256], got)
}

func TestFlowAPIKeyIDFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), FlowAPIKeyIDKey, uint(7))
	assert.Equal(t, uint(7), FlowAPIKeyIDFromContext(ctx))
	assert.Equal(t, uint(0), FlowAPIKeyIDFromContext(context.Background()))
	wrong := context.WithValue(context.Background(), FlowAPIKeyIDKey, "not-a-uint")
	assert.Equal(t, uint(0), FlowAPIKeyIDFromContext(wrong))
}

func TestNormalizeXTitle(t *testing.T) {
	tests := []struct {
		name     string
		username string
		text     string
		want     string
	}{
		{"empty text uses handle", "alice", "", "𝕏/@alice"},
		{"collapses whitespace", "alice", "  a   b  ", "a b"},
		{"short text kept", "alice", "hello", "hello"},
		{"long text truncated", "alice", strings.Repeat("x", 60), strings.Repeat("x", 50) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeXTitle(tt.username, tt.text))
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		chars int
		want  int
	}{
		{-1, 0},
		{0, 0},
		{1, 1},
		{3, 1},
		{4, 2},
		{7, 3},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, estimateTokens(tt.chars))
	}
}

func TestToUpstreamRequestMatrix(t *testing.T) {
	temp := 0.7
	maxTokens := 128
	tests := []struct {
		name   string
		req    *ChatRequest
		appCfg *config.AppConfig
		check  func(t *testing.T, got *upstream.ChatRequest)
	}{
		{
			name: "upstream model preferred over request model",
			req:  &ChatRequest{Model: "public", UpstreamModel: "grok-4.20"},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.Equal(t, "grok-4.20", got.Model)
			},
		},
		{
			name: "falls back to request model",
			req:  &ChatRequest{Model: "public"},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.Equal(t, "public", got.Model)
			},
		},
		{
			name: "force thinking defaults effort to high",
			req:  &ChatRequest{ForceThinking: true},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.Equal(t, "high", got.ReasoningEffort)
			},
		},
		{
			name: "explicit effort is preserved",
			req:  &ChatRequest{ForceThinking: true, ReasoningEffort: "low"},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.Equal(t, "low", got.ReasoningEffort)
			},
		},
		{
			name: "copies sampling and tool fields",
			req: &ChatRequest{
				Temperature:                    &temp,
				MaxTokens:                      &maxTokens,
				ParallelToolCalls:              true,
				ToolChoice:                     "auto",
				UpstreamMode:                   "fast",
				DeepSearch:                     "deeper",
				ConsoleWebSearch:               true,
				ConsoleSupportsReasoningEffort: true,
			},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.Same(t, &temp, got.Temperature)
				assert.Same(t, &maxTokens, got.MaxTokens)
				assert.True(t, got.ParallelToolCalls)
				assert.Equal(t, "auto", got.ToolChoice)
				assert.Equal(t, "fast", got.UpstreamMode)
				assert.Equal(t, "deeper", got.DeepSearch)
				assert.True(t, got.WebSearch)
				assert.True(t, got.SupportsReasoningEffort)
			},
		},
		{
			name:   "app config applied",
			req:    &ChatRequest{Model: "m"},
			appCfg: &config.AppConfig{Temporary: true, DisableMemory: true, CustomInstruction: "ci"},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.True(t, got.Temporary)
				assert.True(t, got.DisableMemory)
				assert.Equal(t, "ci", got.CustomInstruction)
			},
		},
		{
			name: "nil app config leaves defaults",
			req:  &ChatRequest{Model: "m"},
			check: func(t *testing.T, got *upstream.ChatRequest) {
				assert.False(t, got.Temporary)
				assert.False(t, got.DisableMemory)
				assert.Empty(t, got.CustomInstruction)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, toUpstreamRequest(tt.req, tt.appCfg))
		})
	}
}

func TestExtractStatusCodeTable(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   int
		wantOK bool
	}{
		{"no digits", errors.New("boom"), 0, false},
		{"two digits", errors.New("99"), 0, false},
		{"five digits", errors.New("12345"), 0, false},
		{"out of range", errors.New("999 oops"), 0, false},
		{"lower bound", errors.New("099"), 0, false},
		{"502", errors.New("server returned 502"), 502, true},
		{"429", errors.New("429 Too Many Requests"), 429, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractStatusCode(tt.err)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSafeGoRecoversPanic(t *testing.T) {
	done := make(chan struct{})
	SafeGo("test-panic", func() {
		defer close(done)
		panic("boom")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("panicking goroutine did not run")
	}
}

func TestChatFlow_ContextCanceledMidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	eventCh := make(chan upstream.StreamEvent)
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventCh, nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(ctx, &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)

	eventCh <- upstream.StreamEvent{Content: "x"} // consumed by the flow goroutine
	cancel()
	events := drainChat(t, ch)

	assert.ErrorIs(t, lastError(events), context.Canceled)
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)
}

// poolAwareTokenService fails picks for configured pools, succeeding otherwise.
type poolAwareTokenService struct {
	mockTokenService
	failPools map[string]bool
}

func (p *poolAwareTokenService) Pick(pool string, mode string) (*store.Token, error) {
	if p.failPools[pool] {
		return nil, errors.New("pool " + pool + " drained")
	}
	return p.mockTokenService.Pick(pool, mode)
}

func (p *poolAwareTokenService) PickExcluding(pool string, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	if p.failPools[pool] {
		return nil, errors.New("pool " + pool + " drained")
	}
	return p.mockTokenService.PickExcluding(pool, mode, exclude)
}

func TestChatFlow_MultiPoolFallback(t *testing.T) {
	svc := &poolAwareTokenService{
		mockTokenService: mockTokenService{
			// Long token string exercises the debug masking branch.
			tokens: []*store.Token{{ID: 2, Token: "0123456789abcdef0123456789abcdef", Pool: tkn.PoolSuper}},
		},
		failPools: map[string]bool{tkn.PoolBasic: true},
	}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(svc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2", // floor basic: tries ssoBasic (drained) then ssoSuper
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}
	assert.Equal(t, []uint{2}, svc.successCalls)
	// The drained ssoBasic pick fails before being recorded; ssoSuper succeeds.
	assert.Equal(t, []string{tkn.PoolSuper}, svc.pickPools)
}

func TestChatFlow_ZeroRetryAttempts(t *testing.T) {
	svc := &mockTokenService{}
	grok := &fakeUpstream{}
	flow := NewChatFlow(svc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig:   &RetryConfig{MaxTokens: 0, PerTokenRetries: 0},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	require.NotNil(t, lastError(events))
	assert.Contains(t, lastError(events).Error(), "all retries exhausted")
	assert.Empty(t, svc.pickCalls)
	assert.Equal(t, 0, grok.callCount())
}

func TestChatFlow_StreamErrorKeepsTokenInFlight(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{
		{ID: 1, Token: "tok1", Pool: "basic"},
		{ID: 2, Token: "tok2", Pool: "basic"},
	}}
	grok := &fakeUpstream{onChat: func(call int) (<-chan upstream.StreamEvent, error) {
		if call == 1 {
			return eventsChannel([]upstream.StreamEvent{{Error: upstream.ErrNetwork}}), nil
		}
		return eventsChannel([]upstream.StreamEvent{{Content: "ok"}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       2,
			PerTokenRetries: 2,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	for _, event := range drainChat(t, ch) {
		assert.NoError(t, event.Error)
	}

	// First stream failure keeps the token in flight; the retry reuses it
	// without picking again.
	assert.Equal(t, []uint{1}, tokenSvc.keepErrorCalls)
	assert.Equal(t, []uint{1}, tokenSvc.pickCalls)
	assert.Equal(t, 2, grok.callCount())
	assert.Equal(t, []uint{1}, tokenSvc.successCalls)
}

// cancelingTokenService cancels a context when a keep-inflight error is
// reported, letting tests drive the post-error loop deterministically.
type cancelingTokenService struct {
	mockTokenService
	cancel func()
}

func (c *cancelingTokenService) ReportErrorKeepInflight(id uint, mode string, recoverable bool, reason string) {
	c.mockTokenService.ReportErrorKeepInflight(id, mode, recoverable, reason)
	c.cancel()
}

func TestChatFlow_ContextCanceledAfterKeepInflightError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tokenSvc := &cancelingTokenService{
		mockTokenService: mockTokenService{
			tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
		},
		cancel: cancel,
	}
	grok := &fakeUpstream{onChat: func(int) (<-chan upstream.StreamEvent, error) {
		return eventsChannel([]upstream.StreamEvent{{Error: upstream.ErrNetwork}}), nil
	}}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grok}, &ChatFlowConfig{
		RetryConfig: &RetryConfig{
			MaxTokens:       2,
			PerTokenRetries: 2,
			BaseDelay:       time.Millisecond,
			MaxDelay:        time.Millisecond,
			JitterFactor:    0,
		},
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(ctx, &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Model:    "grok-2",
	})
	require.NoError(t, err)
	events := drainChat(t, ch)

	// The held token is released by the context check before the next attempt.
	assert.ErrorIs(t, lastError(events), context.Canceled)
	assert.Equal(t, []uint{1}, tokenSvc.keepErrorCalls)
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
	if got := tokenSvc.getInflight(1); got != 0 {
		t.Fatalf("expected inflight released after cancel, got %d", got)
	}
}

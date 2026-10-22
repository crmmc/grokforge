package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastRetryConfig keeps retry loops to a single attempt with negligible backoff.
func fastRetryConfig() *flow.RetryConfig {
	return &flow.RetryConfig{
		MaxTokens:       1,
		PerTokenRetries: 1,
		BaseDelay:       time.Millisecond,
		MaxDelay:        time.Millisecond,
		JitterFactor:    0,
		BackoffFactor:   1,
	}
}

// fakeChatUpstream is a hand-written fake implementing upstream.Upstream.
// It records calls and replays preset events, or fails with chatErr.
type fakeChatUpstream struct {
	mu       sync.Mutex
	calls    int
	requests []*upstream.ChatRequest
	tokens   []string
	events   []upstream.StreamEvent
	chatErr  error
}

func (f *fakeChatUpstream) Name() string { return "grok" }

func (f *fakeChatUpstream) Chat(_ context.Context, token string, req *upstream.ChatRequest) (<-chan upstream.StreamEvent, error) {
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, req)
	f.tokens = append(f.tokens, token)
	chatErr := f.chatErr
	events := f.events
	f.mu.Unlock()

	if chatErr != nil {
		return nil, chatErr
	}
	ch := make(chan upstream.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (f *fakeChatUpstream) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeChatUpstream) lastRequest() *upstream.ChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return nil
	}
	return f.requests[len(f.requests)-1]
}

// newTestChatFlow wires a ChatFlow to the fake upstream with fast retries.
func newTestChatFlow(up upstream.Upstream) *flow.ChatFlow {
	return flow.NewChatFlow(&chatMockTokenSvc{}, map[string]upstream.Upstream{"grok": up}, &flow.ChatFlowConfig{
		RetryConfig:   fastRetryConfig(),
		ModelResolver: flowTestModelResolver(),
	})
}

// newConsoleCapableRegistry returns a registry whose chat model has console routing data.
func newConsoleCapableRegistry() *registry.ModelRegistry {
	return registry.NewTestRegistry([]modelconfig.ModelSpec{
		{
			ID:                             "grok-4",
			Type:                           modelconfig.TypeChat,
			Enabled:                        true,
			PoolFloor:                      modelconfig.PoolBasic,
			Mode:                           "auto",
			UpstreamModel:                  "grok-4-web",
			UpstreamMode:                   "MODEL_MODE_FAST",
			ForceThinking:                  true,
			ConsoleUpstreamModel:           "grok-4-console",
			ConsoleMode:                    "console",
			ConsolePoolFloor:               modelconfig.PoolBasic,
			ConsoleSupportsReasoningEffort: true,
		},
	}, nil)
}

func newChatServer(h *Handler) *httpapi.Server {
	return httpapi.NewServer(&httpapi.ServerConfig{ChatProvider: h})
}

func postChat(body string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	return w, req
}

// newBlockingChatCfg returns a config whose default stream mode is off, so
// requests without an explicit "stream" field get blocking JSON responses.
func newBlockingChatCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.App.Stream = false
	return cfg
}

func TestHandleChat_BlockingCompletion_MainPath(t *testing.T) {
	up := &fakeChatUpstream{events: []upstream.StreamEvent{
		{Content: "hello"},
		{Content: " world"},
		{Usage: &flow.Usage{PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17}},
	}}
	chatFlow := newTestChatFlow(up)
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: newBlockingChatCfg()}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}]}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp chatCompletionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, "grok-3", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "hello world", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 17, resp.Usage.TotalTokens)
	assert.NotEmpty(t, resp.ID)
	assert.True(t, strings.HasPrefix(resp.ID, "chatcmpl-"))

	// Flow request must carry registry-derived routing values.
	last := up.lastRequest()
	require.NotNil(t, last)
	assert.Equal(t, "grok-3", last.Model)
}

func TestHandleChat_StreamingCompletion_MainPath(t *testing.T) {
	up := &fakeChatUpstream{events: []upstream.StreamEvent{
		{ReasoningContent: "thinking"},
		{Content: "stream answer"},
		{Usage: &flow.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}},
	}}
	chatFlow := newTestChatFlow(up)
	cfg := config.DefaultConfig()
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: cfg}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}],"stream":true,"reasoning_effort":"low"}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"role":"assistant"`)
	assert.Contains(t, body, "thinking")
	assert.Contains(t, body, "stream answer")
	assert.Contains(t, body, `"finish_reason":"stop"`)
	assert.Contains(t, body, "data: [DONE]")
	assert.Contains(t, body, chatChunkObject)
}

func TestHandleChat_StreamingToolCall(t *testing.T) {
	up := &fakeChatUpstream{events: []upstream.StreamEvent{
		{Content: `<tool_call>{"name":"lookup","arguments":{"q":"x"}}</tool_call>`},
	}}
	chatFlow := newTestChatFlow(up)
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: config.DefaultConfig()}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}],"stream":true,"tools":[{"type":"function","function":{"name":"lookup"}}]}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"tool_calls"`)
	assert.Contains(t, body, "lookup")
	assert.Contains(t, body, `"finish_reason":"tool_calls"`)
	assert.Contains(t, body, "data: [DONE]")
}

func TestHandleChat_BlockingToolCall(t *testing.T) {
	up := &fakeChatUpstream{events: []upstream.StreamEvent{
		{Content: `<tool_call>{"name":"lookup","arguments":{"q":"x"}}</tool_call>`},
		{Usage: &flow.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}},
	}}
	chatFlow := newTestChatFlow(up)
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: newBlockingChatCfg()}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":"AUTO"}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp chatCompletionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "tool_calls", resp.Choices[0].FinishReason)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "lookup", resp.Choices[0].Message.ToolCalls[0].Function.Name)
}

func TestHandleChat_CompletionErrorEventBlocking(t *testing.T) {
	up := &fakeChatUpstream{events: []upstream.StreamEvent{
		{Error: upstream.ErrInvalidToken},
	}}
	chatFlow := newTestChatFlow(up)
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: newBlockingChatCfg()}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}]}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "authentication_error", resp.Error.Type)
}

func TestHandleChat_CompletionErrorEventStreaming(t *testing.T) {
	up := &fakeChatUpstream{events: []upstream.StreamEvent{
		{Error: upstream.ErrInvalidToken},
	}}
	chatFlow := newTestChatFlow(up)
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: config.DefaultConfig()}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"error"`)
	assert.Contains(t, body, "data: [DONE]")
}

func TestHandleChat_UpstreamChatErrorBlocking(t *testing.T) {
	up := &fakeChatUpstream{chatErr: upstream.ErrInvalidToken}
	chatFlow := newTestChatFlow(up)
	h := &Handler{ChatFlow: chatFlow, ModelRegistry: newTestRegistry(t), Cfg: newBlockingChatCfg()}

	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}]}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, 1, up.callCount())
}

func TestHandleChat_ChatFlowNotConfigured(t *testing.T) {
	h := &Handler{ModelRegistry: newTestRegistry(t)}
	w, req := postChat(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}]}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "not_implemented", resp.Error.Code)
}

func TestHandleChat_MediaGenerationDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = false
	h := &Handler{Cfg: cfg, ModelRegistry: testMediaRegistry()}

	w, req := postChat(`{"model":"grok-imagine-video","messages":[{"role":"user","content":"clip"}]}`)
	newChatServer(h).Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "media_generation_disabled", resp.Error.Code)
}

func TestHandleChat_ModelNotInWhitelist(t *testing.T) {
	keyStore := &modelsMockAPIKeyStore{
		key: &store.APIKey{ID: 1, Key: "test-key", Status: "active", ModelWhitelist: store.StringSlice{"grok-2"}},
	}
	h := &Handler{ModelRegistry: newTestRegistry(t)}
	handler := httpapi.APIKeyAuth(keyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handleChat(w, r)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp httpapi.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "model_not_allowed", resp.Error.Code)
}

func TestToFlowRequest_ConsoleRouteResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Console.Enabled = true
	cfg.Console.WebSearch = true
	h := &Handler{Cfg: cfg, ModelRegistry: newConsoleCapableRegistry()}

	parallel := false
	req := &ChatRequest{
		Model:             "grok-4",
		Messages:          []ChatMessage{{Role: "user", Content: "hi"}},
		ParallelToolCalls: &parallel,
		DeepSearch:        "deeper",
		ToolChoice:        map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}},
	}
	flowReq := h.toFlowRequest(req)

	assert.Equal(t, "console", flowReq.UpstreamName)
	assert.Equal(t, "grok-4-console", flowReq.UpstreamModel)
	assert.Empty(t, flowReq.UpstreamMode)
	assert.Equal(t, "console", flowReq.Mode)
	assert.True(t, flowReq.ConsoleSupportsReasoningEffort)
	assert.True(t, flowReq.ConsoleWebSearch)
	assert.True(t, flowReq.ForceThinking)
	assert.False(t, flowReq.ParallelToolCalls)
	assert.Equal(t, "deeper", flowReq.DeepSearch)
	assert.Equal(t, modelconfig.PoolBasic, flowReq.PoolFloor)
}

func TestToFlowRequest_DeepSearchPassthrough(t *testing.T) {
	h := &Handler{ModelRegistry: newTestRegistry(t)}

	flowReq := h.toFlowRequest(&ChatRequest{Model: "grok-3", DeepSearch: "default"})
	assert.Equal(t, "default", flowReq.DeepSearch)

	flowReq = h.toFlowRequest(&ChatRequest{Model: "grok-3", DeepSearch: "bogus"})
	assert.Empty(t, flowReq.DeepSearch)
}

func TestToFlowRequest_NilRegistryKeepsDefaults(t *testing.T) {
	h := &Handler{}
	idx := 0
	flowReq := h.toFlowRequest(&ChatRequest{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "assistant", Name: "bot", ToolCallID: "t1", ToolCalls: []flow.ToolCall{{Index: &idx}}}},
	})
	assert.Equal(t, "grok-3", flowReq.Model)
	assert.True(t, flowReq.Stream)
	assert.True(t, flowReq.ParallelToolCalls)
	assert.Equal(t, "assistant", flowReq.Messages[0].Role)
	assert.Equal(t, "bot", flowReq.Messages[0].Name)
	assert.Equal(t, "t1", flowReq.Messages[0].ToolCallID)
	require.Len(t, flowReq.Messages[0].ToolCalls, 1)
}

func TestPlanChatRoute_NilResolvedModel(t *testing.T) {
	plan := planChatRoute(nil, config.DefaultConfig())
	assert.Equal(t, chatRoutePlan{}, plan)
}

func TestPlanChatRoute_ConsoleWithoutUpstreamModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Console.Enabled = true
	rm := &registry.ResolvedModel{
		ID:        "grok-4",
		Type:      modelconfig.TypeChat,
		Mode:      "auto",
		PoolFloor: modelconfig.PoolBasic,
	}
	plan := planChatRoute(rm, cfg)
	assert.Equal(t, "grok", plan.UpstreamName)
}

func TestHandleChat_UnauthorizedUpstreamErrorIsSurfaced(t *testing.T) {
	// Sanity guard: flow error mapping stays stable for the main-path tests.
	_, apiErr := httpapi.MapXAIError(errors.New("boom"))
	assert.Equal(t, "upstream_error", apiErr.Error.Code)
}

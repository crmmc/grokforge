package flow

import (
	"context"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

// flowCtxKey is a context key type for flow-layer values.
type flowCtxKey string

// FlowAPIKeyIDKey is the context key for API key ID in the flow layer.
// HTTP handlers must bridge from their own context key to this one.
const FlowAPIKeyIDKey flowCtxKey = "apiKeyID"

// FlowAPIKeyIDFromContext extracts the API key ID from context.
func FlowAPIKeyIDFromContext(ctx context.Context) uint {
	if id, ok := ctx.Value(FlowAPIKeyIDKey).(uint); ok {
		return id
	}
	return 0
}

// TokenServicer defines the interface for token management.
type TokenServicer interface {
	Pick(pool string, cat tkn.QuotaCategory) (*store.Token, error)
	PickExcluding(pool string, cat tkn.QuotaCategory, exclude map[uint]struct{}) (*store.Token, error)
	Consume(tokenID uint, cat tkn.QuotaCategory, cost int) (remaining int, err error)
	ReportSuccess(id uint)
	ReportRateLimit(id uint, reason string)
	ReportError(id uint, reason string)
	MarkExpired(id uint, reason string)
}

// XAIClientFactory creates xai.Client instances for a given token.
type XAIClientFactory func(token string) xai.Client

// ChatFlowConfig holds chat flow configuration.
type ChatFlowConfig struct {
	*RetryConfig
	// RetryConfigProvider returns the current RetryConfig, enabling hot-reload.
	// When set, executeWithRetry calls this on every invocation instead of
	// using the embedded RetryConfig.
	RetryConfigProvider func() *RetryConfig
	// TokenConfig provides static model-to-pool mapping configuration.
	TokenConfig *config.TokenConfig
	// TokenConfigProvider provides model-to-pool mapping configuration.
	TokenConfigProvider func() *config.TokenConfig
	// ModelResolver resolves model names to pool floor and cost.
	ModelResolver tkn.ModelResolver
	// AppConfig provides static Grok-specific parameters.
	AppConfig *config.AppConfig
	// AppConfigProvider provides Grok-specific parameters.
	AppConfigProvider func() *config.AppConfig
	// FilterTags lists static HTML-like tags to strip from streamed tokens.
	FilterTags []string
	// FilterTagsProvider provides HTML-like tags to strip from streamed tokens.
	FilterTagsProvider func() []string
	// ResolveUpstream resolves a request model name to its Grok API upstream
	// model name and mode. Returns false if the model is not found.
	ResolveUpstream func(requestName string) (upstreamModel, upstreamMode string, ok bool)
}

// DefaultChatFlowConfig returns default chat flow configuration.
func DefaultChatFlowConfig() *ChatFlowConfig {
	return &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
	}
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"` // string or []ContentBlock for multimodal
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Messages          []Message `json:"messages"`
	Model             string    `json:"model"`
	Stream            bool      `json:"stream"`
	Temperature       *float64  `json:"temperature,omitempty"`
	TopP              *float64  `json:"top_p,omitempty"`
	MaxTokens         *int      `json:"max_tokens,omitempty"`
	ReasoningEffort   string    `json:"reasoning_effort,omitempty"`
	Tools             []Tool    `json:"tools,omitempty"`
	ToolChoice        any       `json:"tool_choice,omitempty"`
	ParallelToolCalls bool      `json:"parallel_tool_calls,omitempty"`
	UpstreamModel     string    `json:"-"` // Grok API model name from registry
	UpstreamMode      string    `json:"-"` // Grok API model mode from registry
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// DownloadFunc downloads content from a URL using an authenticated session.
type DownloadFunc func(ctx context.Context, url string) ([]byte, error)

// StreamEvent represents a flow-level stream event.
type StreamEvent struct {
	Content          string       `json:"content,omitempty"`
	ReasoningContent string       `json:"reasoning_content,omitempty"`
	FinishReason     *string      `json:"finish_reason,omitempty"`
	Usage            *Usage       `json:"usage,omitempty"`
	ToolCalls        []ToolCall   `json:"tool_calls,omitempty"`
	Error            error        `json:"-"`
	IsThinking       bool         `json:"-"`
	RolloutID        string       `json:"-"`
	Downloader       DownloadFunc `json:"-"`
}

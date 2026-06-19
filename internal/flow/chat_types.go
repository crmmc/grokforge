package flow

import (
	"context"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/upstream"
)

// ModeResolver resolves a model request name to its quota mode string.
type ModeResolver interface {
	ResolveMode(requestName string) (mode string, ok bool)
}

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
	Pick(pool string, mode string) (*store.Token, error)
	PickExcluding(pool string, mode string, exclude map[uint]struct{}) (*store.Token, error)
	PickAnyExcluding(pool string, exclude map[uint]struct{}) (*store.Token, error)
	RefundQuota(id uint, mode string)
	ReportSuccess(id uint)
	ReportRateLimit(id uint, mode string, reason string)
	ReportError(id uint, mode string, recoverable bool, reason string)
	ReportErrorKeepInflight(id uint, mode string, recoverable bool, reason string)
	MarkExpired(id uint, reason string)
	ReleaseToken(id uint)
}

// ChatFlowConfig holds chat flow configuration.
type ChatFlowConfig struct {
	*RetryConfig
	// RetryConfigProvider returns the current RetryConfig, enabling hot-reload.
	// When set, executeWithRetry calls this on every invocation instead of
	// using the embedded RetryConfig.
	RetryConfigProvider func() *RetryConfig
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

type Message = upstream.Message
type Usage = upstream.Usage
type DownloadFunc = upstream.DownloadFunc
type SearchSource = upstream.SearchSource
type StreamEvent = upstream.StreamEvent
type ToolCall = upstream.ToolCall
type Tool = upstream.Tool
type Function = upstream.Function
type FunctionCall = upstream.FunctionCall
type ContentBlock = upstream.ContentBlock
type ImageURLBlock = upstream.ImageURLBlock

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Messages                       []Message `json:"messages"`
	Model                          string    `json:"model"`
	Stream                         bool      `json:"stream"`
	Temperature                    *float64  `json:"temperature,omitempty"`
	TopP                           *float64  `json:"top_p,omitempty"`
	MaxTokens                      *int      `json:"max_tokens,omitempty"`
	ReasoningEffort                string    `json:"reasoning_effort,omitempty"`
	Tools                          []Tool    `json:"tools,omitempty"`
	ToolChoice                     any       `json:"tool_choice,omitempty"`
	ParallelToolCalls              bool      `json:"parallel_tool_calls,omitempty"`
	UpstreamModel                  string    `json:"-"` // Grok API model name from registry
	UpstreamMode                   string    `json:"-"` // Grok API model mode from registry
	UpstreamName                   string    `json:"-"` // upstream route name: grok or console
	PoolFloor                      string    `json:"-"` // effective pool floor for route-aware token picking
	ForceThinking                  bool      `json:"-"` // Force reasoning_effort=high from registry
	DeepSearch                     string    `json:"-"` // "default" | "deeper"
	Mode                           string    `json:"-"` // mode from registry for quota tracking
	ConsoleSupportsReasoningEffort bool      `json:"-"`
	ConsoleWebSearch               bool      `json:"-"`
}

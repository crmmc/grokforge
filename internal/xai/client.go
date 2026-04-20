// Package xai provides HTTP client for Grok API communication.
package xai

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

// Client defines the interface for Grok API communication.
type Client interface {
	// Chat sends a chat request and returns a channel of stream events.
	// The channel is closed when the stream ends or an error occurs.
	Chat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)

	// CreateImagePost creates an image media post and returns the upstream post ID.
	CreateImagePost(ctx context.Context, imageURL string) (string, error)

	// CreateVideoPost creates a video media post and returns the upstream post ID.
	CreateVideoPost(ctx context.Context, prompt string) (string, error)

	// DownloadURL downloads the content at the given URL using the client's session (cookies/proxy).
	DownloadURL(ctx context.Context, url string) ([]byte, error)

	// DownloadTo streams the content at the given URL into the provided writer.
	DownloadTo(ctx context.Context, url string, w io.Writer) error

	// UploadFile uploads an attachment and returns its metadata ID and URI.
	UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error)

	// PollUpscale polls until the upscaled video URL is ready.
	PollUpscale(ctx context.Context, videoID string, interval time.Duration) (string, error)

	// ResetSession rebuilds the HTTP client and cookie jar.
	// Call this after 403/429 errors to clear session state.
	ResetSession() error

	// Close releases resources held by the client.
	Close() error
}

// ChatRequest represents a chat completion request to Grok API.
type ChatRequest struct {
	// Messages is the conversation history.
	Messages []Message `json:"messages"`

	// Model is the request model identifier exposed by this gateway.
	Model string `json:"model,omitempty"`

	// Stream enables streaming response.
	Stream bool `json:"stream,omitempty"`

	// Temperature controls randomness (0.0-2.0).
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling.
	TopP *float64 `json:"top_p,omitempty"`

	// MaxTokens limits the response length.
	MaxTokens *int `json:"max_tokens,omitempty"`

	// Grok-specific parameters (from app config)
	Temporary         bool           `json:"-"`
	DisableMemory     bool           `json:"-"`
	DynamicStatsig    bool           `json:"-"`
	CustomInstruction string         `json:"-"`
	ReasoningEffort   string         `json:"-"`
	FileAttachments   []string       `json:"-"`
	ToolOverrides     map[string]any `json:"-"`
	ModelConfig       map[string]any `json:"-"`

	// IsImageEdit marks this request as an image edit call.
	// When true, buildChatBody() adds "isReasoning": false to the payload.
	IsImageEdit bool `json:"-"`

	// UpstreamModel is the Grok API model name (for example "grok-420").
	// Set by flow layer from ModelRegistry. Used by buildChatBody().
	UpstreamModel string `json:"-"`

	// UpstreamMode is the Grok API mode ID (for example "auto", "fast", "expert", "heavy").
	// Set by flow layer from ModelRegistry. Used by buildChatBody().
	UpstreamMode string `json:"-"`

	// DeepSearch preset: "default" or "deeper".
	DeepSearch string `json:"-"`
}

// Message represents a single message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamEvent represents a single event from the streaming response.
type StreamEvent struct {
	// Data contains the raw JSON payload.
	Data json.RawMessage

	// Error is set if an error occurred during streaming.
	Error error
}

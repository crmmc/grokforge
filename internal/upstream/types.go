package upstream

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
	Index    *int         `json:"index,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ContentBlock struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *ImageURLBlock `json:"image_url,omitempty"`
}

type ImageURLBlock struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type DownloadFunc func(ctx context.Context, url string) ([]byte, error)

type SearchSource struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type StreamEvent struct {
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	FinishReason     *string        `json:"finish_reason,omitempty"`
	Usage            *Usage         `json:"usage,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	Error            error          `json:"-"`
	IsThinking       bool           `json:"-"`
	RolloutID        string         `json:"-"`
	SearchSources    []SearchSource `json:"-"`
	Downloader       DownloadFunc   `json:"-"`
}

type ChatRequest struct {
	Messages          []Message
	Model             string
	Temperature       *float64
	TopP              *float64
	MaxTokens         *int
	ReasoningEffort   string
	Tools             []Tool
	ToolChoice        any
	ParallelToolCalls bool
	CustomInstruction string

	// grok 特有
	UpstreamMode  string
	DeepSearch    string
	Temporary     bool
	DisableMemory bool

	// console 特有
	WebSearch               bool
	SupportsReasoningEffort bool
}

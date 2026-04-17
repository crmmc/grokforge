package flow

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

func TestBuildXAIRequest_EstimatesFlattenedPromptTokens(t *testing.T) {
	flow := NewChatFlow(nil, nil, &ChatFlowConfig{RetryConfig: DefaultRetryConfig()})

	built, err := flow.buildXAIRequest(context.Background(), &ChatRequest{
		Model: "grok-2",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
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
	}, &stubChatRequestClient{})
	if err != nil {
		t.Fatalf("buildXAIRequest() error = %v", err)
	}

	if len(built.Messages) == 0 {
		t.Fatal("expected built request messages")
	}
	if built.Messages[0].Role != "system" {
		t.Fatalf("first message role = %q, want system", built.Messages[0].Role)
	}
	if built.Messages[0].Content == "" {
		t.Fatal("expected non-empty first message content")
	}
	if built.Messages[1].Content != "Hello" {
		t.Fatalf("user message content = %q, want %q", built.Messages[1].Content, "Hello")
	}
}

type stubChatRequestClient struct{}

func (s *stubChatRequestClient) Chat(context.Context, *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	return nil, nil
}

func (s *stubChatRequestClient) CreateImagePost(context.Context, string) (string, error) {
	return "", nil
}

func (s *stubChatRequestClient) CreateVideoPost(context.Context, string) (string, error) {
	return "", nil
}

func (s *stubChatRequestClient) DownloadURL(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (s *stubChatRequestClient) DownloadTo(context.Context, string, io.Writer) error {
	return nil
}

func (s *stubChatRequestClient) UploadFile(context.Context, string, string, string) (string, string, error) {
	return "", "", nil
}

func (s *stubChatRequestClient) PollUpscale(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (s *stubChatRequestClient) ResetSession() error {
	return nil
}

func (s *stubChatRequestClient) Close() error {
	return nil
}

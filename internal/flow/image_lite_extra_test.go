package flow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageLiteRequestValidateMatrix(t *testing.T) {
	tests := []struct {
		name       string
		req        *ImageLiteRequest
		wantErr    string
		wantFormat string
	}{
		{"valid defaults", &ImageLiteRequest{Prompt: "p"}, "", "b64_json"},
		{"empty prompt", &ImageLiteRequest{}, "prompt is required", ""},
		{"n over limit", &ImageLiteRequest{Prompt: "p", N: 5}, "n must be between 1 and 4", ""},
		{"bad format", &ImageLiteRequest{Prompt: "p", ResponseFormat: "gif"}, "response_format must be url, b64_json, or base64", ""},
		{"url accepted", &ImageLiteRequest{Prompt: "p", ResponseFormat: "url"}, "", "url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, 1, tt.req.N)
				assert.Equal(t, tt.wantFormat, tt.req.ResponseFormat)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestImageFlow_GenerateLite_InvalidRequest(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{})
	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestImageFlow_GenerateLite_NoFactory(t *testing.T) {
	flow := newTestImageFlow(&mockImagineClient{})
	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat client not configured")
}

func TestImageFlow_GenerateLite_NilClient(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	flow.SetEditClientFactory(func(string) ImageEditClient { return nil })

	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat client is nil")
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
}

func TestImageFlow_GenerateLite_ChatStartError(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	flow.SetUsageRecorder(recorder)
	flow.SetEditClientFactory(func(string) ImageEditClient {
		return &stubEditClient{chatErrs: []error{errors.New("chat refused")}}
	})

	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lite image chat")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 500, recorder.records[0].Status)
	assert.Equal(t, "image_lite", recorder.records[0].Endpoint)
}

func TestImageFlow_GenerateLite_StreamError(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		chatEvents: [][]xai.StreamEvent{{{Error: errors.New("stream broke")}}},
	})
	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lite image stream")
}

func TestImageFlow_GenerateLite_NoURLs(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		chatEvents: [][]xai.StreamEvent{{{Data: json.RawMessage(`{"result":{"response":{}}}`)}}},
	})
	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no images generated in lite response")
}

func TestImageFlow_GenerateLite_MultipleImages(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	flow.SetUsageRecorder(recorder)
	editor := &stubEditClient{
		imageURLs:    []string{"https://example.com/a.png"},
		downloadBody: []byte("png-bytes"),
	}
	flow.SetEditClientFactory(func(string) ImageEditClient { return editor })

	resp, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p", N: 2})
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, 2, editor.chatCalls)
	assert.Equal(t, []uint{1}, tokenSvc.successCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 200, recorder.records[0].Status)
}

func TestImageFlow_GenerateLite_AppConfigApplied(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		imageURLs:    []string{"https://example.com/a.png"},
		downloadBody: []byte("png-bytes"),
	})
	flow.SetAppConfig(&config.AppConfig{Temporary: true, DisableMemory: true})

	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.NoError(t, err)
}

func TestImageFlow_GenerateLite_ChatRequestShape(t *testing.T) {
	editor := &stubEditClient{
		imageURLs:    []string{"https://example.com/a.png"},
		downloadBody: []byte("png-bytes"),
	}
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)
	flow.SetAppConfig(&config.AppConfig{Temporary: true})

	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{
		Model:        "grok-imagine-image-lite",
		Prompt:       "a lighthouse",
		UpstreamMode: "MODEL_MODE_FAST",
	})
	require.NoError(t, err)
	require.Len(t, editor.chatRequests, 1)
	req := editor.chatRequests[0]
	assert.True(t, req.Stream)
	assert.Equal(t, "MODEL_MODE_FAST", req.UpstreamMode)
	assert.Equal(t, "Drawing: a lighthouse", req.Messages[0].Content)
	assert.True(t, req.Temporary)
	assert.False(t, req.DisableMemory)
}

func TestExtractImageURLsFromEventTable(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want []string
	}{
		{
			"valid payload",
			json.RawMessage(`{"result":{"response":{"modelResponse":{"generatedImageUrls":["u1","u2"]}}}}`),
			[]string{"u1", "u2"},
		},
		{
			"missing model response",
			json.RawMessage(`{"result":{"response":{}}}`),
			nil,
		},
		{
			"null model response",
			json.RawMessage(`{"result":{"response":{"modelResponse":null}}}`),
			nil,
		},
		{
			"invalid json",
			json.RawMessage(`{bad`),
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractImageURLsFromEvent(tt.data))
		})
	}
}

func TestImageFlow_GenerateLite_NoTokens(t *testing.T) {
	flow := NewImageFlow(&mockTokenService{}, nil)
	flow.SetModelResolver(testModelResolver())
	flow.SetEditClientFactory(func(string) ImageEditClient { return &stubEditClient{} })

	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tokens available")
}

func TestImageFlow_GenerateLite_DownloadError(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		imageURLs:   []string{"https://example.com/a.png"},
		downloadErr: errors.New("cdn refused"),
	})

	_, err := flow.GenerateLite(context.Background(), &ImageLiteRequest{Model: "grok-imagine-image-lite", Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download")
}

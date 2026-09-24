package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEditClient is a configurable ImageEditClient fake with per-call chat
// behavior for exercising the image edit and lite generation chains.
type stubEditClient struct {
	mu            sync.Mutex
	chatErrs      []error
	chatEvents    [][]xai.StreamEvent
	uploadErr     error
	uploadURI     string
	createPostErr error
	postID        string
	downloadErr   error
	downloadBody  []byte
	imageURLs     []string

	chatRequests []*xai.ChatRequest
	uploadMimes  []string
	chatCalls    int
	uploadCalls  int
	postCalls    int
}

func (s *stubEditClient) Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	s.mu.Lock()
	call := s.chatCalls
	s.chatCalls++
	s.chatRequests = append(s.chatRequests, req)
	var chatErr error
	if call < len(s.chatErrs) {
		chatErr = s.chatErrs[call]
	}
	var events []xai.StreamEvent
	if call < len(s.chatEvents) {
		events = s.chatEvents[call]
	} else {
		payload, _ := json.Marshal(map[string]any{
			"result": map[string]any{
				"response": map[string]any{
					"modelResponse": map[string]any{
						"generatedImageUrls": s.imageURLs,
					},
				},
			},
		})
		events = []xai.StreamEvent{{Data: payload}}
	}
	s.mu.Unlock()

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

func (s *stubEditClient) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploadCalls++
	s.uploadMimes = append(s.uploadMimes, fileMimeType)
	if s.uploadErr != nil {
		return "", "", s.uploadErr
	}
	uri := s.uploadURI
	if uri == "" {
		uri = "generated/uploaded-" + fileName
	}
	return "file-id", uri, nil
}

func (s *stubEditClient) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postCalls++
	if s.createPostErr != nil {
		return "", s.createPostErr
	}
	return s.postID, nil
}

func (s *stubEditClient) DownloadURL(ctx context.Context, url string) ([]byte, error) {
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}
	return s.downloadBody, nil
}

func newEditRequest() *ImageEditRequest {
	return &ImageEditRequest{
		Model:          "grok-imagine-image-edit",
		UpstreamModel:  "imagine-image-edit",
		UpstreamMode:   "MODEL_MODE_FAST",
		Prompt:         "add a hat",
		OriginalImages: [][]byte{createTestPNG()},
		Size:           "1024x1024",
	}
}

func TestImageFlow_Edit_NilFactory(t *testing.T) {
	flow := newTestImageFlow(&mockImagineClient{})
	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image edit client not configured")
}

func TestImageFlow_Edit_NilClient(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	flow.SetEditClientFactory(func(string) ImageEditClient { return nil })

	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image edit client is nil")
	assert.Equal(t, []uint{1}, tokenSvc.releaseCalls)
}

func TestImageFlow_Edit_NoTokens(t *testing.T) {
	tokenSvc := &mockTokenService{}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	flow.SetEditClientFactory(func(string) ImageEditClient { return &stubEditClient{} })

	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tokens available")
}

func TestImageFlow_Edit_UpstreamModeRequired(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{})
	req := newEditRequest()
	req.UpstreamMode = ""
	_, err := flow.Edit(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream mode is required")
}

func TestImageFlow_Edit_UploadError(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	flow.SetUsageRecorder(recorder)
	flow.SetEditClientFactory(func(string) ImageEditClient {
		return &stubEditClient{uploadErr: errors.New("disk full")}
	})

	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload image reference")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 500, recorder.records[0].Status)
}

func TestImageFlow_Edit_ChatStartError(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		chatErrs: []error{errors.New("chat refused")},
	})
	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start image edit")
}

func TestImageFlow_Edit_StreamErrorEvent(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		chatEvents: [][]xai.StreamEvent{{{Error: errors.New("stream broke")}}},
	})
	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image edit stream")
}

func TestImageFlow_Edit_StreamBadJSON(t *testing.T) {
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, &stubEditClient{
		chatEvents: [][]xai.StreamEvent{{{Data: json.RawMessage("{not-json")}}},
	})
	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode image edit stream")
}

func TestImageFlow_Edit_NoImagesReceived(t *testing.T) {
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	flow.SetUsageRecorder(recorder)
	flow.SetEditClientFactory(func(string) ImageEditClient {
		return &stubEditClient{imageURLs: nil}
	})

	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no edited images received")
	assert.Equal(t, []uint{1}, tokenSvc.errorCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 500, recorder.records[0].Status)
}

func TestImageFlow_Edit_AppConfigApplied(t *testing.T) {
	t.Run("static config", func(t *testing.T) {
		editor := &stubEditClient{postID: "post-1", imageURLs: []string{"https://example.com/a.png"}, downloadBody: []byte("img")}
		flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)
		flow.SetAppConfig(&config.AppConfig{Temporary: true, DisableMemory: true, CustomInstruction: "be nice"})

		_, err := flow.Edit(context.Background(), newEditRequest())
		require.NoError(t, err)
		require.Len(t, editor.chatRequests, 1)
		assert.True(t, editor.chatRequests[0].Temporary)
		assert.True(t, editor.chatRequests[0].DisableMemory)
		assert.Equal(t, "be nice", editor.chatRequests[0].CustomInstruction)
	})

	t.Run("config provider", func(t *testing.T) {
		editor := &stubEditClient{postID: "post-1", imageURLs: []string{"https://example.com/a.png"}, downloadBody: []byte("img")}
		flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)
		flow.SetAppConfigProvider(func() *config.AppConfig {
			return &config.AppConfig{Temporary: true, CustomInstruction: "dynamic"}
		})

		_, err := flow.Edit(context.Background(), newEditRequest())
		require.NoError(t, err)
		require.Len(t, editor.chatRequests, 1)
		assert.True(t, editor.chatRequests[0].Temporary)
		assert.Equal(t, "dynamic", editor.chatRequests[0].CustomInstruction)
	})
}

func TestImageFlow_Edit_ParentPostFallsBackToURLPattern(t *testing.T) {
	uri := "https://assets.grok.com/users/u/generated/0123456789abcdef0123456789abcdef/content"
	editor := &stubEditClient{
		createPostErr: errors.New("post rejected"),
		uploadURI:     uri,
		imageURLs:     []string{"https://example.com/a.png"},
		downloadBody:  []byte("img"),
	}
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)

	resp, err := flow.Edit(context.Background(), newEditRequest())
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)

	require.Len(t, editor.chatRequests, 1)
	mc, ok := editor.chatRequests[0].ModelConfig["modelMap"].(map[string]any)
	require.True(t, ok)
	cfg, ok := mc["imageEditModelConfig"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", cfg["parentPostId"])
}

func TestImageFlow_Edit_SuccessRecordsUsage(t *testing.T) {
	editor := &stubEditClient{
		postID:       "post-9",
		imageURLs:    []string{"https://example.com/a.png", "https://example.com/b.png"},
		downloadBody: []byte("img-bytes"),
	}
	tokenSvc := &mockTokenService{tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}}}
	flow := NewImageFlow(tokenSvc, nil)
	flow.SetModelResolver(testModelResolver())
	recorder := &imageUsageRecorder{}
	flow.SetUsageRecorder(recorder)
	flow.SetEditClientFactory(func(string) ImageEditClient { return editor })

	resp, err := flow.Edit(context.Background(), newEditRequest())
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "post-9", editor.chatRequests[0].ModelConfig["modelMap"].(map[string]any)["imageEditModelConfig"].(map[string]any)["parentPostId"])
	assert.Equal(t, []uint{1}, tokenSvc.successCalls)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.records, 1)
	assert.Equal(t, 200, recorder.records[0].Status)
	assert.Equal(t, "image", recorder.records[0].Endpoint)
}

func TestResolveImageEditParentPostIDTable(t *testing.T) {
	clientOK := &stubEditClient{postID: "p1"}
	clientErr := &stubEditClient{createPostErr: errors.New("nope")}
	tests := []struct {
		name    string
		client  ImageEditClient
		urls    []string
		want    string
		wantErr bool
	}{
		{"no urls", clientOK, nil, "", true},
		{"client post id", clientOK, []string{"https://x/a.png"}, "p1", false},
		{"generated id fallback", clientErr, []string{"https://x/generated/0123456789abcdef0123456789abcdef/y.png"}, "0123456789abcdef0123456789abcdef", false},
		{"user id fallback", clientErr, []string{"https://x/users/u/0123456789abcdef0123456789abcdef/content"}, "0123456789abcdef0123456789abcdef", false},
		{"no pattern", clientErr, []string{"https://x/plain.png"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveImageEditParentPostID(context.Background(), tt.client, tt.urls)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUploadImageEditReferencesTable(t *testing.T) {
	png := createTestPNG()
	t.Run("no images fails", func(t *testing.T) {
		_, err := uploadImageEditReferences(context.Background(), &stubEditClient{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "image upload failed")
	})
	t.Run("upload error", func(t *testing.T) {
		_, err := uploadImageEditReferences(context.Background(), &stubEditClient{uploadErr: errors.New("nope")}, [][]byte{png})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upload image reference")
	})
	t.Run("uploads with detected mime", func(t *testing.T) {
		client := &stubEditClient{uploadURI: "/generated/x"}
		urls, err := uploadImageEditReferences(context.Background(), client, [][]byte{png})
		require.NoError(t, err)
		assert.Equal(t, []string{"https://assets.grok.com/generated/x"}, urls)
		require.Len(t, client.uploadMimes, 1)
		assert.Equal(t, "image/png", client.uploadMimes[0])
	})
}

func TestLastImageEditReferences(t *testing.T) {
	images := [][]byte{{1}, {2}, {3}, {4}, {5}}
	got := lastImageEditReferences(images)
	require.Len(t, got, 3)
	assert.Equal(t, byte(3), got[0][0])
	assert.Equal(t, byte(5), got[2][0])
	assert.Len(t, lastImageEditReferences(images[:2]), 2)
}

func TestNormalizeUploadedImageURLTable(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"https://x/y.png", "https://x/y.png"},
		{"http://x/y.png", "http://x/y.png"},
		{"/a/b.png", "https://assets.grok.com/a/b.png"},
		{"a/b.png", "https://assets.grok.com/a/b.png"},
		{"", "https://assets.grok.com/"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.out, normalizeUploadedImageURL(tt.in))
	}
}

func TestExtensionForMIMETable(t *testing.T) {
	assert.Equal(t, ".png", extensionForMIME("image/png"))
	assert.Contains(t, []string{".jpg", ".jpeg", ".jpe"}, extensionForMIME("image/jpeg"))
	assert.Equal(t, ".bin", extensionForMIME("bogus/type"))
	assert.Equal(t, ".bin", extensionForMIME(""))
}

func TestDetectImageEditMIME(t *testing.T) {
	assert.Equal(t, "image/png", detectImageEditMIME(createTestPNG()))
	// http.DetectContentType never returns empty, so the octet-stream
	// fallback is unreachable through real input; empty bytes classify as text.
	assert.Contains(t, detectImageEditMIME(nil), "text/plain")
}

func TestCollectImageURLsTable(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    []string
	}{
		{"string field", map[string]any{"generatedImageUrls": "u1"}, []string{"u1"}},
		{"array field dedups and drops blanks", map[string]any{"imageUrls": []any{"u1", "u1", ""}}, []string{"u1"}},
		{"nested non-strings skipped", map[string]any{"nested": map[string]any{"imageURLs": []any{"u2", 3}}}, []string{"u2"}},
		{"deep nesting", map[string]any{"a": []any{map[string]any{"b": map[string]any{"generatedImageUrls": []any{"u3"}}}}}, []string{"u3"}},
		{"nothing found", map[string]any{"other": "x"}, []string{}},
		{"top level slice", []any{map[string]any{"imageUrls": []any{"u4"}}}, []string{"u4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, collectImageURLs(tt.payload))
		})
	}
}

func TestExtractImageEditURLsBadJSON(t *testing.T) {
	_, err := extractImageEditURLs(json.RawMessage("{bad"))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "decode image edit stream"))
}

func TestImageFlow_Edit_DuplicateURLsSkipped(t *testing.T) {
	editor := &stubEditClient{
		postID: "post-1",
		imageURLs: []string{
			"https://example.com/same.png",
			"https://example.com/same.png",
			"https://example.com/other.png",
		},
		downloadBody: []byte("img"),
	}
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)

	req := newEditRequest()
	req.N = 2
	resp, err := flow.Edit(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Data, 2, "duplicate URL should be skipped, both unique URLs resolved")
}

func TestImageFlow_Edit_DownloadError(t *testing.T) {
	editor := &stubEditClient{
		postID:      "post-1",
		imageURLs:   []string{"https://example.com/a.png"},
		downloadErr: errors.New("cdn refused"),
	}
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)

	_, err := flow.Edit(context.Background(), newEditRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download")
}

func TestImageFlow_Edit_DuplicateURLsAcrossCallsAndEvents(t *testing.T) {
	url := "https://example.com/dup.png"
	other := "https://example.com/other.png"
	eventFor := func(urls ...string) xai.StreamEvent {
		payload, err := json.Marshal(map[string]any{
			"result": map[string]any{
				"response": map[string]any{
					"modelResponse": map[string]any{"generatedImageUrls": urls},
				},
			},
		})
		require.NoError(t, err)
		return xai.StreamEvent{Data: payload}
	}
	editor := &stubEditClient{
		postID:       "post-1",
		downloadBody: []byte("img"),
		// Call 1 yields the URL once; call 2 repeats it across two events.
		chatEvents: [][]xai.StreamEvent{
			{eventFor(url)},
			{eventFor(url), eventFor(url, other)},
		},
	}
	flow := newTestImageFlowWithEditor(&mockImagineClient{}, editor)

	req := newEditRequest()
	req.N = 3
	resp, err := flow.Edit(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.Data, 2, "duplicate URLs skipped across calls and events")
	for _, data := range resp.Data {
		assert.NotEmpty(t, data.B64JSON)
		assert.Equal(t, "add a hat", data.RevisedPrompt)
	}
}

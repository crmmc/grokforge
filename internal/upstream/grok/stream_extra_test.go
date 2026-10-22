package grok

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGrokEvent_InvalidJSON(t *testing.T) {
	ev := parseGrokEvent(json.RawMessage("{not json"))
	require.Error(t, ev.Error)
	assert.Nil(t, ev.Usage)
}

func TestParseGrokEvent_ThinkingAndModelImages(t *testing.T) {
	ev := parseGrokEvent(json.RawMessage(`{
		"result": {"response": {
			"token": "silent reasoning",
			"isThinking": true,
			"rolloutId": "  r-9  ",
			"modelResponse": {"generatedImageUrls": ["https://assets.grok.com/u/gen/pic.png", "relative.png"]}
		}}
	}`))
	require.NoError(t, ev.Error)
	assert.True(t, ev.IsThinking)
	assert.Equal(t, "silent reasoning", ev.ReasoningContent)
	assert.Equal(t, "r-9", ev.RolloutID)
	assert.Contains(t, ev.Content, "![gen](https://assets.grok.com/u/gen/pic.png)")
	assert.Contains(t, ev.Content, "![image](relative.png)")
}

func TestSplitThinkingToken(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		isThinking   bool
		wantContent  string
		wantThinking string
	}{
		{name: "thinking", token: "why", isThinking: true, wantThinking: "why"},
		{name: "content", token: "answer", wantContent: "answer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, thinking := splitThinkingToken(tt.token, tt.isThinking)
			assert.Equal(t, tt.wantContent, content)
			assert.Equal(t, tt.wantThinking, thinking)
		})
	}
}

func TestAppendModelResponseImages(t *testing.T) {
	tests := []struct {
		name    string
		mr      *chatModelResponse
		content string
		want    string
	}{
		{name: "nil response", mr: nil, content: "base", want: "base"},
		{name: "no images", mr: &chatModelResponse{}, content: "base", want: "base"},
		{
			name:    "appends markdown images",
			mr:      &chatModelResponse{GeneratedImageUrls: []string{"https://assets.grok.com/u/gen/pic.png", "x"}},
			content: "base",
			want:    "base\n![gen](https://assets.grok.com/u/gen/pic.png)\n![image](x)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendModelResponseImages(tt.content, tt.mr))
		})
	}
}

func TestGeneratedImageAlt(t *testing.T) {
	tests := []struct {
		name   string
		imgURL string
		want   string
	}{
		{name: "penultimate segment", imgURL: "https://assets.grok.com/u/gen/pic.png", want: "gen"},
		{name: "too few segments", imgURL: "pic.png", want: "image"},
		{name: "empty", imgURL: "", want: "image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, generatedImageAlt(tt.imgURL))
		})
	}
}

func TestAppendCardAttachmentImage(t *testing.T) {
	tests := []struct {
		name    string
		ca      *chatCardAttachment
		content string
		want    string
	}{
		{name: "nil attachment", ca: nil, content: "base", want: "base"},
		{name: "empty json data", ca: &chatCardAttachment{}, content: "base", want: "base"},
		{name: "invalid json data", ca: &chatCardAttachment{JSONData: "{oops"}, content: "base", want: "base"},
		{
			name: "no image fields",
			ca:   &chatCardAttachment{JSONData: `{"image":{},"image_chunk":{"progress":50}}`},
			want: "",
		},
		{
			name:    "original image wins over chunk",
			ca:      &chatCardAttachment{JSONData: `{"image":{"original":"a/b.png","title":"T"},"image_chunk":{"progress":100,"imageUrl":"chunk.png","imageUuid":"cu"}}`},
			content: "base",
			want:    "base\n![T](https://assets.grok.com/a/b.png)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendCardAttachmentImage(tt.content, tt.ca))
		})
	}
}

func TestAppendMarkdownImage_EmptyTarget(t *testing.T) {
	assert.Equal(t, "base", appendMarkdownImage("base", "alt", "   "))
}

func TestMarkdownImageAlt(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty becomes image", value: "", want: "image"},
		{name: "brackets replaced and collapsed", value: "  a [b]\n c  ", want: "a b c"},
		{name: "only brackets", value: "[]", want: "image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, markdownImageAlt(tt.value))
		})
	}
}

func TestNormalizeXTitle(t *testing.T) {
	tests := []struct {
		name     string
		username string
		text     string
		want     string
	}{
		{name: "empty text falls back", username: "ada", text: "", want: "𝕏/@ada"},
		{name: "whitespace collapsed", username: "ada", text: "a\n  b\t c", want: "a b c"},
		{name: "short text", username: "ada", text: "hello world", want: "hello world"},
		{
			name:     "long text truncated to 50 runes plus ellipsis",
			username: "ada",
			text:     strings.Repeat("字", 60),
			want:     strings.Repeat("字", 50) + "…",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeXTitle(tt.username, tt.text))
		})
	}
}

func TestParseStream_StopsAtDoneAndSkipsNoise(t *testing.T) {
	body := strings.Join([]string{
		": keep-alive",
		"",
		`{"result":{"response":{"token":"A","rolloutId":"r"}}}`,
		"data: {\"result\":{\"response\":{\"token\":\"B\"}}}",
		`{"result":{"response":{}}}`,
		"   ",
		"data: [DONE]",
		`{"result":{"response":{"token":"after-done"}}}`,
	}, "\n")

	g := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	require.NoError(t, g.parseStream(context.Background(), "tok", strings.NewReader(body), ch))
	close(ch)

	var content string
	for ev := range ch {
		require.NoError(t, ev.Error)
		content += ev.Content
	}
	assert.Equal(t, "AB", content)
}

func TestParseStream_ErrorEventForBadJSON(t *testing.T) {
	g := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	require.NoError(t, g.parseStream(context.Background(), "tok", strings.NewReader("{\"broken\"\n"), ch))
	close(ch)

	var sawErr bool
	for ev := range ch {
		if ev.Error != nil {
			sawErr = true
		}
	}
	assert.True(t, sawErr, "expected an error event on the channel")
}

func TestParseStream_ContextCancelled(t *testing.T) {
	g := New("", nil, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan upstream.StreamEvent, 16)
	err := g.parseStream(ctx, "tok", strings.NewReader(`{"result":{"response":{"token":"A"}}}`), ch)
	require.ErrorIs(t, err, context.Canceled)
}

func TestParseStream_ScannerError(t *testing.T) {
	g := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	err := g.parseStream(context.Background(), "tok",
		io.NopCloser(io.MultiReader(strBody(`{"result":{"response":{"token":"A"}}}`), iotest.ErrReader(io.ErrUnexpectedEOF))), ch)
	require.Error(t, err)
	assert.ErrorIs(t, err, upstream.ErrNetwork)
	assert.Contains(t, err.Error(), "unexpected EOF")
}

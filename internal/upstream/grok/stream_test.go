package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestParseStream_Normal(t *testing.T) {
	data, err := os.ReadFile("test/data/normal_stream.txt")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	g := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	if err := g.parseStream(context.Background(), "tok-1", strings.NewReader(string(data)), ch); err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	close(ch)

	var content string
	var rollout string
	seenDownloader := false
	for ev := range ch {
		content += ev.Content
		if ev.RolloutID != "" {
			rollout = ev.RolloutID
		}
		if ev.Downloader != nil {
			seenDownloader = true
		}
	}
	if content != "Hello world" {
		t.Errorf("content=%q", content)
	}
	if rollout != "r-1" {
		t.Errorf("rolloutID=%q", rollout)
	}
	if !seenDownloader {
		t.Error("expected upstream-provided downloader")
	}
}

func TestParseGrokEvent_CardAttachmentImages(t *testing.T) {
	tests := []struct {
		name         string
		cardJSON     string
		wantContains string
		wantEmpty    bool
	}{
		{
			name:         "image chunk",
			cardJSON:     `{"image_chunk":{"progress":100,"imageUuid":"image-1","imageUrl":"users/u/generated/id/image.png"}}`,
			wantContains: "![image-1](https://assets.grok.com/users/u/generated/id/image.png)",
		},
		{
			name:      "moderated image chunk",
			cardJSON:  `{"image_chunk":{"progress":100,"imageUuid":"image-1","imageUrl":"users/u/generated/id/image.png","moderated":true}}`,
			wantEmpty: true,
		},
		{
			name:         "relative original image",
			cardJSON:     `{"image":{"original":"cards/id/image.png","title":"card"}}`,
			wantContains: "![card](https://assets.grok.com/cards/id/image.png)",
		},
		{
			name:         "scheme-relative original image",
			cardJSON:     `{"image":{"original":"//assets.grok.com/cards/id/image.png","title":"card"}}`,
			wantContains: "![card](https://assets.grok.com/cards/id/image.png)",
		},
		{
			name:         "uppercase scheme original image",
			cardJSON:     `{"image":{"original":"HTTPS://assets.grok.com/cards/id/image.png","title":"card"}}`,
			wantContains: "![card](HTTPS://assets.grok.com/cards/id/image.png)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGrokEvent(cardAttachmentEvent(tt.cardJSON))
			if got.Error != nil {
				t.Fatalf("parseGrokEvent() error = %v", got.Error)
			}
			if tt.wantEmpty {
				if got.Content != "" {
					t.Fatalf("Content = %q, want empty", got.Content)
				}
				return
			}
			if !strings.Contains(got.Content, tt.wantContains) {
				t.Fatalf("Content = %q, want markdown image %q", got.Content, tt.wantContains)
			}
		})
	}
}

func TestParseGrokEvent_SearchSources(t *testing.T) {
	event := parseGrokEvent(json.RawMessage(`{
		"result": {
			"response": {
				"token": "news",
				"isThinking": false,
				"webSearchResults": {
					"results": [
						{"url": "https://example.com/a", "title": "Article A"},
						{"url": "", "title": "No URL"}
					]
				},
				"xSearchResults": {
					"results": [
						{"postId": "123456", "username": "elonmusk", "text": "Just announced something big"},
						{"postId": "789012", "username": "xai", "text": ""},
						{"postId": "", "username": "missing", "text": "skipped"}
					]
				}
			}
		}
	}`))
	if event.Error != nil {
		t.Fatalf("parseGrokEvent() error = %v", event.Error)
	}
	if event.Content != "news" {
		t.Fatalf("Content = %q, want news", event.Content)
	}
	if len(event.SearchSources) != 3 {
		t.Fatalf("SearchSources length = %d, want 3", len(event.SearchSources))
	}
	if event.SearchSources[0].URL != "https://example.com/a" || event.SearchSources[0].Title != "Article A" || event.SearchSources[0].Type != "web" {
		t.Fatalf("first source = %#v, want web Article A", event.SearchSources[0])
	}
	if event.SearchSources[1].URL != "https://x.com/elonmusk/status/123456" || event.SearchSources[1].Type != "x_post" {
		t.Fatalf("second source = %#v, want x_post URL", event.SearchSources[1])
	}
	if event.SearchSources[2].Title != "\U0001D54F/@xai" {
		t.Fatalf("third source title = %q, want X fallback", event.SearchSources[2].Title)
	}
}

func cardAttachmentEvent(cardJSON string) json.RawMessage {
	data := fmt.Sprintf(`{"result":{"response":{"token":"","isThinking":false,"cardAttachment":{"jsonData":%s}}}}`, strconv.Quote(cardJSON))
	return json.RawMessage(data)
}

func TestDownloadURL_UsesAuthenticatedHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "sso=tok-1" {
			t.Errorf("Cookie = %q", r.Header.Get("Cookie"))
		}
		_, _ = io.WriteString(w, "asset-body")
	}))
	defer srv.Close()

	g := New("", &upstream.StdlibDoer{Client: srv.Client()}, Options{
		BuildCookieString: func(tok string) string { return "sso=" + tok },
	})
	got, err := g.downloadURL(context.Background(), "tok-1", srv.URL)
	if err != nil {
		t.Fatalf("downloadURL: %v", err)
	}
	if string(got) != "asset-body" {
		t.Errorf("body=%q", string(got))
	}
}

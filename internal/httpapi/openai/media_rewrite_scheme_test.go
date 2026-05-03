package openai

import (
	"context"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
)

func TestMediaRewriter_RewritesSchemeRelativeMarkdownImages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantURL string
	}{
		{
			name:    "assets host",
			content: "![x](//assets.grok.com/cards/id/image.png)",
			wantURL: "https://assets.grok.com/cards/id/image.png",
		},
		{
			name:    "grok image host",
			content: "![x](//grok.com/images/123e4567-e89b-12d3-a456-426614174000.png)",
			wantURL: "https://grok.com/images/123e4567-e89b-12d3-a456-426614174000.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL := ""
			rewriter := newMediaRewriter(func(_ context.Context, rawURL string) ([]byte, error) {
				gotURL = rawURL
				return testPNGBytes(), nil
			})

			got, err := rewriteContent(rewriter, context.Background(), tt.content)
			if err != nil {
				t.Fatalf("rewriteContent() error = %v", err)
			}
			if gotURL != tt.wantURL {
				t.Fatalf("download URL = %q, want %q", gotURL, tt.wantURL)
			}
			if strings.Contains(got, tt.content) || !strings.Contains(got, "data:image/png;base64,") {
				t.Fatalf("rewritten content = %q", got)
			}
		})
	}
}

func TestRenderImagesForChatRejectsSchemeRelativeUpstreamURL(t *testing.T) {
	h := &Handler{}
	target := "//assets.grok.com/users/u/generated/id/image.png"

	content, err := h.renderImagesForChat(&flow.ImageResponse{
		Data: []flow.ImageData{{URL: target}},
	})
	if err == nil {
		t.Fatal("renderImagesForChat() expected error")
	}
	if content != "" {
		t.Fatalf("renderImagesForChat() content = %q, want empty on error", content)
	}
}

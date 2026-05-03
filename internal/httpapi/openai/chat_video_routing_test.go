package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderVideoForChatRequiresLocalVideoCacheURL(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Host = "api.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")

	got, err := h.renderVideoForChat(req, "/api/files/video/video-1.mp4")
	if err != nil {
		t.Fatalf("renderVideoForChat() error = %v", err)
	}
	if got != "https://api.example.test/api/files/video/video-1.mp4" {
		t.Fatalf("renderVideoForChat() = %q", got)
	}

	tests := []string{
		"https://assets.grok.com/users/u/generated/id/video.mp4",
		"https://grok.com/generated/id/video.mp4",
		"/api/files/image/image-1.png",
		"/api/files/video/",
		"/api/files/video/../secret.mp4",
		"/api/files/video/video.mp4?upstream=https://assets.grok.com/video.mp4",
		"/api/files/video/video.mp4#fragment",
		"",
	}
	for _, videoURL := range tests {
		t.Run(videoURL, func(t *testing.T) {
			got, err := h.renderVideoForChat(req, videoURL)
			if err == nil {
				t.Fatal("renderVideoForChat() expected error")
			}
			if got != "" {
				t.Fatalf("renderVideoForChat() = %q, want empty on error", got)
			}
			if videoURL != "" && strings.Contains(err.Error(), videoURL) {
				t.Fatalf("error leaked video URL: %v", err)
			}
		})
	}
}

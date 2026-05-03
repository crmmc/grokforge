package xai

import "testing"

func TestNormalizeAssetURLCanonicalizesGrokHTTP(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "relative asset",
			in:   "users/u/generated/id/video.mp4",
			want: "https://assets.grok.com/users/u/generated/id/video.mp4",
		},
		{
			name: "assets grok http",
			in:   "http://assets.grok.com/users/u/generated/id/video.mp4?token=1",
			want: "https://assets.grok.com/users/u/generated/id/video.mp4?token=1",
		},
		{
			name: "grok http",
			in:   "http://grok.com/images/id.png",
			want: "https://grok.com/images/id.png",
		},
		{
			name: "scheme relative asset",
			in:   "//assets.grok.com/users/u/generated/id/video.mp4",
			want: "https://assets.grok.com/users/u/generated/id/video.mp4",
		},
		{
			name: "uppercase assets http",
			in:   "HTTP://assets.grok.com/users/u/generated/id/video.mp4",
			want: "https://assets.grok.com/users/u/generated/id/video.mp4",
		},
		{
			name: "uppercase assets https",
			in:   "HTTPS://assets.grok.com/users/u/generated/id/video.mp4",
			want: "https://assets.grok.com/users/u/generated/id/video.mp4",
		},
		{
			name: "external http unchanged",
			in:   "http://example.com/video.mp4",
			want: "http://example.com/video.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAssetURL(tt.in); got != tt.want {
				t.Fatalf("normalizeAssetURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

package openai

import (
	"context"
	"testing"
)

func TestRewriteContent_NilRewriter_BareGrokHost(t *testing.T) {
	for _, content := range []string{
		"see grok.com/img/abc/1.png",
		"see assets.grok.com/users/x/generated/y.png",
		"see static.grok.com/image.png",
	} {
		result, err := rewriteContent(nil, context.Background(), content)
		if err != nil {
			t.Fatalf("unexpected error for bare Grok host %q: %v", content, err)
		}
		if result != content {
			t.Errorf("expected original content, got %s", result)
		}
	}
}

func TestIsGrokMediaURLTarget(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		// Grok domains with media paths
		{"https://assets.grok.com/users/x/generated/y.png", true},
		{"https://assets.grok.com/users/x/file/content", true},
		{"https://grok.com/users/x/generated/y.png", true},
		{"https://grok.com/img/abc/1.png", true},
		// imagine-public.grok.com is media-only subdomain
		{"https://imagine-public.grok.com/gen/abc", true},
		{"https://imagine-public.grok.com/any/path", true},
		// Grok domains without media paths
		{"https://grok.com/about", false},
		{"https://grok.com/pricing", false},
		{"https://assets.grok.com/logo.svg", false},
		{"https://assets.grok.com/", false},
		// Non-Grok URLs
		{"https://example.com/image.png", false},
		{"https://evil.example/assets.grok.com/path", false},
		// Invalid URLs
		{"not-a-url", false},
		{"ftp://grok.com/path", false},
		{"grok.com/img/abc", false}, // no scheme
	}
	for _, tt := range tests {
		got := isGrokMediaURLTarget(tt.target)
		if got != tt.want {
			t.Errorf("isGrokMediaURLTarget(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestIsGrokImageHostByTarget(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"https://assets.grok.com/path", true},
		{"https://grok.com/img/abc", true},
		{"https://sub.grok.com/path", true},
		{"https://example.com/path", false},
		{"https://evil.example/assets.grok.com/path", false},
		{"not-a-url", false},
		{"ftp://grok.com/path", false},
	}
	for _, tt := range tests {
		got := isGrokImageHostByTarget(tt.target)
		if got != tt.want {
			t.Errorf("isGrokImageHostByTarget(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestIsRelativeImageTarget(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"users/xxx/generated/yyy.png", true},
		{"/users/xxx/generated/yyy.png", true},
		{"users/u/file/content", true},
		{"users/u/file/content?foo=bar", true},
		{"users/u/file/other", false},
		{"other/path", false},
	}
	for _, tt := range tests {
		got := isRelativeImageTarget(tt.target)
		if got != tt.want {
			t.Errorf("isRelativeImageTarget(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestContainsGrokImageReference(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"clean", "hello world", false},
		{"markdown relative", "![img](users/x/generated/y.png)", true},
		{"markdown absolute", "![img](https://assets.grok.com/x)", true},
		{"plain text url with media path", "see https://assets.grok.com/users/x/generated/y.png", true},
		{"plain text grok img url", "see https://grok.com/img/abc/1.png", true},
		{"plain text url without media path", "see https://assets.grok.com/x", false},
		{"plain text bare grok", "see grok.com/img/abc/1.png", false},
		{"plain text bare assets", "see assets.grok.com/x", false},
		{"plain text bare subdomain", "see static.grok.com/x", false},
		{"plain text bare host punctuation", "see grok.com, then stop", false},
		{"plain text embedded host", "see notgrok.com/x", false},
		{"plain text relative", "users/u/file/content", true},
		{"plain text root relative generated", "/users/x/generated/y.png", true},
		{"plain text non grok url generated path", "see https://example.com/users/x/generated/y.png", false},
		{"plain text non grok url content path", "see https://example.com/users/x/file/content", false},
		{"plain text local filesystem generated path", "backup /var/www/users/x/generated/y.png", false},
		{"plain text embedded relative marker", "abcusers/x/generated/y.png", false},
		{"imagine-public media url", "https://imagine-public.grok.com/gen/abc", true},
		{"imagine-public non-grok", "https://imagine-public.evil.example/a.png", false},
		{"non-grok url", "https://example.com/image.png", false},
		{"data uri", "![x](data:image/png;base64,abc)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsGrokImageReference(tt.content)
			if got != tt.want {
				t.Errorf("containsGrokImageReference(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

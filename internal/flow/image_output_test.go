package flow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
)

func TestResolveBase64_FromB64JSON(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig {
		return &config.ImageConfig{Format: "base64"}
	}}
	data, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		B64JSON: "aGVsbG8=",
		Prompt:  "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.B64JSON != "aGVsbG8=" {
		t.Errorf("expected b64json aGVsbG8=, got %s", data.B64JSON)
	}
	if data.RevisedPrompt != "test" {
		t.Errorf("expected prompt test, got %s", data.RevisedPrompt)
	}
}

func TestResolveBase64_FromRawURL(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig {
		return &config.ImageConfig{Format: "base64"}
	}}
	dl := func(ctx context.Context, url string) ([]byte, error) {
		return []byte("png-data"), nil
	}
	data, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		RawURL:   "https://assets.grok.com/test.png",
		Prompt:   "prompt",
		Download: dl,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.B64JSON == "" {
		t.Error("expected non-empty b64json")
	}
}

func TestResolveBase64_DownloadFailure(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig {
		return &config.ImageConfig{Format: "base64"}
	}}
	dl := func(ctx context.Context, url string) ([]byte, error) {
		return nil, errors.New("network error")
	}
	_, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		RawURL:   "https://assets.grok.com/test.png",
		Download: dl,
	})
	if err == nil {
		t.Fatal("expected error for download failure")
	}
	if strings.Contains(err.Error(), "assets.grok.com") {
		t.Error("error must not contain grok URL")
	}
}

func TestResolveBase64_NilDownload(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig {
		return &config.ImageConfig{Format: "base64"}
	}}
	_, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		RawURL: "https://assets.grok.com/test.png",
	})
	if err == nil {
		t.Fatal("expected error for nil download")
	}
}

func TestResolveLocalURL_FromB64JSON(t *testing.T) {
	cacheSvc := cache.NewService(t.TempDir())
	f := &ImageFlow{
		imageConfigFn: func() *config.ImageConfig {
			return &config.ImageConfig{Format: "local_url"}
		},
		cacheSvc: cacheSvc,
	}
	data, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		B64JSON: "iVBORw0KGgo=", // minimal PNG header
		Prompt:  "local test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(data.URL, "/api/files/image/") {
		t.Errorf("expected /api/files/image/ prefix, got %s", data.URL)
	}
	if strings.Contains(data.URL, "grok.com") {
		t.Error("URL must not contain grok.com")
	}
}

func TestResolveLocalURL_FromRawURL(t *testing.T) {
	cacheSvc := cache.NewService(t.TempDir())
	dl := func(ctx context.Context, url string) ([]byte, error) {
		return []byte("png-bytes"), nil
	}
	f := &ImageFlow{
		imageConfigFn: func() *config.ImageConfig {
			return &config.ImageConfig{Format: "local_url"}
		},
		cacheSvc: cacheSvc,
	}
	data, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		RawURL:   "https://assets.grok.com/test.png",
		Download: dl,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(data.URL, "/api/files/image/") {
		t.Errorf("expected /api/files/image/ prefix, got %s", data.URL)
	}
}

func TestResolveLocalURL_NilCacheSvc(t *testing.T) {
	f := &ImageFlow{
		imageConfigFn: func() *config.ImageConfig {
			return &config.ImageConfig{Format: "local_url"}
		},
		cacheSvc: nil,
	}
	_, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		B64JSON: "aGVsbG8=",
	})
	if err == nil {
		t.Fatal("expected error for nil cache service")
	}
}

func TestResolveImageOutput_UnsupportedFormat(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig {
		return &config.ImageConfig{Format: "grok_url"}
	}}
	_, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		B64JSON: "test",
	})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestResolveImageOutput_DefaultBase64(t *testing.T) {
	f := &ImageFlow{imageConfigFn: func() *config.ImageConfig { return nil }}
	data, err := f.resolveImageOutput(context.Background(), imageOutputInput{
		B64JSON: "dGVzdA==",
		Prompt:  "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.B64JSON != "dGVzdA==" {
		t.Errorf("expected b64json, got %s", data.B64JSON)
	}
}

func TestResolveImageOutput_NoLeakInURL(t *testing.T) {
	cacheSvc := cache.NewService(t.TempDir())
	dl := func(ctx context.Context, url string) ([]byte, error) {
		return []byte("data"), nil
	}
	formats := []string{"base64", "local_url"}
	for _, format := range formats {
		f := &ImageFlow{
			imageConfigFn: func() *config.ImageConfig {
				return &config.ImageConfig{Format: format}
			},
			cacheSvc: cacheSvc,
		}
		data, err := f.resolveImageOutput(context.Background(), imageOutputInput{
			RawURL:   "https://assets.grok.com/users/x/generated/y.png",
			Download: dl,
			Prompt:   "leak test",
		})
		if err != nil {
			t.Fatalf("format %s: unexpected error: %v", format, err)
		}
		for _, forbidden := range []string{"assets.grok.com", "grok.com", "users/", "generated/"} {
			if strings.Contains(data.URL, forbidden) {
				t.Errorf("format %s: URL contains forbidden %q: %s", format, forbidden, data.URL)
			}
		}
	}
}

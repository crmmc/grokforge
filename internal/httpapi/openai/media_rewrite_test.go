package openai

import (
	"context"
	"errors"
	"strings"
	"testing"

)

// mockDownloadFunc tracks calls and returns preset data.
type mockDownloadFunc struct {
	data      []byte
	err       error
	called    bool
	calledURL string
}

func (m *mockDownloadFunc) call(ctx context.Context, url string) ([]byte, error) {
	m.called = true
	m.calledURL = url
	return m.data, m.err
}

func TestRewriteContent_NilRewriter_NoImages(t *testing.T) {
	result, err := rewriteContent(nil, context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected original content, got %s", result)
	}
}

func TestRewriteContent_NilRewriter_WithGrokImage(t *testing.T) {
	content := "![img](users/xxx/generated/yyy.png)"
	result, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error for grok image with nil rewriter: %v", err)
	}
	if result != content {
		t.Errorf("expected original content, got %s", result)
	}
}

func TestRewriteContent_NilRewriter_PlainTextGrokURL(t *testing.T) {
	content := "check this https://assets.grok.com/users/x/generated/y.png"
	result, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error for plain text grok URL with nil rewriter: %v", err)
	}
	if result != content {
		t.Errorf("expected original content, got %s", result)
	}
}

func TestRewriteContent_NilRewriter_PlainTextGrokHost(t *testing.T) {
	content := "see https://grok.com/img/abc/1.png here"
	result, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error for grok.com host with nil rewriter: %v", err)
	}
	if result != content {
		t.Errorf("expected original content, got %s", result)
	}
}

func TestRewriteContent_NilRewriter_RelativePath(t *testing.T) {
	content := "path: users/u/file/content"
	result, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error for relative path with nil rewriter: %v", err)
	}
	if result != content {
		t.Errorf("expected original content, got %s", result)
	}
}

func TestRewriteContent_NilRewriter_ImaginePublic(t *testing.T) {
	content := "see https://imagine-public.evil.example/a.png"
	result, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error for imagine-public with nil rewriter: %v", err)
	}
	if result != content {
		t.Errorf("expected original content, got %s", result)
	}
}

func TestRewriteContent_Base64_RelativePath(t *testing.T) {
	// PNG magic bytes so http.DetectContentType returns "image/png"
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	dl := &mockDownloadFunc{data: pngData}
	rewriter := newMediaRewriter(dl.call)
	result, err := rewriteContent(rewriter, context.Background(),
		"![img](users/xxx/generated/yyy.png)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "![img](data:image/") {
		t.Errorf("expected data URI, got %s", result)
	}
	if !dl.called {
		t.Error("downloader was not called")
	}
	if !strings.HasPrefix(dl.calledURL, "https://assets.grok.com/") {
		t.Errorf("expected assets.grok.com URL, got %s", dl.calledURL)
	}
}

func TestRewriteContent_Base64_AbsoluteGrokURL(t *testing.T) {
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	dl := &mockDownloadFunc{data: pngData}
	rewriter := newMediaRewriter(dl.call)
	result, err := rewriteContent(rewriter, context.Background(),
		"![img](https://assets.grok.com/users/xxx/generated/yyy.png)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "![img](data:image/") {
		t.Errorf("expected data URI, got %s", result)
	}
}

func TestRewriteContent_Base64_ContentPath(t *testing.T) {
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	dl := &mockDownloadFunc{data: pngData}
	rewriter := newMediaRewriter(dl.call)
	result, err := rewriteContent(rewriter, context.Background(),
		"![img](users/u/file/content)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "![img](data:image/") {
		t.Errorf("expected data URI, got %s", result)
	}
}




func TestRewriteContent_DownloadFailure(t *testing.T) {
	dl := &mockDownloadFunc{err: errors.New("network error")}
	rewriter := newMediaRewriter(dl.call)
	_, err := rewriteContent(rewriter, context.Background(),
		"![img](users/xxx/generated/yyy.png)")
	if err == nil {
		t.Fatal("expected error for download failure")
	}
}

func TestRewriteContent_NonGrokImage_Preserved(t *testing.T) {
	dl := &mockDownloadFunc{data: []byte("data")}
	rewriter := newMediaRewriter(dl.call)
	result, err := rewriteContent(rewriter, context.Background(),
		"![x](https://example.com/image.png)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "![x](https://example.com/image.png)" {
		t.Errorf("expected original preserved, got %s", result)
	}
	if dl.called {
		t.Error("downloader should not be called for non-grok images")
	}
}

func TestRewriteContent_DataURI_Preserved(t *testing.T) {
	dl := &mockDownloadFunc{data: []byte("data")}
	rewriter := newMediaRewriter(dl.call)
	result, err := rewriteContent(rewriter, context.Background(),
		"![x](data:image/png;base64,abc123)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "![x](data:image/png;base64,abc123)" {
		t.Errorf("expected data URI preserved, got %s", result)
	}
}

func TestRewriteContent_LocalAPIFiles_Preserved(t *testing.T) {
	dl := &mockDownloadFunc{data: []byte("data")}
	rewriter := newMediaRewriter(dl.call)
	result, err := rewriteContent(rewriter, context.Background(),
		"![x](/api/files/image/test.png)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "![x](/api/files/image/test.png)" {
		t.Errorf("expected local API path preserved, got %s", result)
	}
}

func TestRewriteContent_Base64_ReasoningContent_GrokURL(t *testing.T) {
	// Plain-text Grok URLs (not markdown images) are not rewritten by
	// Rewrite() which only handles markdown image syntax. The final
	// containsGrokImageReference gate catches them and returns error.
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	dl := &mockDownloadFunc{data: pngData}
	rewriter := newMediaRewriter(dl.call)
	_, err := rewriteContent(rewriter, context.Background(),
		"check https://assets.grok.com/users/x/generated/y.png")
	if err == nil {
		t.Fatal("expected error for plain-text Grok URL in reasoning")
	}
}

func TestRewriteContent_NilRewriter_ReasoningContent_GrokURL(t *testing.T) {
	content := "check https://assets.grok.com/users/x/generated/y.png"
	result, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error for grok URL in reasoning with nil rewriter: %v", err)
	}
	if result != content {
		t.Errorf("expected original content, got %s", result)
	}
}

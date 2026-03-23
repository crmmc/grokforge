package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testSPAHandler() http.Handler {
	fs := fstest.MapFS{
		"index.html":                                    {Data: []byte("<!DOCTYPE html><html><body>app</body></html>")},
		"404.html":                                      {Data: []byte("<!DOCTYPE html><html><body>404</body></html>")},
		"tokens/index.html":                             {Data: []byte("<!DOCTYPE html><html><body>tokens</body></html>")},
		"_next/static/chunks/webpack-4c5ae21e88beec46.js": {Data: []byte("/* chunk */")},
	}
	return NewSPAHandler(fs)
}

func TestSPAHandler_IndexHTML(t *testing.T) {
	handler := testSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") && !strings.Contains(body, "<html") {
		t.Errorf("expected HTML content, got: %s", body[:min(100, len(body))])
	}
}

func TestSPAHandler_StaticFile(t *testing.T) {
	handler := testSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/404.html", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for 404.html, got %d", rec.Code)
	}
}

func TestSPAHandler_DirectoryWithIndex(t *testing.T) {
	handler := testSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/tokens/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for /tokens/, got %d", rec.Code)
	}
}

func TestSPAHandler_CacheHeaders(t *testing.T) {
	handler := testSPAHandler()

	// Hashed static asset should get immutable cache header
	req := httptest.NewRequest(http.MethodGet, "/_next/static/chunks/webpack-4c5ae21e88beec46.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("expected immutable cache for _next/static asset, got %q", cc)
	}

	// HTML should get no-cache
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected no-cache for HTML, got %q", cc)
	}
}

func TestSPAHandler_SPAFallback(t *testing.T) {
	handler := testSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/nonexistent-page", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for SPA fallback, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") && !strings.Contains(body, "<html") {
		t.Errorf("expected HTML content for SPA fallback")
	}
}

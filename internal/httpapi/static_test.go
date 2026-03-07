package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandler_IndexHTML(t *testing.T) {
	handler := SPAHandler()

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
	handler := SPAHandler()

	// Test 404.html exists as static file
	req := httptest.NewRequest(http.MethodGet, "/404.html", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for 404.html, got %d", rec.Code)
	}
}

func TestSPAHandler_DirectoryWithIndex(t *testing.T) {
	handler := SPAHandler()

	// Test directory route (Next.js trailingSlash creates dir/index.html)
	// Request with trailing slash to avoid redirect
	req := httptest.NewRequest(http.MethodGet, "/tokens/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should serve tokens/index.html
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for /tokens/, got %d", rec.Code)
	}
}

func TestSPAHandler_CacheHeaders(t *testing.T) {
	handler := SPAHandler()

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
	handler := SPAHandler()

	// Non-existent path should fallback to index.html (SPA routing)
	// Use a path that won't trigger directory redirect
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

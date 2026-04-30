package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/crmmc/grokforge/internal/cache"
)

func newTestCacheService(t *testing.T) (*cache.Service, string) {
	t.Helper()
	base := t.TempDir()
	return cache.NewService(base, nil), base
}

func createCacheFile(t *testing.T, base, mediaType, name string, size int) {
	t.Helper()
	dir := filepath.Join(base, "tmp", mediaType)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHandleCacheStats(t *testing.T) {
	svc, base := newTestCacheService(t)
	createCacheFile(t, base, "image", "a.jpg", 1024)
	createCacheFile(t, base, "video", "b.mp4", 2048)

	req := httptest.NewRequest("GET", "/admin/cache/stats", nil)
	w := httptest.NewRecorder()
	handleCacheStats(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]cache.Stats
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["image"].Count != 1 {
		t.Errorf("expected image count 1, got %d", resp["image"].Count)
	}
	if resp["video"].Count != 1 {
		t.Errorf("expected video count 1, got %d", resp["video"].Count)
	}
}

func TestHandleCacheFiles(t *testing.T) {
	svc, base := newTestCacheService(t)
	createCacheFile(t, base, "image", "a.jpg", 100)
	createCacheFile(t, base, "image", "b.png", 200)

	req := httptest.NewRequest("GET", "/admin/cache/files?type=image&page=1&page_size=50", nil)
	w := httptest.NewRecorder()
	handleCacheFiles(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp cache.FileListResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
}

func TestHandleCacheFiles_BadType(t *testing.T) {
	svc, _ := newTestCacheService(t)

	req := httptest.NewRequest("GET", "/admin/cache/files?type=invalid", nil)
	w := httptest.NewRecorder()
	handleCacheFiles(svc)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeleteCacheFiles(t *testing.T) {
	svc, base := newTestCacheService(t)
	createCacheFile(t, base, "image", "a.jpg", 100)

	body, _ := json.Marshal(deleteRequest{Type: "image", Names: []string{"a.jpg"}})
	req := httptest.NewRequest("POST", "/admin/cache/delete", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handleDeleteCacheFiles(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp cache.BatchResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success != 1 {
		t.Errorf("expected 1 success, got %d", resp.Success)
	}
}

func TestHandleClearCache(t *testing.T) {
	svc, base := newTestCacheService(t)
	createCacheFile(t, base, "image", "a.jpg", 1024)
	createCacheFile(t, base, "image", "b.png", 2048)

	body, _ := json.Marshal(clearRequest{Type: "image"})
	req := httptest.NewRequest("POST", "/admin/cache/clear", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handleClearCache(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp cache.ClearResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", resp.Deleted)
	}
}

func TestHandleServeCacheFile(t *testing.T) {
	svc, base := newTestCacheService(t)
	createCacheFile(t, base, "image", "test.jpg", 100)

	r := chi.NewRouter()
	r.Get("/admin/cache/files/{type}/{name}", handleServeCacheFile(svc))

	req := httptest.NewRequest("GET", "/admin/cache/files/image/test.jpg", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleServeCacheFile_NotFound(t *testing.T) {
	svc, _ := newTestCacheService(t)

	r := chi.NewRouter()
	r.Get("/admin/cache/files/{type}/{name}", handleServeCacheFile(svc))

	req := httptest.NewRequest("GET", "/admin/cache/files/image/nonexistent.jpg", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

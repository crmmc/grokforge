package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCacheFiles_Extra(t *testing.T) {
	t.Run("defaults and clamping", func(t *testing.T) {
		svc, base := newTestCacheService(t)
		createCacheFile(t, base, "video", "v.mp4", 10)

		tests := []struct {
			name     string
			query    string
			wantPage int
			wantSize int
		}{
			{name: "no params uses defaults", query: "", wantPage: 1, wantSize: 50},
			{name: "invalid page falls back", query: "&page=abc", wantPage: 1, wantSize: 50},
			{name: "page_size over 100 falls back", query: "&page_size=999", wantPage: 1, wantSize: 50},
			{name: "valid params kept", query: "&page=2&page_size=25", wantPage: 2, wantSize: 25},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				handleCacheFiles(svc)(w, httptest.NewRequest(http.MethodGet, "/admin/cache/files?type=video"+tc.query, nil))
				require.Equal(t, http.StatusOK, w.Code)
				var resp struct {
					Total    int `json:"total"`
					Page     int `json:"page"`
					PageSize int `json:"page_size"`
				}
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				assert.Equal(t, tc.wantPage, resp.Page)
				assert.Equal(t, tc.wantSize, resp.PageSize)
			})
		}
	})

	t.Run("list error returns 500", func(t *testing.T) {
		svc, base := newTestCacheService(t)
		// Make the image cache path a regular file so ReadDir fails with a non-NotExist error.
		path := filepath.Join(base, "tmp", "image")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("not a dir"), 0o644))

		w := httptest.NewRecorder()
		handleCacheFiles(svc)(w, httptest.NewRequest(http.MethodGet, "/admin/cache/files?type=image", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleDeleteCacheFiles_Extra(t *testing.T) {
	svc, _ := newTestCacheService(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    string
	}{
		{name: "invalid json", body: "{bad", wantStatus: http.StatusBadRequest, wantErr: "invalid request body"},
		{name: "bad type", body: `{"type":"audio","names":["a.mp3"]}`, wantStatus: http.StatusBadRequest, wantErr: "type must be image or video"},
		{name: "empty names", body: `{"type":"image","names":[]}`, wantStatus: http.StatusBadRequest, wantErr: "names must not be empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleDeleteCacheFiles(svc)(w, httptest.NewRequest(http.MethodPost, "/admin/cache/delete", strings.NewReader(tc.body)))
			require.Equal(t, tc.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tc.wantErr)
		})
	}
}

func TestHandleClearCache_Extra(t *testing.T) {
	svc, _ := newTestCacheService(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    string
	}{
		{name: "invalid json", body: "{bad", wantStatus: http.StatusBadRequest, wantErr: "invalid request body"},
		{name: "bad type", body: `{"type":"audio"}`, wantStatus: http.StatusBadRequest, wantErr: "type must be image or video"},
		{name: "video type ok", body: `{"type":"video"}`, wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleClearCache(svc)(w, httptest.NewRequest(http.MethodPost, "/admin/cache/clear", strings.NewReader(tc.body)))
			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantErr != "" {
				assert.Contains(t, w.Body.String(), tc.wantErr)
			}
		})
	}
}

func TestHandleServeCacheFile_Extra(t *testing.T) {
	t.Run("download flag sets content disposition", func(t *testing.T) {
		svc, base := newTestCacheService(t)
		createCacheFile(t, base, "image", "pic.jpg", 64)

		r := chi.NewRouter()
		r.Get("/admin/cache/files/{type}/{name}", handleServeCacheFile(svc))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/cache/files/image/pic.jpg?download=true", nil))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	})
}

func TestHandleServeCacheFileByType_Extra(t *testing.T) {
	t.Run("not found returns 404", func(t *testing.T) {
		svc, _ := newTestCacheService(t)
		r := chi.NewRouter()
		r.Get("/api/files/video/{name}", handleServeCacheFileByType(svc, "video"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/files/video/missing.mp4", nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("serves video with content type", func(t *testing.T) {
		svc, base := newTestCacheService(t)
		createCacheFile(t, base, "video", "clip.mp4", 32)

		r := chi.NewRouter()
		r.Get("/api/files/video/{name}", handleServeCacheFileByType(svc, "video"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/files/video/clip.mp4", nil))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
	})
}

func TestHandleServeCacheFileByType_DownloadFlag(t *testing.T) {
	svc, base := newTestCacheService(t)
	createCacheFile(t, base, "video", "dl.mp4", 16)

	r := chi.NewRouter()
	r.Get("/api/files/video/{name}", handleServeCacheFileByType(svc, "video"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/files/video/dl.mp4?download=true", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
}

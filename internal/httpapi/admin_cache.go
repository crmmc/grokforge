package httpapi

import (
	"encoding/json"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/crmmc/grokforge/internal/cache"
)

// handleCacheStats returns cache statistics for image and video types.
func handleCacheStats(svc *cache.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]cache.Stats{
			"image": svc.GetStats("image"),
			"video": svc.GetStats("video"),
		})
	}
}

// handleCacheFiles returns a paginated list of cached files.
func handleCacheFiles(svc *cache.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaType := r.URL.Query().Get("type")
		if mediaType != "image" && mediaType != "video" {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be image or video"})
			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if pageSize < 1 || pageSize > 100 {
			pageSize = 50
		}

		result, err := svc.ListFiles(mediaType, page, pageSize)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, result)
	}
}

// deleteRequest is the request body for cache delete.
type deleteRequest struct {
	Type  string   `json:"type"`
	Names []string `json:"names"`
}

// handleDeleteCacheFiles deletes specified cache files.
func handleDeleteCacheFiles(svc *cache.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req deleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Type != "image" && req.Type != "video" {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be image or video"})
			return
		}
		if len(req.Names) == 0 {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "names must not be empty"})
			return
		}

		result := svc.DeleteFiles(req.Type, req.Names)
		WriteJSON(w, http.StatusOK, result)
	}
}

// clearRequest is the request body for cache clear.
type clearRequest struct {
	Type string `json:"type"`
}

// handleClearCache clears all cached files of the specified type.
func handleClearCache(svc *cache.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req clearRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Type != "image" && req.Type != "video" {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be image or video"})
			return
		}

		result := svc.Clear(req.Type)
		WriteJSON(w, http.StatusOK, result)
	}
}

// handleServeCacheFile serves a cached file for preview or download.
func handleServeCacheFile(svc *cache.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaType := chi.URLParam(r, "type")
		name := chi.URLParam(r, "name")

		filePath, err := svc.FilePath(mediaType, name)
		if err != nil {
			WriteJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}

		ct := mime.TypeByExtension(filepath.Ext(name))
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}

		if r.URL.Query().Get("download") == "true" {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		}

		http.ServeFile(w, r, filePath)
	}
}

// handleServeCacheFileByType serves cached files for fixed media type routes.
func handleServeCacheFileByType(svc *cache.Service, mediaType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")

		filePath, err := svc.FilePath(mediaType, name)
		if err != nil {
			WriteJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}

		ct := mime.TypeByExtension(filepath.Ext(name))
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}

		if r.URL.Query().Get("download") == "true" {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		}

		http.ServeFile(w, r, filePath)
	}
}

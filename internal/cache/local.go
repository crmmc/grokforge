// Package cache provides file system cache management for image and video files.
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Whitelisted extensions per media type.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".bmp": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".m4v": true,
	".webm": true, ".avi": true, ".mkv": true,
}

// Stats holds cache statistics for a media type.
type Stats struct {
	Count  int     `json:"count"`
	SizeMB float64 `json:"size_mb"`
}

// FileInfo holds metadata for a single cached file.
type FileInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	ModTimeMs int64  `json:"mod_time_ms"`
}

// FileListResult holds paginated file list.
type FileListResult struct {
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Items    []FileInfo `json:"items"`
}

// BatchResult holds results of batch delete.
type BatchResult struct {
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// ClearResult holds results of cache clear.
type ClearResult struct {
	Deleted int     `json:"deleted"`
	FreedMB float64 `json:"freed_mb"`
}

// Service manages file system cache operations.
type Service struct {
	dataDir string
}

// NewService creates a new cache service.
// dataDir is the base data directory (e.g. "data").
// Cache files are expected at {dataDir}/tmp/image and {dataDir}/tmp/video.
func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

// dir returns the cache directory for the given media type.
func (s *Service) dir(mediaType string) string {
	return filepath.Join(s.dataDir, "tmp", mediaType)
}

// exts returns the extension whitelist for the given media type.
func exts(mediaType string) map[string]bool {
	switch mediaType {
	case "image":
		return imageExts
	case "video":
		return videoExts
	default:
		return nil
	}
}

// isWhitelisted checks if the file extension is allowed for the media type.
func isWhitelisted(name, mediaType string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	wl := exts(mediaType)
	return wl != nil && wl[ext]
}

// GetStats returns cache statistics for the given media type.
// Returns empty Stats if the directory doesn't exist.
func (s *Service) GetStats(mediaType string) Stats {
	dir := s.dir(mediaType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Stats{}
	}

	var count int
	var totalBytes int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !isWhitelisted(e.Name(), mediaType) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		count++
		totalBytes += info.Size()
	}

	return Stats{
		Count:  count,
		SizeMB: float64(totalBytes) / (1024 * 1024),
	}
}

// ListFiles returns a paginated list of cached files sorted by modification time (newest first).
func (s *Service) ListFiles(mediaType string, page, pageSize int) (*FileListResult, error) {
	dir := s.dir(mediaType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileListResult{Total: 0, Page: page, PageSize: pageSize}, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var files []FileInfo
	for _, e := range entries {
		if e.IsDir() || !isWhitelisted(e.Name(), mediaType) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:      e.Name(),
			SizeBytes: info.Size(),
			ModTimeMs: info.ModTime().UnixMilli(),
		})
	}

	// Sort by mod time descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTimeMs > files[j].ModTimeMs
	})

	total := len(files)
	start := (page - 1) * pageSize
	if start >= total {
		return &FileListResult{Total: total, Page: page, PageSize: pageSize}, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return &FileListResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Items:    files[start:end],
	}, nil
}

// validateName ensures the filename is safe (no path traversal, no directory separators).
func validateName(name, mediaType string) error {
	if name == "" {
		return errors.New("empty filename")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid filename: contains path separator")
	}
	cleaned := filepath.Base(name)
	if cleaned != name || cleaned == "." || cleaned == ".." {
		return fmt.Errorf("invalid filename: %q", name)
	}
	if !isWhitelisted(name, mediaType) {
		return fmt.Errorf("invalid extension for %s: %q", mediaType, name)
	}
	return nil
}

// DeleteFile removes a single cached file.
func (s *Service) DeleteFile(mediaType, name string) error {
	if err := validateName(name, mediaType); err != nil {
		return err
	}
	path := filepath.Join(s.dir(mediaType), name)
	return os.Remove(path)
}

// DeleteFiles removes multiple cached files and returns success/failure counts.
func (s *Service) DeleteFiles(mediaType string, names []string) *BatchResult {
	result := &BatchResult{}
	for _, name := range names {
		if err := s.DeleteFile(mediaType, name); err != nil {
			result.Failed++
		} else {
			result.Success++
		}
	}
	return result
}

// Clear removes all whitelisted files from the cache directory.
func (s *Service) Clear(mediaType string) *ClearResult {
	dir := s.dir(mediaType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return &ClearResult{}
	}

	result := &ClearResult{}
	for _, e := range entries {
		if e.IsDir() || !isWhitelisted(e.Name(), mediaType) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		size := info.Size()
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			continue // skip errors for concurrent access safety
		}
		result.Deleted++
		result.FreedMB += float64(size) / (1024 * 1024)
	}
	return result
}

// SaveFile writes data to a new UUID-named file in the cache directory for the given media type.
// ext must include the leading dot (e.g. ".png", ".mp4").
// Returns the generated filename (not the full path).
func (s *Service) SaveFile(mediaType string, data []byte, ext string) (string, error) {
	filename := uuid.New().String() + ext
	if err := validateName(filename, mediaType); err != nil {
		return "", err
	}
	dir := s.dir(mediaType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return filename, nil
}

// SaveStream writes reader content to a new UUID-named file.
func (s *Service) SaveStream(mediaType string, reader io.Reader, ext string) (string, error) {
	filename := uuid.New().String() + ext
	if err := validateName(filename, mediaType); err != nil {
		return "", err
	}
	dir := s.dir(mediaType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	path := filepath.Join(dir, filename)
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, reader); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return filename, nil
}

// FilePath returns the absolute path to a cached file after validation.
func (s *Service) FilePath(mediaType, name string) (string, error) {
	if err := validateName(name, mediaType); err != nil {
		return "", err
	}
	path := filepath.Join(s.dir(mediaType), name)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

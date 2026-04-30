// Package cache provides file system cache management for image and video files.
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/google/uuid"
)

// NewService creates a new cache service.
// dataDir is the base data directory (e.g. "data").
// Cache files are expected at {dataDir}/tmp/image and {dataDir}/tmp/video.
// runtime can be nil (all limits disabled).
func NewService(dataDir string, runtime *config.Runtime) *Service {
	s := &Service{
		dataDir: dataDir,
		sizeMap: make(map[string]int64),
		runtime: runtime,
	}
	s.reconcile()
	return s
}

// dir returns the cache directory for the given media type.
func (s *Service) dir(mediaType string) string {
	return filepath.Join(s.dataDir, "tmp", mediaType)
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

	size := int64(len(data))
	limit := s.limitFor(mediaType)
	if limit > 0 && size > limit {
		return "", fmt.Errorf("%w: %d bytes exceeds %s limit %d", ErrFileTooLarge, size, mediaType, limit)
	}

	tmpName := ".tmp-" + uuid.New().String() + ext
	tmpPath := filepath.Join(dir, tmpName)
	finalPath := filepath.Join(dir, filename)

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	defer func() {
		if _, err := os.Stat(tmpPath); err == nil {
			os.Remove(tmpPath)
		}
	}()

	if err := s.publishFile(mediaType, tmpPath, finalPath, filename, size); err != nil {
		return "", err
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

	tmpName := ".tmp-" + uuid.New().String() + ext
	tmpPath := filepath.Join(dir, tmpName)
	finalPath := filepath.Join(dir, filename)

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if _, err := os.Stat(tmpPath); err == nil {
			os.Remove(tmpPath)
		}
	}()

	limit := s.limitFor(mediaType)
	size, err := copyWithLimit(tmpFile, reader, limit)
	if err != nil {
		tmpFile.Close()
		if errors.Is(err, ErrFileTooLarge) {
			closeReader(reader)
		}
		return "", fmt.Errorf("write file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close file: %w", err)
	}

	if err := s.publishFile(mediaType, tmpPath, finalPath, filename, size); err != nil {
		return "", err
	}

	return filename, nil
}

// FilePath returns the absolute path to a cached file after validation.
// It also touches the file's mtime to support LRU eviction ordering.
func (s *Service) FilePath(mediaType, name string) (string, error) {
	if err := validateName(name, mediaType); err != nil {
		return "", err
	}
	path := filepath.Join(s.dir(mediaType), name)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	// Touch mtime so recently accessed files survive LRU eviction
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		return "", fmt.Errorf("touch file: %w", err)
	}
	return path, nil
}

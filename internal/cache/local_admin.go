package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// GetStats returns cache statistics for the given media type.
// Count is scanned from disk; SizeMB is read from sizeMap.
func (s *Service) GetStats(mediaType string) Stats {
	s.mu.Lock()
	totalBytes := s.sizeMap[mediaType]
	s.mu.Unlock()

	dir := s.dir(mediaType)
	entries, err := os.ReadDir(dir)
	count := 0
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && isCacheFile(e.Name(), mediaType) {
				count++
			}
		}
	}

	limitBytes := s.limitFor(mediaType)
	var limitMB float64
	if limitBytes > 0 {
		limitMB = float64(limitBytes) / bytesPerMiB
	}

	return Stats{
		Count:   count,
		SizeMB:  float64(totalBytes) / bytesPerMiB,
		LimitMB: limitMB,
	}
}

// ListFiles returns a paginated list of cached files sorted by modification time.
func (s *Service) ListFiles(mediaType string, page, pageSize int) (*FileListResult, error) {
	dir := s.dir(mediaType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileListResult{Total: 0, Page: page, PageSize: pageSize}, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	files := listFileInfos(entries, mediaType)
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

func listFileInfos(entries []os.DirEntry, mediaType string) []FileInfo {
	var files []FileInfo
	for _, e := range entries {
		if e.IsDir() || !isCacheFile(e.Name(), mediaType) {
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
	return files
}

// DeleteFile removes a single cached file.
func (s *Service) DeleteFile(mediaType, name string) error {
	if err := validateName(name, mediaType); err != nil {
		return err
	}
	path := filepath.Join(s.dir(mediaType), name)

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteFileLocked(mediaType, path)
}

// DeleteFiles removes multiple cached files and returns success/failure counts.
func (s *Service) DeleteFiles(mediaType string, names []string) *BatchResult {
	result := &BatchResult{}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		if err := validateName(name, mediaType); err != nil {
			result.Failed++
			continue
		}
		path := filepath.Join(s.dir(mediaType), name)
		if err := s.deleteFileLocked(mediaType, path); err != nil {
			result.Failed++
			continue
		}
		result.Success++
	}
	return result
}

func (s *Service) deleteFileLocked(mediaType, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	size := info.Size()
	if err := os.Remove(path); err != nil {
		return err
	}
	s.sizeMap[mediaType] -= size
	if s.sizeMap[mediaType] < 0 {
		s.sizeMap[mediaType] = 0
	}
	return nil
}

// Clear removes all whitelisted files from the cache directory.
func (s *Service) Clear(mediaType string) *ClearResult {
	dir := s.dir(mediaType)
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return &ClearResult{}
	}

	result := &ClearResult{}
	for _, e := range entries {
		clearEntry(mediaType, dir, e, result)
	}
	s.sizeMap[mediaType] = 0
	return result
}

func clearEntry(mediaType, dir string, e os.DirEntry, result *ClearResult) {
	if e.IsDir() || !isCacheFile(e.Name(), mediaType) {
		return
	}
	info, err := e.Info()
	if err != nil {
		return
	}
	size := info.Size()
	if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
		return
	}
	result.Deleted++
	result.FreedMB += float64(size) / bytesPerMiB
}

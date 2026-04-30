package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	bytesPerMiB       = 1024 * 1024
	evictionTargetPct = 0.6
)

// limitFor returns the byte limit for the given media type, or 0 if unlimited.
func (s *Service) limitFor(mediaType string) int64 {
	if s.runtime == nil {
		return 0
	}
	cfg := s.runtime.Get()
	if cfg == nil {
		return 0
	}
	var mb int
	switch mediaType {
	case "image":
		mb = cfg.Cache.ImageMaxMB
	case "video":
		mb = cfg.Cache.VideoMaxMB
	default:
		return 0
	}
	if mb <= 0 {
		return 0
	}
	return int64(mb) * bytesPerMiB
}

func (s *Service) publishFile(mediaType, tmpPath, finalPath, protectedName string, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("rename file: %w", err)
	}
	s.sizeMap[mediaType] += size
	s.evictLocked(mediaType, protectedName)
	return nil
}

func copyWithLimit(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit <= 0 {
		return io.Copy(dst, src)
	}

	limited := &io.LimitedReader{R: src, N: limit + 1}
	written, err := io.Copy(dst, limited)
	if err != nil {
		return written, err
	}
	if written > limit {
		return written, fmt.Errorf("%w: %d bytes exceeds limit %d", ErrFileTooLarge, written, limit)
	}
	return written, nil
}

func closeReader(reader io.Reader) {
	closer, ok := reader.(io.Closer)
	if ok {
		_ = closer.Close()
	}
}

// reconcile scans disk to rebuild sizeMap, cleans up .tmp- residuals, and runs initial eviction.
func (s *Service) reconcile() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, mediaType := range []string{"image", "video"} {
		s.reconcileMediaType(mediaType)
	}
}

func (s *Service) reconcileMediaType(mediaType string) {
	dir := s.dir(mediaType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var total int64
	for _, e := range entries {
		total += sizeForReconcileEntry(dir, mediaType, e)
	}

	s.sizeMap[mediaType] = total
	s.evictLocked(mediaType, "")
}

func sizeForReconcileEntry(dir, mediaType string, e os.DirEntry) int64 {
	if e.IsDir() {
		return 0
	}
	name := e.Name()
	if strings.HasPrefix(name, ".tmp-") {
		_ = os.Remove(filepath.Join(dir, name))
		return 0
	}
	if !isWhitelisted(name, mediaType) {
		return 0
	}
	info, err := e.Info()
	if err != nil {
		return 0
	}
	return info.Size()
}

// evictLocked performs LRU eviction to bring usage down to 60% of the limit.
// Files are sorted by mtime ascending (least recently used first).
// Must be called with s.mu held.
func (s *Service) evictLocked(mediaType, protectedName string) {
	limit := s.limitFor(mediaType)
	if limit <= 0 || s.sizeMap[mediaType] <= limit {
		return
	}
	target := int64(float64(limit) * evictionTargetPct)

	files := s.lruFiles(mediaType, protectedName)
	for _, f := range files {
		if s.sizeMap[mediaType] <= target {
			break
		}
		if err := os.Remove(filepath.Join(s.dir(mediaType), f.Name)); err == nil {
			s.sizeMap[mediaType] -= f.Size
		}
	}
	if s.sizeMap[mediaType] < 0 {
		s.sizeMap[mediaType] = 0
	}
}

type fileEntry struct {
	Name string
	Size int64
	Mod  int64
}

func (s *Service) lruFiles(mediaType, protectedName string) []fileEntry {
	entries, err := os.ReadDir(s.dir(mediaType))
	if err != nil {
		return nil
	}

	var files []fileEntry
	for _, e := range entries {
		file, ok := fileEntryForEviction(e, mediaType, protectedName)
		if ok {
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Mod < files[j].Mod
	})
	return files
}

func fileEntryForEviction(e os.DirEntry, mediaType, protectedName string) (fileEntry, bool) {
	name := e.Name()
	if e.IsDir() || name == protectedName || !isCacheFile(name, mediaType) {
		return fileEntry{}, false
	}
	info, err := e.Info()
	if err != nil {
		return fileEntry{}, false
	}
	return fileEntry{
		Name: name,
		Size: info.Size(),
		Mod:  info.ModTime().UnixNano(),
	}, true
}

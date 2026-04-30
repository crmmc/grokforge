package cache

import (
	"errors"
	"sync"

	"github.com/crmmc/grokforge/internal/config"
)

// ErrFileTooLarge is returned when a single file exceeds the cache limit.
var ErrFileTooLarge = errors.New("file size exceeds cache limit")

// Stats holds cache statistics for a media type.
type Stats struct {
	Count   int     `json:"count"`
	SizeMB  float64 `json:"size_mb"`
	LimitMB float64 `json:"limit_mb"`
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
	mu      sync.Mutex
	sizeMap map[string]int64
	runtime *config.Runtime
}

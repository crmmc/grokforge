package cache

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitFor(t *testing.T) {
	tests := []struct {
		name      string
		runtime   *config.Runtime
		mediaType string
		want      int64
	}{
		{name: "nil runtime means unlimited", runtime: nil, mediaType: "image", want: 0},
		{name: "nil snapshot means unlimited", runtime: &config.Runtime{}, mediaType: "video", want: 0},
		{name: "image limit", runtime: newRuntimeWithLimits(2, 0), mediaType: "image", want: 2 * bytesPerMiB},
		{name: "video limit", runtime: newRuntimeWithLimits(0, 3), mediaType: "video", want: 3 * bytesPerMiB},
		{name: "zero image limit means unlimited", runtime: newRuntimeWithLimits(0, 3), mediaType: "image", want: 0},
		{name: "unknown media type means unlimited", runtime: newRuntimeWithLimits(2, 3), mediaType: "audio", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(t.TempDir(), tc.runtime)
			assert.Equal(t, tc.want, svc.limitFor(tc.mediaType))
		})
	}
}

func TestPublishFile_RenameError(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, nil)
	dir := svc.dir("image")

	// The final path is occupied by a directory, so the rename must fail.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "taken.png"), 0o755))
	tmpPath := filepath.Join(dir, ".tmp-source.png")
	require.NoError(t, os.WriteFile(tmpPath, []byte("x"), 0o644))

	err := svc.publishFile("image", tmpPath, filepath.Join(dir, "taken.png"), "taken.png", 1)
	assert.Error(t, err)
}

func TestCopyWithLimit(t *testing.T) {
	tests := []struct {
		name    string
		limit   int64
		reader  io.Reader
		wantN   int64
		wantErr error
	}{
		{name: "no limit copies everything", limit: 0, reader: strings.NewReader("hello"), wantN: 5},
		{name: "no limit propagates reader error", limit: 0, reader: &failReader{prefix: "abc"}, wantN: 3, wantErr: errStreamBoom},
		{name: "limit exactly met", limit: 5, reader: strings.NewReader("hello"), wantN: 5},
		{name: "limit exceeded", limit: 4, reader: strings.NewReader("hello"), wantN: 5, wantErr: ErrFileTooLarge},
		{name: "limit propagates reader error", limit: 10, reader: &failReader{prefix: "abc"}, wantN: 3, wantErr: errStreamBoom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sink strings.Builder
			n, err := copyWithLimit(&sink, tc.reader, tc.limit)
			assert.Equal(t, tc.wantN, n)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "hello", sink.String())
		})
	}
}

func TestEvictLocked_MissingDir(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, newRuntimeWithLimits(0, 1))
	// No video directory exists on disk; inflate the tracked size to force the
	// eviction path, which must fail soft when the directory cannot be listed.
	svc.mu.Lock()
	svc.sizeMap["video"] = 2 * bytesPerMiB
	svc.evictLocked("video", "")
	tracked := svc.sizeMap["video"]
	svc.mu.Unlock()

	assert.Equal(t, int64(2*bytesPerMiB), tracked)
}

func TestEvictLocked_ClampsNegativeSizeMap(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, newRuntimeWithLimits(0, 1))

	// A file much larger than the tracked size: evicting it drives sizeMap
	// below zero, which must be clamped.
	vidDir := filepath.Join(base, "tmp", "video")
	createTestFile(t, vidDir, "big.mp4", 2*bytesPerMiB)

	svc.mu.Lock()
	svc.sizeMap["video"] = bytesPerMiB + 1024 // just over the 1MiB limit
	svc.evictLocked("video", "")
	tracked := svc.sizeMap["video"]
	svc.mu.Unlock()

	assert.Equal(t, int64(0), tracked)
}

func TestFileEntryForEviction(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")
	createTestFile(t, vidDir, "old.mp4", 10)
	createTestFile(t, vidDir, ".tmp-partial.mp4", 10)
	createTestFile(t, vidDir, "protected.mp4", 10)
	require.NoError(t, os.MkdirAll(filepath.Join(vidDir, "subdir"), 0o755))

	entries, err := os.ReadDir(vidDir)
	require.NoError(t, err)

	evictable := map[string]bool{}
	for _, e := range entries {
		fe, ok := fileEntryForEviction(e, "video", "protected.mp4")
		if ok {
			evictable[fe.Name] = true
		}
	}
	assert.Equal(t, map[string]bool{"old.mp4": true}, evictable)
}

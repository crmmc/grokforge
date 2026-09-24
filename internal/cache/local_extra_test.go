package cache

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errStreamBoom is a distinct reader failure, distinguishable from ErrFileTooLarge.
var errStreamBoom = errors.New("stream boom")

// skipOnRootFS skips permission-based tests when running as root.
func skipOnRootFS(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission-based test requires a non-root user")
	}
}

// blockTmpWithFile turns <base>/tmp into a regular file, so creating any cache
// directory below it fails with ENOTDIR.
func blockTmpWithFile(t *testing.T, base string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(base, "tmp"), []byte("blocker"), 0o644))
}

// readOnlyDir makes dir read-only and restores the mode on cleanup.
func readOnlyDir(t *testing.T, dir string) {
	t.Helper()
	skipOnRootFS(t)
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// failReader yields its prefix bytes, then always returns errStreamBoom.
type failReader struct {
	prefix string
	pos    int
}

func (r *failReader) Read(p []byte) (int, error) {
	if r.pos < len(r.prefix) {
		n := copy(p, r.prefix[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, errStreamBoom
}

func TestSaveFile_ErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		newBase func(t *testing.T) string
		ext     string
		wantMsg string
	}{
		{
			name:    "rejected extension",
			newBase: func(t *testing.T) string { return t.TempDir() },
			ext:     ".txt",
			wantMsg: "invalid extension for image",
		},
		{
			name: "cache dir creation fails",
			newBase: func(t *testing.T) string {
				base := t.TempDir()
				blockTmpWithFile(t, base)
				return base
			},
			ext:     ".png",
			wantMsg: "create cache dir",
		},
		{
			name: "write fails in read-only dir",
			newBase: func(t *testing.T) string {
				base := t.TempDir()
				imgDir := filepath.Join(base, "tmp", "image")
				require.NoError(t, os.MkdirAll(imgDir, 0o755))
				readOnlyDir(t, imgDir)
				return base
			},
			ext:     ".png",
			wantMsg: "write file",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(tc.newBase(t), nil)

			name, err := svc.SaveFile("image", []byte("payload"), tc.ext)
			assert.ErrorContains(t, err, tc.wantMsg)
			assert.Empty(t, name)
		})
	}
}

func TestSaveStream_ErrorPaths(t *testing.T) {
	videoLimit := newRuntimeWithLimits(0, 1)
	tests := []struct {
		name    string
		base    func(t *testing.T) string
		runtime *config.Runtime
		ext     string
		reader  io.Reader
		wantMsg string
		wantIs  error
	}{
		{
			name:    "rejected extension",
			base:    func(t *testing.T) string { return t.TempDir() },
			ext:     ".txt",
			reader:  strings.NewReader("x"),
			wantMsg: "invalid extension for video",
		},
		{
			name: "cache dir creation fails",
			base: func(t *testing.T) string {
				b := t.TempDir()
				blockTmpWithFile(t, b)
				return b
			},
			ext:     ".mp4",
			reader:  strings.NewReader("x"),
			wantMsg: "create cache dir",
		},
		{
			name: "create fails in read-only dir",
			base: func(t *testing.T) string {
				b := t.TempDir()
				vidDir := filepath.Join(b, "tmp", "video")
				require.NoError(t, os.MkdirAll(vidDir, 0o755))
				readOnlyDir(t, vidDir)
				return b
			},
			ext:     ".mp4",
			reader:  strings.NewReader("x"),
			wantMsg: "create file",
		},
		{
			name:    "reader fails without limit",
			base:    func(t *testing.T) string { return t.TempDir() },
			ext:     ".mp4",
			reader:  &failReader{},
			wantMsg: "write file",
			wantIs:  errStreamBoom,
		},
		{
			name:    "reader fails under limit",
			base:    func(t *testing.T) string { return t.TempDir() },
			runtime: videoLimit,
			ext:     ".mp4",
			reader:  &failReader{},
			wantMsg: "write file",
			wantIs:  errStreamBoom,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(tc.base(t), tc.runtime)

			name, err := svc.SaveStream("video", tc.reader, tc.ext)
			assert.ErrorContains(t, err, tc.wantMsg)
			assert.Empty(t, name)
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
			}
		})
	}
}

func TestFilePath_MissingFile(t *testing.T) {
	svc := NewService(t.TempDir(), nil)

	path, err := svc.FilePath("image", "missing.jpg")
	assert.Error(t, err)
	assert.Empty(t, path)
}

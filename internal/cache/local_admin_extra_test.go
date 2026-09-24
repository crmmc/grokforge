package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListFiles_ReadDirError(t *testing.T) {
	base := t.TempDir()
	blockTmpWithFile(t, base)
	svc := NewService(base, nil)

	// ENOTDIR is a dir-read failure distinct from "directory missing".
	result, err := svc.ListFiles("image", 1, 10)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestListFiles_PageBeyondTotal(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 10)
	createTestFile(t, imgDir, "notes.txt", 10) // not whitelisted
	require.NoError(t, os.MkdirAll(filepath.Join(imgDir, "subdir"), 0o755))

	svc := NewService(base, nil)
	result, err := svc.ListFiles("image", 3, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 3, result.Page)
	assert.Empty(t, result.Items)
}

func TestDeleteFiles_InvalidNames(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 10)

	svc := NewService(base, nil)
	result := svc.DeleteFiles("image", []string{"a.jpg", "b.txt", "sub/b.jpg", ""})
	assert.Equal(t, 1, result.Success)
	assert.Equal(t, 3, result.Failed)
}

func TestDeleteFile_RemoveError(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 10)

	svc := NewService(base, nil)
	readOnlyDir(t, imgDir)

	assert.Error(t, svc.DeleteFile("image", "a.jpg"))
}

func TestDeleteFile_ClampsNegativeSizeMap(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, nil) // sizeMap starts empty

	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "late.jpg", 100) // file created after service start

	require.NoError(t, svc.DeleteFile("image", "late.jpg"))

	svc.mu.Lock()
	tracked := svc.sizeMap["image"]
	svc.mu.Unlock()
	assert.Equal(t, int64(0), tracked, "deleting an untracked file must clamp sizeMap to zero")
}

func TestClear_SkipsDirsAndTmpFiles(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "keep.jpg", 10)
	createTestFile(t, imgDir, ".tmp-partial.jpg", 10)
	require.NoError(t, os.MkdirAll(filepath.Join(imgDir, "subdir"), 0o755))

	svc := NewService(base, nil)
	result := svc.Clear("image")

	assert.Equal(t, 1, result.Deleted)
	assert.Positive(t, result.FreedMB)
}

func TestClear_RemoveError(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 10)

	svc := NewService(base, nil)
	readOnlyDir(t, imgDir)

	result := svc.Clear("image")
	assert.Equal(t, 0, result.Deleted, "removal failures must not be counted as deleted")
}

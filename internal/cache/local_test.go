package cache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
)

func createTestFile(t *testing.T, dir, name string, size int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, size)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// createTestFileWithAge creates a file and sets its mod time to the given age ago.
func createTestFileWithAge(t *testing.T, dir, name string, size int, age time.Duration) {
	t.Helper()
	createTestFile(t, dir, name, size)
	modTime := time.Now().Add(-age)
	if err := os.Chtimes(filepath.Join(dir, name), modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func newRuntimeWithLimits(imageMB, videoMB int) *config.Runtime {
	cfg := config.DefaultConfig()
	cfg.Cache.ImageMaxMB = imageMB
	cfg.Cache.VideoMaxMB = videoMB
	return config.NewRuntime(cfg)
}

// --- Existing tests (updated for new NewService signature) ---

func TestGetStats_EmptyDir(t *testing.T) {
	svc := NewService(t.TempDir(), nil)
	stats := svc.GetStats("image")
	if stats.Count != 0 {
		t.Errorf("expected count 0, got %d", stats.Count)
	}
	if stats.SizeMB != 0 {
		t.Errorf("expected size 0, got %f", stats.SizeMB)
	}
}

func TestGetStats_NonExistentDir(t *testing.T) {
	svc := NewService("/nonexistent/path/that/does/not/exist", nil)
	stats := svc.GetStats("image")
	if stats.Count != 0 {
		t.Errorf("expected count 0, got %d", stats.Count)
	}
}

func TestGetStats_WithFiles(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, imgDir, "b.png", 2048)

	svc := NewService(base, nil)
	stats := svc.GetStats("image")
	if stats.Count != 2 {
		t.Errorf("expected count 2, got %d", stats.Count)
	}
	expectedMB := float64(1024+2048) / (1024 * 1024)
	if stats.SizeMB < expectedMB*0.99 || stats.SizeMB > expectedMB*1.01 {
		t.Errorf("expected size ~%f MB, got %f", expectedMB, stats.SizeMB)
	}
}

func TestGetStats_IgnoresNonWhitelisted(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, imgDir, "b.txt", 2048)
	createTestFile(t, imgDir, "c.html", 512)

	svc := NewService(base, nil)
	stats := svc.GetStats("image")
	if stats.Count != 1 {
		t.Errorf("expected count 1, got %d", stats.Count)
	}
}

func TestGetStats_IgnoresSubdirectories(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	subDir := filepath.Join(imgDir, "subdir")
	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, subDir, "b.jpg", 2048)

	svc := NewService(base, nil)
	stats := svc.GetStats("image")
	if stats.Count != 1 {
		t.Errorf("expected count 1 (ignoring subdir), got %d", stats.Count)
	}
}

func TestListFiles_Empty(t *testing.T) {
	svc := NewService(t.TempDir(), nil)
	result, err := svc.ListFiles("image", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

func TestListFiles_SortedByMtimeDesc(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "old.jpg", 100)
	time.Sleep(50 * time.Millisecond)
	createTestFile(t, imgDir, "new.jpg", 200)

	svc := NewService(base, nil)
	result, err := svc.ListFiles("image", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 {
		t.Errorf("expected total 2, got %d", result.Total)
	}
	if len(result.Items) < 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Name != "new.jpg" {
		t.Errorf("expected first item new.jpg, got %s", result.Items[0].Name)
	}
}

func TestListFiles_Pagination(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	for i := 0; i < 5; i++ {
		createTestFile(t, imgDir, filepath.Base(string(rune('a'+i))+".jpg"), 100)
		time.Sleep(10 * time.Millisecond)
	}

	svc := NewService(base, nil)
	result, err := svc.ListFiles("image", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items on page 1, got %d", len(result.Items))
	}
	if result.Page != 1 || result.PageSize != 2 {
		t.Errorf("expected page=1 pageSize=2, got page=%d pageSize=%d", result.Page, result.PageSize)
	}
}

func TestDeleteFile_Success(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "test.jpg", 100)

	svc := NewService(base, nil)
	err := svc.DeleteFile("image", "test.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(imgDir, "test.jpg")); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestDeleteFile_PathTraversal(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, nil)

	err := svc.DeleteFile("image", "../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
	err = svc.DeleteFile("image", "../../secret.txt")
	if err == nil {
		t.Error("expected error for path traversal")
	}
	err = svc.DeleteFile("image", "subdir/file.jpg")
	if err == nil {
		t.Error("expected error for directory separator in name")
	}
}

func TestDeleteFiles_Batch(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 100)
	createTestFile(t, imgDir, "b.jpg", 100)

	svc := NewService(base, nil)
	result := svc.DeleteFiles("image", []string{"a.jpg", "b.jpg", "nonexistent.jpg"})
	if result.Success != 2 {
		t.Errorf("expected 2 successes, got %d", result.Success)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", result.Failed)
	}
}

func TestClear_RemovesAllWhitelisted(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, imgDir, "b.png", 2048)
	createTestFile(t, imgDir, "c.txt", 512)

	svc := NewService(base, nil)
	result := svc.Clear("image")
	if result.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", result.Deleted)
	}
	if _, err := os.Stat(filepath.Join(imgDir, "c.txt")); os.IsNotExist(err) {
		t.Error("c.txt should not have been deleted")
	}
}

func TestClear_EmptyDir(t *testing.T) {
	svc := NewService(t.TempDir(), nil)
	result := svc.Clear("image")
	if result.Deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", result.Deleted)
	}
}

func TestVideoExtensions(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")
	createTestFile(t, vidDir, "clip.mp4", 1024)
	createTestFile(t, vidDir, "clip.webm", 2048)
	createTestFile(t, vidDir, "clip.txt", 512)

	svc := NewService(base, nil)
	stats := svc.GetStats("video")
	if stats.Count != 2 {
		t.Errorf("expected count 2 for video, got %d", stats.Count)
	}
}

func TestFilePath(t *testing.T) {
	base := t.TempDir()
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "test.jpg", 100)

	svc := NewService(base, nil)
	p, err := svc.FilePath("image", "test.jpg")
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(imgDir, "test.jpg")
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}
	_, err = svc.FilePath("image", "../secret.txt")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

// --- New eviction and sizeMap tests ---

func TestEviction_FIFOOrder(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	// Create 3 files: oldest (300KB), middle (300KB), newest (300KB) = 900KB total
	createTestFileWithAge(t, vidDir, "oldest.mp4", 300*1024, 3*time.Hour)
	createTestFileWithAge(t, vidDir, "middle.mp4", 300*1024, 2*time.Hour)
	createTestFileWithAge(t, vidDir, "newest.mp4", 300*1024, 1*time.Hour)

	// Set limit to 1MB — 900KB is under limit, no eviction yet
	rt := newRuntimeWithLimits(0, 1)
	svc := NewService(base, rt)

	// Save a new file to push over the 1MB limit
	_, err := svc.SaveFile("video", make([]byte, 300*1024), ".mp4")
	if err != nil {
		t.Fatal(err)
	}

	// After eviction to 60% (614KB), oldest files should be removed first
	if _, err := os.Stat(filepath.Join(vidDir, "oldest.mp4")); !os.IsNotExist(err) {
		t.Error("oldest.mp4 should have been evicted")
	}
	if _, err := os.Stat(filepath.Join(vidDir, "middle.mp4")); !os.IsNotExist(err) {
		t.Error("middle.mp4 should have been evicted")
	}
	// newest and the new file should remain
	if _, err := os.Stat(filepath.Join(vidDir, "newest.mp4")); os.IsNotExist(err) {
		t.Error("newest.mp4 should still exist")
	}
}

func TestEviction_LowWatermark(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	// Create 10 files of 100KB each = 1000KB, limit = 1MB (1024KB)
	for i := 0; i < 10; i++ {
		name := filepath.Base(string(rune('a'+i)) + ".mp4")
		createTestFileWithAge(t, vidDir, name, 100*1024, time.Duration(10-i)*time.Hour)
	}

	rt := newRuntimeWithLimits(0, 1)
	svc := NewService(base, rt)

	// Save one more to trigger eviction (total would be 1100KB > 1024KB)
	_, err := svc.SaveFile("video", make([]byte, 100*1024), ".mp4")
	if err != nil {
		t.Fatal(err)
	}

	// Target is 60% of 1MB = 614KB. Should have ~6 files remaining (600KB)
	stats := svc.GetStats("video")
	limitBytes := float64(1024 * 1024)
	targetBytes := int64(limitBytes * 0.6)
	currentBytes := int64(stats.SizeMB * 1024 * 1024)
	if currentBytes > targetBytes+1024 { // allow 1KB tolerance
		t.Errorf("expected size <= %d bytes (60%% watermark), got %d", targetBytes, currentBytes)
	}
}

func TestEviction_UnlimitedSkips(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	createTestFile(t, vidDir, "a.mp4", 1024*1024) // 1MB

	// limit = 0 means unlimited
	rt := newRuntimeWithLimits(0, 0)
	svc := NewService(base, rt)

	// Save more data — no eviction should happen
	_, err := svc.SaveFile("video", make([]byte, 1024*1024), ".mp4")
	if err != nil {
		t.Fatal(err)
	}

	stats := svc.GetStats("video")
	if stats.Count != 2 {
		t.Errorf("expected 2 files (no eviction), got %d", stats.Count)
	}
}

func TestReconcile_RebuildsSize(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	createTestFile(t, vidDir, "a.mp4", 1024)
	createTestFile(t, vidDir, "b.mp4", 2048)

	svc := NewService(base, nil)
	stats := svc.GetStats("video")

	expectedMB := float64(1024+2048) / (1024 * 1024)
	if stats.SizeMB < expectedMB*0.99 || stats.SizeMB > expectedMB*1.01 {
		t.Errorf("expected size ~%f MB, got %f", expectedMB, stats.SizeMB)
	}
}

func TestReconcile_CleansTmpFiles(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	createTestFile(t, vidDir, ".tmp-abc123.mp4", 1024)
	createTestFile(t, vidDir, ".tmp-def456.mp4", 2048)
	createTestFile(t, vidDir, "real.mp4", 512)

	_ = NewService(base, nil)

	// .tmp- files should be cleaned up
	if _, err := os.Stat(filepath.Join(vidDir, ".tmp-abc123.mp4")); !os.IsNotExist(err) {
		t.Error(".tmp-abc123.mp4 should have been cleaned up")
	}
	if _, err := os.Stat(filepath.Join(vidDir, ".tmp-def456.mp4")); !os.IsNotExist(err) {
		t.Error(".tmp-def456.mp4 should have been cleaned up")
	}
	// real file should remain
	if _, err := os.Stat(filepath.Join(vidDir, "real.mp4")); os.IsNotExist(err) {
		t.Error("real.mp4 should still exist")
	}
}

func TestReconcile_EvictsOnStartup(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	// Create 2MB of files, then start with 1MB limit
	createTestFileWithAge(t, vidDir, "old.mp4", 1024*1024, 2*time.Hour)
	createTestFileWithAge(t, vidDir, "new.mp4", 1024*1024, 1*time.Hour)

	rt := newRuntimeWithLimits(0, 1)
	_ = NewService(base, rt)

	// old.mp4 should be evicted on startup
	if _, err := os.Stat(filepath.Join(vidDir, "old.mp4")); !os.IsNotExist(err) {
		t.Error("old.mp4 should have been evicted on startup")
	}
}

func TestSaveFile_UpdatesSizeMap(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, nil)

	data := make([]byte, 5000)
	_, err := svc.SaveFile("video", data, ".mp4")
	if err != nil {
		t.Fatal(err)
	}

	stats := svc.GetStats("video")
	expectedMB := float64(5000) / (1024 * 1024)
	if stats.SizeMB < expectedMB*0.99 || stats.SizeMB > expectedMB*1.01 {
		t.Errorf("expected size ~%f MB, got %f", expectedMB, stats.SizeMB)
	}
}

func TestSaveStream_StatForSize(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, nil)

	data := strings.NewReader(string(make([]byte, 8000)))
	_, err := svc.SaveStream("video", data, ".mp4")
	if err != nil {
		t.Fatal(err)
	}

	stats := svc.GetStats("video")
	expectedMB := float64(8000) / (1024 * 1024)
	if stats.SizeMB < expectedMB*0.99 || stats.SizeMB > expectedMB*1.01 {
		t.Errorf("expected size ~%f MB, got %f", expectedMB, stats.SizeMB)
	}
}

func TestDeleteFile_UpdatesSizeMap(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")
	createTestFile(t, vidDir, "a.mp4", 1024)
	createTestFile(t, vidDir, "b.mp4", 2048)

	svc := NewService(base, nil)

	if err := svc.DeleteFile("video", "a.mp4"); err != nil {
		t.Fatal(err)
	}

	stats := svc.GetStats("video")
	expectedMB := float64(2048) / (1024 * 1024)
	if stats.SizeMB < expectedMB*0.99 || stats.SizeMB > expectedMB*1.01 {
		t.Errorf("expected size ~%f MB after delete, got %f", expectedMB, stats.SizeMB)
	}
}

func TestClear_ResetsSizeMap(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")
	createTestFile(t, vidDir, "a.mp4", 1024)
	createTestFile(t, vidDir, "b.mp4", 2048)

	svc := NewService(base, nil)
	svc.Clear("video")

	stats := svc.GetStats("video")
	if stats.SizeMB != 0 {
		t.Errorf("expected size 0 after clear, got %f", stats.SizeMB)
	}
	if stats.Count != 0 {
		t.Errorf("expected count 0 after clear, got %d", stats.Count)
	}
}

func TestGetStats_ReturnsLimitMB(t *testing.T) {
	base := t.TempDir()
	rt := newRuntimeWithLimits(100, 200)
	svc := NewService(base, rt)

	imgStats := svc.GetStats("image")
	if imgStats.LimitMB != 100 {
		t.Errorf("expected image limit 100 MB, got %f", imgStats.LimitMB)
	}

	vidStats := svc.GetStats("video")
	if vidStats.LimitMB != 200 {
		t.Errorf("expected video limit 200 MB, got %f", vidStats.LimitMB)
	}
}

func TestGetStats_UnlimitedReturnsZeroLimit(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, nil)

	stats := svc.GetStats("video")
	if stats.LimitMB != 0 {
		t.Errorf("expected limit 0 for unlimited, got %f", stats.LimitMB)
	}
}

func TestSaveFile_ExceedsLimit_ReturnsError(t *testing.T) {
	base := t.TempDir()
	// limit = 1MB, file = 2MB
	rt := newRuntimeWithLimits(0, 1)
	svc := NewService(base, rt)

	data := make([]byte, 2*1024*1024)
	_, err := svc.SaveFile("video", data, ".mp4")
	if err == nil {
		t.Fatal("expected error for file exceeding limit")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge, got %v", err)
	}

	// No file should remain in the directory
	vidDir := filepath.Join(base, "tmp", "video")
	entries, _ := os.ReadDir(vidDir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
}

func TestSaveStream_ExceedsLimit_ReturnsError(t *testing.T) {
	base := t.TempDir()
	// limit = 1MB, stream = 2MB
	rt := newRuntimeWithLimits(0, 1)
	svc := NewService(base, rt)

	reader := strings.NewReader(string(make([]byte, 2*1024*1024)))
	_, err := svc.SaveStream("video", reader, ".mp4")
	if err == nil {
		t.Fatal("expected error for stream exceeding limit")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge, got %v", err)
	}

	// No file should remain in the directory
	vidDir := filepath.Join(base, "tmp", "video")
	entries, _ := os.ReadDir(vidDir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
}

func TestLRU_AccessedFilesSurviveEviction(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")

	// old (3h ago, 200KB) + newer (1h ago, 500KB) = 700KB, under 1MB limit
	createTestFileWithAge(t, vidDir, "old_but_accessed.mp4", 200*1024, 3*time.Hour)
	createTestFileWithAge(t, vidDir, "newer_not_accessed.mp4", 500*1024, 1*time.Hour)

	rt := newRuntimeWithLimits(0, 1) // 1MB limit
	svc := NewService(base, rt)

	// Access the old file via FilePath — this touches its mtime to now
	_, err := svc.FilePath("video", "old_but_accessed.mp4")
	if err != nil {
		t.Fatal(err)
	}

	// Save a new 400KB file: total = 200+500+400 = 1100KB > 1024KB, triggers eviction
	// Target = 60% of 1MB = 614KB
	// Sorted by mtime: newer_not_accessed (1h ago) → old_but_accessed (now) → new (now)
	// Delete newer_not_accessed (500KB): 1100-500 = 600KB <= 614KB → stop
	_, err = svc.SaveFile("video", make([]byte, 400*1024), ".mp4")
	if err != nil {
		t.Fatal(err)
	}

	// The accessed old file should survive; the unaccessed newer file should be evicted
	if _, err := os.Stat(filepath.Join(vidDir, "old_but_accessed.mp4")); os.IsNotExist(err) {
		t.Error("old_but_accessed.mp4 should survive (was recently accessed)")
	}
	if _, err := os.Stat(filepath.Join(vidDir, "newer_not_accessed.mp4")); !os.IsNotExist(err) {
		t.Error("newer_not_accessed.mp4 should have been evicted (least recently used)")
	}
}

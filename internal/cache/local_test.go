package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestGetStats_EmptyDir(t *testing.T) {
	svc := NewService(t.TempDir())
	stats := svc.GetStats("image")
	if stats.Count != 0 {
		t.Errorf("expected count 0, got %d", stats.Count)
	}
	if stats.SizeMB != 0 {
		t.Errorf("expected size 0, got %f", stats.SizeMB)
	}
}

func TestGetStats_NonExistentDir(t *testing.T) {
	svc := NewService("/nonexistent/path/that/does/not/exist")
	stats := svc.GetStats("image")
	if stats.Count != 0 {
		t.Errorf("expected count 0, got %d", stats.Count)
	}
}

func TestGetStats_WithFiles(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")

	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, imgDir, "b.png", 2048)

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
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")

	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, imgDir, "b.txt", 2048)   // not whitelisted
	createTestFile(t, imgDir, "c.html", 512)    // not whitelisted

	stats := svc.GetStats("image")
	if stats.Count != 1 {
		t.Errorf("expected count 1, got %d", stats.Count)
	}
}

func TestGetStats_IgnoresSubdirectories(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")
	subDir := filepath.Join(imgDir, "subdir")

	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, subDir, "b.jpg", 2048) // in subdirectory

	stats := svc.GetStats("image")
	if stats.Count != 1 {
		t.Errorf("expected count 1 (ignoring subdir), got %d", stats.Count)
	}
}

func TestListFiles_Empty(t *testing.T) {
	svc := NewService(t.TempDir())
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
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")

	createTestFile(t, imgDir, "old.jpg", 100)
	time.Sleep(50 * time.Millisecond)
	createTestFile(t, imgDir, "new.jpg", 200)

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
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")

	for i := 0; i < 5; i++ {
		createTestFile(t, imgDir, filepath.Base(string(rune('a'+i))+".jpg"), 100)
		time.Sleep(10 * time.Millisecond)
	}

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
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "test.jpg", 100)

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
	svc := NewService(base)

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
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "a.jpg", 100)
	createTestFile(t, imgDir, "b.jpg", 100)

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
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")

	createTestFile(t, imgDir, "a.jpg", 1024)
	createTestFile(t, imgDir, "b.png", 2048)
	createTestFile(t, imgDir, "c.txt", 512) // not whitelisted

	result := svc.Clear("image")
	if result.Deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", result.Deleted)
	}

	// c.txt should still exist
	if _, err := os.Stat(filepath.Join(imgDir, "c.txt")); os.IsNotExist(err) {
		t.Error("c.txt should not have been deleted")
	}
}

func TestClear_EmptyDir(t *testing.T) {
	svc := NewService(t.TempDir())
	result := svc.Clear("image")
	if result.Deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", result.Deleted)
	}
}

func TestVideoExtensions(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base)
	vidDir := filepath.Join(base, "tmp", "video")

	createTestFile(t, vidDir, "clip.mp4", 1024)
	createTestFile(t, vidDir, "clip.webm", 2048)
	createTestFile(t, vidDir, "clip.txt", 512)

	stats := svc.GetStats("video")
	if stats.Count != 2 {
		t.Errorf("expected count 2 for video, got %d", stats.Count)
	}
}

func TestFilePath(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base)
	imgDir := filepath.Join(base, "tmp", "image")
	createTestFile(t, imgDir, "test.jpg", 100)

	p, err := svc.FilePath("image", "test.jpg")
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(imgDir, "test.jpg")
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}

	// path traversal
	_, err = svc.FilePath("image", "../secret.txt")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

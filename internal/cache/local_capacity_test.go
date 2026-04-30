package cache

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	bytesPerKiB            = 1024
	oneMiB                 = bytesPerKiB * bytesPerKiB
	streamChunkBytes       = 128 * bytesPerKiB
	streamOverflowBytes    = 2 * oneMiB
	publishFileBytes       = 700 * bytesPerKiB
	clearSaveStressRuns    = 100
	clearSaveStressPayload = 64 * bytesPerKiB
)

type countingReader struct {
	remaining int
	chunkSize int
	readBytes int
}

func newCountingReader(size int) *countingReader {
	return &countingReader{
		remaining: size,
		chunkSize: streamChunkBytes,
	}
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}

	n := len(p)
	if n > r.chunkSize {
		n = r.chunkSize
	}
	if n > r.remaining {
		n = r.remaining
	}

	clear(p[:n])
	r.remaining -= n
	r.readBytes += n
	return n, nil
}

type closeTrackingReader struct {
	reader *countingReader
	closed bool
}

func (r *closeTrackingReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

func TestSaveStreamStopsAfterLimitExceeded(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, newRuntimeWithLimits(0, 1))
	reader := newCountingReader(streamOverflowBytes)

	_, err := svc.SaveStream("video", reader, ".mp4")
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
	if reader.readBytes >= streamOverflowBytes {
		t.Fatalf("expected early stop before reading full stream, read %d", reader.readBytes)
	}
	if reader.readBytes > oneMiB+1 {
		t.Fatalf("expected read at most limit+1 bytes, read %d", reader.readBytes)
	}

	assertCacheDirEmpty(t, svc.dir("video"))
}

func TestSaveStreamClosesReaderAfterLimitExceeded(t *testing.T) {
	base := t.TempDir()
	svc := NewService(base, newRuntimeWithLimits(0, 1))
	reader := &closeTrackingReader{reader: newCountingReader(streamOverflowBytes)}

	_, err := svc.SaveStream("video", reader, ".mp4")
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
	if !reader.closed {
		t.Fatal("expected SaveStream to close the reader after limit overflow")
	}
}

func TestClearConcurrentSaveKeepsSizeMapConsistent(t *testing.T) {
	for i := 0; i < clearSaveStressRuns; i++ {
		base := t.TempDir()
		svc := NewService(base, nil)
		var wg sync.WaitGroup
		var saveErr error

		wg.Add(2)
		go func() {
			defer wg.Done()
			_, saveErr = svc.SaveFile("video", make([]byte, clearSaveStressPayload), ".mp4")
		}()
		go func() {
			defer wg.Done()
			svc.Clear("video")
		}()
		wg.Wait()

		if saveErr != nil {
			t.Fatalf("save file: %v", saveErr)
		}
		assertSizeMapMatchesDisk(t, svc, "video")
	}
}

func TestSaveFileProtectsPublishedFileDuringEviction(t *testing.T) {
	base := t.TempDir()
	vidDir := filepath.Join(base, "tmp", "video")
	createTestFileWithAge(t, vidDir, "old.mp4", publishFileBytes, time.Hour)
	svc := NewService(base, newRuntimeWithLimits(0, 1))

	name, err := svc.SaveFile("video", make([]byte, publishFileBytes), ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vidDir, name)); err != nil {
		t.Fatalf("published file should remain addressable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vidDir, "old.mp4")); !os.IsNotExist(err) {
		t.Fatalf("old.mp4 should have been evicted, got %v", err)
	}
	assertSizeMapMatchesDisk(t, svc, "video")
}

func assertCacheDirEmpty(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		t.Fatalf("expected empty cache dir, found %d entries", len(entries))
	}
}

func assertSizeMapMatchesDisk(t *testing.T, svc *Service, mediaType string) {
	t.Helper()

	expected := diskCacheBytes(t, svc.dir(mediaType), mediaType)
	svc.mu.Lock()
	actual := svc.sizeMap[mediaType]
	svc.mu.Unlock()
	if actual != expected {
		t.Fatalf("sizeMap[%s] = %d, disk has %d", mediaType, actual, expected)
	}
}

func diskCacheBytes(t *testing.T, dir, mediaType string) int64 {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}

	var total int64
	for _, e := range entries {
		if e.IsDir() || !isCacheFile(e.Name(), mediaType) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}

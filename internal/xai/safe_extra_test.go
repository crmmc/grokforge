package xai

import (
	"runtime"
	"testing"
)

func TestSafeGoRecoversPanic(t *testing.T) {
	started := make(chan struct{})
	safeGo("test_panic", func() {
		close(started)
		panic("boom")
	})
	<-started
	// The panic is recovered inside safeGo; yield the scheduler a few rounds so
	// the recovered goroutine finishes before the test binary moves on.
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
}

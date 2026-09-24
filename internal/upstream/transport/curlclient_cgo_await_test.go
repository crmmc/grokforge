//go:build cgo

package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwaitCurlResult(t *testing.T) {
	t.Run("header ready returns nil", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()
		state.signalHeaders(nil)
		errCh := make(chan error, 1)

		got := awaitCurlResult(state, errCh, context.Background())

		assert.NoError(t, got)
	})

	t.Run("worker error via errCh marks header error", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()
		wantErr := errors.New("curl exploded")
		errCh := make(chan error, 1)
		errCh <- wantErr

		got := awaitCurlResult(state, errCh, context.Background())

		assert.Same(t, wantErr, got)
		select {
		case <-state.headerReady:
		default:
			t.Fatal("errCh path must signal header readiness")
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		assert.Same(t, wantErr, state.headerErr)
	})

	t.Run("worker exit without signal yields nil", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()
		errCh := make(chan error, 1)
		errCh <- nil

		got := awaitCurlResult(state, errCh, context.Background())

		assert.NoError(t, got)
		select {
		case <-state.headerReady:
		default:
			t.Fatal("nil-error errCh path must signal header readiness")
		}
	})

	t.Run("canceled context wins", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		errCh := make(chan error, 1)

		got := awaitCurlResult(state, errCh, ctx)

		assert.ErrorIs(t, got, context.Canceled)
		select {
		case <-state.headerReady:
		default:
			t.Fatal("context-cancel path must signal header readiness")
		}
	})
}

func TestCurlPerform_ContextDoneAfterStart(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	client, err := newCurlClient(Options{})
	require.NoError(t, err)

	go cancel()

	resp, err := client.Do(req)
	require.Error(t, err)
	if resp != nil {
		resp.Body.Close()
	}
}

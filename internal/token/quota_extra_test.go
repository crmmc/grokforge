package token

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRateLimits_RequestErrors(t *testing.T) {
	t.Run("invalid base url", func(t *testing.T) {
		_, err := fetchRateLimits(context.Background(), "tok", "http://bad url", "auto")
		require.Error(t, err)
	})

	t.Run("connection refused", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := server.URL
		server.Close()

		_, err := fetchRateLimits(context.Background(), "tok", url, "auto")
		require.Error(t, err)
	})
}

// startTruncatedServer serves a raw HTTP response whose Content-Length promises
// more body bytes than are written, forcing a client-side body read error.
func startTruncatedServer(t *testing.T, statusLine string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte(statusLine + "\r\nContent-Length: 100\r\nConnection: close\r\n\r\nshort"))
			_ = conn.Close()
		}
	}()
	return "http://" + ln.Addr().String()
}

func TestFetchRateLimits_BodyReadErrors(t *testing.T) {
	t.Run("http error body read fails", func(t *testing.T) {
		url := startTruncatedServer(t, "HTTP/1.1 500 Server Error")
		_, err := fetchRateLimits(context.Background(), "tok", url, "auto")
		require.Error(t, err)

		var httpErr *rateLimitsHTTPError
		assert.NotErrorAs(t, err, &httpErr, "read failures must surface raw, not as rateLimitsHTTPError")
	})

	t.Run("success body read fails", func(t *testing.T) {
		url := startTruncatedServer(t, "HTTP/1.1 200 OK")
		_, err := fetchRateLimits(context.Background(), "tok", url, "auto")
		require.Error(t, err)
	})
}

func TestDecodeRateLimitsResponse_InvalidJSON(t *testing.T) {
	_, err := decodeRateLimitsResponse([]byte("not-json"))
	require.Error(t, err)
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadBodyPreview(t *testing.T) {
	t.Run("truncates above limit", func(t *testing.T) {
		preview, truncated, err := readBodyPreview(strings.NewReader("hello world"), 5)
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Equal(t, "hello", preview)
	})

	t.Run("non positive limit uses default", func(t *testing.T) {
		long := strings.Repeat("x", int(rateLimitsBodyPreviewLimit)+10)
		preview, truncated, err := readBodyPreview(strings.NewReader(long), 0)
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Len(t, preview, int(rateLimitsBodyPreviewLimit))
	})

	t.Run("read error surfaces", func(t *testing.T) {
		wantErr := errors.New("read failed")
		_, _, err := readBodyPreview(errReader{err: wantErr}, 10)
		require.ErrorIs(t, err, wantErr)
	})
}

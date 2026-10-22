package grok

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapError(t *testing.T) {
	g := New("", nil, Options{})

	t.Run("url error wraps ErrNetwork", func(t *testing.T) {
		urlErr := &url.Error{Op: "Post", URL: "https://grok.com", Err: io.EOF}
		err := g.mapError(urlErr)
		require.Error(t, err)
		assert.ErrorIs(t, err, upstream.ErrNetwork)
		assert.Contains(t, err.Error(), "EOF")
	})

	t.Run("wrapped url error still wraps ErrNetwork", func(t *testing.T) {
		inner := &url.Error{Op: "Post", URL: "https://grok.com", Err: io.EOF}
		wrapped := errors.Join(errors.New("context"), inner)
		err := g.mapError(wrapped)
		require.Error(t, err)
		assert.ErrorIs(t, err, upstream.ErrNetwork)
	})

	t.Run("plain error passes through", func(t *testing.T) {
		sentinel := errors.New("boom")
		err := g.mapError(sentinel)
		assert.Same(t, sentinel, err)
		assert.NotErrorIs(t, err, upstream.ErrNetwork)
	})
}

func TestMapHTTPError(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        error
		wantBody    string
	}{
		{
			name:   "too many requests",
			status: http.StatusTooManyRequests,
			want:   upstream.ErrRateLimited,
		},
		{
			name:        "forbidden cf challenge by content type",
			status:      http.StatusForbidden,
			contentType: "text/html; charset=utf-8",
			body:        "<html></html>",
			want:        upstream.ErrCFChallenge,
		},
		{
			name:   "forbidden cf challenge by body",
			status: http.StatusForbidden,
			body:   "Attention Required! Cloudflare challenge-platform",
			want:   upstream.ErrCFChallenge,
		},
		{
			name:   "forbidden plain",
			status: http.StatusForbidden,
			body:   `{"error":"blocked"}`,
			want:   upstream.ErrForbidden,
		},
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized,
			want:   upstream.ErrInvalidToken,
		},
		{
			name:     "server error includes body",
			status:   http.StatusInternalServerError,
			body:     "kaboom",
			want:     upstream.ErrServerError,
			wantBody: "kaboom",
		},
		{
			name:   "bad gateway",
			status: http.StatusBadGateway,
			want:   upstream.ErrServerError,
		},
	}

	g := New("", nil, Options{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Header:     http.Header{"Content-Type": []string{tt.contentType}},
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			err := g.mapHTTPError(resp)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
			if tt.wantBody != "" {
				assert.Contains(t, err.Error(), tt.wantBody)
			}
		})
	}
}

func TestIsCFChallenge(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{name: "html content type", contentType: "text/html", body: "", want: true},
		{name: "cf- marker", body: "error code: 1020 cf-ray", want: true},
		{name: "cloudflare marker", body: "CLOUDFLARE error", want: true},
		{name: "challenge-platform marker", body: "/cdn-cgi/challenge-platform/h/b", want: true},
		{name: "just a moment", body: "Just a moment...", want: true},
		{name: "uppercase body marker", body: "JUST A MOMENT", want: true},
		{name: "plain json", contentType: "application/json", body: `{"error":"no"}`, want: false},
		{name: "empty", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCFChallenge(tt.contentType, tt.body))
		})
	}
}

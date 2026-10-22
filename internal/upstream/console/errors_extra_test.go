package console

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
	c := New("", nil, Options{})

	t.Run("url error wraps ErrNetwork", func(t *testing.T) {
		urlErr := &url.Error{Op: "Post", URL: "https://console.x.ai", Err: io.EOF}
		err := c.mapError(urlErr)
		require.Error(t, err)
		assert.ErrorIs(t, err, upstream.ErrNetwork)
		assert.Contains(t, err.Error(), "EOF")
	})

	t.Run("wrapped url error still wraps ErrNetwork", func(t *testing.T) {
		inner := &url.Error{Op: "Post", URL: "https://console.x.ai", Err: io.EOF}
		err := c.mapError(errors.Join(errors.New("ctx"), inner))
		require.Error(t, err)
		assert.ErrorIs(t, err, upstream.ErrNetwork)
	})

	t.Run("plain error passes through", func(t *testing.T) {
		sentinel := errors.New("boom")
		err := c.mapError(sentinel)
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
			name:   "unauthorized",
			status: http.StatusUnauthorized,
			want:   upstream.ErrInvalidToken,
		},
		{
			name:   "payment required",
			status: http.StatusPaymentRequired,
			want:   upstream.ErrCreditExhausted,
		},
		{
			name:        "forbidden cf challenge by content type",
			status:      http.StatusForbidden,
			contentType: "text/html",
			body:        "<html></html>",
			want:        upstream.ErrCFChallenge,
		},
		{
			name:   "forbidden cf challenge by body",
			status: http.StatusForbidden,
			body:   "Just a moment...",
			want:   upstream.ErrCFChallenge,
		},
		{
			name:   "forbidden plain",
			status: http.StatusForbidden,
			body:   `{"error":"no"}`,
			want:   upstream.ErrForbidden,
		},
		{
			name:   "too many requests",
			status: http.StatusTooManyRequests,
			want:   upstream.ErrRateLimited,
		},
		{
			name:     "server error includes body",
			status:   http.StatusBadGateway,
			body:     "upstream down",
			want:     upstream.ErrServerError,
			wantBody: "upstream down",
		},
	}

	c := New("", nil, Options{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Header:     http.Header{"Content-Type": []string{tt.contentType}},
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			err := c.mapHTTPError(resp)
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
		{name: "html content type", contentType: "text/html; charset=utf-8", want: true},
		{name: "cf- marker", body: "error 1020 cf-ray:abc", want: true},
		{name: "cloudflare marker", body: "Cloudflare Ray ID", want: true},
		{name: "challenge-platform marker", body: "/cdn-cgi/challenge-platform/x", want: true},
		{name: "just a moment", body: "Just a Moment", want: true},
		{name: "plain body", contentType: "application/json", body: `{"ok":true}`, want: false},
		{name: "empty", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCFChallenge(tt.contentType, tt.body))
		})
	}
}

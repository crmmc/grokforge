package grok

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"testing/iotest"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zeroReader produces an endless stream of zero bytes without allocating.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestNormalizeAssetURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "empty", rawURL: "", want: assetsBaseURL},
		{name: "scheme relative", rawURL: "//assets.grok.com/a.png", want: "https://assets.grok.com/a.png"},
		{name: "http canonicalized to https", rawURL: "http://assets.grok.com/a.png", want: "https://assets.grok.com/a.png"},
		{name: "uppercase host and scheme", rawURL: "HTTPS://Grok.COM/a.png", want: "https://Grok.COM/a.png"},
		{name: "grok.com host allowed", rawURL: "http://grok.com/x.png", want: "https://grok.com/x.png"},
		{name: "foreign host untouched", rawURL: "http://example.com/a.png", want: "http://example.com/a.png"},
		{name: "unparseable untouched", rawURL: "http://[", want: "http://["},
		{name: "relative path", rawURL: "cards/id/img.png", want: assetsBaseURL + "cards/id/img.png"},
		{name: "leading slash stripped", rawURL: "/cards/id/img.png", want: assetsBaseURL + "cards/id/img.png"},
		{name: "whitespace trimmed", rawURL: "  https://grok.com/x  ", want: "https://grok.com/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeAssetURL(tt.rawURL))
		})
	}
}

func TestCanonicalAssetURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "https untouched", rawURL: "https://assets.grok.com/a.png", want: "https://assets.grok.com/a.png"},
		{name: "non http scheme untouched", rawURL: "ftp://grok.com/a.png", want: "ftp://grok.com/a.png"},
		{name: "unparseable untouched", rawURL: "http://[", want: "http://["},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canonicalAssetURL(tt.rawURL))
		})
	}
}

func TestDownloadURL_Success(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       strBody("asset-body"),
	}}
	g := New("", doer, Options{
		BuildCookieString: func(tok string) string { return "sso=" + tok },
	})

	got, err := g.downloadURL(context.Background(), "tok-1", "assets.grok.com/x")
	require.NoError(t, err)
	assert.Equal(t, "asset-body", string(got))
	require.Len(t, doer.reqs, 1)
	assert.Equal(t, "sso=tok-1", doer.reqs[0].Header.Get("Cookie"))
}

func TestDownloadFunc_InvokesDownloadURL(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       strBody("bytes"),
	}}
	g := New("", doer, Options{})

	got, err := g.downloadFunc("tok")(context.Background(), "//grok.com/a")
	require.NoError(t, err)
	assert.Equal(t, "bytes", string(got))
}

func TestDownloadURL_Errors(t *testing.T) {
	tests := []struct {
		name    string
		doer    *fakeDoer
		rawURL  string
		wantErr error
		wantSub string
	}{
		{
			name: "http status error",
			doer: &fakeDoer{resp: &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       strBody("<html>denied</html>"),
			}},
			rawURL:  "https://assets.grok.com/a.png",
			wantErr: upstream.ErrCFChallenge,
		},
		{
			name:    "network error",
			doer:    &fakeDoer{err: errors.New("dial failed")},
			rawURL:  "https://assets.grok.com/a.png",
			wantSub: "dial failed",
		},
		{
			name: "body read error",
			doer: &fakeDoer{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(io.MultiReader(strBody("partial "), iotest.ErrReader(io.ErrUnexpectedEOF))),
			}},
			rawURL:  "https://assets.grok.com/a.png",
			wantSub: "read body",
		},
		{
			name: "invalid request url",
			doer: &fakeDoer{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       strBody(""),
			}},
			rawURL:  "http://[",
			wantSub: "create request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New("", tt.doer, Options{})
			_, err := g.downloadURL(context.Background(), "tok", tt.rawURL)
			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantSub != "" {
				assert.Contains(t, err.Error(), tt.wantSub)
			}
		})
	}
}

func TestDownloadURL_OversizeBody(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(zeroReader{}),
	}}
	g := New("", doer, Options{})

	_, err := g.downloadURL(context.Background(), "tok", "https://assets.grok.com/huge")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "asset body exceeds")
}

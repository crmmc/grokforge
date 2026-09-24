package xai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateImagePost(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"mediaUrl":"https://assets.grok.com/i.png","post":{"id":"post-9"}}`)
		})
		c := newTestClient(stub.doer(), nil)

		id, err := c.CreateImagePost(context.Background(), "https://example.com/in.png")
		require.NoError(t, err)
		assert.Equal(t, "post-9", id)

		calls := stub.calls()
		require.Len(t, calls, 1)
		assert.Equal(t, "/rest/media/post/create", calls[0].Path)
		assert.Contains(t, string(calls[0].Body), `"mediaType":"MEDIA_POST_TYPE_IMAGE"`)
		assert.Contains(t, string(calls[0].Body), `"mediaUrl":"https://example.com/in.png"`)
	})

	t.Run("empty url rejected", func(t *testing.T) {
		c := newTestClient(&staticDoer{}, nil)
		_, err := c.CreateImagePost(context.Background(), "   ")
		assert.EqualError(t, err, "create image post: imageURL is required")
	})

	t.Run("missing post id", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"post":{}}`)
		})
		c := newTestClient(stub.doer(), nil)
		_, err := c.CreateImagePost(context.Background(), "https://example.com/in.png")
		assert.EqualError(t, err, "create image post: missing post id")
	})
}

func TestCreateVideoPost(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"post":{"id":"vpost-1"}}`)
		})
		c := newTestClient(stub.doer(), nil)

		id, err := c.CreateVideoPost(context.Background(), "a cat runs")
		require.NoError(t, err)
		assert.Equal(t, "vpost-1", id)

		calls := stub.calls()
		require.Len(t, calls, 1)
		assert.Contains(t, string(calls[0].Body), `"mediaType":"MEDIA_POST_TYPE_VIDEO"`)
		assert.Contains(t, string(calls[0].Body), `"prompt":"a cat runs"`)
	})

	t.Run("empty prompt rejected", func(t *testing.T) {
		c := newTestClient(&staticDoer{}, nil)
		_, err := c.CreateVideoPost(context.Background(), "")
		assert.EqualError(t, err, "create video post: prompt is required")
	})

	t.Run("upstream error propagates", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "boom")
		})
		c := newTestClient(stub.doer(), nil)
		_, err := c.CreateVideoPost(context.Background(), "prompt")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVideoUpstreamFailed)
	})

	t.Run("missing post id", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"post":{"id":""}}`)
		})
		c := newTestClient(stub.doer(), nil)
		_, err := c.CreateVideoPost(context.Background(), "prompt")
		assert.EqualError(t, err, "create video post: missing post id")
	})
}

func TestCreateMediaPostErrors(t *testing.T) {
	tests := []struct {
		name    string
		doer    func(t *testing.T) transport.Doer
		wantErr string
	}{
		{
			name: "non-200 status",
			doer: func(t *testing.T) transport.Doer {
				stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, "no")
				})
				return stub.doer()
			},
			wantErr: "video upstream request failed",
		},
		{
			name: "invalid json",
			doer: func(t *testing.T) transport.Doer {
				stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, "not-json")
				})
				return stub.doer()
			},
			wantErr: "unmarshal response",
		},
		{
			name: "doer error",
			doer: func(t *testing.T) transport.Doer {
				return &staticDoer{err: errors.New("network down")}
			},
			wantErr: "do request",
		},
		{
			name: "body read error",
			doer: func(t *testing.T) transport.Doer {
				return &staticDoer{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
				}}
			},
			wantErr: "read response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(tt.doer(t), nil)
			_, err := c.CreateImagePost(context.Background(), "https://example.com/in.png")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestVideoUpscale(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"hdMediaUrl":"https://assets.grok.com/v.mp4","status":"done"}`)
		})
		c := newTestClient(stub.doer(), nil)

		url, err := c.VideoUpscale(context.Background(), "video-1")
		require.NoError(t, err)
		assert.Equal(t, "https://assets.grok.com/v.mp4", url)

		calls := stub.calls()
		require.Len(t, calls, 1)
		assert.Equal(t, "/rest/media/video/upscale", calls[0].Path)
		assert.Contains(t, string(calls[0].Body), `"videoId":"video-1"`)
	})

	t.Run("non-200 status", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "no")
		})
		c := newTestClient(stub.doer(), nil)
		_, err := c.VideoUpscale(context.Background(), "video-1")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVideoUpstreamFailed)
	})

	t.Run("invalid json", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "not-json")
		})
		c := newTestClient(stub.doer(), nil)
		_, err := c.VideoUpscale(context.Background(), "video-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal response")
	})

	t.Run("body read error", func(t *testing.T) {
		c := newTestClient(&staticDoer{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
		}}, nil)
		_, err := c.VideoUpscale(context.Background(), "video-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read response")
	})
}

func TestPollUpscaleSuccess(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"hdMediaUrl":"https://assets.grok.com/v.mp4","status":"done"}`)
	})
	c := newTestClient(stub.doer(), nil)

	url, err := c.PollUpscale(context.Background(), "video-1", time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, "https://assets.grok.com/v.mp4", url)
}

func TestPollUpscaleStopsOnFatalError(t *testing.T) {
	c := newTestClient(&staticDoer{err: ErrForbidden}, nil)

	_, err := c.PollUpscale(context.Background(), "video-1", time.Millisecond)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestPollUpscaleContextCancelled(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.PollUpscale(ctx, "video-1", time.Hour)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPollUpscaleExhaustsAttempts(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "still processing")
	})
	c := newTestClient(stub.doer(), nil)

	_, err := c.PollUpscale(context.Background(), "video-1", time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded 120 attempts")
}

func TestPollUpscaleKeepsPollingOnEmptyURL(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"hdMediaUrl":"","status":"running"}`)
	})
	c := newTestClient(stub.doer(), nil)

	_, err := c.PollUpscale(context.Background(), "video-1", time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded 120 attempts")
}

func TestCanonicalAssetURLDirect(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"non http scheme unchanged", "ftp://assets.grok.com/x.mp4", "ftp://assets.grok.com/x.mp4"},
		{"https assets canonical", "https://assets.grok.com/x.mp4", "https://assets.grok.com/x.mp4"},
		{"unparseable url unchanged", "http://[::1", "http://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canonicalAssetURL(tt.in))
		})
	}

	t.Run("parse error via normalize", func(t *testing.T) {
		assert.Equal(t, "http://[::1", normalizeAssetURL("http://[::1"))
	})
}

// sizedReader returns exactly `remaining` bytes of filler without allocating.
type sizedReader struct {
	remaining int64
}

func (r *sizedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= int64(n)
	return n, nil
}

func TestDownloadTo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "video-bytes")
		})
		c := newTestClient(stub.doer(), nil)

		var buf []byte
		err := c.DownloadTo(context.Background(), "files/generated/v.mp4", &bufWriter{buf: &buf})
		require.NoError(t, err)
		assert.Equal(t, "video-bytes", string(buf))

		calls := stub.calls()
		require.Len(t, calls, 1)
		assert.Equal(t, http.MethodGet, calls[0].Method)
		assert.Equal(t, "/files/generated/v.mp4", calls[0].Path)
	})

	t.Run("download url returns bytes", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "image-bytes")
		})
		c := newTestClient(stub.doer(), nil)

		data, err := c.DownloadURL(context.Background(), "https://grok.com/images/x.png")
		require.NoError(t, err)
		assert.Equal(t, "image-bytes", string(data))
	})

	t.Run("non-200 status", func(t *testing.T) {
		stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		c := newTestClient(stub.doer(), nil)
		_, err := c.DownloadURL(context.Background(), "files/v.mp4")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download failed: status 403")
	})

	t.Run("body read error", func(t *testing.T) {
		c := newTestClient(&staticDoer{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
		}}, nil)
		err := c.DownloadTo(context.Background(), "files/v.mp4", &bufWriter{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read body")
	})

	t.Run("body exceeds size limit", func(t *testing.T) {
		c := newTestClient(&staticDoer{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(&sizedReader{remaining: maxAssetDownloadSize}),
		}}, nil)
		err := c.DownloadTo(context.Background(), "files/v.mp4", &bufWriter{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "asset body exceeds")
	})

	t.Run("closed client", func(t *testing.T) {
		c := newTestClient(&staticDoer{}, nil)
		require.NoError(t, c.Close())
		err := c.DownloadTo(context.Background(), "files/v.mp4", &bufWriter{})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStreamClosed)
	})

	t.Run("unparseable url fails request creation", func(t *testing.T) {
		c := newTestClient(&staticDoer{}, nil)
		err := c.DownloadTo(context.Background(), "http://[::1", &bufWriter{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create request")
	})
}

// bufWriter is a minimal io.Writer over a byte slice.
type bufWriter struct {
	buf *[]byte
}

func (w *bufWriter) Write(p []byte) (int, error) {
	if w.buf == nil {
		return len(p), nil
	}
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

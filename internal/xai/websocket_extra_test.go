package xai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func wsURL(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http")
}

func TestNewImagineClientOptions(t *testing.T) {
	tests := []struct {
		name   string
		opts   []ImagineClientOption
		verify func(t *testing.T, c *ImagineClient)
	}{
		{
			name: "defaults",
			opts: nil,
			verify: func(t *testing.T, c *ImagineClient) {
				assert.Equal(t, WSImagineURL, c.wsURL)
				assert.Equal(t, DefaultUserAgent, c.userAgent)
				assert.EqualValues(t, 10000, c.blockedGraceMillis)
				assert.Equal(t, "tok", c.token)
			},
		},
		{
			name: "user agent",
			opts: []ImagineClientOption{WithImagineUserAgent("custom-ua/2")},
			verify: func(t *testing.T, c *ImagineClient) {
				assert.Equal(t, "custom-ua/2", c.userAgent)
			},
		},
		{
			name: "cf clearance",
			opts: []ImagineClientOption{WithImagineCFClearance("clr")},
			verify: func(t *testing.T, c *ImagineClient) {
				assert.Equal(t, "clr", c.cfClearance)
			},
		},
		{
			name: "cf cookies",
			opts: []ImagineClientOption{WithImagineCFCookies("cfk=1")},
			verify: func(t *testing.T, c *ImagineClient) {
				assert.Equal(t, "cfk=1", c.cfCookies)
			},
		},
		{
			name: "proxy",
			opts: []ImagineClientOption{WithImagineProxy("http://proxy.local:8080")},
			verify: func(t *testing.T, c *ImagineClient) {
				assert.Equal(t, "http://proxy.local:8080", c.proxyURL)
			},
		},
		{
			name: "skip proxy ssl verify",
			opts: []ImagineClientOption{WithImagineSkipProxySSLVerify(true)},
			verify: func(t *testing.T, c *ImagineClient) {
				assert.True(t, c.skipProxySSLVerify)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewImagineClient("tok", tt.opts...)
			require.NotNil(t, c)
			tt.verify(t, c)
		})
	}
}

func TestBuildWSHeaders(t *testing.T) {
	t.Run("chromium ua adds client hints", func(t *testing.T) {
		c := NewImagineClient("tok", WithImagineUserAgent("Mozilla/5.0 (aarch64) Chrome/136.0.0.0"))
		h := c.buildWSHeaders()
		assert.Contains(t, h.Get("Cookie"), "sso=tok")
		assert.Equal(t, "https://grok.com", h.Get("Origin"))
		assert.NotEmpty(t, h.Get("Sec-Ch-Ua"))
		assert.NotEmpty(t, h.Get("Sec-Ch-Ua-Mobile"))
		assert.NotEmpty(t, h.Get("Sec-Ch-Ua-Platform"))
		assert.NotEmpty(t, h.Get("Sec-Ch-Ua-Arch"))
		assert.NotEmpty(t, h.Get("Sec-Ch-Ua-Bitness"))
		assert.Empty(t, h.Get("Sec-Ch-Ua-Model"))
	})

	t.Run("firefox ua skips client hints", func(t *testing.T) {
		c := NewImagineClient("tok", WithImagineUserAgent("Mozilla/5.0 Gecko/20100101 Firefox/135.0"))
		h := c.buildWSHeaders()
		assert.Empty(t, h.Get("Sec-Ch-Ua"))
		assert.Empty(t, h.Get("Sec-Ch-Ua-Platform"))
		assert.Empty(t, h.Get("Sec-Ch-Ua-Arch"))
	})

	t.Run("cf cookies and clearance in cookie", func(t *testing.T) {
		c := NewImagineClient("tok",
			WithImagineCFCookies("a=1"),
			WithImagineCFClearance("CLR"),
		)
		h := c.buildWSHeaders()
		cookie := h.Get("Cookie")
		assert.Contains(t, cookie, "a=1")
		assert.Contains(t, cookie, "cf_clearance=CLR")
	})
}

func TestBuildDialer(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		c := NewImagineClient("tok")
		d, err := c.buildDialer()
		require.NoError(t, err)
		assert.Nil(t, d.Proxy)
		assert.Nil(t, d.NetDialContext)
		assert.Nil(t, d.TLSClientConfig)
	})

	t.Run("skip proxy ssl verify", func(t *testing.T) {
		c := NewImagineClient("tok", WithImagineSkipProxySSLVerify(true))
		d, err := c.buildDialer()
		require.NoError(t, err)
		require.NotNil(t, d.TLSClientConfig)
		assert.True(t, d.TLSClientConfig.InsecureSkipVerify)
	})

	t.Run("http proxy", func(t *testing.T) {
		c := NewImagineClient("tok", WithImagineProxy("http://proxy.local:8080"))
		d, err := c.buildDialer()
		require.NoError(t, err)
		assert.NotNil(t, d.Proxy)
	})

	t.Run("socks5 proxy", func(t *testing.T) {
		c := NewImagineClient("tok", WithImagineProxy("socks5://127.0.0.1:1080"))
		d, err := c.buildDialer()
		require.NoError(t, err)
		assert.NotNil(t, d.NetDialContext)
	})
}

func TestApplyProxyToDialer(t *testing.T) {
	t.Run("https scheme", func(t *testing.T) {
		d := &websocket.Dialer{}
		require.NoError(t, applyProxyToDialer(d, "https://proxy.local:8443"))
		assert.NotNil(t, d.Proxy)
	})

	t.Run("socks5h with auth", func(t *testing.T) {
		d := &websocket.Dialer{}
		require.NoError(t, applyProxyToDialer(d, "socks5h://user:pass@127.0.0.1:1080"))
		require.NotNil(t, d.NetDialContext)

		// Invoke the dial closure: the SOCKS5 handshake against a closed
		// localhost port fails without touching the network.
		_, err := d.NetDialContext(context.Background(), "tcp", "127.0.0.1:1")
		require.Error(t, err)
	})

	t.Run("unparseable url", func(t *testing.T) {
		d := &websocket.Dialer{}
		err := applyProxyToDialer(d, "http://[::1")
		require.Error(t, err)
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		d := &websocket.Dialer{}
		err := applyProxyToDialer(d, "ftp://proxy.local:21")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported proxy scheme")
	})
}

func TestNewSocksProxyDialer(t *testing.T) {
	t.Run("with auth", func(t *testing.T) {
		u, err := url.Parse("socks5://user:secret@127.0.0.1:1080")
		require.NoError(t, err)
		d, err := newSocksProxyDialer(u)
		require.NoError(t, err)
		assert.NotNil(t, d)
	})

	t.Run("without auth", func(t *testing.T) {
		u, err := url.Parse("socks5://127.0.0.1:1080")
		require.NoError(t, err)
		d, err := newSocksProxyDialer(u)
		require.NoError(t, err)
		assert.NotNil(t, d)
	})
}

func TestMapWebsocketHandshakeError(t *testing.T) {
	tests := []struct {
		name    string
		resp    *http.Response
		wantErr error
		wantTxt string
	}{
		{"nil response", nil, nil, ""},
		{"401", bodyResponse(http.StatusUnauthorized, "application/json", "{}"), ErrInvalidToken, ""},
		{"429", bodyResponse(http.StatusTooManyRequests, "application/json", "{}"), ErrRateLimited, ""},
		{"403 challenge", bodyResponse(http.StatusForbidden, "text/html", "<html>moment</html>"), ErrCFChallenge, ""},
		{"403 plain", bodyResponse(http.StatusForbidden, "application/json", `{"e":1}`), ErrForbidden, ""},
		{"500", bodyResponse(http.StatusInternalServerError, "text/plain", "boom"), nil, "websocket bad handshake: 500"},
		{
			"body read error",
			&http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{},
				Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
			},
			nil,
			"read websocket handshake body",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mapWebsocketHandshakeError(tt.resp)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			if tt.wantTxt != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantTxt)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestGenerateHandshakeStatusMapping(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"401 maps to invalid token", http.StatusUnauthorized, ErrInvalidToken},
		{"429 maps to rate limited", http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			c := &ImagineClient{wsURL: wsURL(server.URL), token: "tok"}
			_, err := c.Generate(context.Background(), "p", "1:1", false, false)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestGenerateDialRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := server.URL
	server.Close() // nothing is listening anymore

	c := &ImagineClient{wsURL: wsURL(addr), token: "tok"}
	_, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "websocket dial")
}

func TestGenerateBuildDialerError(t *testing.T) {
	c := NewImagineClient("tok", WithImagineProxy("ftp://proxy.local:21"))
	_, err := c.Generate(context.Background(), "p", "1:1", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build websocket dialer")
}

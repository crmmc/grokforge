package xai

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/crmmc/grokforge/internal/upstream/transport"
)

// client implements the Client interface using curl-impersonate transport.
type client struct {
	token     string
	opts      *Options
	http      transport.Doer
	assetHTTP transport.Doer
	statsigID string
	mu        sync.Mutex
	closed    bool
}

// NewClient creates a new Grok API client with the given token and options.
func NewClient(token string, opts ...ClientOption) (Client, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	c := &client{
		token: token,
		opts:  options,
	}
	if !options.DynamicStatsig {
		c.statsigID = staticStatsigID
	}

	if err := c.initHTTPClients(); err != nil {
		return nil, err
	}

	return c, nil
}

// initHTTPClients creates the underlying curl-impersonate HTTP clients.
func (c *client) initHTTPClients() error {
	httpClient, err := c.newHTTPClient(c.opts.ProxyURL)
	if err != nil {
		return err
	}
	assetProxyURL := c.opts.ProxyURL
	if c.opts.AssetProxyURL != "" {
		assetProxyURL = c.opts.AssetProxyURL
	}
	assetHTTP, err := c.newHTTPClient(assetProxyURL)
	if err != nil {
		return err
	}
	c.http = httpClient
	c.assetHTTP = assetHTTP
	return nil
}

func newTransportDoer(opts *Options, proxyURL string) (transport.Doer, error) {
	profile := transport.EffectiveProfile(opts.Browser, opts.UserAgent)
	httpClient, err := transport.NewStatelessDoer(transport.Options{
		RequestTimeout:     opts.RequestTimeout,
		Browser:            profile,
		ProxyURL:           proxyURL,
		SkipProxySSLVerify: opts.SkipProxySSLVerify,
	})
	if err != nil {
		slog.Debug("xai: curl-impersonate init failed", "error", err, "browser", profile)
		return nil, err
	}

	// Mask proxy for logging
	maskedProxy := "(none)"
	if proxyURL != "" {
		maskedProxy = proxyURL
		if len(proxyURL) > 30 {
			maskedProxy = proxyURL[:30] + "..."
		}
	}
	slog.Debug("xai: curl-impersonate transport initialized",
		"browser_profile", opts.Browser,
		"effective_profile", profile,
		"proxy", maskedProxy,
		"timeout_sec", int(opts.RequestTimeout.Seconds()),
		"skip_proxy_ssl_verify", opts.SkipProxySSLVerify)

	return httpClient, nil
}

func (c *client) newHTTPClient(proxyURL string) (transport.Doer, error) {
	return newTransportDoer(c.opts, proxyURL)
}

// setProxy switches the underlying HTTP client's proxy by rebuilding clients.
func (c *client) setProxy(proxyURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrStreamClosed
	}
	c.opts.ProxyURL = proxyURL
	return c.initHTTPClients()
}

// ResetSession rebuilds the HTTP clients.
func (c *client) ResetSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrStreamClosed
	}

	slog.Debug("xai: resetting session (rebuilding curl-impersonate transport)")
	return c.initHTTPClients()
}

// Close releases resources held by the client.
func (c *client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	c.http = nil
	c.assetHTTP = nil
	return nil
}

// doRequest sends an HTTP request with anti-bot headers.
func (c *client) doRequest(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStreamClosed
	}
	httpClient := c.http
	c.mu.Unlock()
	return c.doRequestWithClient(req, httpClient)
}

func (c *client) doAssetRequest(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStreamClosed
	}
	httpClient := c.assetHTTP
	c.mu.Unlock()
	return c.doRequestWithClient(req, httpClient)
}

func (c *client) doRequestWithClient(req *http.Request, httpClient transport.Doer) (*http.Response, error) {
	// Set anti-bot headers
	headers := buildHeaders(c.token, c.opts, c.statsigID)
	return c.doRequestWithClientAndHeaders(req, httpClient, headers)
}

func (c *client) doConsoleRequest(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStreamClosed
	}
	httpClient := c.http
	c.mu.Unlock()
	headers := buildHeadersWithOrigin(c.token, c.opts, c.statsigID, "https://console.x.ai", "https://console.x.ai/")
	return c.doRequestWithClientAndHeaders(req, httpClient, headers)
}

func (c *client) doRequestWithClientAndHeaders(req *http.Request, httpClient transport.Doer, headers http.Header) (*http.Response, error) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	for k, v := range headers {
		if k == transport.HeaderOrderKey {
			req.Header[transport.HeaderOrderKey] = v
		} else {
			req.Header.Set(k, v[0])
		}
	}

	// Dump all outgoing headers at DEBUG level
	var hdrDump strings.Builder
	for _, key := range headers[transport.HeaderOrderKey] {
		val := req.Header.Get(key)
		// Mask sensitive Cookie value
		if key == "cookie" && len(val) > 40 {
			val = val[:20] + "..." + val[len(val)-20:]
		}
		fmt.Fprintf(&hdrDump, "\n  %s: %s", key, val)
	}
	slog.Debug("xai: outgoing request headers",
		"url", req.URL.String(),
		"method", req.Method,
		"headers", hdrDump.String())

	return httpClient.Do(req)
}

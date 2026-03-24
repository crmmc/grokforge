package xai

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
)

// client implements the Client interface using tls-client.
type client struct {
	token     string
	opts      *Options
	http      tls_client.HttpClient
	assetHTTP tls_client.HttpClient
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

// initHTTPClients creates the underlying tls-client HTTP clients.
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

func (c *client) newHTTPClient(proxyURL string) (tls_client.HttpClient, error) {
	jar := tls_client.NewCookieJar()
	profile := ResolveBrowserProfile(c.opts.Browser)

	tlsOpts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(int(c.opts.RequestTimeout.Seconds())),
		tls_client.WithClientProfile(profile),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
	}
	if c.opts.SkipProxySSLVerify {
		tlsOpts = append(tlsOpts, tls_client.WithInsecureSkipVerify())
	}

	if proxyURL != "" {
		tlsOpts = append(tlsOpts, tls_client.WithProxyUrl(proxyURL))
	}

	httpClient, err := tls_client.NewHttpClient(nil, tlsOpts...)
	if err != nil {
		slog.Debug("xai: tls-client init failed", "error", err, "browser", c.opts.Browser)
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
	slog.Debug("xai: tls-client initialized",
		"browser_profile", c.opts.Browser,
		"tls_profile", profile,
		"proxy", maskedProxy,
		"timeout_sec", int(c.opts.RequestTimeout.Seconds()),
		"skip_proxy_ssl_verify", c.opts.SkipProxySSLVerify)

	return httpClient, nil
}

// setProxy switches the underlying HTTP client's proxy.
func (c *client) setProxy(proxyURL string) error {
	if c.http == nil {
		return ErrStreamClosed
	}
	return c.http.SetProxy(proxyURL)
}

// ResetSession rebuilds the HTTP client and cookie jar.
func (c *client) ResetSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrStreamClosed
	}

	slog.Debug("xai: resetting session (clearing cookies, rebuilding TLS client)")
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

func (c *client) doRequestWithClient(req *http.Request, httpClient tls_client.HttpClient) (*http.Response, error) {
	// Set anti-bot headers
	headers := buildHeaders(c.token, c.opts, c.statsigID)
	for k, v := range headers {
		if k == http.HeaderOrderKey {
			req.Header[http.HeaderOrderKey] = v
		} else {
			req.Header.Set(k, v[0])
		}
	}

	// Dump all outgoing headers at DEBUG level
	var hdrDump strings.Builder
	for _, key := range headers[http.HeaderOrderKey] {
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

package xai

import "time"

// Options configures the xai.Client behavior.
type Options struct {
	// RequestTimeout sets the total timeout for each HTTP request
	// (including body reads). For streaming requests this acts as a
	// hard ceiling, so set it high enough to cover long thinking
	// sessions. Zero means no client-level timeout — the request's
	// context deadline is used instead.
	RequestTimeout time.Duration

	// RetryInterval is the delay between retry attempts.
	RetryInterval time.Duration

	// MaxRetry is the maximum number of retry attempts.
	MaxRetry int

	// UserAgent is the User-Agent header value.
	UserAgent string

	// Browser is the TLS client profile name (e.g. "chrome_133", "firefox_135").
	Browser string

	// ProxyURL is the optional proxy server URL (single proxy).
	ProxyURL string

	// AssetProxyURL is the optional proxy for asset downloads.
	AssetProxyURL string

	// SkipProxySSLVerify skips TLS verification when using proxies.
	SkipProxySSLVerify bool

	// DynamicStatsig controls per-request statsig ID generation.
	DynamicStatsig bool

	// CFClearance is the optional Cloudflare cf_clearance cookie value.
	CFClearance string

	// CFCookies is the optional Cloudflare cookies string.
	CFCookies string
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() *Options {
	return &Options{
		RequestTimeout:     0, // rely on context deadline; no client-level timeout
		RetryInterval:      1 * time.Second,
		MaxRetry:           3,
		UserAgent:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
		ProxyURL:           "",
		AssetProxyURL:      "",
		DynamicStatsig:     true,
		SkipProxySSLVerify: false,
	}
}

// ClientOption is a function that modifies Options.
type ClientOption func(*Options)

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(o *Options) {
		o.RequestTimeout = d
	}
}

// WithRetryInterval sets the retry interval.
func WithRetryInterval(d time.Duration) ClientOption {
	return func(o *Options) {
		o.RetryInterval = d
	}
}

// WithMaxRetry sets the maximum retry count.
func WithMaxRetry(n int) ClientOption {
	return func(o *Options) {
		o.MaxRetry = n
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(o *Options) {
		o.UserAgent = ua
	}
}

// WithProxy sets the proxy URL.
func WithProxy(url string) ClientOption {
	return func(o *Options) {
		o.ProxyURL = url
	}
}

// WithAssetProxy sets the proxy URL for asset downloads.
func WithAssetProxy(url string) ClientOption {
	return func(o *Options) {
		o.AssetProxyURL = url
	}
}

// WithSkipProxySSLVerify toggles TLS verification when using proxies.
func WithSkipProxySSLVerify(enabled bool) ClientOption {
	return func(o *Options) {
		o.SkipProxySSLVerify = enabled
	}
}

// WithDynamicStatsig toggles per-request statsig ID generation.
func WithDynamicStatsig(enabled bool) ClientOption {
	return func(o *Options) {
		o.DynamicStatsig = enabled
	}
}

// WithBrowser sets the TLS client profile name.
func WithBrowser(name string) ClientOption {
	return func(o *Options) {
		o.Browser = name
	}
}

// WithCFClearance sets the Cloudflare cf_clearance cookie value.
func WithCFClearance(val string) ClientOption {
	return func(o *Options) {
		o.CFClearance = val
	}
}

// WithCFCookies sets additional Cloudflare cookies.
func WithCFCookies(val string) ClientOption {
	return func(o *Options) {
		o.CFCookies = val
	}
}

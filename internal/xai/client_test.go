package xai

import (
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c, err := NewClient("test-token")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	defer c.Close()
}

func TestNewClient_WithOptions(t *testing.T) {
	c, err := NewClient("test-token",
		WithTimeout(60*time.Second),
		WithMaxRetry(5),
		WithRetryInterval(2*time.Second),
		WithUserAgent("custom-agent"),
		WithAssetProxy("http://asset-proxy.local:8080"),
		WithSkipProxySSLVerify(true),
	)
	if err != nil {
		t.Fatalf("NewClient with options failed: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	defer c.Close()

	// Verify options were applied
	impl, ok := c.(*client)
	if !ok {
		t.Fatal("Client is not *client type")
	}
	if impl.opts.RequestTimeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", impl.opts.RequestTimeout)
	}
	if impl.opts.MaxRetry != 5 {
		t.Errorf("MaxRetry = %d, want 5", impl.opts.MaxRetry)
	}
	if impl.opts.RetryInterval != 2*time.Second {
		t.Errorf("RetryInterval = %v, want 2s", impl.opts.RetryInterval)
	}
	if impl.opts.UserAgent != "custom-agent" {
		t.Errorf("UserAgent = %q, want %q", impl.opts.UserAgent, "custom-agent")
	}
	if impl.opts.AssetProxyURL != "http://asset-proxy.local:8080" {
		t.Errorf("AssetProxyURL = %q, want %q", impl.opts.AssetProxyURL, "http://asset-proxy.local:8080")
	}
	if !impl.opts.SkipProxySSLVerify {
		t.Error("SkipProxySSLVerify = false, want true")
	}
}

func TestBuildHeaders(t *testing.T) {
	opts := &Options{
		UserAgent: "test-agent",
		Browser:   "chrome133",
	}
	h := buildHeaders("test-token", opts, "")

	// Check canonical headers via Get()
	canonicalHeaders := []string{
		"Accept", "Accept-Encoding", "Accept-Language",
		"Baggage", "Content-Type", "Cookie", "Origin",
		"Priority", "Referer",
		"Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform",
		"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site",
		"User-Agent",
	}

	for _, key := range canonicalHeaders {
		if h.Get(key) == "" {
			t.Errorf("Missing header: %s", key)
		}
	}

	// Check non-canonical headers directly (lowercase keys)
	if _, ok := h["x-statsig-id"]; !ok {
		t.Error("Missing header: x-statsig-id")
	}
	if _, ok := h["x-xai-request-id"]; !ok {
		t.Error("Missing header: x-xai-request-id")
	}

	// Check User-Agent value
	if h.Get("User-Agent") != "test-agent" {
		t.Errorf("User-Agent = %q, want %q", h.Get("User-Agent"), "test-agent")
	}
}

func TestBuildHeaders_Cookie(t *testing.T) {
	opts := &Options{
		UserAgent: "test-agent",
	}
	h := buildHeaders("my-secret-token", opts, "")

	cookie := h.Get("Cookie")
	if !strings.Contains(cookie, "sso=my-secret-token") {
		t.Errorf("Cookie missing sso=token: %s", cookie)
	}
	if !strings.Contains(cookie, "sso-rw=my-secret-token") {
		t.Errorf("Cookie missing sso-rw=token: %s", cookie)
	}

	expected := "sso=my-secret-token; sso-rw=my-secret-token"
	if cookie != expected {
		t.Errorf("Cookie = %q, want %q", cookie, expected)
	}
}

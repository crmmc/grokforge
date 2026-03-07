// Package xai provides WebSocket client for Grok imagine API.
package xai

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"
)

// WebSocket imagine endpoint
const WSImagineURL = "wss://grok.com/ws/imagine/listen"

const (
	defaultWSReadDeadline = 60 * time.Second
	legacyMediumMinBytes  = 30000
	legacyFinalMinBytes   = 100000
)

// DefaultUserAgent is the default User-Agent for WebSocket connections.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

var imagineImageURLPattern = regexp.MustCompile(`/images/([a-f0-9-]+)\.(png|jpg|jpeg)`)

// ImageEventType represents the type of image generation event.
type ImageEventType string

const (
	ImageEventPreview ImageEventType = "preview"
	ImageEventMedium  ImageEventType = "medium"
	ImageEventFinal   ImageEventType = "final"
	ImageEventBlocked ImageEventType = "blocked"
	ImageEventError   ImageEventType = "error"
)

// ImageEvent represents an event from the image generation stream.
type ImageEvent struct {
	Type      ImageEventType
	RequestID string
	ImageData string
	Error     error
}

// ImagineClient handles WebSocket communication for image generation.
type ImagineClient struct {
	wsURL              string
	token              string
	blockedGraceMillis int64
	userAgent          string
	cfClearance        string
	cfCookies          string
	proxyURL           string
	skipProxySSLVerify bool
}

// ImagineClientOption configures ImagineClient.
type ImagineClientOption func(*ImagineClient)

// WithImagineUserAgent sets custom WebSocket User-Agent.
func WithImagineUserAgent(ua string) ImagineClientOption {
	return func(c *ImagineClient) { c.userAgent = ua }
}

// WithImagineCFClearance sets cf_clearance cookie for WebSocket.
func WithImagineCFClearance(v string) ImagineClientOption {
	return func(c *ImagineClient) { c.cfClearance = v }
}

// WithImagineCFCookies sets additional Cloudflare cookies for WebSocket.
func WithImagineCFCookies(v string) ImagineClientOption {
	return func(c *ImagineClient) { c.cfCookies = v }
}

// WithImagineProxy sets the proxy URL for WebSocket connections.
func WithImagineProxy(url string) ImagineClientOption {
	return func(c *ImagineClient) { c.proxyURL = url }
}

// WithImagineSkipProxySSLVerify toggles TLS verification for WebSocket connections.
func WithImagineSkipProxySSLVerify(enabled bool) ImagineClientOption {
	return func(c *ImagineClient) { c.skipProxySSLVerify = enabled }
}

// NewImagineClient creates a new imagine client.
func NewImagineClient(token string, opts ...ImagineClientOption) *ImagineClient {
	c := &ImagineClient{
		wsURL:              WSImagineURL,
		token:              token,
		blockedGraceMillis: 10000, // 10 seconds default
		userAgent:          DefaultUserAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ImagineRequest is the WebSocket request message for image generation.
type ImagineRequest struct {
	Type      string             `json:"type"`
	Timestamp int64              `json:"timestamp"`
	Item      ImagineRequestItem `json:"item"`
}

// ImagineRequestItem contains the request content.
type ImagineRequestItem struct {
	Type    string                  `json:"type"`
	Content []ImagineRequestContent `json:"content"`
}

// ImagineRequestContent contains the prompt and properties.
type ImagineRequestContent struct {
	RequestID  string                   `json:"requestId"`
	Text       string                   `json:"text"`
	Type       string                   `json:"type"`
	Properties ImagineRequestProperties `json:"properties"`
}

// ImagineRequestProperties contains generation parameters.
type ImagineRequestProperties struct {
	SectionCount  int    `json:"section_count"`
	IsKidsMode    bool   `json:"is_kids_mode"`
	EnableNSFW    bool   `json:"enable_nsfw"`
	SkipUpsampler bool   `json:"skip_upsampler"`
	IsInitial     bool   `json:"is_initial"`
	AspectRatio   string `json:"aspect_ratio"`
	OriginalImage string `json:"original_image,omitempty"`
}

// ImagineResponse is the WebSocket response message.
type ImagineResponse struct {
	Type  string              `json:"type"`
	Item  ImagineResponseItem `json:"item,omitempty"`
	Error error               `json:"-"`
}

// ImagineResponseItem contains the response data.
type ImagineResponseItem struct {
	RequestID string `json:"requestId"`
	ImageData string `json:"imageData"`
}

// Generate starts image generation and returns a channel of events.
func (c *ImagineClient) Generate(ctx context.Context, prompt, aspectRatio string, enableNSFW bool) (<-chan ImageEvent, error) {
	return c.doGenerate(ctx, prompt, aspectRatio, "", enableNSFW)
}

// doGenerate is the internal implementation for Generate.
func (c *ImagineClient) doGenerate(
	ctx context.Context,
	prompt,
	aspectRatio,
	originalImageB64 string,
	enableNSFW bool,
) (<-chan ImageEvent, error) {
	headers := c.buildWSHeaders()

	// Connect to WebSocket
	dialer, err := c.buildDialer()
	if err != nil {
		return nil, fmt.Errorf("build websocket dialer: %w", err)
	}

	conn, _, err := dialer.DialContext(ctx, c.wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}

	eventCh := make(chan ImageEvent, 16)
	requestID := uuid.New().String()

	go c.streamImages(ctx, conn, requestID, prompt, aspectRatio, originalImageB64, enableNSFW, eventCh)

	return eventCh, nil
}

func (c *ImagineClient) buildWSHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Cookie", ssoCookie(c.token, c.cfCookies, c.cfClearance))
	headers.Set("Origin", "https://grok.com")
	headers.Set("User-Agent", c.userAgent)
	headers.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Pragma", "no-cache")
	hints := buildClientHints("", c.userAgent)
	if hints.SecChUa != "" {
		headers.Set("Sec-Ch-Ua", hints.SecChUa)
		headers.Set("Sec-Ch-Ua-Mobile", hints.SecChUaMobile)
	}
	if hints.SecChUaPlatform != "" {
		headers.Set("Sec-Ch-Ua-Platform", hints.SecChUaPlatform)
	}
	if hints.SecChUaArch != "" {
		headers.Set("Sec-Ch-Ua-Arch", hints.SecChUaArch)
		headers.Set("Sec-Ch-Ua-Bitness", hints.SecChUaBitness)
	}
	headers.Set("Sec-Ch-Ua-Model", hints.SecChUaModel)
	return headers
}

func (c *ImagineClient) buildDialer() (*websocket.Dialer, error) {
	dialer := &websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}
	if c.skipProxySSLVerify {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if c.proxyURL == "" {
		return dialer, nil
	}
	if err := applyProxyToDialer(dialer, c.proxyURL); err != nil {
		return nil, err
	}
	return dialer, nil
}

func applyProxyToDialer(dialer *websocket.Dialer, proxyURL string) error {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "http", "https":
		dialer.Proxy = http.ProxyURL(parsed)
		return nil
	case "socks5", "socks5h":
		socksDialer, err := newSocksProxyDialer(parsed)
		if err != nil {
			return err
		}
		dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if ctxDialer, ok := socksDialer.(proxy.ContextDialer); ok {
				return ctxDialer.DialContext(ctx, network, addr)
			}
			return socksDialer.Dial(network, addr)
		}
		return nil
	default:
		return fmt.Errorf("unsupported proxy scheme: %s", parsed.Scheme)
	}
}

func newSocksProxyDialer(u *url.URL) (proxy.Dialer, error) {
	var auth *proxy.Auth
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = &proxy.Auth{
			User:     u.User.Username(),
			Password: pass,
		}
	}
	return proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
}


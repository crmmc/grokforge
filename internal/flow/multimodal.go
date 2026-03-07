package flow

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/png" // PNG decoder
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // WebP decoder
)

// ContentBlock represents a single content block in a multimodal message.
type ContentBlock struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *ImageURLBlock `json:"image_url,omitempty"`
}

// ImageURLBlock represents an image URL with optional detail level.
type ImageURLBlock struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// ProcessedContent holds the result of processing multimodal content.
type ProcessedContent struct {
	Text   string   // Combined text from all text blocks
	Images []string // Data URIs for all images
}

// ParseMultimodalContent parses OpenAI-style content into ContentBlocks.
// Content can be a string or an array of content parts.
func ParseMultimodalContent(content any) ([]ContentBlock, error) {
	switch v := content.(type) {
	case string:
		return []ContentBlock{{Type: "text", Text: v}}, nil

	case []any:
		blocks := make([]ContentBlock, 0, len(v))
		for _, item := range v {
			block, err := parseContentPart(item)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		}
		return blocks, nil

	default:
		return nil, fmt.Errorf("unsupported content type: %T", content)
	}
}

func parseContentPart(item any) (ContentBlock, error) {
	m, ok := item.(map[string]any)
	if !ok {
		return ContentBlock{}, fmt.Errorf("content part must be object, got %T", item)
	}

	typ, _ := m["type"].(string)
	switch typ {
	case "text":
		text, _ := m["text"].(string)
		return ContentBlock{Type: "text", Text: text}, nil

	case "image_url":
		imgURL, ok := m["image_url"].(map[string]any)
		if !ok {
			return ContentBlock{}, fmt.Errorf("image_url must be object")
		}
		url, _ := imgURL["url"].(string)
		detail, _ := imgURL["detail"].(string)
		return ContentBlock{
			Type: "image_url",
			ImageURL: &ImageURLBlock{
				URL:    url,
				Detail: detail,
			},
		}, nil
	case "file", "input_file":
		return ContentBlock{Type: "text", Text: "[file input]"}, nil
	case "input_audio", "audio":
		return ContentBlock{Type: "text", Text: "[audio input]"}, nil

	default:
		return ContentBlock{}, fmt.Errorf("unknown content type: %s", typ)
	}
}

// ProcessContent processes content blocks, downloading images as needed.
func ProcessContent(ctx context.Context, blocks []ContentBlock) (*ProcessedContent, error) {
	result := &ProcessedContent{}
	var textParts []string

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)

		case "image_url":
			if block.ImageURL == nil {
				continue
			}
			dataURI, err := processImageURL(ctx, block.ImageURL.URL)
			if err != nil {
				return nil, fmt.Errorf("process image: %w", err)
			}
			result.Images = append(result.Images, dataURI)
		}
	}

	result.Text = strings.Join(textParts, "")
	return result, nil
}

// processImageURL converts an image URL to a data URI.
// If already a data URI, returns as-is.
// If HTTP(S) URL, downloads and converts to data URI.
func processImageURL(ctx context.Context, rawURL string) (string, error) {
	// Data URI - pass through
	if strings.HasPrefix(rawURL, "data:") {
		return rawURL, nil
	}

	// HTTP(S) URL - download with SSRF protection
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		resolvedIPs, err := validateURLSafety(rawURL)
		if err != nil {
			return "", err
		}
		return downloadAsDataURI(ctx, rawURL, resolvedIPs)
	}

	return "", fmt.Errorf("unsupported URL scheme: %s", rawURL)
}

func downloadAsDataURI(ctx context.Context, rawURL string, resolvedIPs []net.IP) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: pinnedDialer(resolvedIPs, port),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	// Read body with size limit (20MB)
	const maxSize = 20 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	// Decode, resize, and convert to JPEG
	return processImageData(data, resp.Header.Get("Content-Type"))
}

// processImageData preserves the original image bytes and MIME type.
func processImageData(data []byte, contentType string) (string, error) {
	mimeType := detectImageMIME(contentType, data)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("unsupported image mime type: %s", mimeType)
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return "data:" + mimeType + ";base64," + b64, nil
}

func detectImageMIME(contentType string, data []byte) string {
	mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if strings.HasPrefix(mimeType, "image/") {
		return mimeType
	}
	return strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
}

// resizeImageMaxDim resizes image if any dimension exceeds maxDim.
func resizeImageMaxDim(img image.Image, maxDim int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w <= maxDim && h <= maxDim {
		return img
	}

	// Calculate new dimensions maintaining aspect ratio
	var newW, newH int
	if w > h {
		newW = maxDim
		newH = h * maxDim / w
	} else {
		newH = maxDim
		newW = w * maxDim / h
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}

// validateURLSafety blocks requests to private/internal IP ranges (SSRF protection).
// Returns the resolved IPs for use with pinnedDialer to prevent DNS rebinding.
func validateURLSafety(rawURL string) ([]net.IP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	host := parsed.Hostname()

	// Block obvious internal hostnames
	if host == "localhost" || host == "metadata.google.internal" {
		return nil, fmt.Errorf("blocked internal host: %s", host)
	}

	// Resolve and check IP ranges
	addrs, err := net.LookupHost(host)
	if err != nil {
		return nil, fmt.Errorf("resolve host: %w", err)
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, ipStr := range addrs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return nil, fmt.Errorf("blocked private/internal IP: %s", ipStr)
		}
		ips = append(ips, ip)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no valid IPs resolved for host: %s", host)
	}

	return ips, nil
}

// pinnedDialer returns a DialContext function that connects directly to the
// pre-resolved IPs, preventing DNS rebinding attacks (TOCTOU between
// validateURLSafety and the actual HTTP request).
func pinnedDialer(resolvedIPs []net.IP, port string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var lastErr error
		for _, ip := range resolvedIPs {
			addr := net.JoinHostPort(ip.String(), port)
			conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
			if err != nil {
				lastErr = err
				continue
			}
			return conn, nil
		}
		return nil, fmt.Errorf("all resolved IPs failed: %w", lastErr)
	}
}

package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

// Video API endpoints.
const (
	MediaPostAPI    = "https://grok.com/rest/media/post/create"
	VideoUpscaleAPI = "https://grok.com/rest/media/video/upscale"
)

// Response size limits for io.LimitReader.
const (
	maxJSONResponseSize  = 1 << 20   // 1MB — API JSON metadata responses
	maxAssetDownloadSize = 200 << 20 // 200MB — video/image binary downloads
)

// Video errors.
var (
	ErrVideoUpstreamFailed = errors.New("video upstream request failed")
)

// MediaPostRequest is the request body for media post create.
type MediaPostRequest struct {
	MediaType string `json:"mediaType"`
	MediaURL  string `json:"mediaUrl,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Size      string `json:"size,omitempty"`
	Seconds   int    `json:"seconds,omitempty"`
	Quality   string `json:"quality,omitempty"`
}

// MediaPostResponse is the response from media post create.
type MediaPostResponse struct {
	MediaURL string `json:"mediaUrl"`
	Post     struct {
		ID string `json:"id"`
	} `json:"post"`
	// Other fields may exist but we only need mediaUrl
}

// VideoUpscaleRequest is the request body for video upscale.
type VideoUpscaleRequest struct {
	VideoID string `json:"videoId"`
}

// VideoUpscaleResponse is the response from video upscale.
type VideoUpscaleResponse struct {
	HDMediaURL string `json:"hdMediaUrl"`
	Status     string `json:"status"`
}

// CreateImagePost creates an image media post and returns the post ID.
func (c *client) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	if strings.TrimSpace(imageURL) == "" {
		return "", fmt.Errorf("create image post: imageURL is required")
	}

	result, err := c.createMediaPost(ctx, MediaPostRequest{
		MediaType: "MEDIA_POST_TYPE_IMAGE",
		MediaURL:  imageURL,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Post.ID) == "" {
		return "", fmt.Errorf("create image post: missing post id")
	}

	return result.Post.ID, nil
}

// CreateVideoPost creates a video media post and returns the post ID.
func (c *client) CreateVideoPost(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("create video post: prompt is required")
	}

	result, err := c.createMediaPost(ctx, MediaPostRequest{
		MediaType: "MEDIA_POST_TYPE_VIDEO",
		Prompt:    prompt,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Post.ID) == "" {
		return "", fmt.Errorf("create video post: missing post id")
	}

	return result.Post.ID, nil
}

func (c *client) createMediaPost(ctx context.Context, payload MediaPostRequest) (*MediaPostResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, MediaPostAPI, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrVideoUpstreamFailed, resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result MediaPostResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// VideoUpscale triggers video upscale and returns the video URL.
func (c *client) VideoUpscale(ctx context.Context, videoID string) (string, error) {
	payload := VideoUpscaleRequest{
		VideoID: videoID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, VideoUpscaleAPI, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
		return "", fmt.Errorf("%w: status %d, body: %s", ErrVideoUpstreamFailed, resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var result VideoUpscaleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return result.HDMediaURL, nil
}

// PollUpscale polls for video completion with retries.
func (c *client) PollUpscale(ctx context.Context, videoID string, interval time.Duration) (string, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			videoURL, err := c.VideoUpscale(ctx, videoID)
			if err != nil {
				// Fatal errors: stop polling immediately
				if errors.Is(err, ErrForbidden) || errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrCFChallenge) {
					return "", err
				}
				// Transient errors: continue polling
				continue
			}
			if videoURL != "" {
				return videoURL, nil
			}
		}
	}
}

const assetsBaseURL = "https://assets.grok.com/"

// normalizeAssetURL ensures the URL has a scheme. Grok API sometimes returns
// relative paths like "users/xxx/generated_video.mp4" without the base URL.
func normalizeAssetURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return assetsBaseURL + strings.TrimPrefix(trimmed, "/")
}

// DownloadURL downloads the content at the given URL using the client's session (cookies/proxy).
func (c *client) DownloadURL(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = normalizeAssetURL(rawURL)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStreamClosed
	}
	httpClient := c.http
	c.mu.Unlock()
	if httpClient == nil {
		return nil, ErrStreamClosed
	}

	var restoreProxy string
	restoreNeeded := false
	if c.opts.AssetProxyURL != "" {
		currentProxy := httpClient.GetProxy()
		if currentProxy != c.opts.AssetProxyURL {
			if err := c.setProxy(c.opts.AssetProxyURL); err != nil {
				return nil, fmt.Errorf("set asset proxy: %w", err)
			}
			restoreProxy = currentProxy
			restoreNeeded = true
		}
	}
	if restoreNeeded {
		defer func() {
			if err := c.setProxy(restoreProxy); err != nil {
				slog.Warn("failed to restore proxy", "error", err)
			}
		}()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetDownloadSize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

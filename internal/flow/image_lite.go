package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
)

// ImageLiteRequest represents a lite image generation request (Aurora path).
type ImageLiteRequest struct {
	Model          string
	Prompt         string
	N              int
	UpstreamMode   string // "fast"
	ResponseFormat string
	Mode           string // mode from registry for quota tracking
}

// Validate validates the lite image request.
func (r *ImageLiteRequest) Validate() error {
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	if r.N == 0 {
		r.N = 1
	}
	if r.N > 4 {
		return errors.New("n must be between 1 and 4")
	}
	if r.ResponseFormat == "" {
		r.ResponseFormat = "b64_json"
	}
	if r.ResponseFormat != "url" && r.ResponseFormat != "b64_json" && r.ResponseFormat != "base64" {
		return errors.New("response_format must be url, b64_json, or base64")
	}
	return nil
}

// GenerateLite generates images via the chat endpoint (Aurora path).
// This is used for image-lite models that don't go through WebSocket.
func (f *ImageFlow) GenerateLite(ctx context.Context, req *ImageLiteRequest) (*ImageResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	if f.editClientFactory == nil {
		return nil, errors.New("chat client not configured for lite image generation")
	}

	ctx, cancel := context.WithTimeout(ctx, imageGenerationTimeout)
	defer cancel()

	// Resolve mode from request or model registry
	mode := req.Mode
	if mode == "" {
		mode = f.resolveMode(req.Model)
	}

	start := time.Now()
	apiKeyID := FlowAPIKeyIDFromContext(ctx)

	tok, err := f.pickTokenForModel(req.Model, mode)
	if err != nil {
		return nil, err
	}

	client := f.editClientFactory(tok.Token)
	if client == nil {
		f.tokenSvc.ReleaseToken(tok.ID)
		return nil, errors.New("chat client is nil")
	}

	images := make([]ImageData, 0, req.N)
	for i := 0; i < req.N; i++ {
		data, err := f.generateLiteSingle(ctx, tok.ID, mode, client, req)
		if err != nil {
			reportTrackedTokenError(f.tokenSvc, tok.ID, mode, err)
			f.recordLiteUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
			return nil, err
		}
		images = append(images, *data)
	}

	f.tokenSvc.ReportSuccess(tok.ID)
	f.recordLiteUsage(apiKeyID, tok.ID, req.Model, 200, time.Since(start))
	return &ImageResponse{
		Created: time.Now().Unix(),
		Data:    images,
	}, nil
}

func (f *ImageFlow) generateLiteSingle(
	ctx context.Context,
	tokenID uint,
	mode string,
	client ImageEditClient,
	req *ImageLiteRequest,
) (*ImageData, error) {
	chatReq := &xai.ChatRequest{
		Messages: []xai.Message{
			{Role: "user", Content: "Drawing: " + req.Prompt},
		},
		Stream:       true,
		UpstreamMode: req.UpstreamMode,
	}

	// Populate Grok-specific params from app config
	if appCfg := f.appConfig(); appCfg != nil {
		chatReq.Temporary = appCfg.Temporary
		chatReq.DisableMemory = appCfg.DisableMemory
	}

	eventCh, err := client.Chat(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("lite image chat: %w", err)
	}
	f.tokenSvc.RecordFirstUse(tokenID, mode)

	// Collect image URLs from SSE stream
	var imageURLs []string
	for event := range eventCh {
		if event.Error != nil {
			return nil, fmt.Errorf("lite image stream: %w", event.Error)
		}
		urls := extractImageURLsFromEvent(event.Data)
		imageURLs = append(imageURLs, urls...)
	}

	if len(imageURLs) == 0 {
		return nil, errors.New("no images generated in lite response")
	}

	// Use the first image URL
	imgURL := imageURLs[0]

	if req.ResponseFormat == "url" {
		return &ImageData{URL: imgURL}, nil
	}

	// Download and convert to base64
	imgBytes, err := client.DownloadURL(ctx, imgURL)
	if err != nil {
		return nil, fmt.Errorf("download lite image: %w", err)
	}
	return &ImageData{
		B64JSON: base64.StdEncoding.EncodeToString(imgBytes),
	}, nil
}

// extractImageURLsFromEvent extracts image URLs from a stream event's JSON data.
func extractImageURLsFromEvent(data json.RawMessage) []string {
	var result struct {
		Result struct {
			Response struct {
				ModelResponse *struct {
					GeneratedImageUrls []string `json:"generatedImageUrls"`
				} `json:"modelResponse"`
			} `json:"response"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		slog.Debug("lite image: failed to parse event", "error", err)
		return nil
	}
	if mr := result.Result.Response.ModelResponse; mr != nil {
		return mr.GeneratedImageUrls
	}
	return nil
}

func (f *ImageFlow) recordLiteUsage(apiKeyID, tokenID uint, model string, status int, latency time.Duration) {
	if f.usageLog == nil {
		return
	}
	_ = f.usageLog.Record(context.Background(), &store.UsageLog{
		APIKeyID:   apiKeyID,
		TokenID:    tokenID,
		Model:      model,
		Endpoint:   "image_lite",
		Status:     status,
		DurationMs: latency.Milliseconds(),
		CreatedAt:  time.Now(),
	})
}

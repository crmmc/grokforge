package flow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

var imageGenerationTimeout = 120 * time.Second

// ImagineGenerator defines the interface for image generation.
type ImagineGenerator interface {
	Generate(ctx context.Context, prompt, aspectRatio string, enableNSFW bool) (<-chan xai.ImageEvent, error)
}

// ImageRequest represents an OpenAI-compatible image generation request.
type ImageRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"`
	User           string `json:"user,omitempty"`
	EnableNSFW     *bool  `json:"enable_nsfw,omitempty"`
}

// Validate validates the image request.
func (r *ImageRequest) Validate() error {
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	if r.N == 0 {
		r.N = 1
	}
	if r.N > 10 {
		return errors.New("n must be between 1 and 10")
	}
	if r.Size == "" {
		r.Size = "1024x1024"
	}
	if r.ResponseFormat == "" {
		r.ResponseFormat = "b64_json"
	}
	if r.ResponseFormat != "url" && r.ResponseFormat != "b64_json" && r.ResponseFormat != "base64" {
		return errors.New("response_format must be url, b64_json, or base64")
	}
	if r.Quality != "" {
		return errors.New("quality is not supported")
	}
	if r.Style != "" {
		return errors.New("style is not supported")
	}
	return nil
}

// ImageResponse represents an OpenAI-compatible image generation response.
type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

// ImageData represents a single generated image.
type ImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageEditRequest represents an image edit request.
type ImageEditRequest struct {
	Model          string
	Prompt         string
	OriginalImages [][]byte
	N              int
	Size           string
	ResponseFormat string
	EnableNSFW     *bool
}

// Validate validates the image edit request.
func (r *ImageEditRequest) Validate() error {
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	if len(r.OriginalImages) == 0 {
		return errors.New("at least one original image is required")
	}
	if r.N == 0 {
		r.N = 1
	}
	if r.N > 10 {
		return errors.New("n must be between 1 and 10")
	}
	if r.Size == "" {
		r.Size = "1024x1024"
	}
	if r.ResponseFormat == "" {
		r.ResponseFormat = "b64_json"
	}
	if r.ResponseFormat != "url" && r.ResponseFormat != "b64_json" && r.ResponseFormat != "base64" {
		return errors.New("response_format must be url, b64_json, or base64")
	}
	return nil
}

// ImagineClientFactory creates ImagineGenerator instances for a given token.
type ImagineClientFactory func(token string) ImagineGenerator

// ImageFlow handles image generation orchestration.
type ImageFlow struct {
	tokenSvc          TokenServicer
	clientFactory     ImagineClientFactory
	editClientFactory ImageEditClientFactory
	usageLog          UsageRecorder
	tokenConfigFn     func() *config.TokenConfig
	appConfigFn       func() *config.AppConfig
	imageConfigFn     func() *config.ImageConfig
	modelResolver     tkn.ModelResolver
}

// NewImageFlow creates a new image flow with per-request token selection.
func NewImageFlow(tokenSvc TokenServicer, clientFactory ImagineClientFactory) *ImageFlow {
	return &ImageFlow{
		tokenSvc:      tokenSvc,
		clientFactory: clientFactory,
	}
}

// SetUsageRecorder sets the usage recorder for logging API usage.
func (f *ImageFlow) SetUsageRecorder(ur UsageRecorder) {
	f.usageLog = ur
}

// SetTokenConfig sets model-to-pool mapping config for token selection.
func (f *ImageFlow) SetTokenConfig(cfg *config.TokenConfig) {
	f.tokenConfigFn = func() *config.TokenConfig { return cfg }
}

// SetTokenConfigProvider sets a dynamic token config provider.
func (f *ImageFlow) SetTokenConfigProvider(fn func() *config.TokenConfig) {
	f.tokenConfigFn = fn
}

// SetEditClientFactory sets the app-chat client factory used by image edits.
func (f *ImageFlow) SetEditClientFactory(factory ImageEditClientFactory) {
	f.editClientFactory = factory
}

// SetAppConfig sets app-level defaults for app-chat based image edits.
func (f *ImageFlow) SetAppConfig(cfg *config.AppConfig) {
	f.appConfigFn = func() *config.AppConfig { return cfg }
}

// SetAppConfigProvider sets a dynamic app config provider.
func (f *ImageFlow) SetAppConfigProvider(fn func() *config.AppConfig) {
	f.appConfigFn = fn
}

// SetModelResolver sets the model resolver for pool routing.
func (f *ImageFlow) SetModelResolver(resolver tkn.ModelResolver) {
	f.modelResolver = resolver
}

// SetImageConfig sets image-generation defaults and retry behavior.
func (f *ImageFlow) SetImageConfig(cfg *config.ImageConfig) {
	f.imageConfigFn = func() *config.ImageConfig { return cfg }
}

// SetImageConfigProvider sets a dynamic image config provider.
func (f *ImageFlow) SetImageConfigProvider(fn func() *config.ImageConfig) {
	f.imageConfigFn = fn
}

// Generate generates images based on the request.
func (f *ImageFlow) Generate(ctx context.Context, req *ImageRequest) (*ImageResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, imageGenerationTimeout)
	defer cancel()

	start := time.Now()
	aspectRatio := xai.ParseAspectRatio(req.Size)
	apiKeyID := FlowAPIKeyIDFromContext(ctx)

	// Pick token per request
	tok, err := f.pickTokenForModel(req.Model)
	if err != nil {
		return nil, fmt.Errorf("no token available: %w", err)
	}

	// Generate N images (consume quota per image, only after success)
	images := make([]ImageData, 0, req.N)
	enableNSFW := resolveEnableNSFW(req.EnableNSFW, f.imageConfig())
	usedTokenIDs := make(map[uint]struct{})
	for i := 0; i < req.N; i++ {
		result, err := f.generateWithRecovery(ctx, req.Model, tok, req.Prompt, aspectRatio, enableNSFW)
		if err != nil {
			if !isTransportError(err) {
				f.tokenSvc.ReportError(tok.ID, err.Error())
			}
			f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
			return nil, err
		}
		tok = result.token
		if !result.consumed {
			if _, err := f.tokenSvc.Consume(tok.ID, tkn.CategoryImage, 1); err != nil {
				f.recordUsage(apiKeyID, tok.ID, req.Model, 429, time.Since(start))
				return nil, fmt.Errorf("token quota exhausted: %w", err)
			}
		}
		images = append(images, *result.data)
		usedTokenIDs[tok.ID] = struct{}{}
	}

	for tokenID := range usedTokenIDs {
		f.tokenSvc.ReportSuccess(tokenID)
	}
	f.recordUsage(apiKeyID, tok.ID, req.Model, 200, time.Since(start))
	return &ImageResponse{
		Created: time.Now().Unix(),
		Data:    images,
	}, nil
}

// recordUsage records an API usage log entry via the buffer (non-blocking).
func (f *ImageFlow) recordUsage(apiKeyID, tokenID uint, model string, status int, latency time.Duration) {
	if f.usageLog == nil {
		return
	}
	_ = f.usageLog.Record(context.Background(), &store.UsageLog{
		APIKeyID:    apiKeyID,
		TokenID:     tokenID,
		Model:       model,
		Endpoint:    "image",
		Status:      status,
		DurationMs:  latency.Milliseconds(),
		TTFTMs:      0,
		CacheTokens: 0,
		CreatedAt:   time.Now(),
	})
}

func (f *ImageFlow) pickTokenForModel(model string) (*store.Token, error) {
	return f.pickTokenForModelExcluding(model, nil)
}

func (f *ImageFlow) pickTokenForModelExcluding(model string, exclude map[uint]struct{}) (*store.Token, error) {
	if f.modelResolver != nil {
		pools, ok := tkn.GetPoolForModel(model, f.modelResolver)
		if ok {
			var lastErr error
			for _, pool := range pools {
				tok, err := f.tokenSvc.PickExcluding(pool, tkn.CategoryImage, exclude)
				if err == nil {
					return tok, nil
				}
				lastErr = err
			}
			if lastErr != nil {
				return nil, lastErr
			}
		}
	}
	return f.tokenSvc.PickExcluding(tkn.PoolBasic, tkn.CategoryImage, exclude)
}

func (f *ImageFlow) tokenConfig() *config.TokenConfig {
	if f.tokenConfigFn == nil {
		return nil
	}
	return f.tokenConfigFn()
}

func (f *ImageFlow) appConfig() *config.AppConfig {
	if f.appConfigFn == nil {
		return nil
	}
	return f.appConfigFn()
}

func (f *ImageFlow) imageConfig() *config.ImageConfig {
	if f.imageConfigFn == nil {
		return nil
	}
	return f.imageConfigFn()
}

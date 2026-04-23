package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

var imageGenerationTimeout = 120 * time.Second

// ImagineGenerator defines the interface for image generation.
type ImagineGenerator interface {
	Generate(ctx context.Context, prompt, aspectRatio string, enableNSFW, enablePro bool) (<-chan xai.ImageEvent, error)
}

// ImageRequest represents an OpenAI-compatible image generation request.
type ImageRequest struct {
	Model           string `json:"model,omitempty"`
	Prompt          string `json:"prompt"`
	N               int    `json:"n,omitempty"`
	Size            string `json:"size,omitempty"`
	Quality         string `json:"quality,omitempty"`
	ResponseFormat  string `json:"response_format,omitempty"`
	Style           string `json:"style,omitempty"`
	User            string `json:"user,omitempty"`
	EnableNSFW      *bool  `json:"enable_nsfw,omitempty"`
	Mode            string `json:"-"` // mode from registry for quota tracking
	CooldownSeconds int    `json:"-"`
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
	UpstreamModel  string
	UpstreamMode   string
	Mode           string // mode from registry for quota tracking
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
	appConfigFn       func() *config.AppConfig
	imageConfigFn     func() *config.ImageConfig
	modelResolver     tkn.ModelResolver
	modeResolver      ModeResolver
	enableProFn       func(model string) bool
	cooldownMu        sync.Mutex
	cooldownUntil     map[string]time.Time
}

// NewImageFlow creates a new image flow with per-request token selection.
func NewImageFlow(tokenSvc TokenServicer, clientFactory ImagineClientFactory) *ImageFlow {
	return &ImageFlow{
		tokenSvc:      tokenSvc,
		clientFactory: clientFactory,
		cooldownUntil: make(map[string]time.Time),
	}
}

// SetUsageRecorder sets the usage recorder for logging API usage.
func (f *ImageFlow) SetUsageRecorder(ur UsageRecorder) {
	f.usageLog = ur
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

// SetModeResolver sets the mode resolver for quota tracking.
func (f *ImageFlow) SetModeResolver(resolver ModeResolver) {
	f.modeResolver = resolver
}

// resolveMode returns the quota mode for a model, or empty string if unknown.
func (f *ImageFlow) resolveMode(model string) string {
	if f.modeResolver == nil {
		return ""
	}
	mode, _ := f.modeResolver.ResolveMode(model)
	return mode
}

// SetImageConfig sets image-generation defaults and retry behavior.
func (f *ImageFlow) SetImageConfig(cfg *config.ImageConfig) {
	f.imageConfigFn = func() *config.ImageConfig { return cfg }
}

// SetImageConfigProvider sets a dynamic image config provider.
func (f *ImageFlow) SetImageConfigProvider(fn func() *config.ImageConfig) {
	f.imageConfigFn = fn
}

// SetEnableProResolver sets a function that resolves whether pro mode is enabled for a model.
func (f *ImageFlow) SetEnableProResolver(fn func(model string) bool) {
	f.enableProFn = fn
}

// Generate generates images based on the request.
func (f *ImageFlow) Generate(ctx context.Context, req *ImageRequest) (*ImageResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, imageGenerationTimeout)
	defer cancel()
	return f.generateWS(ctx, req)
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

func (f *ImageFlow) pickTokenForModel(model, mode string) (*store.Token, error) {
	return f.pickTokenForModelExcluding(model, mode, nil)
}

func (f *ImageFlow) pickTokenForModelExcluding(model, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	if f.modelResolver == nil {
		return nil, tkn.ErrModelNotFound
	}
	pools, ok := tkn.GetPoolForModel(model, f.modelResolver)
	if !ok {
		return nil, tkn.ErrModelNotFound
	}
	var lastErr error
	for _, pool := range pools {
		var (
			tok *store.Token
			err error
		)
		if mode == "" {
			tok, err = f.tokenSvc.PickAnyExcluding(pool, exclude)
		} else {
			tok, err = f.tokenSvc.PickExcluding(pool, mode, exclude)
		}
		if err == nil {
			return tok, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, tkn.ErrNoTokenAvailable
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

func (f *ImageFlow) resolveEnablePro(model string) bool {
	if f.enableProFn == nil {
		return false
	}
	return f.enableProFn(model)
}

package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

var (
	// ErrVideoCache indicates that the generated video could not be cached locally.
	ErrVideoCache = errors.New("video cache failed")
	// ErrVideoPostProcess indicates that post-generation video processing failed.
	ErrVideoPostProcess = errors.New("video postprocess failed")
)

// VideoClient defines the interface for video generation API calls.
type VideoClient interface {
	Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error)
	CreateImagePost(ctx context.Context, imageURL string) (string, error)
	CreateVideoPost(ctx context.Context, prompt string) (string, error)
	PollUpscale(ctx context.Context, videoID string, interval time.Duration) (string, error)
	DownloadTo(ctx context.Context, url string, w io.Writer) error
	DownloadURL(ctx context.Context, url string) ([]byte, error)
	UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error)
}

// VideoFlowConfig holds configuration for video processing.
type VideoFlowConfig struct {
	TimeoutSeconds      int
	PollIntervalSeconds int
	ModelResolver       tkn.ModelResolver
}

// VideoRequest represents a video generation request.
type VideoRequest struct {
	Prompt          string
	Model           string
	UpstreamModel   string
	UpstreamMode    string
	Mode            string // mode from registry for quota tracking
	Size            string
	AspectRatio     string // e.g. "16:9", "3:2" — passed directly to xAI
	Seconds         int
	Quality         string
	Preset          string
	ReferenceImages [][]byte
}

// VideoFlow handles async video generation.
type VideoFlow struct {
	tokenSvc      TokenServicer
	clientFactory func(token string) VideoClient
	cfg           *VideoFlowConfig
	usageLog      UsageRecorder
	cacheSvc      *cache.Service
	appConfigFn   func() *config.AppConfig
	modeResolver  ModeResolver
}

// NewVideoFlow creates a new VideoFlow.
func NewVideoFlow(
	tokenSvc TokenServicer,
	clientFactory func(token string) VideoClient,
	cfg *VideoFlowConfig,
) *VideoFlow {
	if cfg == nil {
		cfg = &VideoFlowConfig{
			TimeoutSeconds:      300,
			PollIntervalSeconds: 5,
		}
	}
	return &VideoFlow{
		tokenSvc:      tokenSvc,
		clientFactory: clientFactory,
		cfg:           cfg,
	}
}

// SetUsageRecorder sets the usage recorder for logging API usage.
func (f *VideoFlow) SetUsageRecorder(ur UsageRecorder) {
	f.usageLog = ur
}

// SetCacheService sets the cache service for video download proxy.
func (f *VideoFlow) SetCacheService(svc *cache.Service) {
	f.cacheSvc = svc
}

// SetAppConfig sets app-level defaults for app-chat based video generation.
func (f *VideoFlow) SetAppConfig(cfg *config.AppConfig) {
	f.appConfigFn = func() *config.AppConfig { return cfg }
}

// SetAppConfigProvider sets a dynamic app config provider.
func (f *VideoFlow) SetAppConfigProvider(fn func() *config.AppConfig) {
	f.appConfigFn = fn
}

// SetModeResolver sets the mode resolver for quota tracking.
func (f *VideoFlow) SetModeResolver(resolver ModeResolver) {
	f.modeResolver = resolver
}

// resolveMode returns the quota mode for a model, or empty string if unknown.
func (f *VideoFlow) resolveMode(model string) string {
	if f.modeResolver == nil {
		return ""
	}
	mode, _ := f.modeResolver.ResolveMode(model)
	return mode
}

func (f *VideoFlow) appConfig() *config.AppConfig {
	if f.appConfigFn == nil {
		return nil
	}
	return f.appConfigFn()
}

// GenerateSync runs video generation synchronously and returns the final URL.
func (f *VideoFlow) GenerateSync(ctx context.Context, req *VideoRequest) (string, error) {
	if err := validateMediaUpstream(req.UpstreamModel, req.UpstreamMode); err != nil {
		return "", err
	}

	// Resolve mode from request or model registry
	mode := req.Mode
	if mode == "" {
		mode = f.resolveMode(req.Model)
	}

	apiKeyID := FlowAPIKeyIDFromContext(ctx)
	tok, err := f.pickTokenForModel(req.Model, mode)
	if err != nil {
		return "", err
	}

	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(f.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	videoURL, err := f.generateVideoViaChat(timeoutCtx, tok, req, mode)
	if err != nil {
		if errors.Is(err, errVideoClientNil) {
			f.tokenSvc.ReleaseToken(tok.ID)
			f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
			return "", err
		}
		if errors.Is(err, ErrVideoCache) || isNonTokenVideoPostProcessError(err) {
			f.tokenSvc.ReportSuccess(tok.ID)
			f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
			return "", err
		}
		f.reportTokenError(tok.ID, mode, err)
		f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
		return "", err
	}

	f.tokenSvc.ReportSuccess(tok.ID)
	f.recordUsage(apiKeyID, tok.ID, req.Model, 200, time.Since(start))
	return videoURL, nil
}

func isNonTokenVideoPostProcessError(err error) bool {
	if !errors.Is(err, ErrVideoPostProcess) {
		return false
	}
	return !errors.Is(err, xai.ErrInvalidToken) &&
		!errors.Is(err, xai.ErrForbidden) &&
		!errors.Is(err, xai.ErrCFChallenge) &&
		!errors.Is(err, xai.ErrRateLimited)
}

// reportTokenError reports the appropriate token error based on error type.
func (f *VideoFlow) reportTokenError(tokenID uint, mode string, err error) {
	reportTrackedTokenError(f.tokenSvc, tokenID, mode, err)
}

// recordUsage records a video API usage log entry via the buffer (non-blocking).
func (f *VideoFlow) recordUsage(apiKeyID, tokenID uint, model string, status int, latency time.Duration) {
	if f.usageLog == nil {
		return
	}
	_ = f.usageLog.Record(context.Background(), &store.UsageLog{
		APIKeyID:    apiKeyID,
		TokenID:     tokenID,
		Model:       model,
		Endpoint:    "video",
		Status:      status,
		DurationMs:  latency.Milliseconds(),
		TTFTMs:      0,
		CacheTokens: 0,
		CreatedAt:   time.Now(),
	})
}

func (f *VideoFlow) pickTokenForModel(model, mode string) (*store.Token, error) {
	cfg := f.cfg
	if cfg == nil || cfg.ModelResolver == nil {
		return nil, tkn.ErrModelNotFound
	}
	pools, ok := tkn.GetPoolForModel(model, cfg.ModelResolver)
	if !ok {
		return nil, tkn.ErrModelNotFound
	}
	var lastErr error
	for _, pool := range pools {
		tok, err := f.tokenSvc.Pick(pool, mode)
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

func (f *VideoFlow) cacheVideo(ctx context.Context, client VideoClient, videoURL string) (string, error) {
	if f.cacheSvc == nil {
		return "", fmt.Errorf("%w: cache service is not configured", ErrVideoCache)
	}
	reader, writer := io.Pipe()
	doneCh := make(chan error, 1)
	SafeGo("video_cache_download", func() {
		var dlErr error
		defer func() {
			if recovered := recover(); recovered != nil {
				dlErr = fmt.Errorf("download video panic: %v", recovered)
				slog.Error("video cache download panic", "panic", recovered)
			}
			_ = writer.CloseWithError(dlErr)
			doneCh <- dlErr
		}()
		dlErr = client.DownloadTo(ctx, videoURL, writer)
	})
	filename, saveErr := f.cacheSvc.SaveStream("video", reader, ".mp4")
	if saveErr != nil {
		_ = reader.CloseWithError(saveErr)
	}
	if dlErr := <-doneCh; dlErr != nil {
		if saveErr != nil && errors.Is(dlErr, io.ErrClosedPipe) {
			return "", fmt.Errorf("%w: save video: %w", ErrVideoCache, saveErr)
		}
		return "", fmt.Errorf("%w: download video: %w", ErrVideoCache, dlErr)
	}
	if saveErr != nil {
		return "", fmt.Errorf("%w: save video: %w", ErrVideoCache, saveErr)
	}
	return "/api/files/video/" + filename, nil
}

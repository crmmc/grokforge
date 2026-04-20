// Package main is the entry point for GrokForge.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"path/filepath"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/cfrefresh"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/httpapi/openai"
	"github.com/crmmc/grokforge/internal/logging"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

const serverWriteTimeout = 330 * time.Second
const tokenFlushInterval = 30 * time.Second

func main() {
	// Parse flags
	configPath := flag.String("config", "config.toml", "path to config file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("grokforge %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	logging.Setup(cfg.App.LogLevel, cfg.App.LogJSON, &logging.FileConfig{
		Path:       cfg.App.LogFilePath,
		MaxSizeMB:  cfg.App.LogMaxSizeMB,
		MaxBackups: cfg.App.LogMaxBackups,
	})
	logging.Info("starting grokforge", "version", version, "config", *configPath)
	if cfg.App.AppKey == "grokforge" {
		logging.Warn("default admin app key is in use; change app.app_key before exposing the service")
	}

	// Open database
	db, err := store.Open(cfg)
	if err != nil {
		logging.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer store.Close(db)

	// Run migrations
	if err := store.AutoMigrate(db); err != nil {
		logging.Error("failed to migrate database", "error", err)
		os.Exit(1)
	}
	logging.Info("database ready", "driver", cfg.App.DBDriver)

	// Load static model catalog
	configDir := filepath.Dir(*configPath)
	modelsFile := ""
	if cfg.App.ModelsFile != "" {
		modelsFile = filepath.Join(configDir, cfg.App.ModelsFile)
	}
	modelSpecs, err := modelconfig.Load(modelconfig.EmbeddedFS, modelsFile)
	if err != nil {
		logging.Error("failed to load model catalog", "error", err)
		os.Exit(1)
	}
	catalogSource := "embedded"
	if modelsFile != "" {
		catalogSource = modelsFile
	}
	reg := registry.NewModelRegistry(modelSpecs)
	logging.Info("model catalog loaded", "source", catalogSource, "models", reg.Count())

	// Load DB config overrides (DB > config file > defaults)
	configStore := store.NewConfigStore(db)
	dbOverrides, err := configStore.GetAll()
	if err != nil {
		logging.Error("failed to load config overrides from database", "error", err)
	} else if len(dbOverrides) > 0 {
		if err := cfg.ApplyDBOverrides(dbOverrides); err != nil {
			logging.Error("failed to apply config overrides from database", "error", err)
			os.Exit(1)
		}
		logging.Info("applied database config overrides", "count", len(dbOverrides))
	}
	runtimeCfg := config.NewRuntime(cfg)
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Start CF refresh scheduler (FlareSolverr auto-refresh)
	cfScheduler := cfrefresh.NewScheduler(runtimeCfg, configStore)
	cfScheduler.Start()
	logging.Info("cf_refresh scheduler started")

	// Create token service
	tokenStore := store.NewTokenStore(db)
	tokenSvc := token.NewTokenService(&cfg.Token, tokenStore, "https://grok.com")
	if err := tokenSvc.LoadTokens(rootCtx); err != nil {
		logging.Error("failed to load tokens", "error", err)
		os.Exit(1)
	}
	tokenSvc.StartTicker(rootCtx)
	logging.Info("token service ready", "stats", tokenSvc.Stats())

	// Start quota recovery scheduler (auto-replenish or upstream sync)
	scheduler := token.NewScheduler(tokenSvc.Manager(), &cfg.Token, "https://grok.com")
	scheduler.SetConfigProvider(func() *config.TokenConfig {
		return &runtimeCfg.Get().Token
	})
	scheduler.Start(rootCtx)
	logging.Info("token quota recovery scheduler started")

	// Start token state persistence loop
	tokenPersister := token.NewPersister(tokenSvc.Manager(), db)
	tokenPersister.Start(rootCtx, tokenFlushInterval)
	logging.Info("token persistence loop started")

	// Create video flow
	videoFlow := flow.NewVideoFlow(
		tokenSvc,
		func(tok string) flow.VideoClient {
			client, err := newXAIClient(runtimeCfg, tok, false)
			if err != nil {
				logging.Error("failed to create xai client", "error", err)
				return nil
			}
			return client
		},
		&flow.VideoFlowConfig{
			TimeoutSeconds:      300,
			PollIntervalSeconds: 5,
			ModelResolver:       reg,
		},
	)
	videoFlow.SetAppConfigProvider(func() *config.AppConfig {
		return &runtimeCfg.Get().App
	})
	logging.Info("video flow ready")

	// Create ChatFlow
	chatFlow := flow.NewChatFlow(
		tokenSvc,
		func(tok string) xai.Client {
			client, err := newXAIClient(runtimeCfg, tok, true)
			if err != nil {
				logging.Error("failed to create xai client", "error", err)
				return nil
			}
			return client
		},
		&flow.ChatFlowConfig{
			RetryConfig: flow.DefaultRetryConfig(),
			RetryConfigProvider: func() *flow.RetryConfig {
				current := runtimeCfg.Get()
				retry := current.Retry
				return &flow.RetryConfig{
					MaxTokens:               retry.MaxTokens,
					PerTokenRetries:         retry.PerTokenRetries,
					BaseDelay:               time.Duration(retry.RetryBackoffBase * float64(time.Second)),
					MaxDelay:                time.Duration(retry.RetryBackoffMax * float64(time.Second)),
					JitterFactor:            0.25,
					BackoffFactor:           retry.RetryBackoffFactor,
					ResetSessionStatusCodes: append([]int(nil), retry.ResetSessionStatusCodes...),
					CoolingStatusCodes:      append([]int(nil), retry.CoolingStatusCodes...),
					RetryBudget:             time.Duration(retry.RetryBudget * float64(time.Second)),
				}
			},
			ModelResolver: reg,
			ResolveUpstream: func(name string) (string, string, bool) {
				rm, ok := reg.Resolve(name)
				if !ok {
					return "", "", false
				}
				return rm.UpstreamModel, rm.UpstreamMode, true
			},
			AppConfigProvider: func() *config.AppConfig {
				return &runtimeCfg.Get().App
			},
			FilterTagsProvider: func() []string {
				current := runtimeCfg.Get()
				return append([]string(nil), current.App.FilterTags...)
			},
		},
	)
	logging.Info("chat flow ready")

	// Wire CF refresh trigger into chat flow (403 → immediate refresh)
	chatFlow.SetCFRefreshTrigger(cfScheduler.TriggerRefresh)

	// Create usage log store and buffer
	usageLogStore := store.NewUsageLogStore(db)
	flushInterval := time.Duration(cfg.Token.UsageFlushIntervalSec) * time.Second
	usageBuffer := flow.NewUsageBuffer(usageLogStore, flushInterval)
	usageBuffer.Start()
	chatFlow.SetUsageRecorder(usageBuffer)
	videoFlow.SetUsageRecorder(usageBuffer)
	logging.Info("usage buffer ready", "flush_interval", flushInterval)

	// Create API key store
	apiKeyStore := store.NewAPIKeyStore(db)

	// Wire API key usage increment into chat flow (only on success)
	chatFlow.SetAPIKeyUsageInc(func(ctx context.Context, apiKeyID uint) {
		_ = apiKeyStore.IncrementUsage(ctx, apiKeyID)
	})

	// Create ImageFlow with per-request token selection
	imageFlow := flow.NewImageFlow(tokenSvc, func(token string) flow.ImagineGenerator {
		return newImagineClient(runtimeCfg, token)
	})
	imageFlow.SetEditClientFactory(func(token string) flow.ImageEditClient {
		client, err := newXAIClient(runtimeCfg, token, true)
		if err != nil {
			logging.Error("failed to create image edit client", "error", err)
			return nil
		}
		return client
	})
	imageFlow.SetAppConfigProvider(func() *config.AppConfig {
		return &runtimeCfg.Get().App
	})
	imageFlow.SetImageConfigProvider(func() *config.ImageConfig {
		return &runtimeCfg.Get().Image
	})
	imageFlow.SetModelResolver(reg)
	imageFlow.SetEnableProResolver(func(model string) bool {
		rm, ok := reg.Resolve(model)
		if !ok {
			return false
		}
		return rm.EnablePro
	})
	imageFlow.SetUsageRecorder(usageBuffer)
	logging.Info("image flow ready")

	// Create cache service
	cacheSvc := cache.NewService("data")
	logging.Info("cache service ready", "data_dir", "data")

	// Wire cache service to video flow for download proxy
	videoFlow.SetCacheService(cacheSvc)

	// Create OpenAI provider
	openaiHandler := &openai.Handler{
		ChatFlow:      chatFlow,
		VideoFlow:     videoFlow,
		ImageFlow:     imageFlow,
		Cfg:           runtimeCfg.Get(),
		Runtime:       runtimeCfg,
		ModelRegistry: reg,
	}

	// Create HTTP server
	srv := httpapi.NewServer(&httpapi.ServerConfig{
		AppKey:          runtimeCfg.Get().App.AppKey,
		Version:         version,
		Config:          runtimeCfg.Get(),
		Runtime:         runtimeCfg,
		ChatProvider:    openaiHandler,
		TokenStore:      tokenStore,
		TokenRefresher:  tokenSvc,
		TokenPoolSyncer: tokenSvc,
		UsageLogStore:   usageLogStore,
		APIKeyStore:     apiKeyStore,
		CacheService:    cacheSvc,
		ConfigStore:     configStore,
		ModelRegistry:   reg,
	})
	addr := fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: time.Duration(cfg.App.ReadHeaderTimeout) * time.Second,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    cfg.App.MaxHeaderBytes,
	}

	// Start server in goroutine
	flow.SafeGo("http_server_listen", func() {
		logging.Info("server listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error("server error", "error", err)
			os.Exit(1)
		}
	})

	// Start API Key daily usage reset ticker
	flow.SafeGo("apikey_daily_reset", func() {
		for {
			now := time.Now().UTC()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			timer := time.NewTimer(nextMidnight.Sub(now))
			select {
			case <-rootCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if err := apiKeyStore.ResetDailyUsage(context.Background()); err != nil {
				logging.Error("failed to reset API key daily usage", "error", err)
			} else {
				logging.Info("API key daily usage reset complete")
			}
			// Quotas are now managed by the recovery scheduler (auto/upstream mode),
			// no midnight reset needed.
		}
	})

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logging.Info("shutting down server...")

	// Graceful shutdown: HTTP server first (stop accepting new requests)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logging.Error("server shutdown error", "error", err)
	}
	rootCancel()

	// Stop CF refresh scheduler
	cfScheduler.Stop()
	scheduler.Stop()

	// Then flush remaining usage records
	usageBuffer.Stop()

	// Then flush dirty token state and stop persistence loop
	tokenPersister.Stop()
	if err := tokenSvc.FlushDirty(context.Background()); err != nil {
		logging.Error("failed to flush dirty tokens on shutdown", "error", err)
	}

	logging.Info("server stopped")
}

func newXAIClient(runtime *config.Runtime, token string, noRetry bool) (xai.Client, error) {
	cfg := runtime.Get()
	opts := []xai.ClientOption{
		xai.WithDynamicStatsig(cfg.App.DynamicStatsig),
	}
	if noRetry {
		opts = append(opts, xai.WithMaxRetry(0))
	}
	if cfg.Proxy.Timeout > 0 {
		opts = append(opts, xai.WithTimeout(time.Duration(cfg.Proxy.Timeout)*time.Second))
	}
	if cfg.Proxy.BaseProxyURL != "" {
		opts = append(opts, xai.WithProxy(cfg.Proxy.BaseProxyURL))
	}
	if cfg.Proxy.AssetProxyURL != "" {
		opts = append(opts, xai.WithAssetProxy(cfg.Proxy.AssetProxyURL))
	}
	if cfg.Proxy.SkipProxySSLVerify {
		opts = append(opts, xai.WithSkipProxySSLVerify(true))
	}
	if cfg.Proxy.Browser != "" {
		opts = append(opts, xai.WithBrowser(cfg.Proxy.Browser))
	}
	if cfg.Proxy.UserAgent != "" {
		opts = append(opts, xai.WithUserAgent(cfg.Proxy.UserAgent))
	}
	if cfg.Proxy.CFClearance != "" {
		opts = append(opts, xai.WithCFClearance(cfg.Proxy.CFClearance))
	}
	if cfg.Proxy.CFCookies != "" {
		opts = append(opts, xai.WithCFCookies(cfg.Proxy.CFCookies))
	}
	return xai.NewClient(token, opts...)
}

func newImagineClient(runtime *config.Runtime, token string) flow.ImagineGenerator {
	cfg := runtime.Get()
	opts := []xai.ImagineClientOption{}
	if cfg.Proxy.BaseProxyURL != "" {
		opts = append(opts, xai.WithImagineProxy(cfg.Proxy.BaseProxyURL))
	}
	if cfg.Proxy.SkipProxySSLVerify {
		opts = append(opts, xai.WithImagineSkipProxySSLVerify(true))
	}
	if cfg.Proxy.UserAgent != "" {
		opts = append(opts, xai.WithImagineUserAgent(cfg.Proxy.UserAgent))
	}
	if cfg.Proxy.CFClearance != "" {
		opts = append(opts, xai.WithImagineCFClearance(cfg.Proxy.CFClearance))
	}
	if cfg.Proxy.CFCookies != "" {
		opts = append(opts, xai.WithImagineCFCookies(cfg.Proxy.CFCookies))
	}
	return xai.NewImagineClient(token, opts...)
}

// Package main is the entry point for GrokForge.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/upstream/console"
	"github.com/crmmc/grokforge/internal/upstream/grok"
	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/crmmc/grokforge/internal/xai"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

// signalNotify is indirected so tests can trigger the graceful shutdown path
// by sending on the injected channel instead of delivering real OS signals.
var signalNotify = signal.Notify

// osExit is indirected so the http-server failure branch can be exercised in
// tests without terminating the test process.
var osExit = os.Exit

// adminKeyEntropy is the randomness source for the bootstrap admin app key.
// nil means crypto/rand, which is what production uses.
var adminKeyEntropy io.Reader

// xaiClientConstructor is indirected so the client-factory failure branches
// can be exercised in tests; production always uses xai.NewClient.
var xaiClientConstructor = xai.NewClient

const serverWriteTimeout = 330 * time.Second
const tokenFlushInterval = 30 * time.Second
const upstreamStaticStatsigID = "ZTpUeXBlRXJyb3I6IENhbm5vdCByZWFkIHByb3BlcnRpZXMgb2YgdW5kZWZpbmVkIChyZWFkaW5nICdjaGlsZE5vZGVzJyk="

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}

// run boots GrokForge end to end: flag parsing, configuration, storage,
// background schedulers, flows and the HTTP server, then blocks until an
// interrupt signal arrives and performs the graceful shutdown sequence.
// All user-facing output (logging, stderr diagnostics, version banner) is
// produced inside; the returned error is only an exit-code signal for main.
func run(argv []string, stdout, stderr io.Writer) error {
	// Parse flags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath := fs.String("config", "config.toml", "path to config file")
	showVersion := fs.Bool("version", false, "show version and exit")
	// ExitOnError terminates the process on parse errors (including -h),
	// matching the previous flag.Parse() behavior.
	_ = fs.Parse(argv)

	if *showVersion {
		fmt.Fprintf(stdout, "grokforge %s (built %s)\n", version, buildTime)
		return nil
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load config: %v\n", err)
		return err
	}

	// Setup logging
	logging.Setup(cfg.App.LogLevel, cfg.App.LogJSON, &logging.FileConfig{
		Path:       cfg.App.LogFilePath,
		MaxSizeMB:  cfg.App.LogMaxSizeMB,
		MaxBackups: cfg.App.LogMaxBackups,
	})
	logging.Info("starting grokforge", "version", version, "config", *configPath)

	// Open database
	db, err := store.Open(cfg)
	if err != nil {
		return startupError("failed to open database", err)
	}
	defer store.Close(db)

	// Run migrations
	if err := store.AutoMigrate(db); err != nil {
		return startupError("failed to migrate database", err)
	}
	logging.Info("database ready", "driver", cfg.App.DBDriver)

	// Load static model catalog
	configDir := filepath.Dir(*configPath)
	modelsFile := ""
	if cfg.App.ModelsFile != "" {
		modelsFile = filepath.Join(configDir, cfg.App.ModelsFile)
	}
	modelSpecs, modeSpecs, err := modelconfig.Load(modelconfig.EmbeddedFS, modelsFile)
	if err != nil {
		return startupError("failed to load model catalog", err)
	}
	catalogSource := "embedded"
	if modelsFile != "" {
		catalogSource = modelsFile
	}
	reg := registry.NewModelRegistry(modelSpecs, modeSpecs)
	logging.Info("model catalog loaded", "source", catalogSource, "models", reg.Count())

	// Load DB config overrides (DB > config file > defaults)
	configStore := store.NewConfigStore(db)
	dbOverrides, err := configStore.GetAll()
	if err != nil {
		logging.Error("failed to load config overrides from database", "error", err)
	} else if len(dbOverrides) > 0 {
		if err := cfg.ApplyDBOverrides(dbOverrides); err != nil {
			return startupError("failed to apply config overrides from database", err)
		}
		logging.Info("applied database config overrides", "count", len(dbOverrides))
	}
	ensureProxyBrowserUserAgent(cfg)
	bootstrapAppKey, bootstrapGenerated, err := config.EnsureAdminAppKey(cfg, adminKeyEntropy)
	if err != nil {
		return startupError("failed to prepare admin app key", err)
	}
	if bootstrapGenerated {
		logTemporaryBootstrapAdminPassword(bootstrapAppKey)
	} else if cfg.App.AppKey == "grokforge" {
		logging.Warn("default admin app key is in use; change app.app_key before exposing the service")
	}
	runtimeCfg := config.NewRuntime(cfg)
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	wiring := &runtimeWiring{runtime: runtimeCfg, reg: reg}

	// Start CF refresh scheduler (FlareSolverr auto-refresh)
	cfScheduler := cfrefresh.NewScheduler(runtimeCfg, configStore)
	cfScheduler.Start()
	logging.Info("cf_refresh scheduler started")

	// Create token service
	tokenStore := store.NewTokenStore(db)
	tokenSvc := token.NewTokenService(&cfg.Token, tokenStore, modeSpecs, "https://grok.com")
	scheduler := token.NewScheduler(tokenSvc.Manager(), modeSpecs, "https://grok.com")
	tokenSvc.SetRefreshRequester(scheduler)
	if err := tokenSvc.LoadTokens(rootCtx); err != nil {
		return startupError("failed to load tokens", err)
	}
	if err := tokenSvc.FlushDirty(rootCtx); err != nil {
		return startupError("failed to persist normalized tokens", err)
	}
	logging.Info("token service ready", "stats", tokenSvc.Stats())

	// Start quota refresh scheduler.
	scheduler.Start(rootCtx)
	logging.Info("token quota refresh scheduler started")

	// Start token state persistence loop
	tokenPersister := token.NewPersister(tokenSvc.Manager(), db)
	tokenPersister.Start(rootCtx, tokenFlushInterval)
	logging.Info("token persistence loop started")

	// Create video flow
	videoFlow := flow.NewVideoFlow(
		tokenSvc,
		wiring.newVideoClient,
		&flow.VideoFlowConfig{
			TimeoutSeconds:      300,
			PollIntervalSeconds: 5,
			ModelResolver:       reg,
		},
	)
	videoFlow.SetAppConfigProvider(wiring.appConfig)
	videoFlow.SetModeResolver(reg)
	logging.Info("video flow ready")

	// Create ChatFlow
	chatDoer := transport.NewDynamicStatelessDoer(wiring.transportOptions)
	grokOpts := grok.Options{
		BuildCookieString: wiring.grokCookie,
		BrowserProfile:    wiring.browserProfile,
		UserAgent:         wiring.userAgent,
		StatsigID:         wiring.statsigID,
	}
	consoleOpts := console.Options{
		BuildCookieString: wiring.consoleCookie,
		BrowserProfile:    wiring.browserProfile,
		UserAgent:         wiring.userAgent,
		StatsigID:         wiring.statsigID,
	}
	upstreams := map[string]upstream.Upstream{
		"grok":    grok.New(grok.DefaultURL, chatDoer, grokOpts),
		"console": console.New(console.DefaultURL, chatDoer, consoleOpts),
	}
	chatFlow := flow.NewChatFlow(
		tokenSvc,
		upstreams,
		&flow.ChatFlowConfig{
			RetryConfig:         flow.DefaultRetryConfig(),
			RetryConfigProvider: wiring.retryConfig,
			ModelResolver:       reg,
			AppConfigProvider:   wiring.appConfig,
			FilterTagsProvider:  wiring.filterTags,
			ResolveUpstream:     wiring.resolveUpstream,
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
	wiring.apiKeys = apiKeyStore

	// Wire API key usage increment into chat flow (only on success)
	chatFlow.SetAPIKeyUsageInc(wiring.incAPIKeyUsage)

	// Create ImageFlow with per-request token selection
	imageFlow := flow.NewImageFlow(tokenSvc, wiring.newImagineGenerator)
	imageFlow.SetEditClientFactory(wiring.newImageEditClient)
	imageFlow.SetAppConfigProvider(wiring.appConfig)
	imageFlow.SetImageConfigProvider(wiring.imageConfig)
	imageFlow.SetModelResolver(reg)
	imageFlow.SetModeResolver(reg)
	imageFlow.SetEnableProResolver(wiring.enablePro)
	imageFlow.SetUsageRecorder(usageBuffer)
	logging.Info("image flow ready")

	// Create cache service
	cacheSvc := cache.NewService("data", runtimeCfg)
	logging.Info("cache service ready", "data_dir", "data")

	// Wire cache service to video flow for download proxy
	imageFlow.SetCacheService(cacheSvc)
	videoFlow.SetCacheService(cacheSvc)

	// Create OpenAI provider
	openaiHandler := &openai.Handler{
		ChatFlow:      chatFlow,
		VideoFlow:     videoFlow,
		ImageFlow:     imageFlow,
		CacheService:  cacheSvc,
		Cfg:           runtimeCfg.Get(),
		Runtime:       runtimeCfg,
		ModelRegistry: reg,
	}

	// Create HTTP server
	nsfwEnabler := &nsfwEnablerImpl{runtime: runtimeCfg}

	srv := httpapi.NewServer(&httpapi.ServerConfig{
		AppKey:          runtimeCfg.Get().App.AppKey,
		Version:         version,
		Config:          runtimeCfg.Get(),
		Runtime:         runtimeCfg,
		ChatProvider:    openaiHandler,
		TokenStore:      tokenStore,
		TokenRefresher:  tokenSvc,
		TokenPoolSyncer: tokenSvc,
		TokenInflight:   tokenSvc.Manager(),
		TokenCfgSync:    tokenSvc.Manager().UpdateConfig,
		NsfwEnabler:     nsfwEnabler,
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
		serveHTTP(httpServer)
	})

	// Start API Key daily usage reset ticker
	flow.SafeGo("apikey_daily_reset", func() {
		apiKeyDailyResetLoop(rootCtx, apiKeyStore, time.Now)
	})

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signalNotify(quit, syscall.SIGINT, syscall.SIGTERM)
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
	return nil
}

// startupError logs a fatal startup failure and returns it so that run exits
// with status 1 after its deferred cleanup.
func startupError(msg string, err error) error {
	logging.Error(msg, "error", err)
	return err
}

// serveHTTP runs the HTTP server until it fails or is shut down. A listen
// error other than a deliberate shutdown is fatal: it logs and exits the
// process, matching the previous inline goroutine behavior.
func serveHTTP(srv *http.Server) {
	logging.Info("server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logging.Error("server error", "error", err)
		osExit(1)
	}
}

// apiKeyDailyResetLoop resets API key daily usage at each UTC midnight until
// ctx is canceled. now is injected for deterministic tests.
func apiKeyDailyResetLoop(ctx context.Context, apiKeyStore *store.APIKeyStore, now func() time.Time) {
	for {
		current := now().UTC()
		nextMidnight := time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, time.UTC)
		timer := time.NewTimer(nextMidnight.Sub(current))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		resetAPIKeyDailyUsage(apiKeyStore)
	}
}

func resetAPIKeyDailyUsage(apiKeyStore *store.APIKeyStore) {
	if err := apiKeyStore.ResetDailyUsage(context.Background()); err != nil {
		logging.Error("failed to reset API key daily usage", "error", err)
	} else {
		logging.Info("API key daily usage reset complete")
	}
}

// runtimeWiring groups the provider callbacks that translate the live runtime
// configuration into flow and upstream options. The methods are used as
// function values so that each one can be unit-tested in isolation.
type runtimeWiring struct {
	runtime *config.Runtime
	reg     *registry.ModelRegistry
	apiKeys *store.APIKeyStore
}

func (w *runtimeWiring) appConfig() *config.AppConfig {
	return &w.runtime.Get().App
}

func (w *runtimeWiring) imageConfig() *config.ImageConfig {
	return &w.runtime.Get().Image
}

func (w *runtimeWiring) transportOptions() transport.Options {
	current := w.runtime.Get()
	return transport.Options{
		RequestTimeout:     time.Duration(current.Proxy.Timeout) * time.Second,
		Browser:            current.Proxy.Browser,
		ProxyURL:           current.Proxy.BaseProxyURL,
		SkipProxySSLVerify: current.Proxy.SkipProxySSLVerify,
	}
}

func (w *runtimeWiring) browserProfile() string {
	return w.runtime.Get().Proxy.Browser
}

func (w *runtimeWiring) userAgent() string {
	return w.runtime.Get().Proxy.UserAgent
}

func (w *runtimeWiring) statsigID() string {
	if w.runtime.Get().App.DynamicStatsig {
		return ""
	}
	return upstreamStaticStatsigID
}

func (w *runtimeWiring) grokCookie(tok string) string {
	current := w.runtime.Get()
	return grok.BuildCookie(tok, current.Proxy.CFCookies, current.Proxy.CFClearance)
}

func (w *runtimeWiring) consoleCookie(tok string) string {
	current := w.runtime.Get()
	return console.BuildCookie(tok, current.Proxy.CFCookies, current.Proxy.CFClearance)
}

func (w *runtimeWiring) retryConfig() *flow.RetryConfig {
	current := w.runtime.Get()
	retry := current.Retry
	return &flow.RetryConfig{
		MaxTokens:       retry.MaxTokens,
		PerTokenRetries: retry.PerTokenRetries,
		BaseDelay:       time.Duration(retry.RetryBackoffBase * float64(time.Second)),
		MaxDelay:        time.Duration(retry.RetryBackoffMax * float64(time.Second)),
		JitterFactor:    0.25,
		BackoffFactor:   retry.RetryBackoffFactor,
		RetryBudget:     time.Duration(retry.RetryBudget * float64(time.Second)),
	}
}

func (w *runtimeWiring) filterTags() []string {
	current := w.runtime.Get()
	return append([]string(nil), current.App.FilterTags...)
}

func (w *runtimeWiring) resolveUpstream(name string) (string, string, bool) {
	rm, ok := w.reg.Resolve(name)
	if !ok {
		return "", "", false
	}
	return rm.UpstreamModel, rm.UpstreamMode, true
}

func (w *runtimeWiring) enablePro(model string) bool {
	rm, ok := w.reg.Resolve(model)
	if !ok {
		return false
	}
	return rm.EnablePro
}

func (w *runtimeWiring) newVideoClient(tok string) flow.VideoClient {
	client, err := newXAIClient(w.runtime, tok, false)
	if err != nil {
		logging.Error("failed to create xai client", "error", err)
		return nil
	}
	return client
}

func (w *runtimeWiring) newImagineGenerator(tok string) flow.ImagineGenerator {
	return newImagineClient(w.runtime, tok)
}

func (w *runtimeWiring) newImageEditClient(tok string) flow.ImageEditClient {
	client, err := newXAIClient(w.runtime, tok, true)
	if err != nil {
		logging.Error("failed to create image edit client", "error", err)
		return nil
	}
	return client
}

func (w *runtimeWiring) incAPIKeyUsage(ctx context.Context, apiKeyID uint) {
	_ = w.apiKeys.IncrementUsage(ctx, apiKeyID)
}

func buildXAIOptions(cfg *config.Config) []xai.ClientOption {
	opts := []xai.ClientOption{
		xai.WithDynamicStatsig(cfg.App.DynamicStatsig),
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
	return opts
}

func ensureProxyBrowserUserAgent(cfg *config.Config) {
	if cfg == nil {
		return
	}
	originalBrowser := cfg.Proxy.Browser
	originalUA := cfg.Proxy.UserAgent

	profile := transport.EffectiveProfile(cfg.Proxy.Browser, cfg.Proxy.UserAgent)
	cfg.Proxy.Browser = profile

	if strings.TrimSpace(cfg.Proxy.UserAgent) == "" {
		if ua, ok := transport.UserAgentForProfile(profile); ok {
			cfg.Proxy.UserAgent = ua
		} else {
			cfg.Proxy.UserAgent = transport.DefaultUserAgent()
		}
	}

	if originalBrowser != cfg.Proxy.Browser || originalUA != cfg.Proxy.UserAgent {
		logging.Info("proxy browser profile resolved",
			"configured_browser", originalBrowser,
			"effective_browser", cfg.Proxy.Browser)
	}
}

func newXAIClient(runtime *config.Runtime, token string, noRetry bool) (xai.Client, error) {
	opts := buildXAIOptions(runtime.Get())
	if noRetry {
		opts = append(opts, xai.WithMaxRetry(0))
	}
	return xaiClientConstructor(token, opts...)
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

// nsfwEnablerImpl implements httpapi.NsfwEnabler using xai.NsfwClient.
type nsfwEnablerImpl struct {
	runtime *config.Runtime
}

func (s *nsfwEnablerImpl) EnableNsfwUpstream(ctx context.Context, tokenStr string) error {
	cfg := s.runtime.Get()
	opts := buildXAIOptions(cfg)
	client, err := xai.NewNsfwClient(tokenStr, opts...)
	if err != nil {
		return err
	}
	defer client.Close()

	timeout := time.Duration(cfg.Proxy.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return xai.EnableNSFW(tctx, client)
}

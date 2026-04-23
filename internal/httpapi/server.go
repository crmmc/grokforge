// Package httpapi provides HTTP routing and handlers.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
)

// ChatProvider abstracts downstream protocol handling for API routes.
type ChatProvider interface {
	SetupRoutes(r chi.Router)
}

type noopChatProvider struct{}

func (noopChatProvider) SetupRoutes(r chi.Router) {
	r.Get("/models", notImplementedHandler)
	r.Post("/chat/completions", notImplementedHandler)
}

func notImplementedHandler(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented",
		"Chat provider not configured")
}

// Server holds HTTP server dependencies.
type Server struct {
	router          chi.Router
	startTime       time.Time
	chatProvider    ChatProvider
	appKey          string
	version         string
	cfg             *config.Config
	runtime         *config.Runtime
	tokenStore      TokenStoreInterface
	tokenRefresher  TokenRefresher
	tokenPoolSyncer TokenPoolSyncer
	usageLogStore   UsageLogStoreInterface
	apiKeyStore     APIKeyStoreInterface
	cacheService    *cache.Service
	configStore     *store.ConfigStore
	modelRegistry   *registry.ModelRegistry
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	AppKey          string
	Version         string
	Config          *config.Config
	Runtime         *config.Runtime
	ChatProvider    ChatProvider
	TokenStore      TokenStoreInterface
	TokenRefresher  TokenRefresher
	TokenPoolSyncer TokenPoolSyncer
	UsageLogStore   UsageLogStoreInterface
	APIKeyStore     APIKeyStoreInterface
	CacheService    *cache.Service
	ConfigStore     *store.ConfigStore
	ModelRegistry   *registry.ModelRegistry
}

// NewServer creates a new HTTP server with configured routes.
func NewServer(cfg *ServerConfig) *Server {
	if cfg == nil {
		cfg = &ServerConfig{}
	}
	chatProvider := cfg.ChatProvider
	if chatProvider == nil {
		chatProvider = noopChatProvider{}
	}
	s := &Server{
		router:          chi.NewRouter(),
		startTime:       time.Now(),
		chatProvider:    chatProvider,
		appKey:          cfg.AppKey,
		version:         cfg.Version,
		cfg:             cfg.Config,
		runtime:         cfg.Runtime,
		tokenStore:      cfg.TokenStore,
		tokenRefresher:  cfg.TokenRefresher,
		tokenPoolSyncer: cfg.TokenPoolSyncer,
		usageLogStore:   cfg.UsageLogStore,
		apiKeyStore:     cfg.APIKeyStore,
		cacheService:    cfg.CacheService,
		configStore:     cfg.ConfigStore,
		modelRegistry:   cfg.ModelRegistry,
	}
	s.setupMiddleware()
	s.setupRoutes()
	return s
}

// setupMiddleware configures global middleware.
func (s *Server) setupMiddleware() {
	s.router.Use(middleware.CleanPath)
	s.router.Use(securityHeaders)
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Recoverer)
	s.router.Use(debugRequestLogger)
	if s.runtime != nil {
		s.router.Use(requestTimeoutRuntimeMiddleware(s.runtime))
		s.router.Use(bodySizeLimitRuntimeMiddleware(s.runtime))
		return
	}
	s.router.Use(requestTimeoutMiddleware(s.cfg))
	s.router.Use(bodySizeLimitMiddleware(s.cfg))
}

// debugRequestLogger logs every incoming request at DEBUG level.
func debugRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		slog.Debug("http: incoming request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"content_length", r.ContentLength,
			"request_id", middleware.GetReqID(r.Context()))

		next.ServeHTTP(ww, r)

		slog.Debug("http: response sent",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"elapsed_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()))
	})
}

// setupRoutes configures all routes.
func (s *Server) setupRoutes() {
	// Health check endpoints (no auth)
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/healthz", s.handleHealth)

	// Public cached file serving (video only — images are now base64-inlined)
	if s.cacheService != nil {
		s.router.Get("/api/files/video/{name}", handleServeCacheFileByType(s.cacheService, "video"))
		s.router.Get("/api/files/{type}/{name}", handleServeCacheFile(s.cacheService))
	}

	// API routes (with auth)
	s.router.Route("/v1", func(r chi.Router) {
		s.applyAPIAuth(r)
		if s.chatProvider != nil {
			s.chatProvider.SetupRoutes(r)
		}
	})

	// Admin API routes (with AppKey auth)
	s.router.Route("/admin", func(r chi.Router) {
		if s.runtime != nil {
			r.Use(AdminRateLimitRuntime(s.runtime))
		} else {
			r.Use(AdminRateLimit(s.cfg))
		}

		// Public (no AppKeyAuth) — login + logout endpoints
		if s.runtime != nil {
			r.Post("/login", handleAdminLoginRuntime(s.runtime))
		} else {
			r.Post("/login", handleAdminLogin(s.appKey))
		}
		r.Post("/logout", handleAdminLogout())

		// Protected — all other admin routes
		r.Group(func(r chi.Router) {
			if s.runtime != nil {
				r.Use(AppKeyAuthRuntime(s.runtime))
			} else {
				r.Use(AppKeyAuth(s.appKey))
			}

			// Verify endpoint (auth check - middleware handles validation)
			r.Get("/verify", func(w http.ResponseWriter, r *http.Request) {
				WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			})

			// System status endpoint
			r.Get("/system/status", handleSystemStatus(s.tokenStore, s.apiKeyStore, s.startTime, s.version))

			// Config endpoints
			if s.runtime != nil {
				r.Get("/config", handleGetConfigRuntime(s.runtime))
				r.Put("/config", handlePutConfigRuntime(s.runtime, s.configStore))
			} else if s.cfg != nil {
				r.Get("/config", handleGetConfig(s.cfg))
				r.Put("/config", handlePutConfig(s.cfg, s.configStore))
			}

			// Token endpoints
			if s.tokenStore != nil {
				r.Get("/tokens", handleListTokens(s.tokenStore, s.modelRegistry))
				r.Get("/tokens/ids", handleListTokenIDs(s.tokenStore, s.modelRegistry))
				r.Get("/tokens/{id}", handleGetToken(s.tokenStore, s.modelRegistry))
				r.Put("/tokens/{id}", handleUpdateToken(s.tokenStore, s.tokenPoolSyncer, s.modelRegistry))
				r.Post("/tokens/{id}/refresh", handleRefreshToken(s.tokenStore, s.tokenRefresher, s.modelRegistry))
				r.Delete("/tokens/{id}", handleDeleteToken(s.tokenStore, s.tokenPoolSyncer))
				if s.runtime != nil {
					r.Post("/tokens/batch", handleBatchTokensFromProvider(s.tokenStore, s.tokenPoolSyncer, s.modelRegistry))
				} else {
					r.Post("/tokens/batch", handleBatchTokens(s.tokenStore, s.tokenPoolSyncer, s.modelRegistry))
				}

				// Stats endpoints (token-based)
				r.Get("/stats/tokens", handleTokenStats(s.tokenStore, s.modelRegistry))
				r.Get("/stats/quota", handleQuotaStats(s.tokenStore, s.modelRegistry))
			}

			// Stats endpoints (usage-based)
			if s.usageLogStore != nil {
				r.Get("/stats/usage", handleUsageStats(s.usageLogStore))
			}

			// System usage endpoint (period-based aggregation for /usage page)
			if s.usageLogStore != nil {
				r.Get("/system/usage", handleSystemUsage(s.usageLogStore))
				r.Get("/usage/logs", handleUsageLogs(s.usageLogStore))
			}

			// API Key management endpoints
			if s.apiKeyStore != nil {
				r.Route("/apikeys", func(r chi.Router) {
					r.Get("/", handleListAPIKeys(s.apiKeyStore))
					r.Get("/stats", handleAPIKeyStats(s.apiKeyStore))
					r.Post("/", handleCreateAPIKey(s.apiKeyStore))
					r.Get("/{id}", handleGetAPIKey(s.apiKeyStore))
					r.Patch("/{id}", handleUpdateAPIKey(s.apiKeyStore))
					r.Delete("/{id}", handleDeleteAPIKey(s.apiKeyStore))
					r.Post("/{id}/regenerate", handleRegenerateAPIKey(s.apiKeyStore))
				})
			}

			// Cache management endpoints
			if s.cacheService != nil {
				r.Get("/cache/stats", handleCacheStats(s.cacheService))
				r.Get("/cache/files", handleCacheFiles(s.cacheService))
				r.Post("/cache/delete", handleDeleteCacheFiles(s.cacheService))
				r.Post("/cache/clear", handleClearCache(s.cacheService))
				r.Get("/cache/files/{type}/{name}", handleServeCacheFile(s.cacheService))
			}

			// Model catalog (read-only)
			if s.modelRegistry != nil {
				r.Get("/models", handleListModels(s.modelRegistry))
			}
		})
	})

	// Frontend SPA (catch-all, must be last)
	s.router.Handle("/*", SPAHandler())
}

// applyAPIAuth adds authentication middleware to an API route group.
func (s *Server) applyAPIAuth(r chi.Router) {
	if s.apiKeyStore != nil {
		r.Use(APIKeyAuth(s.apiKeyStore))
	}
}

// Router returns the chi router for http.Server.
func (s *Server) Router() http.Handler {
	return s.router
}

// HealthResponse is the health check response.
type HealthResponse struct {
	Status    string `json:"status"`
	Uptime    string `json:"uptime"`
	Timestamp string `json:"timestamp"`
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:    "ok",
		Uptime:    time.Since(s.startTime).Round(time.Second).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	WriteJSON(w, http.StatusOK, resp)
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

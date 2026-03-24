# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GrokForge is a Go rewrite of [grok2api](~/project_ai/grok2api/) (Python). It's an OpenAI-compatible API gateway that proxies requests to Grok (xAI), supporting chat completions, image generation/editing, and video generation. The frontend admin panel is a Next.js static export embedded into the Go binary via `embed.FS`.

Reference project path: `~/project_ai/grok2api/` — always check how the original implements a feature before designing new ones. The token relay chain and parameter conversion logic are production-verified and must be preserved.

## Build & Test

All build verification MUST use `make build` from project root. Never use sub-module build commands independently.

```bash
make build          # Frontend (npm ci + next build) → Go binary → bin/grokforge
make dev            # Go-only dev run (skips frontend build)
make test           # go test -v ./...
make run            # Build + run
make clean          # Remove bin/, data/, web/out/, web/.next/

# Run a single test
go test -v -run TestFunctionName ./internal/flow/

# Run tests for a specific package
go test -v ./internal/token/

# Frontend dev (manual, not via make)
cd web && npm run dev
cd web && npm run lint
```

Configuration: `config.toml` (copy from `config.defaults.toml`). Supports TOML sections: `[app]`, `[image]`, `[imagine_fast]`, `[proxy]`, `[retry]`, `[token]`. DB config overrides take precedence over file config.

## Architecture

Three-layer design: `httpapi → flow → {token, xai, store, config}`. `/v1` routes strictly follow this direction; `/admin` routes directly access store/token/cache for CRUD operations.

```
cmd/grokforge/main.go     — Wiring: config → DB → token service → flows → HTTP server
internal/
  httpapi/                 — HTTP layer: chi router, middleware, admin handlers
    openai/                — OpenAI-compatible /v1 endpoints (chat, models)
  flow/                    — Business logic orchestration
    chat*.go               — Chat completion: request build → stream → parse → format
    image*.go              — Image generation + editing (parallel retry on blocked)
    video*.go              — Video generation with polling + upscale
    retry.go               — Retry utilities: exponential backoff, budget, recoverability checks
    usage_buffer.go        — Async batch flush: sync Record() → periodic DB write
    toolcall.go            — OpenAI tool_calls extraction from Grok stream
  token/                   — Token pool management
    service.go             — Top-level API: LoadTokens, Pick, ReportSuccess/Error, Stats
    manager.go             — Pool state: cooling, quota, dirty tracking
    picker.go              — Selection algorithms: high_quota_first / random / round_robin
    pool.go                — Dual pool: ssoBasic + ssoSuper, config-driven model routing
    quota.go               — Quota deduction and recovery
    persist.go             — Dirty-flag based DB sync
  xai/                     — Grok API HTTP client (tls-client fingerprinting)
    client.go              — Client interface: Chat, CreateImagePost, CreateVideoPost, UploadFile, etc.
    chat.go                — Chat method + SSE/NDJSON dual-format stream parser (per-line auto-detect)
    websocket.go           — WebSocket client for image generation (wss://grok.com/ws/imagine)
    headers.go             — Dynamic Statsig fingerprint generation
  store/                   — GORM data layer (SQLite default, PostgreSQL optional)
    models.go              — DB models: Token, UsageLog, APIKey, ConfigEntry
    token_store.go         — Token CRUD + pool queries
    usage_log_store.go     — Usage aggregation queries (period-based)
    apikey_store.go        — API key management with daily usage reset
  config/                  — TOML config loading with DB override merge
  cache/                   — Local file cache for video downloads
  cfrefresh/               — Cloudflare cookie auto-refresh via FlareSolverr
  logging/                 — slog setup with lumberjack rotation
web/                       — Next.js 15 + shadcn/ui + Tailwind + React Query
  embed.go                 — go:embed all:out (frontend static files embedded here)
  src/app/                 — Pages: tokens, apikeys, usage, settings, function (playground), cache, login
```

## Key Patterns

- **Token pool routing**: `GetPoolForModel(model, *config.TokenConfig)` — config-driven, not hardcoded maps. Models are assigned to ssoBasic/ssoSuper pools via `basic_models`/`super_models` in config with optional `#cost` suffix.
- **Client factory**: Flows receive `func(token string) Client` factories, not pre-built clients. Each request gets a fresh client with the selected token's cookie.
- **tls-client**: Uses `bogdanfinn/tls-client` for browser TLS fingerprinting, not `net/http`. The `xai` package wraps this. WebSocket connections (image generation) use gorilla/websocket with stdlib `net/http`.
- **Dual stream parser**: `xai/chat.go` `parseSSEStream()` handles SSE (`data: ` prefix) and NDJSON (`{` prefix) per-line, supporting mixed formats in a single stream.
- **Retry at flow layer**: Chat/image flows set `xai.Client` `MaxRetry(0)` — retry orchestration in `flow/chat.go` using `flow/retry.go` utilities (exponential backoff, budget, per-token limits). 403 triggers CF refresh + session reset for ChatFlow.
- **Usage buffer**: Synchronous `Record()` → async batch flush to DB. Re-queues on failure.
- **Frontend embed**: `web/out/` is built by Next.js static export and embedded directly via `web/embed.go` (`go:embed all:out`) + SPA catch-all handler.
- **Admin auth**: HMAC-SHA256 signed session from `/admin/login` with AppKey, verified by `AppKeyAuth` middleware. Rate-limited with IP lockout.
- **API auth**: `/v1` routes use `APIKeyAuth` middleware checking the `apikey_store`.

## Route Structure

- `GET /health`, `GET /healthz` — Health check (no auth)
- `GET /api/files/{type}/{name}` — Cached media file serving (no auth)
- `GET /v1/models` — List available models (API key auth)
- `POST /v1/chat/completions` — Chat + image + video via OpenAI protocol (API key auth)
- `/admin/*` — Admin panel API (AppKey auth): tokens, apikeys, config, usage, cache, system
- `/*` — SPA frontend catch-all

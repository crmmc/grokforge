<h1 align="center">GrokForge</h1>

<p align="center">
  <b>Full-model OpenAI-compatible Grok API gateway — single binary, ready to run</b>
</p>

<p align="center">
  <b>English</b> · <a href="./README.zh.md">简体中文</a>
</p>

<p align="center">
  <a href="https://github.com/crmmc/grokforge/releases"><img src="https://img.shields.io/github/v/release/crmmc/grokforge?style=flat-square&color=blue" alt="Release"></a>
  <a href="https://github.com/crmmc/grokforge/blob/main/LICENSE"><img src="https://img.shields.io/github/license/crmmc/grokforge?style=flat-square" alt="License"></a>
  <a href="https://github.com/crmmc/grokforge"><img src="https://img.shields.io/github/stars/crmmc/grokforge?style=flat-square" alt="Stars"></a>
  <a href="https://github.com/crmmc/grokforge"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go" alt="Go Version"></a>
</p>

<p align="center">
  <img src="docs/screenshots/dashboard.png" width="800" alt="Dashboard" />
</p>
<p align="center">
  <img src="docs/screenshots/tokens.png" width="400" alt="Token Management" />&nbsp;
  <img src="docs/screenshots/apikeys.png" width="400" alt="API Keys" />
</p>
<p align="center">
  <img src="docs/screenshots/usage.png" width="400" alt="Usage Analytics" />&nbsp;
  <img src="docs/screenshots/settings.png" width="400" alt="Settings" />
</p>

---

## What is this

GrokForge wraps all Grok web capabilities (chat, reasoning, image generation/editing, video generation) into a standard OpenAI API format (a.k.a. 2api gateway). You can seamlessly connect any OpenAI-compatible client (ChatGPT Next Web, LobeChat, Open WebUI, Cursor, bots, etc.) to Grok models.

> Go rewrite + Next.js admin panel, compiled into a single binary, SQLite out of the box, zero external dependencies.

---

## Highlights

- **Single binary deployment** — Frontend embedded via `go:embed`, just copy and run
- **Modern admin panel** — Next.js + shadcn/ui, one-stop Dashboard / Token / API Key / Settings / Usage / Cache management
- **Multi-pool token routing** — ssoBasic / ssoSuper / ssoHeavy routed by `pool_floor`, with 3 selection algorithms + priority tiers + recent-use penalty
- **Static model catalog** — Models defined in a TOML file embedded in the binary, overridable via external file
- **Mode-based dynamic quotas** — Quota windows driven by the model catalog; `image_ws` uses transient cooldown only
- **Image output modes** — Configurable `base64` (inline data URI) or `local_url` (cache to disk, serve via `/api/files/`)
- **Media leak protection** — Stream-safe URL rewriting ensures no upstream Grok URLs leak to clients
- **SSE heartbeat** — 2KB initial padding + 15s ping keeps connections alive through proxies and CDNs
- **DeepSearch** — Pass `deepsearch` parameter to enable Grok's deep search capability
- **Hot-reload config** — Admin panel changes take effect immediately, no restart needed
- **Structured logging** — slog + file rotation, JSON / Text formats
- **Bilingual UI** — Admin panel supports English and Chinese

---

## Features

### Core

- [x] **OpenAI Chat Completions API** — Streaming / non-streaming, fully compatible
- [x] **Chain-of-thought reasoning** — `<think>` tag output, `reasoning_effort` control
- [x] **Tool Calling** — Hermes-style tool calls with parallel_tool_calls support
- [x] **Multimodal input** — Image URL / base64, auto download, decode and resize
- [x] **Image generation / editing** — WebSocket channel, multiple images, various sizes
- [x] **Video generation** — Multiple aspect ratios and resolutions
- [x] **Model listing** — `GET /v1/models` returns enabled models from the static catalog
- [x] **SSE heartbeat** — 2KB padding + 15s ping prevents proxy/CDN timeout disconnections
- [x] **DeepSearch** — `deepsearch` parameter passthrough for Grok's deep search capability

### Token Management

- [x] **Multi-pool routing** — ssoBasic / ssoSuper / ssoHeavy selected by `pool_floor`
- [x] **3 selection algorithms** — high_quota_first / random / round_robin
- [x] **Priority tiers** — Higher priority tokens are selected first
- [x] **Recent-use penalty** — Configurable deprioritization window (default 15s) prevents the same token from being picked consecutively
- [x] **Batch quota refresh** — SSE-streamed batch refresh with real-time progress and cancel support
- [x] **Mode-based quotas** — chat / image_lite / image_edit / video share quota windows by catalog mode
- [x] **`image_ws` exception** — WebSocket image models stay outside quota sync and use transient token+model cooldown only
- [x] **Auto refresh** — Periodic session refresh, auto rebuild on failure
- [x] **Token state model** — persisted `active / disabled / expired`, derived `exhausted` display state

### Model Catalog

- [x] **Static TOML catalog** — Models defined in `internal/modelconfig/models.toml`, embedded in binary
- [x] **External override** — Set `models_file` in `config.toml` to replace the default catalog entirely
- [x] **Read-only admin view** — Settings page displays the full model catalog (no editing)
- [x] **Registry-driven routing** — O(1) request-name resolution via in-memory snapshot
- [x] **Per-mode pool_floor override** — Expert → basic, Heavy → heavy, etc.

### Security & Reliability

- [x] **API Key management** — CRUD + model whitelist + daily limit + rate limit
- [x] **Exponential backoff retry** — Jitter + budget control + session auto-reset
- [x] **Cloudflare defense** — FlareSolverr integration, instant 403 refresh + debounce
- [x] **Secure authentication** — Constant-time comparison; auto-generated bootstrap password when `app_key` is unset (process-local, logged on startup)

### Admin Panel

- [x] **Dashboard** — Stats cards + quota progress + usage trend charts
- [x] **Token management** — Batch import / enable / disable / delete, status filtering, health indicators
- [x] **API Key management** — Create / disable / expire / regenerate keys
- [x] **System settings** — General + Models dual tabs, hot-reload on save
- [x] **Usage stats** — Aggregate overview + per-request logs (with TTFT)
- [x] **Cache management** — Image / video stats, preview / download / batch cleanup
- [x] **Playground** — Chat / Image / Video generation online, multi-turn conversation with Markdown rendering

---

## Supported Models

<details>
<summary><b>Chat Models</b></summary>

| Model | Mode | Pool Floor | Description |
|-------|------|------------|-------------|
| `grok-4.20` | auto | super | Default Grok 4.20 mode |
| `grok-4.20-fast` | fast | basic | Faster Grok 4.20 variant |
| `grok-4.20-think` | auto (force_thinking) | super | Deep reasoning mode |
| `grok-4.20-expert` | expert | super | Expert mode |
| `grok-4.20-heavy` | heavy | heavy | Heavy pool only |
| `grok-4.3-beta` | grok43 | super | Grok 4.3 beta |

</details>

<details>
<summary><b>Media Models</b></summary>

| Model | Type | Pool Floor | Description |
|-------|------|------------|-------------|
| `grok-imagine-image` | image_ws (WebSocket) | super | Full image generation (no quota sync) |
| `grok-imagine-image-pro` | image_ws + enable_pro | super | Pro image generation (no quota sync) |
| `grok-imagine-image-lite` | image_lite (HTTP) | basic | Lightweight image generation, Basic pool |
| `grok-imagine-image-edit` | image_edit | super | Image editing (supports reference images) |
| `grok-imagine-video` | video | super | Video generation |

</details>

> All models are defined in `internal/modelconfig/models.toml` (embedded in the binary). To customize, set `models_file` in `config.toml` to point to your own TOML catalog file — it replaces the default entirely.

---

## Quick Start

### 30-Second Setup

```bash
# 1. Download & start
./grokforge -config config.toml
# If app_key is not set, a temporary admin password is printed in the startup log.

# 2. Open admin panel and add your Grok Token
#    http://localhost:8080

# 3. Test
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-4.20",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Build from Source

**Prerequisites**: Go 1.25+, Node.js 18+

```bash
git clone https://github.com/crmmc/grokforge.git
cd grokforge

# Copy config template
cp config.defaults.toml config.toml

# One-command build (frontend + backend)
make build

# Run
./bin/grokforge
```

The build output is a single binary with the frontend embedded via `go:embed`.

---

## Configuration

GrokForge uses TOML configuration. See [`config.defaults.toml`](./config.defaults.toml) for the full template.

### Minimal Config

```toml
[app]
app_key = "your-admin-password"   # Admin password (omit to auto-generate a temporary one on startup)
port = 8080                        # Server port

[proxy]
base_proxy_url = ""                # Optional: proxy URL
```

After startup, add Grok Tokens in the admin panel. All other settings can be modified online.

### Config Priority

```
Admin panel (DB) > config.toml > Built-in defaults
```

Admin panel changes take effect immediately without restart.

### Core Settings

<details>
<summary><b>Application [app]</b></summary>

| Key | Default | Description |
|-----|---------|-------------|
| `app_key` | `""` | Admin password (if empty, a temporary bootstrap password is auto-generated and logged on startup) |
| `port` | `8080` | Server port |
| `host` | `"0.0.0.0"` | Listen address |
| `db_driver` | `"sqlite"` | Database driver: `sqlite` / `postgres` |
| `db_path` | `"data/grokforge.db"` | SQLite file path |
| `db_dsn` | `""` | PostgreSQL connection string |
| `log_level` | `"info"` | Log level: debug/info/warn/error |
| `log_json` | `false` | JSON format logs |
| `request_timeout` | `60` | Default timeout for non-LLM routes (seconds) |
| `temporary` | `true` | Temporary conversation mode |
| `thinking` | `true` | Enable chain-of-thought by default |
| `stream` | `true` | Streaming response by default |
| `filter_tags` | `[...]` | Special tags to filter |

</details>

<details>
<summary><b>Proxy [proxy]</b></summary>

| Key | Default | Description |
|-----|---------|-------------|
| `base_proxy_url` | `""` | Upstream proxy (HTTP/HTTPS/SOCKS5) |
| `asset_proxy_url` | `""` | Asset proxy (image downloads, etc.) |
| `cf_clearance` | `""` | Cloudflare clearance cookie |
| `browser` | `"chrome_146"` | TLS fingerprint browser profile |
| `enabled` | `false` | Enable CF auto-refresh |
| `flaresolverr_url` | `""` | FlareSolverr service URL |
| `refresh_interval` | `3600` | CF refresh interval (seconds) |

</details>

<details>
<summary><b>Retry Policy [retry]</b></summary>

| Key | Default | Description |
|-----|---------|-------------|
| `max_tokens` | `5` | Maximum tokens to try |
| `per_token_retries` | `2` | Maximum retries per token before switching |
| `reset_session_status_codes` | `[403]` | Status codes that trigger session reset |
| `retry_backoff_base` | `0.5` | Backoff base delay (seconds) |
| `retry_backoff_factor` | `2.0` | Backoff multiplier |
| `retry_backoff_max` | `20.0` | Maximum single delay (seconds) |
| `retry_budget` | `60.0` | Total retry budget (seconds) |

</details>

<details>
<summary><b>Token Management [token]</b></summary>

| Key | Default | Description |
|-----|---------|-------------|
| `fail_threshold` | `5` | Consecutive failure threshold (marks disabled) |
| `usage_flush_interval_sec` | `30` | Interval for flushing usage stats to DB |
| `selection_algorithm` | `"high_quota_first"` | Algorithm: high_quota_first / random / round_robin |
| `recent_use_penalty_sec` | `15` | Deprioritization window after a token is picked (seconds, 0 = disabled) |

</details>

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   Client                        │
│   (ChatGPT Next Web / LobeChat / curl / ...)    │
└─────────────────────┬───────────────────────────┘
                      │ OpenAI API
                      ▼
┌─────────────────────────────────────────────────┐
│                   GrokForge                     │
│                                                 │
│  ┌───────────┐  ┌───────────┐  ┌────────────┐  │
│  │  httpapi   │  │   Admin   │  │  Static    │  │
│  │ (OpenAI)  │  │   API     │  │  Frontend  │  │
│  └─────┬─────┘  └─────┬─────┘  └────────────┘  │
│        │              │                         │
│        ▼              ▼                         │
│  ┌─────────────────────────────────────────┐    │
│  │           flow (orchestration)          │    │
│  │   chat / image / video / model registry  │    │
│  └──────┬──────────┬──────────┬────────────┘    │
│         │          │          │                  │
│         ▼          ▼          ▼                  │
│  ┌──────────┐ ┌─────────┐ ┌──────────┐         │
│  │  token   │ │   xai   │ │  store   │         │
│  │  (pool)  │ │(upstream)│ │(persist) │         │
│  └──────────┘ └─────────┘ └──────────┘         │
│                    │                            │
└────────────────────┼────────────────────────────┘
                     │
                     ▼
              ┌─────────────┐
              │   grok.com  │
              │ (SSE / WS)  │
              └─────────────┘
```

Three-tier architecture: **httpapi** (protocol translation) → **flow** (business orchestration) → **xai / token / store** (infrastructure)

Unidirectional dependencies, no circular references.

---

## Request Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant H as httpapi
    participant F as flow
    participant T as token
    participant X as xai
    participant G as grok.com

    C->>H: POST /v1/chat/completions
    H->>H: Auth check + param parsing
    H->>F: Route to chat/image/video flow
    F->>T: Request available Token (`pool_floor` + algorithm selection)
    T-->>F: Return Token
    F->>X: Build upstream request
    X->>G: SSE / WebSocket request
    G-->>X: Streaming response
    X-->>F: Parse + transform
    F-->>H: OpenAI format output
    H-->>C: SSE stream / JSON response

    Note over F,T: Auto retry on failure<br/>Switch Token + Session reset
```

---

## Admin Panel

> Admin panel URL: `http://your-host:8080`

The admin panel includes:

- **Dashboard** — System status at a glance: Token count, API Key count, call volume, quota progress, trend charts
- **Token Management** — Batch import / enable / disable / delete, status filtering, quota editing, priority settings, batch quota refresh with SSE progress
- **API Key** — Create and manage API keys, model whitelist, daily limit, rate limit
- **Model Catalog** — Read-only view of the static model catalog with pool_floor and upstream details
- **Settings** — General config online editing (hot-reload) + read-only model catalog view
- **Usage Stats** — Aggregate overview + per-request logs (including TTFT, token consumption metrics)
- **Cache** — Image / video cache browsing, preview, download, cleanup
- **Playground** — Chat / Image / Video generation online, multi-turn conversation with Markdown rendering

---

## API Examples

### Chat Completion (Streaming)

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-4.20",
    "messages": [{"role": "user", "content": "Explain quantum computing in one sentence"}],
    "stream": true
  }'
```

### Chain-of-Thought Model

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-4.20-expert",
    "messages": [{"role": "user", "content": "Prove that √2 is irrational"}],
    "reasoning_effort": "high"
  }'
```

### Tool Calling

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-4.20",
    "messages": [{"role": "user", "content": "What is the weather in Beijing today?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city",
        "parameters": {
          "type": "object",
          "properties": {
            "city": {"type": "string", "description": "City name"}
          },
          "required": ["city"]
        }
      }
    }]
  }'
```

### Multimodal (Image Input)

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-4.20",
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "Describe this image"},
        {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
      ]
    }]
  }'
```

### Image Generation

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-image",
    "messages": [{"role": "user", "content": "A shiba inu in a spacesuit walking on the moon"}]
  }'
```

### Video Generation

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-video",
    "messages": [{"role": "user", "content": "A cat dancing on a piano"}]
  }'
```

---

## Client Integration

GrokForge is compatible with all OpenAI API clients — just point the API URL to GrokForge:

| Client | Configuration |
|--------|---------------|
| **ChatGPT Next Web** | Settings → API URL = `http://your-host:8080` |
| **LobeChat** | Settings → OpenAI → API URL = `http://your-host:8080/v1` |
| **Open WebUI** | Admin → Connections → OpenAI API = `http://your-host:8080/v1` |
| **Cursor** | Settings → Models → OpenAI Base URL = `http://your-host:8080/v1` |
| **Any OpenAI SDK** | `base_url="http://your-host:8080/v1"` |

---

## FAQ

<details>
<summary><b>How to get a Grok Token?</b></summary>

1. Log in to [grok.com](https://grok.com)
2. Open browser DevTools (F12)
3. Find the `sso` or `sso-rw` cookie value in Application → Cookies
4. Import it in the admin panel

</details>

<details>
<summary><b>What's the difference between Basic, Super, and Heavy pools?</b></summary>

- **Basic pool (`ssoBasic`)**: Lowest capability floor. Can serve models/modes whose `pool_floor` is `basic`.
- **Super pool (`ssoSuper`)**: Higher capability floor. Can serve `super` and `basic` models/modes.
- **Heavy pool (`ssoHeavy`)**: Highest capability floor. Reserved for `heavy` models/modes and can also serve lower-floor requests.
- Model routing is defined in the static model catalog (`internal/modelconfig/models.toml`), not by editing per-pool model lists.
- `pool_floor` is a hard requirement. If no eligible token exists in pools with level >= the model's floor, the request fails with no silent downgrade.

</details>

<details>
<summary><b>What to do about 403 errors?</b></summary>

Usually triggered by Cloudflare protection. Solutions:

1. **Configure proxy**: Set `base_proxy_url` with a clean IP
2. **FlareSolverr**: Configure `flaresolverr_url`, GrokForge auto-refreshes CF cookies
3. **Manual update**: Update `cf_clearance` cookie in the admin panel

</details>

<details>
<summary><b>How long until token quotas recover?</b></summary>

Quota-tracked models (`chat`, `image_lite`, `image_edit`, `video`) recover per mode window.

- The scheduler starts a token+mode window from the first successful upstream response after refresh.
- When that window expires, GrokForge refreshes the mode from `/rest/rate-limits` and learns both remaining quota and total quota from upstream.
- `image_ws` is not quota-tracked; it only uses transient token+model cooldown in memory.

</details>

<details>
<summary><b>How to share with multiple users?</b></summary>

1. Create API Keys in the admin panel, assign one per user
2. Set **Model Whitelist** to restrict available models
3. Set **Daily Limit** to control daily usage per user
4. Set **Rate Limit** to prevent burst requests

</details>

<details>
<summary><b>Which databases are supported?</b></summary>

- **SQLite** (default): Zero config, data stored in `data/grokforge.db`
- **PostgreSQL**: Recommended for production, set `db_driver = "postgres"` and `db_dsn`

Both databases have identical functionality with current-schema initialization on startup.
If the schema changes in local development, delete `data/grokforge.db` and rebuild instead of expecting in-place migration.

</details>

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25 · chi · GORM · slog |
| Frontend | Next.js · shadcn/ui · Tailwind CSS · Recharts |
| Storage | SQLite (default) · PostgreSQL (optional) |
| Build | Make · go:embed (frontend embedded in binary) |

---

## Project Structure

```
grokforge/
├── cmd/grokforge/       # Entry point
├── internal/
│   ├── httpapi/         # HTTP layer (OpenAI compat + Admin API)
│   │   └── openai/      # OpenAI protocol implementation
│   ├── flow/            # Business orchestration (chat / image / video)
│   ├── token/           # Token pool management (routing / selection / quota / refresh)
│   ├── xai/             # Upstream communication (SSE / WebSocket)
│   ├── store/           # Persistence (GORM + current schema/constraints)
│   ├── modelconfig/    # Static model catalog (TOML embedded + loader)
│   ├── config/          # Config management (TOML + DB override + hot-reload)
│   ├── cfrefresh/       # Cloudflare defense (FlareSolverr integration)
│   ├── cache/           # Cache management (image / video local cache)
│   └── logging/         # Log management (slog + file rotation)
├── web/                 # Next.js frontend
│   └── src/app/         # Page routes
├── config.defaults.toml # Config template
└── Makefile             # Build script
```

---

## Acknowledgements

- [grok2api](https://github.com/chenyme/grok2api) — Original Python project that proved the concept
- [chi](https://github.com/go-chi/chi) — Lightweight HTTP router
- [GORM](https://gorm.io) — Go ORM
- [shadcn/ui](https://ui.shadcn.com) — UI component library

---

## Changelog

See [CHANGELOG.md](./CHANGELOG.md) for release history and version details.

---

## Star History

<a href="https://github.com/crmmc/grokforge/stargazers">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=crmmc/grokforge&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=crmmc/grokforge&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=crmmc/grokforge&type=Date" />
  </picture>
</a>

---

## License

[MIT](./LICENSE)

---

> **Disclaimer**: This project is for educational and research purposes only. Users must comply with the terms of service of relevant platforms. Any consequences arising from the use of this project are the sole responsibility of the user.

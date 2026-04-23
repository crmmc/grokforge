# Static Model Catalog Redesign

## Status

Accepted for implementation planning after review fixes.

## Summary

GrokForge will remove runtime-editable model management and replace it with a static model catalog.

The catalog is developer-authored protocol metadata, not operator-authored business configuration:

- the binary ships with an embedded default catalog
- operators may optionally point `app.models_file` to an external catalog file
- the external file, when configured, fully replaces the embedded catalog
- model definitions are loaded only at startup

The server will no longer read model definitions from the database, no longer expose `/admin/models/*` CRUD APIs, and no longer provide a writable model management page in the admin UI.

This redesign is still intentionally not backward compatible for admin model CRUD, but it preserves the public `/v1/models` contract where practical.

## Context

The current model system mixes three concerns that should not be mixed:

- upstream protocol knowledge
- operator deployment configuration
- runtime CRUD and database state

That coupling creates the worst parts of both designs:

- too much runtime configurability for metadata that rarely changes
- extra storage, migration, and refresh complexity
- public request model names derived from `family + mode` DB state
- model routing semantics that are harder to inspect than the actual upstream contract

At the same time, the current runtime already depends on static upstream facts:

- chat requests depend on upstream `modeId`
- `image_lite` depends on upstream `modeId`
- `image_edit` and `video` depend on both upstream `modelName` and `modeId`
- `image_ws` depends on fixed WebSocket behavior plus flags such as `enable_pro`

The redesign therefore treats GrokForge as an explicit adapter to upstream behavior, with a static catalog as the single runtime source of model metadata.

## Goals

- Make model definition logic explicit, deterministic, and file-driven.
- Remove database-backed model definition state and CRUD refresh behavior.
- Keep `/v1/models` stable for existing clients where possible.
- Preserve exact upstream routing semantics by keeping required protocol fields explicit.
- Allow fast catalog-only updates when upstream model metadata changes but code paths do not.
- Fail fast on invalid catalog data with clear startup-time errors.

## Non-Goals

- No backward compatibility for old `/admin/models/*` write APIs or model management UI.
- No migration of existing `model_families` and `model_modes` rows into the new catalog format.
- No hot reload of model definitions.
- No DB override for model catalog source or content.
- No partial merge between embedded and external catalogs.
- No silent fallback from an invalid external catalog back to the embedded catalog.

## Hard Decisions

### Static catalog ownership

The model catalog is developer-authored release metadata.

It is not expected to be hand-maintained by operators in normal deployments.

### Embedded default catalog

The binary includes an embedded default catalog.

This is the default source when no external catalog is configured.

### Optional external override

`config.toml` adds an optional field:

```toml
[app]
models_file = "models.toml"
```

If `app.models_file` is configured, the server loads only that file and treats it as a complete replacement for the embedded catalog.

There is no merge, patch, or layered override of individual models.

### Path resolution

`app.models_file` is resolved relative to the directory containing the active `config.toml`.

This keeps behavior stable for:

- `./grokforge -config ./config.toml`
- `./grokforge -config /etc/grokforge/config.toml`
- container mounts that map only the config directory

### Startup-only loading

Catalog loading is startup-only.

Changing the catalog requires:

1. replace the external file or upgrade to a new binary
2. restart the service

### No DB ownership

`app.models_file` and the loaded catalog are not part of the DB override chain.

Model catalog source is file-only and startup-only.

## Catalog File Format

The file format is flat: one `[[model]]` entry per public request model.

The system does not preserve `family + mode` as a runtime concept.

### Schema

```toml
version = 1

[[model]]
id = "grok-4.20"
display_name = "Grok 4.20"
type = "chat"
enabled = true
pool_floor = "basic"
quota_mode = "auto"
upstream_mode = "auto"

[[model]]
id = "grok-4.20-fast"
display_name = "Grok 4.20 Fast"
type = "chat"
enabled = true
pool_floor = "basic"
quota_mode = "fast"
upstream_mode = "fast"

[[model]]
id = "grok-4.20-think"
display_name = "Grok 4.20 Think"
type = "chat"
enabled = true
pool_floor = "basic"
quota_mode = "auto"
upstream_mode = "auto"
force_thinking = true

[[model]]
id = "grok-4.20-expert"
display_name = "Grok 4.20 Expert"
type = "chat"
enabled = true
pool_floor = "super"
quota_mode = "expert"
upstream_mode = "expert"

[[model]]
id = "grok-imagine-image"
display_name = "Grok Imagine Image"
type = "image_ws"
enabled = true
pool_floor = "super"
quota_mode = "auto"

[[model]]
id = "grok-imagine-image-pro"
display_name = "Grok Imagine Image Pro"
type = "image_ws"
enabled = true
pool_floor = "super"
quota_mode = "auto"
enable_pro = true

[[model]]
id = "grok-imagine-image-lite"
display_name = "Grok Imagine Image Lite"
type = "image_lite"
enabled = true
pool_floor = "basic"
quota_mode = "fast"
upstream_mode = "fast"

[[model]]
id = "grok-imagine-image-edit"
display_name = "Grok Imagine Image Edit"
type = "image_edit"
enabled = true
pool_floor = "super"
quota_mode = "auto"
upstream_model = "imagine-image-edit"
upstream_mode = "auto"

[[model]]
id = "grok-imagine-video"
display_name = "Grok Imagine Video"
type = "video"
enabled = true
pool_floor = "super"
quota_mode = "auto"
upstream_model = "grok-3"
upstream_mode = "auto"
```

### Field Definitions

- `version`
  - required
  - currently fixed to `1`
- `[[model]]`
  - repeated top-level model entries
- `id`
  - required
  - public request model identifier
- `display_name`
  - required
  - operator-facing label for read-only display
- `type`
  - required
  - internal runtime type
  - enum: `chat | image_ws | image_lite | image_edit | video`
- `enabled`
  - required
  - controls whether the model is accepted and listed
- `pool_floor`
  - required
  - enum: `basic | super | heavy`
- `quota_mode`
  - required
  - enum: `auto | fast | expert | heavy`
  - drives internal pool and quota semantics
- `upstream_model`
  - required for `image_edit` and `video`
  - forbidden for `chat`, `image_ws`, and `image_lite`
- `upstream_mode`
  - required for `chat`, `image_lite`, `image_edit`, and `video`
  - forbidden for `image_ws`
- `force_thinking`
  - optional
  - allowed only for `chat`
  - default `false`
- `enable_pro`
  - optional
  - allowed only for `image_ws`
  - default `false`

### Derived Public Type

The schema keeps only the internal runtime `type`.

Public API responses derive a stable `public_type` at load time:

- `chat -> chat`
- `image_ws -> image_ws`
- `image_lite -> image`
- `image_edit -> image_edit`
- `video -> video`

`public_type` is not user-configurable.

It exists only in runtime metadata so `/v1/models` can preserve its current public shape.

### Validation Rules

- `version` must equal `1`
- at least one `[[model]]` entry must exist
- `id` must be unique
- `type` must be in the allowed enum
- `pool_floor` must be in the allowed enum
- `quota_mode` must be in the allowed enum
- `quota_mode = "expert"` requires `pool_floor` of at least `super`
- `quota_mode = "heavy"` requires `pool_floor = "heavy"`
- `chat` must define `upstream_mode` and must not define `upstream_model`
- `image_lite` must define `upstream_mode` and must not define `upstream_model`
- `image_edit` and `video` must define both `upstream_model` and `upstream_mode`
- `image_ws` must not define `upstream_model` or `upstream_mode`
- `force_thinking` is valid only for `chat`
- `enable_pro` is valid only for `image_ws`
- at least one model must have `enabled = true`

Unknown fields must be rejected.

## Runtime Architecture

### New Loading Flow

Startup sequence becomes:

1. load `config.toml`
2. resolve catalog source
3. load the embedded catalog or the configured external file
4. validate catalog schema and semantics
5. derive immutable runtime specs, including `public_type`
6. build the in-memory model registry
7. inject the registry into HTTP and flow layers
8. start the server

### New Package

Add a focused package:

- `internal/modelconfig`

Package structure:

```
internal/modelconfig/
├── models.toml      # embedded default catalog source
├── embed.go         # //go:embed models.toml
├── loader.go        # Load + Validate
└── spec.go          # ModelSpec type definition
```

Responsibilities:

- load catalog bytes from embedded or external source
- parse TOML
- validate schema and semantic rules
- convert entries into immutable runtime specs

This package must not depend on GORM or any DB state.

### Registry Role After Redesign

`internal/registry` may remain, but only as a read-only in-memory index.

It will no longer:

- load from database
- build snapshots from `ModelStore`
- support refresh after CRUD
- participate in transactional commit/apply behavior

It will only:

- resolve a model by `id`
- list enabled models
- expose `pool_floor`
- expose internal `type`
- expose derived `public_type`
- expose `quota_mode`
- expose `upstream_model`
- expose `upstream_mode`
- expose `force_thinking`
- expose `enable_pro`

## Backend Changes

### Keep

- `/v1/models`
- existing flow routing split by model type
- pool selection logic
- quota mode usage in internal flow logic

### Modify

Files that require changes but are not deleted:

- `internal/httpapi/server.go` — remove `modelStore` field, remove `/models/*` CRUD routes, add `GET /admin/models` read-only route
- `internal/httpapi/openai/chat_routing.go` — `"image"` → `"image_lite"` in type checks (`isImageModel`, `isMediaModel`)
- `internal/httpapi/openai/provider.go` — `rm.Family.Type` → `rm.Type`, `rm.ForceThinking` access path changes
- `internal/httpapi/openai/models.go` — `rm.Family.Type` → `rm.PublicType` for response
- `internal/httpapi/openai/handler.go` — registry type changes
- `internal/registry/registry.go` — rewrite to build from `[]modelconfig.ModelSpec` instead of `ModelStore`
- `cmd/grokforge/main.go` — replace model seeding + ModelStore + Refresh with catalog loading
- `internal/store/models.go` — remove `ModelFamily`, `ModelMode` structs and `AllModels()` references
- `internal/httpapi/openai/chat_test.go` — test helpers use new registry construction
- `internal/httpapi/openai/models_test.go` — same

### Remove Completely

Types and packages:

- `store.ModelFamily`
- `store.ModelMode`
- `store.ModelStore`
- `store.ModelStoreTx`

Specific files to delete:

- `internal/store/model_store.go`
- `internal/store/model_tx.go`
- `internal/store/model_helpers.go`
- `internal/store/model_constraints.go`
- `internal/store/seed.go`
- `internal/store/model_store_test.go`
- `internal/store/seed_test.go`
- `internal/httpapi/admin_model.go`
- `internal/httpapi/admin_model_test.go`
- `internal/httpapi/model_integration_test.go`
- `config/models.seed.toml`
- `web/src/app/(admin)/models/` (entire directory)
- `web/src/lib/hooks/use-model-families.ts`

Code references to clean up:

- `store.AllModels()` must remove `&ModelFamily{}` and `&ModelMode{}`
- `store.DeriveRequestName()` becomes dead code and must be deleted
- `internal/httpapi/server.go` must remove `modelStore` field and `/models/*` CRUD routes
- model CRUD tests
- model DB integration tests that exist only for CRUD and registry refresh

### Server Wiring Changes

`cmd/grokforge/main.go` must stop:

- seeding model definitions into the database
- constructing the registry from `ModelStore`
- refreshing registry state from DB rows

Instead it must:

- load catalog source via `internal/modelconfig`
- build the read-only registry directly from catalog specs
- pass the registry into HTTP and flow layers

### Routing Metadata Contract

The runtime model spec must expose exactly the fields required by request routing:

- `id`
- `display_name`
- `type`
- `public_type`
- `enabled`
- `pool_floor`
- `quota_mode`
- `upstream_model`
- `upstream_mode`
- `force_thinking`
- `enable_pro`

There is no separate family or mode entity.

### Quota Mode vs Upstream Mode

`quota_mode` and `upstream_mode` are independent fields that happen to share the same value for most models.

- `quota_mode` drives token pool selection and quota consumption in the token layer
- `upstream_mode` drives the Grok API request payload

Flow code must use `quota_mode` for `Pick()` and `Consume()`, not `upstream_mode`.

Example where they differ: `grok-4.20-think` has `quota_mode = "auto"` and `upstream_mode = "auto"` with `force_thinking = true`. If a future model maps to a different upstream mode while sharing quota with auto, the distinction becomes critical.

### Internal Type Rename: `"image"` to `"image_lite"`

The current codebase uses `"image"` as the internal type for image_lite models. This redesign renames it to `"image_lite"` for clarity.

Affected code paths:

- `chat_routing.go`: `isImageModel()` must check `"image_lite"` instead of `"image"`
- `isMediaModel()` must include `"image_lite"` instead of `"image"`
- `registry.EnabledByType("image")` calls must use `"image_lite"`
- `requiresUpstreamModel()` and `requiresUpstreamMode()` must use `"image_lite"`

The public API is unaffected: `/v1/models` returns derived `public_type = "image"` for `image_lite` models.

### OpenAI Models Endpoint

`/v1/models` must list enabled models from the read-only registry.

The public response remains stable:

- `id` remains the public request model identifier
- `type` returns derived `public_type`, not internal runtime `type`
- `grok-imagine-image-lite` therefore still appears as `type = "image"`

## Admin and Frontend Changes

### Admin API

Delete all `/admin/models/*` write APIs.

Add one read-only endpoint:

- `GET /admin/models`

This endpoint returns the resolved static catalog with operator-facing metadata, including fields not exposed by `/v1/models`.

Recommended response fields:

- `id`
- `display_name`
- `type`
- `public_type`
- `pool_floor`
- `quota_mode`
- `upstream_model`
- `upstream_mode`
- `force_thinking`
- `enable_pro`
- `enabled`

### Frontend

Remove:

- admin `/models` page
- family create/edit dialogs
- mode create/edit dialogs
- model CRUD hooks
- model management sidebar entry

Keep the Settings page `Models` tab, but make it read-only.

The tab contains:

- existing token, image, and Grok default configuration forms
- a read-only model table sourced from `GET /admin/models`

Display columns:

- `ID`
- `Display Name`
- `Type`
- `Pool`
- `Quota Mode`
- `Upstream Model`
- `Upstream Mode`
- `Flags`

No edit, delete, create, toggle, or save controls exist for model definitions.

All UI text that currently instructs operators to manage routing in the old model page must be rewritten to reflect static catalog behavior.

## Failure Strategy

Catalog loading is strict and fail-fast.

The process must exit during startup when:

- the embedded catalog cannot be read
- the embedded catalog is invalid and no external override is configured
- `app.models_file` is configured but the file does not exist
- the external catalog cannot be read
- TOML parsing fails
- any validation rule fails
- no enabled model exists

If `app.models_file` is configured, there is no fallback to the embedded catalog.

There is no fallback to:

- database values
- previous runtime state
- partially valid model subsets
- per-model merge behavior

Startup logs must state:

- catalog source: `embedded` or `external`
- external absolute path when applicable
- catalog version
- enabled model count

## Compatibility Policy

This redesign is not backward compatible for model administration.

Impacts:

- old `/admin/models/*` write API consumers will break
- the admin model management page disappears
- DB-stored model definitions become dead state

However, the redesign preserves these public behaviors:

- `/v1/models` remains available
- public model IDs remain file-defined and deterministic
- `/v1/models` `type` remains stable for existing clients where the current public contract already depends on it

## Release and Update Policy

The embedded catalog is the default release artifact.

When upstream model metadata changes but request parsing logic does not, developers may publish a standalone replacement `models.toml`.

Operators can then:

1. place that file in a deployment-accessible path
2. point `app.models_file` to it
3. restart the service

This enables catalog-only updates without requiring a binary upgrade.

## Testing Plan

### 1. `internal/modelconfig` unit tests

- load embedded catalog successfully
- load external catalog successfully
- resolve relative `app.models_file` against the active `config.toml` directory
- reject invalid TOML
- reject duplicate IDs
- reject invalid enums
- reject unknown fields
- reject invalid `upstream_model` and `upstream_mode` placement
- reject invalid `expert -> super` and `heavy -> heavy` combinations
- reject zero enabled models

### 2. registry unit tests

- `Resolve(id)` returns the correct runtime spec
- `AllEnabled()` returns enabled models only
- `ResolvePoolFloor(id)` returns configured floor
- `public_type` is derived correctly from internal `type`
- metadata for `quota_mode`, `upstream_model`, `upstream_mode`, and flags is preserved

### 3. `/v1/models` endpoint tests

- returns enabled models from the static catalog
- returns stable order
- returns derived public `type`
- keeps `grok-imagine-image-lite` exposed as `image`

### 4. routing and payload tests

- chat models use configured `upstream_mode`
- chat `force_thinking` is applied correctly
- `image_ws` uses `enable_pro` correctly
- `image_lite` resolves as runtime `image_lite` while public `type` remains `image`
- `image_lite` uses configured `upstream_mode`
- `image_edit` passes both `upstream_model` and `upstream_mode`
- `video` passes both `upstream_model` and `upstream_mode`
- chat payload must not send media `modelName`

### 5. startup tests

- default startup succeeds with embedded catalog
- configured external catalog is loaded instead of the embedded catalog
- missing external catalog fails startup
- invalid embedded catalog fails startup when no override is configured
- invalid external catalog fails startup without fallback

### 6. frontend tests

- Settings page renders the read-only model table
- no model CRUD controls remain
- no `/models` nav entry remains
- text no longer points users to the removed model management page

### 7. documentation checks

- README and README.zh reflect static catalog behavior
- deployment docs explain `app.models_file`
- upgrade docs explain catalog-only update flow

## Implementation Order

1. add embedded static catalog source
2. add `app.models_file` to config loading
3. add `internal/modelconfig`
4. rewrite registry to build from static catalog specs
5. switch `/v1/models` and routing to the new registry metadata
6. add read-only `GET /admin/models`
7. replace Settings tab content with read-only model table
8. remove `/admin/models/*` write APIs
9. remove `/models` page and related hooks/components
10. remove DB-backed model store and seed logic
11. remove obsolete tests and add static-catalog tests
12. update README, README.zh, and deployment docs

## Scope Check

This remains one coherent architectural redesign.

The work spans startup config, runtime registry wiring, HTTP APIs, frontend cleanup, and docs, but all changes serve one shift:

- model definitions become static catalog data
- model management stops being runtime CRUD

## Self-Review

Checked for:

- placeholders
- contradictions between embedded default behavior and external override behavior
- ambiguity around `upstream_mode`
- ambiguity around public versus internal model type
- hidden fallback behavior

Result:

- no placeholders remain
- catalog source priority is explicit
- `upstream_mode` is preserved where the runtime requires it
- `/v1/models` public compatibility behavior is explicit
- override failure semantics are fail-fast and non-silent

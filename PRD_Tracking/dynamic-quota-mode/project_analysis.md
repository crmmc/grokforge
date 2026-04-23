# 项目分析：Mode-Quota 刷新重设计

## Spec 来源

`docs/superpowers/specs/2026-04-21-dynamic-quota-mode-design.md`

## 变更范围总结

破坏性重构：固定 quota 字段 → mode-based quota map。涉及 30+ Go 文件、20+ 前端文件。

## 当前系统

- Token struct 包含 8 个固定 quota 字段（chat/image/video/grok43 × 当前值+初始值）
- `QuotaCategory` 枚举映射 quota_mode → 4 个固定 bucket
- 刷新调度器基于 `CoolUntil` 过期时间触发，只刷新 chat quota
- `cooling` 作为持久化状态存在
- `persist.go` 不持久化 `grok43_quota`（已知 bug）
- `ticker.go` 不检查 `Grok43Quota`（已知 bug）

## 目标系统

- Token 存储 `quotas map[string]int` + `limit_quotas map[string]int`，key = mode.id
- Catalog 新增 `[[mode]]` section，model 绑定 mode
- 乐观预扣：pick 即扣减，成功不操作，可恢复失败回补
- 刷新调度器基于 `first_used_at[token][mode]` + `observed_window[mode]` 驱动
- per-mode 并发刷新，部分成功部分失败独立处理
- `image_ws` 显式例外（`quota_sync=false`，transient cooldown）
- `cooling` 不再是持久化状态，新增 `display_status` 派生（含 `exhausted`）

## 关键依赖链

```
modelconfig (ModeSpec) → registry → store (Token struct)
    → token 核心 (quota/manager/picker/pool/persist/service)
    → refresh scheduler
    → flow 层 → admin API → config 清理 → main.go
    → 后端测试 → 前端改造
```

## 需要删除的文件

- `internal/token/category.go` — QuotaCategory 枚举，整个文件废弃
- `internal/token/category_test.go` — 对应测试（如存在）

## 需要重大重写的文件（按影响排序）

1. `internal/token/quota.go` — Consume/SyncQuota 全面重写
2. `internal/token/manager.go` — TokenSnapshot/normalize/first_used_at
3. `internal/token/refresh.go` — 调度器重写
4. `internal/store/models.go` — Token struct 8 字段 → 2 map
5. `internal/modelconfig/spec.go` + `loader.go` + `models.toml` — Catalog 重设计
6. `internal/httpapi/admin_token.go` + `admin_token_batch.go` — API 响应重写
7. `internal/httpapi/admin_stats.go` — Quota 统计重写
8. `internal/token/service.go` — 公开 API 签名全面修改
9. `internal/flow/chat.go` + `chat_types.go` — 接口 + 实现

## 已知 Bug（本次修复）

1. `persist.go` 不持久化 `grok43_quota` — 重写后自动修复
2. `ticker.go` 不检查 `Grok43Quota` — 重写后自动修复

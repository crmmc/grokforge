# Dynamic Quota Mode — TodoList

## 恢复元数据

- **Spec**: `docs/superpowers/specs/2026-04-21-dynamic-quota-mode-design.md`
- **分析**: `PRD_Tracking/dynamic-quota-mode/project_analysis.md`
- **设计**: Spec 即设计，不另建 design.md
- **当前阶段**: 任务规划
- **最后完成**: T18 — 2026-04-22

## 依赖关系

```
T1 → T2(依赖 ModeSpec)
T1+T2 → T3(catalog 测试)
T1 → T4(registry 适配)
T1+T4 → T5(Token struct + store)
T5 → T6(token 核心: quota/manager/picker/pool/persist/state)
T5+T6 → T7(service 层重写)
T7 → T8(refresh scheduler 重写)
T7 → T9(flow 层适配)
T7 → T10(admin API 适配)
T7 → T11(config 清理)
T8+T9+T10+T11 → T12(main.go 接线)
T12 → T13(make build 验证)
T13 → T14(后端测试重写 + make test)
T14 → T15(前端类型 + hooks)
T15 → T16(前端组件改造)
T16 → T17(前端构建验证)
T17 → T18(废弃代码清理 + 最终验证)
```

## 执行顺序

1. [x][H] **T1: Catalog ModeSpec — spec.go + models.toml 重设计**
   - `internal/modelconfig/spec.go` — 新增 `ModeSpec` struct（id, upstream_name, window_seconds, default_quota map[string]int）
   - `internal/modelconfig/spec.go` — `ModelSpec` 字段变更：`QuotaMode` → `Mode`，新增 `QuotaSync bool`、`CooldownSeconds int`，移除旧 QuotaMode 常量
   - `internal/modelconfig/models.toml` — 重构为 `[[mode]]` + `[[model]]` 两层结构（按 Spec 第 101-176 行）
   - `internal/modelconfig/embed.go` — 确认 embed 不需要变更
   - 验证: `go build ./internal/modelconfig/...`

2. [x][H] **T2: Catalog Loader 重写 — 加载 + 验证 [[mode]]**
   - `internal/modelconfig/loader.go` — `catalogFile` 新增 `Mode []ModeSpec`
   - `Load()` 返回 `([]ModelSpec, []ModeSpec, error)`
   - 验证规则（Spec 第 178-185 行）：quota-tracked 必须有 mode、不得有 cooldown_seconds；quota_sync=false 不得有 mode、必须有 cooldown_seconds；upstream_name 唯一；mode 引用存在；default_quota 每个 pool 有值
   - 移除旧 `validQuotaModes` 硬编码
   - 验证: `go build ./internal/modelconfig/...`

3. [x][M] **T3: Catalog 单元测试更新**
   - `internal/modelconfig/loader_test.go` — 更新所有测试用例适配 `[[mode]]` + `[[model]]` 新格式
   - 新增测试: mode upstream_name 唯一、quota_sync 互斥规则、default_quota 完整性、mode 引用验证
   - 验证: `go test -v ./internal/modelconfig/`

4. [x][M] **T4: Registry 适配 ModeSpec**
   - `internal/registry/registry.go` — `ResolvedModel` 字段：`QuotaMode` → `Mode`，新增 `QuotaSync bool`、`CooldownSeconds int`
   - `NewModelRegistry()` 签名改为接受 `(specs []ModelSpec, modes []ModeSpec)`，存储 mode 元数据供查询
   - 新增 `GetMode(id string) *ModeSpec` 查询方法
   - 新增 `AllModes() []ModeSpec` 方法
   - `internal/registry/registry_test.go` — 更新测试
   - 验证: `go test -v -race ./internal/registry/`

5. [x][H] **T5: Token 数据模型 + Store 重写**
   - `internal/store/models.go` — Token struct：移除 8 个固定 quota 字段 + `CoolUntil`，新增 `Quotas IntMap`、`LimitQuotas IntMap`（自定义类型实现 `driver.Valuer`/`sql.Scanner` JSON 序列化）
   - `internal/store/models.go` — 移除 `StatusCooling` 常量（如在此文件定义）
   - `internal/store/token_store.go` — `TokenSnapshotData` 重写为 map 字段，`UpdateTokenSnapshots()` 重写
   - DB 迁移: `cooling` → `active` 状态迁移，旧 quota 列删除（GORM AutoMigrate 处理列新增，手动处理列删除和数据迁移）
   - 验证: `go build ./internal/store/...`

6. [x][H] **T6: Token 核心包重写（quota/manager/picker/pool/persist/state）**
   - **删除** `internal/token/category.go`（整个文件，QuotaCategory 枚举废弃）
   - `internal/token/state.go` — 移除 `StatusCooling`
   - `internal/token/quota.go` — 重写：移除 `Consume()`（预扣在 picker 中完成），`SyncQuota()` 改为 `SyncModeQuota(token, mode ModeSpec) error`，`fetchRateLimits()` 接受 upstream_name 参数
   - `internal/token/manager.go` — `TokenSnapshot` 重写为 map 字段，新增 `first_used_at map[uint]map[string]time.Time` + `observed_window map[string]int` 内存状态，`normalizeTokenQuotaBaselines()` 改为 catalog-based normalization（Spec 第 229-247 行），`RestoreToken()` 改为接受 mode+value，`AddToken()` 适配，`GetDirtyTokens()` 适配
   - `internal/token/picker.go` — `Select()`/`selectWithExclude()` 参数 `QuotaCategory` → `mode string`，选取时在锁内预扣 `quotas[mode]--`，`selectHighQuotaFirst()` 用 `Quotas[mode]` 排序
   - `internal/token/pool.go` — `PickForModel()` 参数 `QuotaCategory` → `mode string`
   - `internal/token/persist.go` — `updateToken()` 列名改为 `quotas`/`limit_quotas`
   - 验证: `go build ./internal/token/...`

7. [x][H] **T7: Token Service 层重写**
   - `internal/token/service.go` — 公开 API 签名全面修改：
     - `Pick(pool, mode string)` — 内部调用 picker 预扣
     - `PickExcluding(pool, mode string, exclude)` — 同上
     - 移除 `Consume()` 公开方法（预扣在 Pick 中完成）
     - 新增 `RefundQuota(tokenID uint, mode string)` — 可恢复失败回补
     - `ReportRateLimit(id uint, mode string)` — 清零特定 mode
     - `ReportSuccess(id uint)` — 简化（不操作 quota）
     - `ReportError(id uint, mode string, recoverable bool)` — 根据 recoverable 决定回补
     - `FlushDirty()` — 适配新 snapshot
     - 新增 `RecordFirstUsed(tokenID uint, mode string)` — 记录 first_used_at
     - 新增 `GetDisplayStatus(token) string` — 派生 display_status
   - 验证: `go build ./internal/token/...`

8. [x][H] **T8: Refresh Scheduler 重写**
   - `internal/token/refresh.go` — 完全重写：
     - 基于 `first_used_at[token][mode]` + `observed_window[mode]` 驱动（Spec 第 357-393 行）
     - per-mode 独立判断 deadline
     - 收集待刷新 mode 列表，并发发起 per-mode HTTP 请求
     - 成功路径：更新 quotas + limit_quotas + observed_window，清除 first_used_at
     - 失败路径：保持当前值，不清除 first_used_at
     - per-token 并发上限 + 全局 rate limiter
   - `internal/token/ticker.go` — 评估是否保留（image_ws transient cooldown）或移除
   - 验证: `go build ./internal/token/...`

9. [x][H] **T9: Flow 层适配 mode-based quota**
   - `internal/flow/chat_types.go` — `TokenServicer` 接口重写（Pick/PickExcluding/RefundQuota/ReportRateLimit/ReportError 签名变更），`ChatRequest.QuotaMode` → `Mode`
   - `internal/flow/chat.go` — `CategoryFromQuotaMode` → 直接用 `req.Mode`，移除单独 `Consume` 调用，错误处理适配（5xx 回补、429 清零 mode、401 expire）
   - `internal/flow/image.go` — `CategoryImage` → `mode` 参数
   - `internal/flow/image_generate.go` — 同上
   - `internal/flow/image_edit.go` — 同上
   - `internal/flow/image_lite.go` — 检查是否需要适配
   - `internal/flow/video.go` — `CategoryVideo` → `mode` 参数
   - 验证: `go build ./internal/flow/...`

10. [x][H] **T10: Admin API 适配**
    - `internal/httpapi/admin_token.go` — `TokenResponse` 重写（quotas/limit_quotas/display_status），`TokenUpdateRequest` 改为 `Quotas map[string]int`，`handleUpdateToken()` 重写（验证 mode key 存在、quotas <= limit_quotas）
    - `internal/httpapi/admin_token_batch.go` — `BatchTokenRequest` 改为 `Quotas map[string]int`，import/export 逻辑重写
    - `internal/httpapi/admin_stats.go` — `PoolQuota` 改为 mode-based 聚合，`handleQuotaStats()` 重写，`resolveTokenQuotaTotals()` 重写
    - `internal/httpapi/admin_config_types.go` — 移除 `Default*Quota` 字段
    - `internal/httpapi/admin_config_update.go` — 移除 `Default*Quota` 更新处理
    - 新增 `GET /admin/models` 返回 mode-group 元数据（Spec 第 398-413 行）
    - 验证: `go build ./internal/httpapi/...`

11. [x][M] **T11: Config 清理**
    - `internal/config/config.go` — `TokenConfig` 移除 `DefaultChatQuota/DefaultImageQuota/DefaultVideoQuota/DefaultGrok43Quota/QuotaRecoveryMode`
    - `internal/config/defaults.go` — 移除对应默认值
    - `internal/config/overrides.go` — 移除 `token.default_*_quota` DB override 处理
    - `config/config.defaults.toml` — 移除对应注释行
    - 验证: `go build ./internal/config/...`

12. [x][M] **T12: main.go 接线 + 启动流程**
    - `cmd/grokforge/main.go` — 适配新 API 签名：
      - `modelconfig.Load()` 返回 `(specs, modes, error)`
      - `registry.NewModelRegistry(specs, modes)`
      - `token.NewTokenService()` 传入 modes 信息
      - Scheduler 初始化传入 catalog mode 信息
      - 启动 normalization 日志
    - 验证: `go build ./cmd/grokforge/...`

13. [x][H] **T13: make build 全量编译验证**
    - `make build` 通过
    - 检查无编译警告
    - 验证: `make build`

14. [x][H] **T14: 后端测试重写 + make test**
    - `internal/token/` — 所有测试文件重写适配 mode-based API
    - `internal/flow/` — chat_test.go, image_test.go, video_test.go 适配
    - `internal/httpapi/` — admin_token_test.go, admin_stats_test.go 适配
    - `internal/modelconfig/loader_test.go` — T3 已覆盖
    - `internal/registry/registry_test.go` — T4 已覆盖
    - 新增测试（Spec 第 466-483 行）：共享 mode quota 扣减、429 仅影响当前 mode、per-mode 并发刷新、部分成功部分失败、first_used_at 驱动刷新、image_ws 跳过 quota
    - 验证: `make test`

15. [x][H] **T15: 前端类型 + hooks 适配**
    - `web/src/types/token.ts` — Token 接口重写（quotas/limit_quotas/display_status），移除固定 quota 字段，TokenStatus 移除 cooling 新增 exhausted
    - `web/src/types/dashboard.ts` — PoolQuota 改为 mode-based
    - `web/src/types/system.ts` — 移除 default_*_quota
    - `web/src/lib/hooks/use-tokens.ts` — BatchTokenRequest 适配
    - `web/src/lib/hooks/use-dashboard.ts` — 适配新 PoolQuota
    - `web/src/lib/hooks/use-admin-models.ts` — 适配 mode-group 元数据
    - `web/src/lib/validations/token.ts` — 改为 quotas map 验证
    - `web/src/lib/validations/config.ts` — 移除 default_*_quota 验证
    - 验证: `cd web && npx tsc --noEmit`

16. [x][H] **T16: 前端组件改造**
    - `web/src/components/features/token-quota-utils.ts` — 改为动态 mode keys，从 admin/models API 获取 mode 列表
    - `web/src/components/features/token-table.tsx` — 动态 quota 列
    - `web/src/components/features/token-details.tsx` — 动态 quota 展示
    - `web/src/components/features/token-filter-tabs.tsx` — cooling → exhausted
    - `web/src/app/tokens/token-dialog.tsx` — 动态 mode quota 编辑器 + limit_quotas 只读展示
    - `web/src/app/tokens/import-dialog.tsx` — 动态 mode quota 输入
    - `web/src/app/dashboard-page.tsx` — mode-based quota 仪表盘
    - `web/src/app/settings/models-config-form.tsx` — 移除 quota 默认值 UI
    - `web/src/app/settings/model-catalog-table.tsx` — 展示 mode-group 信息
    - `web/src/lib/i18n/zh.ts` + `en.ts` — 新增 mode 相关翻译
    - 验证: `cd web && npm run build`

17. [x][M] **T17: 前端构建验证**
    - `cd web && npm run lint && npm run build`
    - `make build`（含前端嵌入）
    - 验证: `make build`

18. [x][M] **T18: 废弃代码清理 + 最终验证**
    - 确认 `internal/token/category.go` 已删除
    - 确认 `internal/token/category_test.go` 已删除（如存在）
    - 全局搜索残留：`QuotaCategory`、`CategoryChat`、`CategoryImage`、`CategoryVideo`、`CategoryGrok43`、`GetQuota(`、`SetQuota(`、`InitialChatQuota`、`InitialImageQuota`、`InitialVideoQuota`、`InitialGrok43Quota`、`Grok43Quota`、`StatusCooling`、`cooling`（在 Go 代码中）、`DefaultChatQuota`、`DefaultImageQuota`、`DefaultVideoQuota`、`DefaultGrok43Quota`、`QuotaRecoveryMode`、`quota_recovery_mode`
    - 确认无调试残留（print/fmt.Println/console.log）
    - `make build && make test`
    - 验证: 全部通过

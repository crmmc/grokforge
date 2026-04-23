# Mode-Quota 刷新重设计

## 状态

2026-04-22 交互式设计评审后，接受进入实施规划阶段。

本文档在 2026-04-21 初稿基础上原地更新。

## 概述

GrokForge 将对所有 quota-tracked 模型采用 mode-based quota 体系，同时保留 `image_ws` 作为显式的非跟踪例外。

Quota-tracked 模型类型：

- `chat`
- `image_lite`
- `image_edit`
- `video`

非跟踪例外：

- `image_ws`，标记 `quota_sync = false`

对于 quota-tracked 模型，quota 身份始终是 `mode`，而非 model type。绑定到 `mode = "auto"` 的模型与所有其他绑定到 `auto` 的 quota-tracked 模型共享同一个 quota 窗口。

每个 token 持久化两个 quota map：

- `quotas[mode]`：当前剩余配额
- `limit_quotas[mode]`：当前已知的该 token 该 mode 的配额上限

`limit_quotas` 取代旧的 `initial_quotas` 概念。缺失时从 catalog 默认值初始化，刷新成功时被上游 `totalQueries` 覆写。

## 目标

- 所有 quota-tracked 模型的配额核算改为 mode-based
- 保留 grok2api 生产验证的行为：`chat / image_lite / image_edit / video` 共享 mode 配额
- 刷新成功时以上游 `remainingQueries` 和 `totalQueries` 为权威运行时事实
- 简单显式的 fallback：上游无法提供某 mode 配额时，保持当前值等待下一轮重试
- `image_ws` 保持在 mode quota 域之外，但共享 token pool 和 auth 状态
- 移除旧的固定 quota 字段模型
- 设计足够简单，可从日志和持久化 token 状态中调试

## 非目标

- 不为 quota-tracked 模型做 per-model 持久化 quota bucket
- 不为旧 `/admin/tokens` 固定 quota 字段做静默兼容层
- 不做 `models.toml` 启动热重载
- 不为 `quota_sync = false` 模型做 quota 跟踪
- 不持久化 `cooling` token 状态
- 不在代码中推断上游别名（必须有显式 catalog 映射）

## 核心决策

### Quota 身份是 mode-based

对于 quota-tracked 模型，唯一的持久化 quota key 是 `mode.id`。

- 请求路由仍使用 `pool_floor`
- quota 资格判断使用 `quotas[mode] > 0`
- quota 扣减使用绑定的 `mode`
- quota 刷新更新绑定的 `mode`

Model type 不创建独立的 quota bucket。

### `image_ws` 是显式例外

`image_ws` 不参与 mode quota 核算。

表示方式：

- `quota_sync = false`
- 无 `mode`
- model 级别 `cooldown_seconds`

它仍使用正常的 token pool 路由和 auth 失败处理，但跳过：

- mode quota 扣减
- mode quota 刷新
- mode quota 统计
- admin token 视图中的 mode-group 渲染

### 持久化 token 状态简化

持久化 token `status` 限定为：

- `active`
- `disabled`
- `expired`

移除 `cooling` 作为持久化状态。

Admin API 返回派生的 `display_status`：

- `disabled`：持久化状态为 `disabled`
- `expired`：持久化状态为 `expired`
- `exhausted`：该 token 所有**其 pool 支持的** quota-tracked mode 均 `quotas[mode] = 0`（`default_quota[pool] = 0` 的 mode 不参与判断）
- `active`：其他情况

## Catalog 设计

### Mode schema

```toml
[[mode]]
id = "auto"
upstream_name = "auto"
window_seconds = 72000
default_quota.basic = 20
default_quota.super = 50
default_quota.heavy = 150

[[mode]]
id = "fast"
upstream_name = "fast"
window_seconds = 36000
default_quota.basic = 60
default_quota.super = 140
default_quota.heavy = 400

[[mode]]
id = "expert"
upstream_name = "expert"
window_seconds = 36000
default_quota.basic = 8
default_quota.super = 50
default_quota.heavy = 150

[[mode]]
id = "heavy"
upstream_name = "heavy"
window_seconds = 7200
default_quota.basic = 0
default_quota.super = 0
default_quota.heavy = 20

[[mode]]
id = "grok43"
upstream_name = "grok-420-computer-use-sa"
window_seconds = 36000
default_quota.basic = 0
default_quota.super = 50
default_quota.heavy = 150
```

Catalog 规则：

- `id` 是内部持久化 quota key
- `upstream_name` 是刷新时使用的唯一上游标识符
- `window_seconds` 是该 mode 的默认刷新窗口
- `default_quota` 必须为每个 pool level 提供值
- `default_quota[pool] = 0` 表示该 pool 不支持此 mode（启动 normalization 跳过，刷新调度器也跳过）

### Model schema

Quota-tracked 模型：

```toml
[[model]]
id = "grok-4.20-think"
type = "chat"
pool_floor = "basic"
mode = "auto"
enabled = true
```

非跟踪模型：

```toml
[[model]]
id = "grok-imagine"
type = "image_ws"
pool_floor = "basic"
quota_sync = false
cooldown_seconds = 300
enabled = true
```

验证规则：

- quota-tracked 模型必须定义 `mode`
- quota-tracked 模型不得定义 model 级别 `cooldown_seconds`
- `quota_sync = false` 模型不得定义 `mode`
- `quota_sync = false` 模型必须定义 model 级别 `cooldown_seconds`
- `mode.upstream_name` 必须唯一
- 所有引用的 `mode` 必须存在

## 数据模型

### Token 存储

`store.Token` 移除旧的固定 quota 列，改为存储：

- `quotas map[string]int`
- `limit_quotas map[string]int`

两个 map 均以 `mode.id` 为 key，序列化为 JSON。

含义：

- `quotas[mode]`：当前剩余配额
- `limit_quotas[mode]`：当前已知的该 token 该 mode 的配额上限

`limit_quotas` 填充规则：

- 如果 DB 中有值，使用 DB 值
- 否则从 catalog `default_quota[pool]` 初始化
- 刷新成功时被上游 `totalQueries` 覆写

硬约束：

```text
quotas[mode] <= limit_quotas[mode]
```

如果刷新报告的上限低于当前剩余配额，剩余配额立即 clamp down。

### 运行时状态（仅内存）

以下数据仅存在于内存中，重启时清除：

- `first_used_at[token_id][mode]`：该 token 该 mode 上次刷新后首次使用的时间戳
- `observed_window[mode]`：上游返回的 `windowSizeSeconds`，覆盖 catalog `window_seconds`
- `quota_sync = false` 模型的 transient cooldown 状态

不持久化任何 quota 窗口计时状态到 DB。

设计意图：`first_used_at` 不持久化是刻意选择。持久化窗口计时会增加 DB 写入频率和 schema 复杂度，而收益有限——重启是低频事件，且启动 normalization 会为 `quotas[mode] = 0` 的 token 设置 `first_used_at = now`（见下方），确保它们进入刷新调度窗口，不会死锁。

## 启动 Normalization

启动时，每个加载的 token 根据当前 catalog mode 集合进行 normalization。

规则：

- 如果 `limit_quotas` 缺少某个 catalog mode key 且 `default_quota[pool] > 0`，从 catalog `default_quota[pool]` 初始化
- 如果 `quotas` 缺少某个 catalog mode key 且 `default_quota[pool] > 0`，从 `limit_quotas[mode]` 初始化
- `default_quota[pool] = 0` 的 mode 跳过（该 pool 不支持此 mode）
- 如果存储的 mode key 在 catalog 中已不存在，从两个 map 中移除
- 如果 `quotas[mode] > limit_quotas[mode]`，clamp down 到 `limit_quotas[mode]` 并发出警告
- **如果 `quotas[mode] = 0` 且 `default_quota[pool] > 0`，设置 `first_used_at[token][mode] = now`**——确保重启后耗尽的 mode 立即进入刷新调度窗口，避免 token 死锁

Normalization 不会：

- 触发上游刷新
- 恢复已耗尽的 quota（由刷新调度器异步完成）
- 重建非零 quota mode 的 `first_used_at`（等待新流量自然重建）

## 迁移

本次重设计是破坏性 schema 迁移。

- 移除旧的固定 quota 字段（同时修复 `persist.go` 不持久化 `grok43_quota` 和 `ticker.go` 不检查 `Grok43Quota` 的已知 bug）
- 旧的固定 quota 值不迁移
- 没有 map 数据的 token 从 catalog 默认值初始化
- 旧二进制无法读取新 schema
- 现有 `status = 'cooling'` 的 token 迁移为 `active`

运维要求：

- 部署前必须备份
- 迁移后必须使用新二进制
- 部署后需要验证

设计意图：这是一次性破坏性迁移，不提供旧二进制兼容层。回滚路径 = 恢复 DB 备份 + 部署旧二进制，中间数据（quota 变更、新 token）会丢失。这是可接受的——GrokForge 是单实例部署的代理网关，不是多租户 SaaS，迁移窗口内的数据丢失影响可控。保留旧字段做兼容层会增加长期维护负担，不值得。

## 运行时行为

### Quota-tracked 请求流程

适用于 `chat / image_lite / image_edit / video`：

1. 解析 model
2. 解析 `pool_floor`
3. 解析绑定的 `mode`
4. 选取匹配 pool 资格且 `quotas[mode] > 0` 的 token，**选取时在锁内预扣** `quotas[mode]--`
5. 执行上游请求
6. 收到该 mode 和 token 的首个上游 HTTP 响应时，如果 `first_used_at[token][mode]` 不存在则记录
7. 根据上游结果决定是否回补（见下方错误处理）

### 并发扣减语义：乐观预扣

采用乐观预扣（optimistic pre-deduction）策略解决 pick 与上游请求之间的并发窗口问题。

核心规则：

- **pick 即扣减**：步骤 4 的 token 选取和 `quotas[mode]--` 在同一把锁内原子完成。其他并发请求立即看到扣减后的值，不会重复选中同一份 quota。
- **成功不操作**：上游请求成功时，quota 已在 pick 阶段扣减，无需额外操作。
- **可恢复失败回补**：上游返回 `5xx`、传输错误（超时、连接断开）等可恢复错误时，在锁内执行 `quotas[mode]++` 回补。这些错误不代表 quota 被消耗。
- **不可恢复失败不回补**：`429`（quota 耗尽）、`401`（token 过期）不回补。这些场景下 quota 视为已消耗或 token 已失效。
- **`403` 不回补**：视为 session/auth 问题，quota 视为已消耗（上游可能已计数）。

回补的判断标准是：**上游是否有机会计入了这次请求**。如果请求可能到达了上游业务层（收到了 HTTP 响应），则不回补；如果请求在传输层就失败了（连接超时、DNS 失败），则回补。

边界情况：

- `quotas[mode]` 可以被预扣到 0，此时后续请求不再选中该 token 的该 mode——这是预期行为。
- 回补后 `quotas[mode]` 可能超过回补前的值（如果期间有刷新），但不会超过 `limit_quotas[mode]`，回补时 clamp：`quotas[mode] = min(quotas[mode]+1, limit_quotas[mode])`。

### 错误处理

`429` 处理：

- 设置 `quotas[current_mode] = 0`（不回补预扣的 1，额外清零剩余）
- 不 disable token
- 不影响同一 token 的其他 mode
- 如果 `first_used_at[token][mode]` 不存在，立即设置为 `now`（确保该 mode 进入刷新调度窗口）

设计意图：429 一律清零是刻意的激进策略。上游 Grok 的 429 语义明确为"该 mode 配额耗尽"，不存在瞬时过载型 429。CDN/WAF 层的 429 在本项目中由 `cfrefresh` 模块处理，不会到达 quota 层。因此无需区分 429 子类型或做 grace period。清零后由 `first_used_at` 驱动刷新调度器在窗口到期时恢复——这是最简单且与上游语义一致的处理方式。

`401` 处理：

- 标记 token `expired`
- 不回补（token 已失效）

`403` 处理：

- 视为 session/auth 问题
- 不回补（上游可能已计数）
- 不直接清零 quota

`5xx` 和传输错误：

- **回补预扣的 quota**：`quotas[mode] = min(quotas[mode]+1, limit_quotas[mode])`
- 不清零，不影响其他 mode

### `image_ws` 请求流程

适用于 `quota_sync = false` 模型：

- token 选择忽略 mode quota
- 成功不扣减 mode quota
- 成功不创建 `first_used_at`
- 上游 auth 失败仍影响 token 状态
- 显式的 WS rate-limit 或 busy 错误为该 token + model 创建 transient 内存冷却

Transient cooldown：

- 作用域为 `token + model`
- 使用 model 的 `cooldown_seconds`
- 不影响同一 token 上的 quota-tracked 模型
- 仅按时间自动过期
- 不持久化

## 刷新调度器

### 上游 API 事实

上游 `POST /rest/rate-limits` 是 per-mode 单次调用：

- 请求体：`{"modelName": "<mode.upstream_name>"}`
- 响应体：`{"remainingQueries": N, "totalQueries": N, "windowSizeSeconds": N}`
- 每次调用只返回一个 mode 的数据
- 要获取多个 mode 的数据，需要并发发多个请求

### 触发模型

调度器按 `first_used_at` 驱动，per-mode 独立判断。

对每个 active token：

- 遍历该 token 支持的 mode（`default_quota[token.pool] > 0` 的 mode）
- 对每个 mode：
  - 如果 `first_used_at[token][mode]` 不存在 → 跳过（未使用，不刷新）
  - `deadline = first_used_at[token][mode] + observed_window[mode]`
  - 如果 `observed_window[mode]` 不存在，使用 catalog `window_seconds`
  - 如果 `now >= deadline` → 标记为待刷新

收集该 token 所有待刷新的 mode，并发发起 per-mode HTTP 请求。

并发控制：

- per-token 并发上限（如 5）
- 全局 rate limiter（计数单位是单次 HTTP 请求）

### 成功路径

对每个 per-mode 响应（HTTP 200 + 有效 body）：

- `quotas[mode] = remainingQueries`
- `limit_quotas[mode] = totalQueries`（从上游学习上限）
- `observed_window[mode] = windowSizeSeconds`（从上游学习窗口）
- 如果 `quotas[mode] > limit_quotas[mode]`，clamp down
- 清除 `first_used_at[token][mode]`（仅清除成功刷新的 mode）

### Fallback 路径

per-mode 请求失败（网络错误、非 200、404 等）：

- 保持 `quotas[mode]` 和 `limit_quotas[mode]` 当前值不变
- 不清除 `first_used_at[token][mode]`
- 下一轮调度时自然重试（因为 deadline 已过，`first_used_at` 还在）

设计意图：刷新调度器不做指数退避。调度器本身是 per-tick 扫描（间隔分钟级），且受 per-token 并发上限和全局 rate limiter 双重约束，即使上游持续不可用，请求量也是有界的（token 数 × mode 数 × tick 频率，受 rate limiter 截断）。加退避会增加状态复杂度，且延迟 quota 恢复——对代理网关来说，尽快恢复 quota 比保护上游更重要。

## Admin API 契约

### `/admin/models`

端点必须返回：

- 扁平的 model 条目
- quota-tracked 模型的 mode-group 元数据
- 非跟踪模型标记 `quota_sync = false`

每个 quota-tracked mode group 必须包含：

- `mode`
- `display_name`
- `upstream_name`
- `window_seconds`
- `default_quotas`
- `models`

### `/admin/tokens`

端点必须返回：

- `status`
- `display_status`
- `quotas`
- `limit_quotas`
- 现有 token 元数据

手动编辑规则：

- 管理员可编辑 `quotas`
- 管理员不可编辑 `limit_quotas`
- 未知 mode key 返回 `400`
- 使 `quotas[mode] > limit_quotas[mode]` 的编辑返回 `400`

设计意图：`limit_quotas` 仅由上游 `totalQueries` 或 catalog 默认值填充，管理员不可直接编辑。如果上游返回异常值（如 `totalQueries = 0`），正确的修复路径是等待下一次刷新（上游恢复后自动覆写），或通过手动触发 `/admin/tokens/{id}/refresh` 强制刷新。不提供 `limit_quotas` 的手动编辑入口，避免管理员设置与上游不一致的上限导致 quota 漂移。

### 统计和渲染

Quota 仪表盘和 token 详情视图必须：

- 仅聚合 quota-tracked mode group
- 从 quota 总计中跳过 `quota_sync = false` 模型
- 将非跟踪模型渲染为上游管理 / 未跟踪

## 日志

Quota 和刷新日志必须包含以下结构化字段（适用时）：

- `token_id`
- `model`
- `mode`
- `upstream_name`
- `pool`
- `action`
- `remaining`
- `limit`
- `http_status`
- `response_body_preview`
- `response_body_truncated`

重要事件：

- quota 扣减
- `429` 清零某个 mode
- per-mode 刷新成功（含 `observed_window` 学习）
- per-mode 刷新失败（保持当前值）
- 启动 normalization clamp
- `quota_sync = false` 模型的 transient cooldown

## 测试要求

- catalog 验证：`mode`、`upstream_name`、`quota_sync`、model 级别 `cooldown_seconds`
- `quotas` 和 `limit_quotas` 的 JSON scan/value
- 启动 normalization：add/remove/clamp 场景，`default_quota[pool] = 0` 的 mode 跳过
- 共享 mode 的 quota 扣减：`chat / image_lite / image_edit / video` 跨类型共享
- `429` 仅影响当前 mode
- per-mode 并发刷新：N 个 mode 发 N 个独立请求
- 部分 mode 成功、部分失败：成功的更新 quota，失败的保持不变
- `remainingQueries` 映射到 `quotas`
- `totalQueries` 映射到 `limit_quotas`
- `windowSizeSeconds` 映射到 `observed_window`（从上游学习）
- `first_used_at` 不存在的 mode 不参与刷新
- `429` 后设置 `first_used_at` 确保进入刷新窗口
- 刷新成功后清除对应 mode 的 `first_used_at`，失败的保留
- 重启清除内存刷新计时但保留持久化 quota 事实
- `image_ws` 跳过 quota 跟踪，仅使用 transient token+model cooldown
- admin API 暴露 `limit_quotas` 和 `display_status`

## 验证要求

实现未完成，除非满足以下全部条件：

1. `make build`
2. `make test`
3. 共享同一 mode 的 quota-tracked 模型确实共享一个 quota bucket
4. 刷新成功时同时更新 remaining 和 upper limit
5. per-mode 刷新失败时保持当前值，下一轮自然重试
6. `observed_window` 从上游 `windowSizeSeconds` 学习
7. `image_ws` 不出现在 mode quota 总计中
8. 重启服务清除刷新计时但保留持久化 quota 事实

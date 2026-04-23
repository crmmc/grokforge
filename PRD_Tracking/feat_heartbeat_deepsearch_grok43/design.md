# 设计方案：SSE Heartbeat + DeepSearch + Grok 4.3 Beta

## 📌 功能概述

从 grok2api (Python) 移植三个特性到 grokforge (Go)：

1. **SSE Heartbeat** — 流式响应中添加心跳保活，防止反代/CDN 超时断连
2. **DeepSearch 透传** — 请求体新增 `deepsearch` 参数，透传到上游 Grok payload
3. **Grok 4.3 Beta 模型** — 新增模型注册 + quota 字段 + pool 路由

---

## 🏗️ 架构设计

### 特性 1: SSE Heartbeat

**改动范围**: `internal/httpapi/openai/stream.go`

**设计**:
- 在 `streamResponse()` 的 `for event := range eventCh` 循环中，使用 `select` + `time.After` 实现超时心跳
- 初始连接时发送 2KB padding comment 强制 flush（绕过 nginx/CDN 缓冲）
- 每 15s 无数据时发送 `: ping\n\n` SSE 注释

**实现细节**:
```go
// stream.go - streamResponse() 改造
const heartbeatInterval = 15 * time.Second

// 1. 写 header 后立即发送 padding comment
w.Write([]byte(": heartbeat stream connected\n" + strings.Repeat(" ", 2048) + "\n\n"))
flusher.Flush()

// 2. 主循环改为 select
for {
    select {
    case event, ok := <-eventCh:
        if !ok { goto done }
        // 原有处理逻辑
    case <-time.After(heartbeatInterval):
        w.Write([]byte(": ping\n\n"))
        flusher.Flush()
    case <-r.Context().Done():
        return
    }
}
```

**注意**: `: ` 开头的 SSE 行是注释，所有 SSE 客户端都会忽略，不影响业务数据。

---

### 特性 2: DeepSearch 透传

**改动范围**:
- `internal/httpapi/openai/chat.go` — ChatRequest struct 加字段
- `internal/flow/chat_types.go` — flow.ChatRequest 加字段
- `internal/flow/chat_request.go` — buildXAIRequest 透传
- `internal/xai/client.go` — xai.ChatRequest 加字段
- `internal/xai/chat.go` — payload 构建时注入

**设计**:
```go
// httpapi/openai/chat.go - ChatRequest
DeepSearch string `json:"deepsearch,omitempty"` // "default" | "deeper"

// flow/chat_types.go - ChatRequest
DeepSearch string `json:"-"`

// xai/client.go - ChatRequest
DeepSearch string `json:"-"`
```

**payload 注入** (`xai/chat.go` 的 buildPayload):
```go
if req.DeepSearch != "" {
    payload["deepsearchPreset"] = req.DeepSearch
}
```

---

### 特性 3: Grok 4.3 Beta 模型

**改动范围**:
- `internal/modelconfig/models.toml` — 新增模型条目
- `internal/modelconfig/spec.go` — 新增 QuotaMode 常量
- `internal/modelconfig/loader.go` — 验证规则适配
- `internal/store/models.go` — Token struct 新增 Grok43Quota 字段
- `internal/token/category.go` — 新增 CategoryGrok43
- `internal/token/quota.go` — Consume/SyncQuota 适配

**模型注册**:
```toml
[[model]]
id = "grok-4.3-beta"
display_name = "Grok 4.3 Beta"
type = "chat"
enabled = true
pool_floor = "super"
quota_mode = "grok_4_3"
upstream_mode = "grok-420-computer-use-sa"
```

**Quota 结构**:
```go
// store/models.go - Token struct
Grok43Quota        int `json:"grok43_quota"`
InitialGrok43Quota int `gorm:"default:0" json:"-"`

// token/category.go
CategoryGrok43 QuotaCategory = "grok43"
```

**Pool 路由**: pool_floor = "super"，所以只有 ssoSuper 和 ssoHeavy 池的 token 可用。

---

## 🔄 业务流程

### Heartbeat 流程
```
Client → POST /v1/chat/completions (stream=true)
  ← ": heartbeat stream connected\n" + 2KB padding
  ← data: {"choices":[...]}  (正常数据)
  ... 15s 无数据 ...
  ← ": ping\n\n"             (心跳)
  ← data: {"choices":[...]}  (继续数据)
  ← data: [DONE]
```

### DeepSearch 流程
```
Client → POST /v1/chat/completions {"deepsearch":"deeper", ...}
  → flow.ChatRequest.DeepSearch = "deeper"
  → xai.ChatRequest.DeepSearch = "deeper"
  → upstream payload: {"deepsearchPreset":"deeper", ...}
```

### Grok 4.3 路由
```
Client → model: "grok-4.3-beta"
  → registry: pool_floor=super, quota_mode=grok_4_3, upstream_mode=grok-420-computer-use-sa
  → pool routing: [ssoSuper, ssoHeavy]
  → quota deduction: CategoryGrok43
  → upstream: modeId="grok-420-computer-use-sa"
```

---

## 🎨 设计原则

1. **最小改动** — 每个特性改动控制在 3-5 个文件
2. **继承现有模式** — Heartbeat 复用 SSEWriter，DeepSearch 复用现有透传链路，Grok 4.3 复用 QuotaCategory 模式
3. **向后兼容** — 所有新字段都是 optional，不影响现有请求

---

## 🚨 风险分析

| 风险 | 影响 | 缓解 |
|------|------|------|
| Heartbeat 的 time.After 每次循环创建新 timer | 轻微 GC 压力 | 改用 time.NewTimer + Reset |
| DeepSearch 参数值校验 | 无效值传到上游 | 只允许 "default"/"deeper"，其他忽略 |
| Grok 4.3 quota 字段需要 DB migration | 已有 token 无此字段 | GORM AutoMigrate 自动加列，默认 0 |

---

## 📏 验收标准

1. ✅ SSE 流式响应首行包含 `: heartbeat stream connected` + 2KB padding
2. ✅ 15s 无数据时自动发送 `: ping\n\n`
3. ✅ `deepsearch` 参数正确透传到上游 payload 的 `deepsearchPreset` 字段
4. ✅ `grok-4.3-beta` 出现在 `/v1/models` 列表中
5. ✅ `grok-4.3-beta` 请求正确路由到 super/heavy 池
6. ✅ `grok-4.3-beta` 使用独立的 grok43 quota 计费
7. ✅ `make build` 编译通过
8. ✅ `make test` 测试通过

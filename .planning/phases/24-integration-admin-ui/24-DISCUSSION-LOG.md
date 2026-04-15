# Phase 24: Integration & Admin UI - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 24-integration-admin-ui
**Areas discussed:** 主链路接入策略, Admin 模型 CRUD API, 前端模型管理页面, mapModel 兼容与 fallback

---

## 主链路接入策略

### 硬编码处理方式

| Option | Description | Selected |
|--------|-------------|----------|
| Registry 优先 + 硬编码 fallback | mapModel() 先查 Registry，查不到再 fallback 到硬编码 map | |
| 一步到位，删除硬编码 | 直接删除 modelMappings/imageModels/videoModels，全部走 Registry | ✓ |
| 保留但不调用 | 保留硬编码作为参考但注释为 deprecated | |

**User's choice:** 一步到位，删除硬编码
**Notes:** 种子数据已在 Phase 22 导入，不需要过渡期

### 类型判断改造

| Option | Description | Selected |
|--------|-------------|----------|
| Registry Resolve + family.Type | 用 Resolve 查 family.Type 判断路由类型 | ✓ |
| Registry 便捷方法 | 新增 IsImageModel/IsVideoModel 封装方法 | |

**User's choice:** Registry Resolve + family.Type

---

## Admin 模型 CRUD API

### 端点结构

| Option | Description | Selected |
|--------|-------------|----------|
| 分离端点 | /admin/models/families 和 /admin/models/modes 分开 | ✓ |
| 嵌套结构端点 | 单一 /admin/models，family+modes 嵌套返回 | |

**User's choice:** 分离端点

### Registry 刷新方式

| Option | Description | Selected |
|--------|-------------|----------|
| 同步刷新 | CRUD 后同步调用 registry.Refresh() | ✓ |
| 异步事件刷新 | 发事件通知，异步刷新 | |

**User's choice:** 同步刷新

---

## 前端模型管理页面

### 布局风格

| Option | Description | Selected |
|--------|-------------|----------|
| Master-Detail 布局 | 左侧 family 列表，右侧 modes 详情 | ✓ |
| 可展开表格 | 单一表格，family 行可展开显示 modes | |
| You decide | Claude 自行决定 | |

**User's choice:** Master-Detail 布局

### 编辑交互

| Option | Description | Selected |
|--------|-------------|----------|
| Dialog 编辑 | 弹窗编辑，与 token-dialog 模式一致 | ✓ |
| 行内编辑 | 直接在表格内编辑 | |

**User's choice:** Dialog 编辑

---

## mapModel 兼容与 fallback

### 未知模型处理

| Option | Description | Selected |
|--------|-------------|----------|
| 严格模式，查不到报错 | Registry 查不到直接 400，不做 passthrough | ✓ |
| 宽松模式，grok- 前缀 passthrough | 保留当前 passthrough 行为 | |

**User's choice:** 严格模式

---

## Claude's Discretion

- Admin API 的具体 DTO 结构
- 前端组件拆分和样式细节
- 错误提示文案
- 测试覆盖范围

## Deferred Ideas

None

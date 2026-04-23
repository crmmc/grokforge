# TodoList - SSE Heartbeat + DeepSearch + Grok 4.3 Beta

## 任务列表

1. [x][S] SSE Heartbeat — 改造 stream.go 的流式循环，添加心跳保活机制
2. [x][S] DeepSearch 透传 — 在请求链路中添加 deepsearch 字段并透传到上游 payload
3. [x][S] Grok 4.3 Beta 模型注册 — models.toml 新增条目 + spec 常量 + loader 验证适配
4. [x][S] Grok 4.3 Quota 支持 — Token struct 新增字段 + category 扩展 + quota 消费逻辑
5. [x][S] 编译验证 — make build 通过
6. [x][S] 测试验证 — make test 通过

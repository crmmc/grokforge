# Requirements: GrokForge

**Defined:** 2026-04-14
**Core Value:** Chat 主链路稳定可用

## v1.4 Requirements

Requirements for v1.4 Model Management & Three-Pool. Each maps to roadmap phases.

### Model Schema

- [ ] **MODEL-01**: model_family 表 + GORM model + store CRUD
- [ ] **MODEL-02**: model_mode 表 + GORM model + store CRUD，外键关联 family
- [ ] **MODEL-03**: 派生请求名写入时冲突校验

### Seed & Registry

- [ ] **SEED-01**: models.seed.toml 解析 + 首次启动导入 + embed.FS 内嵌默认
- [ ] **REG-01**: ModelRegistry 内存快照 (byRequestName + enabledByType)，启动加载 + 变更刷新

### Pool

- [ ] **POOL-01**: 新增 PoolHeavy = ssoHeavy，token 表支持 heavy 池
- [ ] **POOL-02**: pool_floor 硬门槛路由，>= 匹配，不做级联降级
- [ ] **POOL-03**: 废弃 basic_models / super_models / preferred_pool，统一由 ModelRegistry 驱动

### Integration

- [x] **INTG-01**: xai/chat.go modelMappings + chat_routing.go 类型判断改为读 ModelRegistry
- [x] **INTG-02**: /v1/models 从 ModelRegistry 构建模型列表
- [x] **INTG-03**: Admin 模型 CRUD API + 前端模型管理页面 + token 管理 heavy 池选项

## Future Requirements

### Quota Runtime

- **QUOTA-01**: quota_default / quota_override 运行时接入
- **QUOTA-02**: 上游 /rest/rate-limits 配额实时同步

### Compatibility

- **COMPAT-01**: 旧模型名 alias 兼容

## Out of Scope

| Feature | Reason |
|---------|--------|
| quota 运行时接入 | v1.4 预留字段，独立后续任务 |
| prefer_best 级联降级 | pool_floor 硬门槛足够 |
| 旧模型名 alias | 当前版本不做，后续按需追加 |
| 多协议平台化 | 只聚焦 OpenAI 兼容主路径 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| MODEL-01 | Phase 21 | Pending |
| MODEL-02 | Phase 21 | Pending |
| MODEL-03 | Phase 21 | Pending |
| SEED-01 | Phase 22 | Pending |
| REG-01 | Phase 22 | Pending |
| POOL-01 | Phase 23 | Pending |
| POOL-02 | Phase 23 | Pending |
| POOL-03 | Phase 23 | Pending |
| INTG-01 | Phase 24 | Complete |
| INTG-02 | Phase 24 | Complete |
| INTG-03 | Phase 24 | Complete |

**Coverage:**
- v1.4 requirements: 11 total
- Mapped to phases: 11/11 ✓
- Unmapped: 0

---
*Requirements defined: 2026-04-14*
*Last updated: 2026-04-14 after roadmap creation*

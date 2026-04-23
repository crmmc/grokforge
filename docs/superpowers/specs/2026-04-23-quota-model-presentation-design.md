# 配额展示从 Mode 转向 Model 的交互设计

## 状态

2026-04-23 视觉方案评审通过，等待落地实现。

本设计是对 [2026-04-21-dynamic-quota-mode-design.md](/Users/easyops/project_ai/grokforge/docs/superpowers/specs/2026-04-21-dynamic-quota-mode-design.md) 的用户界面补充，不改变底层 quota 数据模型：

- 后端 quota 身份仍然是 `mode`
- 用户界面不再把 `mode` 作为主要展示对象

## 背景

当前配额系统在持久化、调度和统计上都以 `mode` 为真实身份，这是正确的底层设计；但在管理界面中直接向用户展示 `auto / fast / expert / heavy / grok43` 会产生两个问题：

- 普通用户无法理解这些内部术语
- 如果直接把 mode 改名成模型名，又容易误导用户以为每个模型各有一份独立额度

已有前端数据足够建立“用户可读展示层”：

- `/admin/models` 可提供 `mode -> models[]` 的映射
- `/admin/stats/quota` 可提供各 pool 下 `mode_quotas`
- token 实体自身已包含 `quotas` 与 `limit_quotas`

因此本次设计采用“展示层重构”，而不是重做 quota 存储。

## 目标

- Dashboard 和 Token 管理界面对用户展示 `model` 视角，而不是 raw `mode`
- 明确表达“多个模型共享同一份额度”，避免 per-model quota 的错误心智
- Token 列表未展开状态只保留一条平均健康度进度条，减少横向占用
- Dashboard 不再把 pool 内不同 mode 简单加总后的百分比作为主要健康指标
- 移除现有用户界面中直接暴露 raw mode 的旧实现

## 非目标

- 不改变底层 quota 的持久化结构，仍然使用 `quotas[mode]` 与 `limit_quotas[mode]`
- 不为每个 model 创建独立 quota bucket
- 不新增后端 quota 专用 API，只为展示去复制一层模式数据
- 不在用户主界面保留 raw mode 文案作为主标题

## 核心术语

### Internal Mode

内部 quota bucket 身份，例如 `auto`、`fast`、`expert`。

只用于：

- 持久化
- 路由判断
- 刷新与扣减
- 前端展示层内部关联

### Shared Model Group

用户可见的配额展示对象。本质上是“一个 mode 对应的一组可调用模型”。

它不是新的后端实体，而是前端根据：

- `mode`
- 关联模型列表
- 该 mode 在当前 pool / token 下的 remaining 与 total

组合出来的展示层对象。

## 设计决策

### 1. 用户看到的是 Shared Model Group，而不是 mode

所有面向用户的配额展示区域统一改为 Shared Model Group。

每个 Group 的展示包含：

- `title`：用户熟悉的模型名组合
- `models[]`：该共享组影响的所有模型 chips
- `remaining / total`
- `percent`
- `status tone`：健康、紧张、危险

同一个共享组只显示一条进度条。

这条进度条表达的是“这一组模型共用的额度”，不是某一个模型独享的额度。

### 2. Group 标题规则必须固定，不能临时拼接

为避免不同页面显示不同标题，Group 标题规则固定为：

1. 仅考虑 `quota_sync = true` 的 enabled models
2. 按以下优先级排序：
   - `public_type = chat`
   - `public_type = image`
   - `public_type = image_edit`
   - `public_type = video`
   - 其余类型按 `display_name` 排序
3. 取排序后的前两个 `display_name` 作为主标题
4. 若组内模型数大于 2，则标题追加“等共享”

示例：

- `auto` 组标题：`Grok 4.20 / Grok 4.20 Think 等共享`
- `fast` 组标题：`Grok 4.20 Fast / Grok Imagine Image Lite`
- `grok43` 组标题：`Grok 4.3 Beta`

完整模型列表始终在 chips 区域展示，不依赖 tooltip。

### 3. Dashboard 改为“按 pool 分区，按共享组看健康度”

Dashboard 配额卡不再使用当前实现中的 pool 总剩余 / 总量作为主视觉。

原因：

- 该数值把不同共享组简单加总
- 不同 mode 的额度窗口与业务含义不同
- 总百分比会掩盖真正紧张的组

新的 Dashboard 配额区域规则：

- 先按 pool 分组
- 每个 pool 下渲染多个 Shared Model Group 卡片
- 每张卡片显示一条共享额度进度条
- 组内显示受影响模型 chips
- 组排序按 `percent` 升序，优先暴露最紧张的共享组

Pool 级信息只保留辅助信息，例如：

- pool 名称
- token count / active count

不再把“pool 总 quota 百分比”作为主指标。

### 4. Token 列表未展开状态只显示一条平均健康度

Token 列表行内配额区只保留：

- 一条平均健康度进度条
- 一个百分比数字

该百分比定义为：

- 当前 token 下所有“支持且 total > 0”的 Shared Model Group 的 `percent` 简单平均值

明确采用简单平均，而不是加权平均，原因是：

- 用户希望看到的是概览健康度，而不是数学上的总量占比
- 加权平均会让大 quota 组压掉小 quota 组的风险信号

无 Group 时显示 `0%`，不显示 raw mode 明细。

### 5. Token 展开详情按 Shared Model Group 展示

Token 展开区域不再逐条显示 raw mode 卡片，而是显示 Shared Model Group 卡片。

每张卡片包含：

- Group 标题
- `remaining / total`
- 一条进度条
- 组内模型 chips

如某组只有 1 个模型，也仍然视为 Shared Model Group，而不是退回 raw mode 展示。

### 6. Token 编辑弹窗按 Shared Model Group 输入

Token 编辑弹窗中的 quota 输入区不再直接以 `auto / fast / expert ...` 为 label。

改为：

- 每个输入框对应一个 Shared Model Group
- label 使用 Group 标题
- 次级说明展示该组影响的模型 chips
- 输入值本质仍写回对应的 `quotas[mode]`

实现约束：

- 表单层不引入新的 per-model quota 字段
- `mode` 只作为隐藏的关联 key 存在于表单结构内部

### 7. Raw mode 不允许静默回流到用户界面

这是本次设计的硬约束。

如果前端无法从模型目录构建展示层，例如：

- `/admin/models` 加载失败
- `mode_quotas` 中出现 catalog 不存在的 mode
- 某个 mode 没有任何 `quota_sync = true` 模型

系统不得静默回退为直接显示 raw mode。

要求：

- 在对应界面显示明确的错误或警告文案
- 在 console 输出 warning
- 阻止展示一组“看不懂但看起来像正常工作”的 mode 标签

允许的异常展示形式：

- `无法生成人类可读的配额视图`
- `发现未映射的共享配额组: auto`

raw mode 只能出现在错误说明的次级信息里，不能作为正常 UI 主标题。

## 前端实现方案

### 数据来源

沿用现有接口：

- `/admin/models`
- `/admin/stats/quota`
- `/admin/tokens`

不新增后端 API。

### 新的展示层抽象

新增统一的前端展示层转换模块，负责把：

- model catalog
- dashboard quota stats
- token quota maps

转换成一致的 Shared Model Group 结构。

建议抽象：

- `buildQuotaCatalogPresentation(...)`
- `buildPoolQuotaGroups(...)`
- `buildTokenQuotaGroups(...)`
- `summarizeTokenQuotaGroups(...)`

这些 util 负责：

- mode 到 group 的映射
- group 标题生成
- 组排序
- 平均健康度计算
- 异常显式暴露

### 需要替换的现有前端逻辑

以下旧实现必须移除或被新抽象完全接管：

- `dashboard-page.tsx` 中直接遍历 `pool.mode_quotas`
- `dashboard-page.tsx` 中 `modeLabel(mode, t)` 的主展示逻辑
- `token-quota-utils.ts` 中以 raw mode 直接生成 `label`
- `token-details.tsx` 中逐条 raw mode 卡片展示
- `token-dialog.tsx` 中直接把 raw mode 作为输入框 label

本次改动后，用户可见层不得再出现这些旧路径的 raw mode 展示。

## 文案与视觉约束

### 主文案

用户界面主文案统一使用：

- `模型额度`
- `共享额度`
- `平均健康度`
- `这些模型共享同一份额度`

不使用：

- `mode quota`
- `auto mode`
- `fast mode`

### 信息层级

信息层级固定为：

1. Group 标题
2. 共享额度进度
3. 受影响模型 chips
4. 次级说明

internal key 不进入主层级。

### Dashboard 告警优先级

按 `percent` 着色：

- `> 50%`：健康
- `> 20% && <= 50%`：紧张
- `<= 20%`：危险

并且低百分比共享组排在前面。

## 边界条件

### Pool 不支持某 mode

若该 pool 下该 mode 的 `total = 0` 或 catalog `default_quota[pool] = 0`，则该组不参与：

- Dashboard 展示
- Token 平均健康度
- Token 编辑输入

### Group 无模型

若某 mode 没有关联到任何 `quota_sync = true` 模型：

- 视为数据一致性错误
- 在 UI 显示警告
- 在 console 输出 warning
- 不以正常 group 渲染

### Catalog 与 token/quota 数据不一致

若 token 或 quota stats 中出现未知 mode：

- 视为显式异常
- 直接暴露警告，不做静默兼容

## 测试要求

### 前端自动化

至少覆盖：

- Group 标题生成规则
- 多模型共享组的排序与 chips 输出
- 平均健康度计算仅基于 `total > 0` 的组
- catalog 缺失时显式报错，不回退 raw mode
- 未知 mode 时显式报错，不回退 raw mode

### 手工验证

需要使用 Chrome DevTools 检查：

- Dashboard 配额区
- Token 列表未展开行
- Token 展开详情
- Token 编辑弹窗

验证点：

- 用户界面主层级不再出现 raw mode
- 未展开状态只有一条平均进度条
- 展开与编辑都清晰表达共享关系
- 异常状态时有明确警告，而不是静默显示旧字段

## 实施范围结论

本次设计预计只需要前端改动，不要求新增后端 API。

如果实现中发现现有 `/admin/models` 无法稳定支撑共享组映射，再单独补充后端接口设计；在此之前，默认不扩大范围。

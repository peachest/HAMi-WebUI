# Ascend 切分型号：卡查询失败时跳过 core/memory 指标而非回退

切分型号（910B/A3 等）的 `container_npu_*` 指标返回整卡值，回退路径会产生 >100% 利用率——用户报告的核心问题。因此当比例拆分不可行时（卡级查询失败或无容器分配），跳过 core/memory 指标而非回退到 `taskCoreUsed`/`taskMemoryUsed`。不切分型号（910C）容器级指标本身准确，仍走回退路径。

**Considered Options**

- 回退到 `taskCoreUsed`/`taskMemoryUsed`（原规格方案）——产生整卡值归因，利用率 >100%
- 卡查询失败时上报 0——无法区分"卡空闲"与"查询失败"，且与卡查询成功但返回 0 的正常情况混淆
- 回退但 cap 到 100%——掩盖真实数据，隐藏数据源异常
- **跳过 core/memory 指标**（采纳）——指标缺失比 >100% 误值更易被监控系统识别为异常，`log.Warnf` 已记录失败原因

**Consequences**

- 分配量指标（`HamiContainerVgpuAllocated` 等）不受影响，仍正常上报
- 910C 不受影响，仍走 `taskCoreUsed`/`taskMemoryUsed` 回退路径
- 原规格条目 "ascendCardQueriesOK guard: fall back to taskCoreUsed/MemoryUsed if card queries fail" 被本决策推翻

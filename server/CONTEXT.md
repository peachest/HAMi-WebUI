# HAMi-WebUI Server

GPU 虚拟化管理平台的后端服务，负责采集多厂商 GPU/NPU 设备与容器级监控指标并暴露给 Prometheus。

## Language

### Ascend 指标采集

**Card-level metric (卡级指标)**:
`npu_chip_info_*` 系列 PromQL 指标，反映整张物理卡的状态（利用率、HBM 已用/总量）。`npu_chip_info_hbm_used_memory` 的单位是 MB。
_Avoid_: device-level metric

**Container-level metric (容器级指标)**:
`vnpu_pod_*` / `container_npu_*` 系列 PromQL 指标，反映单个容器的资源使用。对切分型号，npu-exporter 尚不支持 vnpu，`container_npu_*` 会返回整卡值而非容器级值。
_Avoid_: task-level metric, pod-level metric

**Proportional split (比例拆分)**:
当容器级指标不可用时，将卡级指标按各容器的 Usedmem 分配比例分摊到各容器的技术。
_Avoid_: ratio split, weighted split

**Split model (切分型号)**:
使用 vNPU template 切分的 Ascend 型号（910B、A3 等），多容器共享一张物理卡。容器级指标需通过比例拆分估算。
_Avoid_: vnpu model

**Non-split model (不切分型号)**:
不使用 vNPU 切分的 Ascend 型号（910C），容器独占整卡。容器级指标本身准确，无需比例拆分。
_Avoid_: exclusive model

**Whole-card value (整卡值)**:
切分型号下 `container_npu_*` 指标错误返回的值——将整张卡的使用量归因到单个容器，导致利用率超过 100%。这是用户报告的核心问题。
_Avoid_: card-level fallback value

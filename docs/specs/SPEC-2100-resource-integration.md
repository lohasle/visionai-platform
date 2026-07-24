# SPEC-2100 资源中心、集成服务与系统审计

状态：已完成
Linear：LOH-16

## 范围

覆盖 FR-RES-001..006、FR-INT-001..006、FR-SYS-001..005：GPU 节点/队列/配额/用量/存储，外部实例/Secret Ref/连接测试/兼容矩阵/同步异常/修复升级，以及 AI 字典、策略、审计和配置变更。

## 规则

- 不自研 GPU 调度器；ComputeQueue 映射 ClearML Queue。
- CompatibilityRule 是升级门禁；Binding、SYNC_WARNING、ORPHANED、MAPPING_MISSING、AUTH_FAILED 可诊断与重放。
- Secret 只保存引用且日志脱敏；关键配置变更保存 Before/After 摘要。

## 验收

- GPU 心跳、显存/驱动/CUDA/标签、队列等待和项目配额可见并影响训练提交。
- 单个 Provider 故障不影响历史查询和其他 Provider；恢复后自动对账。
- 审计可按用户、项目、动作、资源、结果和时间检索导出。

## 实现与证据

- Windows GPU 节点心跳记录 RTX 4090、驱动/CUDA、显存和标签；队列映射 ClearML，项目并发/GPU 时/存储配额生效。
- CVAT、FiftyOne、MinIO、ClearML、推理实例均支持 Secret Ref、连接测试、兼容规则和同步故障重放。
- 关键业务写操作产生 Before/After 审计；审计支持用户、项目、动作、资源、结果、时间过滤和 CSV 导出。

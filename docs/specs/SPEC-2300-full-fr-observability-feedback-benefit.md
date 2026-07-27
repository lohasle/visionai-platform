# SPEC-2300 全量 FR 验收、可观测性与反馈收益闭环

状态：已完成
Linear：LOH-27
设计基线：V1.3 全部 134 条 FR 中未由 SPEC-2280/2290 覆盖的非 VERIFIED 项

## 目标

消除需求追踪矩阵中的全部 `AUDIT_PENDING`、`PARTIAL` 和 `MISSING`，让每条需求都有
实现、自动化检查、浏览器证据或故障注入证据，而不是仅凭代码存在判定完成。

## 范围

- 工作台、项目、数据导入/元数据/去重/质量/引用/回收站逐项验收。
- 标注状态、审核、返工、操作差分与单位耗时；真实模型预标注及类别/场景效率比较。
- 数据集来源和全血缘、模板高级配置、兼容矩阵、LocalDocker 开关和清理、实验筛选。
- 场景/设备/时间/Embedding 真实切片，模型及生产性能比较。
- GPU/CPU/内存/磁盘/网络、QPS/P50/P95/P99/队列、漂移基线和时间窗口。
- 告警静默、通知、确认、处置和恢复；反馈隐私审批、相似去重和收益比较。
- GPU 设备占用、MinIO 容量/对象/失败/生命周期和真实 GPU 小时。
- 集成连接/异常处置、系统字典及策略审计。

## 强制闭环

从真实生产困难样本创建返标任务和新 DatasetVersion，执行再训练、再评估、再审批、
再部署，并展示基线与新版本在相同切片上的质量和生产指标差异。

## 验收

1. 需求矩阵为 `134/134 VERIFIED`，不存在其他状态。
2. 全量 Go/Python/前端/Compose/Swagger/路由/浏览器 E2E/PR CI 通过。
3. 所有截图无乱码，所有错误提示、权限边界和局域网/本机地址通过人工浏览器复核。
4. 机器可读证据能从 FR 追溯到测试、页面、运行对象、checksum 和截图。

## 最终验收结果

- 需求追踪矩阵：134 / 134 `VERIFIED`，0 个其他状态。
- 公开数据全链路：`docs/evidence/visionai-final-acceptance-20260727.json`
  （原始运行输出：`runtime/coco128-acceptance-20260727-195218.json`）。
- 最终对象：Project #33、128 图、929 框、CVAT Task #35、DatasetVersion #26、
  TrainingRun #26、EvaluationRun #25、ModelVersion #16、Approval #18、
  Deployment #14、FeedbackBatch #13、返标 Task #36。
- GPU：NVIDIA GeForce RTX 3060 12 GB，CUDA 12.6；真实 Faster R-CNN 训练、
  评估、部署和推理通过。
- 标签：业务、场景、来源三类治理标签各应用于 128 个资产；80 类 COCO
  Ontology 发布并绑定 CVAT。
- 数据血缘：DatasetVersion #26 同时关联训练、评估、模型和部署四类引用。
- 供应链：`scripts/e2e-supply-chain.ps1` 最终回归通过，包含复制、升级、回退、
  审批失效和双人顺序审批。
- 测试：Go 全量、前端类型检查、生产构建、Compose、Swagger、浏览器 13 路由、
  三实例连接测试全部通过。
- 浏览器证据：`docs/evidence/visionai-final-*.jpg`，无乱码或加载错误。

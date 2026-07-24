# SPEC-2000 生产反馈与再训练闭环

状态：已完成
Linear：LOH-15

## 范围

覆盖 FR-FB-001..007：FeedbackPolicy、随机/低置信/空结果/人工/错误/漂移采样、脱敏与保留、哈希/相似去重、批次审核、返标任务、闭环追踪和收益评估。

## 状态

COLLECTING→READY→REVIEWING→ACCEPTED→ANNOTATING→COMPLETED；支持 REJECTED/EXPIRED/FAILED。

## 验收

- E2E-CV-10：错误样本形成批次，接受样本进入真实 CVAT 任务并保留 DeploymentRevision 来源。
- 可追踪 FeedbackBatch→AnnotationRevision→DatasetVersion→ModelVersion→DeploymentRevision。
- 默认不全量保存生产请求；脱敏、保留期、每日上限和敏感项目审批生效。

## 实现与证据

- 策略支持低置信度、空结果、错误、每日上限、保留期和脱敏元数据，默认不全量保存。
- 在线低置信度请求被采集并按 SHA-256 去重；Batch 审核后创建真实 CVAT Task。
- Batch 详情可反查 DeploymentRevision、ModelVersion、Trace、AnnotationTask，并继续追踪数据集、模型和新部署。

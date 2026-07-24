# SPEC-1000 VisionAI 产品总纲与验收矩阵

状态：已完成
Linear：LOH-5
设计基线：VisionAI 产品设计说明书 V1.3（2026-07-21）

## 目标

在 Nimbus Framework Go 上交付企业私有化、多租户的计算机视觉 AI 生命周期控制面。MVP 必须贯通项目、图片数据、CVAT 标注、不可变数据集、LocalDocker/ClearML 训练、FiftyOne 评估、模型注册审批、测试部署、监控、反馈回流及资源与集成运维。

## 技术决策

- 保留 Nimbus 的 Go 模块化单体、Vue 运营后台、MySQL 8.4、GORM 幂等迁移、JWT、租户、RBAC、菜单和审计。
- 新增 `internal/modules/visionai` 领域模块；耗时任务和外部系统调用通过独立 Orchestrator/Runner 进程执行。
- 原设计中的 Java/Spring、PostgreSQL、Flyway 由用户指定的 Nimbus 脚手架覆盖，详见 ADR-013。
- 外部系统仅通过 Provider SPI 接入，平台数据库是治理事实源，外部系统 ID 只保存在 Binding。
- DatasetVersion、AnnotationRevision、ModelVersion、DeploymentRevision 均不可变。
- 所有长任务使用 PlatformJob、Outbox/Inbox、幂等键、补偿和 Reconciler。

## SPEC 与 Linear 映射

| SPEC | Linear | 范围 | 依赖 |
| --- | --- | --- | --- |
| 1100 | LOH-6 | 工程底座、工作台、Job、事件、审计 | Nimbus |
| 1200 | LOH-7 | 项目、成员、角色、隔离 | 1100 |
| 1300 | LOH-8 | 数据资产、上传、S3 | 1100、1200 |
| 1400 | LOH-9 | CVAT、预标注、AnnotationRevision | 1300 |
| 1500 | LOH-10 | DatasetVersion、Manifest、冻结 | 1300、1400 |
| 1600 | LOH-11 | Template、TrainingRun、Provider、实验 | 1500 |
| 1700 | LOH-12 | FiftyOne、EvaluationRun、切片 | 1600 |
| 1800 | LOH-13 | ModelVersion、供应链、审批 | 1700 |
| 1900 | LOH-14 | DeploymentRevision、推理、监控 | 1800 |
| 2000 | LOH-15 | FeedbackBatch、返标、再训练 | 1900、1400 |
| 2100 | LOH-16 | 资源、配额、集成、对账、系统策略 | 1100 |
| 2200 | LOH-17 | Compose、跨平台 E2E、文档、发布 | 全部 |

## 功能覆盖矩阵

- 工作台：FR-WB-001..007 → SPEC-1100。
- 项目：FR-PRJ-001..006 → SPEC-1200。
- 数据：FR-DATA-001..008 → SPEC-1300。
- 标注：FR-ANN-001..009；预标注：FR-PRE-001..006 → SPEC-1400。
- 数据集：FR-DS-001..008 → SPEC-1500。
- 模板：FR-TPL-001..006；训练：FR-TRN-001..009；本地执行：FR-LOCAL-001..008；实验：FR-EXP-001..006 → SPEC-1600。
- 评估：FR-EVAL-001..003、005..009 → SPEC-1700。
- 模型：FR-MDL-001..008；审批：FR-APR-001..006 → SPEC-1800。
- 部署：FR-DEP-001..008；监控：FR-MON-001..007 → SPEC-1900。
- 反馈：FR-FB-001..007 → SPEC-2000。
- 资源：FR-RES-001..006；集成：FR-INT-001..006；系统：FR-SYS-001..005 → SPEC-2100。
- API、事件、数据库、S3 路径、保留、安全、性能、可用性、UI、Compose、Windows/Linux、备份恢复和附录验收 → SPEC-1100、2100、2200。

## 总体验收

1. `docker compose up -d` 可一键启动默认演示栈，所有必需服务通过 healthcheck。
2. 设计文档附录 D、H 的适用 E2E 场景有自动化结果、截图和可复验命令。
3. 普通用户从统一门户完成目标检测黄金路径，数据与任务不是页面假数据。
4. 关键写操作经过后端权限校验、幂等、审计并产生 Trace。
5. 代码、OpenAPI、数据库模型、Provider 契约、兼容矩阵、产品手册和 GitHub Pages 一致。
6. 仓库发布到用户 GitHub；CI 通过；文档站点可访问。

## 完成证据

- 2026-07-24：全部子 SPEC（1100–2200）完成并通过对应自动化与运行验收。
- 完整 Compose 栈在 Windows Docker Desktop 上启动并通过健康检查，Linux 与 GPU 覆盖配置通过解析校验。
- 黄金路径 E2E、真实 CVAT/FiftyOne/LocalDocker 训练、在线推理、回滚和反馈闭环均已验证。
- 源码、CI、产品手册、验收报告、截图和 GitHub Pages 发布工作流已纳入交付仓库。

## 非目标

MVP 不自研标注画布、训练算法、GPU 调度器或推理引擎；不实现 3D、多摄像头融合、生产级 Kubernetes HA；不以空壳页面冒充专业执行能力。

# SPEC-1900 部署、在线推理与监控

状态：已完成
Linear：LOH-14

## 范围

覆盖 FR-DEP-001..008、FR-MON-001..007：Deployment/Revision/Endpoint、Docker/FastAPI InferenceProvider、资格校验、健康检查、发布、停止/重启、回滚、在线图片/视频测试、基础设施/服务/模型指标、告警与样本采集。

## 状态

DRAFT→DEPLOYING→HEALTH_CHECKING→RUNNING；支持 DEGRADED/FAILED/STOPPED/ROLLING_BACK。每次配置变化创建不可变 Revision。

## 验收

- E2E-CV-09：Endpoint 可用，示例推理含 Trace、ModelVersion 和 Revision；回滚切换历史成功 Revision。
- 生产部署仅接受 APPROVED ModelVersion；测试放宽必须显式配置。
- QPS、错误率、P50/P95/P99、资源、类别/置信度/空结果指标和告警处理记录可见。

## 实现与证据

- FastAPI InferenceProvider 实际加载发布配置，并提供图片预测和真实视频帧解码接口。
- 生产部署仅接受 APPROVED 模型；创建、配置修订、停止、重启、告警和不可变回滚均已实现。
- 实际推理返回 Trace ID/ModelVersion/Revision；记录 QPS、错误率、P50/P95/P99、空结果率和平均置信度，告警可确认。

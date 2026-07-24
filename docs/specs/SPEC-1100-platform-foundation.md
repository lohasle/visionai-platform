# SPEC-1100 工程底座、工作台与跨模块基础能力

状态：已完成
Linear：LOH-6

## 范围

- 新增 VisionAI 模块、菜单、权限和统一 `/admin-api/ai-platform` API。
- PlatformJob：PENDING→QUEUED→RUNNING→SUCCEEDED/FAILED/CANCELLED，支持进度、Attempt、取消、重试、恢复与 SSE。
- Outbox/Inbox、领域事件、幂等键、死信与 Reconciler 基础设施。
- 审计只追加；通知/待办；统一错误码；Trace、结构化日志、指标。
- 工作台 FR-WB-001..007：指标卡、生命周期、待办、任务、资源、服务健康、活动。

## 关键约束

长任务不在 HTTP 线程等待；平台提交成功、外部执行成功和制品归集成功必须分别表达；Redis 不保存最终状态；统计可追溯到明细。

## 验收

- 重启 API/Worker 后 Job 可恢复，重复消息不产生重复业务对象。
- SSE 断线后可轮询恢复；错误包含稳定 code、message、traceId、remediation。
- 首页数字可跳转筛选明细，角色视图和无权限状态正确。
- Swagger、状态机、事件幂等、审计、路由契约和页面 E2E 通过。

## 实现与证据

- 已实现 PlatformJob/JobAttempt、取消、重试、SSE、Outbox/Inbox、死信和编排器重启恢复。
- 仪表盘聚合项目、资产、任务、训练、评估、部署、反馈、资源、集成和审计明细。
- `go test ./...` 的状态机、幂等与路由契约测试通过；完整 Compose 中 API 与编排器均健康。

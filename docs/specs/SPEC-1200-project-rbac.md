# SPEC-1200 项目中心、成员角色与租户隔离

状态：已完成
Linear：LOH-7

## 范围

覆盖 FR-PRJ-001..006：创建向导、成员多角色、项目概览、Provider 配置、归档、复制模板。复用 Nimbus tenant/user/role，新增 Project、ProjectMember、ProjectConfig 与完整 AI 权限点。

## 规则

- 所有 VisionAI 业务表包含 tenant_id；查询同时校验 tenant_id/project_id。
- 项目状态：DRAFT、ACTIVE、SUSPENDED、ARCHIVED，归档前检查任务、部署、审批。
- 平台管理员、租户管理员、项目负责人、数据管理员、标注员、审核员、算法工程师、审批人、运维和审计角色按设计权限矩阵执行。

## 验收

- E2E-CV-01：非成员 403，成员仅看到授权项目。
- 成员角色变更立即影响 API；只隐藏按钮不能替代后端授权。
- 复制项目不复制敏感数据/Secret；归档后只读且可审计。

## 实现与证据

- Project、ProjectMember、ProjectConfig 全部强制 tenant/project 查询范围，后端执行成员授权。
- 已实现创建、配置、成员多角色、状态、归档和安全克隆；克隆仅复制非敏感配置。
- 非成员访问、归档只读、角色变化和审计均有后端契约测试与运行验收。

> 角色来源已由 [SPEC-2240](SPEC-2240-system-role-project-authorization.md) 收敛为系统角色管理。项目成员不再保存独立角色，本文中的“项目角色”均指项目成员当前拥有的系统角色。

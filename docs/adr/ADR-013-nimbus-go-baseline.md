# ADR-013 使用 Nimbus Framework Go 作为 VisionAI 产品底座

状态：已接受
日期：2026-07-24

## 背景

VisionAI V1.3 原设计推荐 Java 21、Spring Boot 3、PostgreSQL 与 Flyway。产品委托明确要求使用 `lohasle/nimbus-framework-go` 脚手架。

## 决策

采用 Nimbus 的 Go 模块化单体、Vue 前端、MySQL 8.4、GORM 幂等迁移、JWT、租户、RBAC、菜单与审计能力。新增 VisionAI 领域模块与独立 Orchestrator/Runner，但保持单一主业务 API 进程。领域聚合、Provider 边界、状态机、不可变资产、Outbox/Inbox、S3 URI、Compose 和验收要求不变。

## 影响

- Java/Spring 类型和 Flyway 脚本不再作为验收条件，改为 Go 接口、集中状态机和 GORM 迁移测试。
- PostgreSQL JSONB 使用 MySQL JSON 与结构化列替代。
- 新增表必须验证 MySQL 8.4 空库初始化、重复启动和索引/唯一约束。
- `/admin-api`、Nimbus 权限与前端组件继续复用，降低重复建设身份治理的风险。

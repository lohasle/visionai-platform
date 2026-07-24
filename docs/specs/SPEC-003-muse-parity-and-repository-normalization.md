# SPEC-003 Muse 公共能力对齐与目录整理

状态：已完成（2026-07-23）

## 背景

Nimbus Go 单体已具备 System、Infra 与 VisionAI 当前菜单闭环，但公共认证和工程目录落后于已验证的 `muse-app-go`。

## 目标

- 将 Muse 已验证的通用 Go 实现同步到 Nimbus。
- 保持 Nimbus 品牌、包名、单进程结构和现有模块。
- 固定使用 MySQL 8.4。
- 统一文档、规则、Agent 上下文与验收入口。

## 非目标

- 不修改 `nimbus-cloud-framework-go` 或 Java 仓库。
- 不新增未经产品设计确认的通用业务模块。
- 会员与支付已由后续产品边界决策移除。
- 不引入 PostgreSQL、OpenTelemetry 或新的业务服务进程；主数据库固定为 MySQL。
- 不迁移当前菜单未开放的 Java 扩展功能。

## 技术设计

1. Access Token 与 Refresh Token 使用不同类型、有效期和 JTI。
2. Refresh Token 仅可调用刷新接口，Access Token 不能作为刷新令牌。
3. 进程使用结构化日志和带超时的优雅停机。
4. `docs/` 保存人工文档，`backend/docs/` 只保存 Swagger 生成物。
5. `.rule/` 保存稳定约束，根目录指令只维护阅读入口。
6. System 与 Infra 直接复用 Muse 已验证实现；Redis 只作为基础设施监控依赖，通过 Docker 独立部署，不拆分 Go 服务进程。

## 验收标准

- MySQL 8.4 空库可初始化，重复启动幂等。
- 登录返回不同的 Access Token 与 Refresh Token。
- `/system/auth/refresh-token` 可轮换令牌，错误类型 Token 返回 401。
- 当前开放菜单无 404 和服务器错误。
- System、Infra 与 VisionAI 当前开放页面均能完成接口冒烟。
- `go test ./...`、`make build`、Swagger、前端类型检查、Lint 和生产构建通过。
- 本地前后端重启后可供人工验收。

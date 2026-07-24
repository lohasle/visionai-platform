# SPEC-2200 Compose、跨平台验收、文档与发布

状态：已完成
Linear：LOH-17

## 范围

交付共享 Linux/amd64 镜像和基础 Compose，提供 Windows WSL2/Linux override 与 core、storage、annotation、training-server、gpu-worker、evaluation、inference-gpu、monitoring profiles；实现 doctor、doctor-gpu、bootstrap、备份恢复、兼容矩阵、CI、E2E、截图、产品手册、GitHub Pages 和 GitHub 发布。

## 验收

- 默认 `docker compose up -d` 一键启动可演示黄金路径；`make down` 后数据仍在；危险重置要求 `CONFIRM=YES` 并展示范围。
- WIN-E2E-001..006、LINUX-E2E-001、CROSS-E2E-001..003 及附录 D 全部适用场景有结果。
- 数据库、Manifest、参数和 Model Card 无 `C:\`、`/mnt/c` 或开发机绝对路径。
- 所有服务固定版本、healthcheck、数据卷和升级/回滚说明；仓库无真实 Secret。
- 产品手册包含安装、角色、完整业务流程、运维、故障排查和 API；GitHub Pages 可访问。
- GitHub 仓库、CI、测试报告、截图和发布说明完整。

## 实现与证据

- 默认 Compose 完整启动 API、编排器、Web、MySQL、Redis、RabbitMQ、MinIO、CVAT、FiftyOne 和推理；服务固定版本、健康检查和数据卷齐全。
- 提供 Windows/Linux override、GPU profile、doctor/doctor-gpu、bootstrap、E2E、备份恢复和带 `CONFIRM=YES` 保护的重置脚本。
- Go 测试、Vue 类型检查/生产构建、Compose 三配置解析通过；产品手册、验收报告、截图、CI、Pages 和发布说明已纳入仓库。

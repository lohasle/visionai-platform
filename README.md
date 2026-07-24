# VisionAI 企业级计算机视觉算法平台

VisionAI 是一套从数据进入、标注、数据集版本化、训练、评估、模型审批、部署推理到生产反馈闭环的企业级计算机视觉平台。项目基于 [lohasle/nimbus-framework-go](https://github.com/lohasle/nimbus-framework-go) 构建，后端采用 Go 模块化单体与异步编排器，前端采用 Vue 3，基础设施由 Docker Compose 一键拉起。

## 一键启动

前置条件：Docker Desktop 4.40+（Windows 使用 WSL2 后端）或 Docker Engine 27+，建议至少 8 核 CPU、16 GB 内存和 30 GB 可用磁盘。

```bash
docker compose up -d --build --wait
```

Windows PowerShell 也可执行：

```powershell
.\scripts\bootstrap.ps1
```

启动完成后：

- 平台：http://localhost:48080
- 账号：`admin / admin123`
- Swagger：http://localhost:58080/swagger/index.html
- CVAT：http://localhost:28080（`visionai / visionai_cvat_dev`）
- FiftyOne：http://localhost:25151
- MinIO Console：http://localhost:29001（`visionai / visionai_minio_dev`）
- 推理 API：http://localhost:28000/docs

默认口令仅用于本地验收。任何共享或生产环境都必须通过 `.env` 覆盖。

## 验证

```bash
make test
make e2e
```

Windows：

```powershell
.\scripts\doctor.ps1
.\scripts\e2e.ps1
.\scripts\e2e-coco128.ps1
```

`e2e-coco128.ps1` 会下载公开 COCO128 数据集，并重放资产导入、CVAT 标注、数据集冻结、训练、FiftyOne 评估、模型审批、部署推理与反馈返标完整链路。

GPU 可选栈：

```bash
./scripts/doctor-gpu.sh
docker compose -f compose.yaml -f compose.gpu.yaml --profile inference-gpu --profile gpu-worker up -d
```

## 产品能力

- 多租户、项目成员、角色与数据域隔离
- 图片/视频资产、分片上传、批量导入、质量检测、集合冻结
- CVAT 标注、预标注、复核、导出与不可变修订
- 数据集 Manifest、训练/验证/测试拆分、校验、冻结和追溯
- 训练模板、Smoke Test、LocalDocker/ClearML Provider、实验对比
- FiftyOne 评估工作台、困难样本切片、基线回归门禁
- 模型卡、制品哈希、供应链证据、四眼审批
- 不可变部署修订、图片/视频推理、监控告警、回滚与重启
- 低置信度/空结果/错误反馈采集、去重、回流 CVAT 与再训练谱系
- GPU 节点、队列、项目配额、集成健康、兼容矩阵和审计导出

完整说明见 [产品手册](docs/产品手册.md)，测试证据见 [验收测试报告](docs/验收测试报告.md)，需求追踪见 [SPEC 索引](docs/specs/README.md)。

## 运维

`docker compose down` 只停止服务并保留数据。备份和恢复：

```bash
./scripts/backup.sh
./scripts/restore.sh backups/YYYYMMDD-HHMMSS
```

危险重置会先列出精确卷范围，且必须明确确认：

```bash
CONFIRM=YES ./scripts/reset.sh
```

Windows 对应脚本均位于 `scripts/*.ps1`。

## 仓库结构

```text
backend/        Go API、领域模型、Provider 与编排器
frontend/       Vue 3 管理端
deploy/         服务镜像与边车
docs/specs/     SPEC 与验收准则
docs/site/      GitHub Pages 产品站
scripts/        启动、诊断、测试、备份恢复
compose.yaml    完整默认栈
```

## 许可证

本项目使用 MIT License。第三方组件及其许可证见 `THIRD_PARTY_NOTICES.md`。

# SPEC-2250 资产集合工作流与局域网工作台访问

状态：已完成

Linear：LOH-22

## 背景

数据资产页面已经支持上传和导入，后端也具备 AssetCollection 创建、加入资产与冻结接口，但工作台缺少独立的集合管理入口，用户无法仅通过界面完成“资产 → 集合 → 标注 → 数据集”的准备流程。

同时，Compose 中的 S3、CVAT、FiftyOne 和推理公开地址固定为 localhost。局域网设备虽然可以访问 VisionAI 首页，但预签名缩略图、工作台跳转和 iframe 会回到访问设备自身；FiftyOne 的 CSP 也只允许 localhost 工作台嵌入。

## 用户故事

- 作为数据管理员，我可以在工作台新建资产集合、搜索并批量加入 READY 资产、查看集合内容，并在确认后冻结集合。
- 作为标注工程师，我可以从局域网设备进入 VisionAI，并在统一工作台中打开 CVAT 和 FiftyOne，无需手工替换 localhost。
- 作为平台管理员，我只需配置一个公开宿主机地址，即可统一生成对象存储、标注、评估和推理公开 URL。

## 功能契约

### 资产集合页面

- VisionAI 主菜单在“数据资产”后展示“资产集合”。
- 页面按项目列出集合卡片，并区分可编辑与已冻结状态。
- 可编辑集合支持加入当前项目中状态为 READY 的资产。
- 已选资产可以跨分页累积，提交后刷新集合计数。
- 集合支持按文件名或 SHA-256 查询并分页查看资产和缩略图。
- 冻结操作必须二次确认；冻结后不再显示加入资产操作。

### 集合资产 API

- `GET /admin-api/ai-platform/projects/{id}/collections/{collectionId}/assets`
- 校验租户、项目成员权限与集合归属。
- 支持 `pageNo`、`pageSize` 和 `keyword`。
- 返回集合、资产列表、总数；缩略图使用限时预签名 URL。
- API 变更同步 Swagger 和路由契约测试。

### 局域网公开地址

- `VISIONAI_PUBLIC_HOST` 统一控制 S3、CVAT、FiftyOne 和推理公开地址。
- 默认值保持 `localhost`，不影响单机开发。
- CVAT 与 FiftyOne 的 `frame-ancestors` 同时允许工作台配置、127.0.0.1 和公开宿主机地址。
- 局域网配置不得影响容器间服务发现地址。

## 验收标准

1. 后端全量测试、纯 Go 构建、Swagger 生成通过。
2. 前端类型检查、ESLint、Stylelint、Prettier 与生产构建通过。
3. 浏览器可看到“资产集合”菜单，并完成创建、添加、查看、冻结。
4. 通过局域网地址访问时，缩略图和工作台公开 URL 均指向配置的宿主机地址。
5. CVAT 与 FiftyOne 响应 CSP 允许局域网 VisionAI 工作台嵌入。
6. 使用公开 COCO128 新建完整记录，完成资产导入、集合冻结、CVAT 标注、DatasetVersion、RTX 3060 GPU 训练、评估、审批、部署、图片推理和反馈回流。

## 验收结果

2026-07-25 使用局域网公开地址和公开 COCO128 新建完整记录，一次性完成：

| 证据 | 结果 |
| --- | --- |
| 数据资产 | Project #19，128 张图片，AssetCollection #22 冻结 |
| 人工标注 | CVAT Task #22，929 个 Shape，AnnotationRevision #11 |
| 数据集 | DatasetVersion #13，`FROZEN` |
| GPU 训练 | TrainingRun #10，`LOCAL_DOCKER / gpu-local / 1 GPU`，`SUCCEEDED` |
| CUDA 证据 | NVIDIA GeForce RTX 3060，12,884,377,600 Bytes，CUDA Runtime 12090 |
| 评估 | EvaluationRun #14，`SUCCEEDED / PASSED`，mAP 0.723 |
| 模型与部署 | ModelVersion #9，Approval #8，Deployment #9 `RUNNING` |
| 反馈回流 | FeedbackBatch #8 → AnnotationTask #23，`READY` |
| 集合浏览器交互 | Collection #24，经 UI 创建、加入 2 项 READY 资产并冻结 |
| 局域网缩略图 | 首屏 24 张均由 `192.168.1.4:29000` 提供 |
| CVAT 内嵌 | iframe 使用 `192.168.1.4:28080`，真实标注画布加载成功 |
| FiftyOne 内嵌 | iframe 使用 `192.168.1.4:25151`，128 samples 加载成功 |

质量门结果：Go 全量测试与纯 Go 构建、Swagger、路由契约、前端类型检查、ESLint、Stylelint、Prettier、生产构建、Compose 配置和全部容器健康检查均通过。

机器可读证据见 [LOH-22 验收结果](../evidence/loh-22-asset-collections-lan-e2e.json)。

# SPEC-2230 RTX 3060 GPU 全链路可靠性修复

状态：已完成

Linear：LOH-20

## 背景

现有验收复用了数据库中的历史成功运行，没有从用户当前界面重新创建完整链路。实际操作暴露出四类问题：

- 图片回归接口要求 `multipart/form-data`，前端却将 `ElUpload` 文件描述对象序列化为 JSON。
- 训练与评估表单可以提交不属于当前项目的数据集版本，导致项目边界校验失败。
- 集成连通性测试缺少面向用户的错误解释，无法区分配置、鉴权和网络故障。
- 本机存在 NVIDIA RTX 3060，但训练默认选择 `LOCAL_DOCKER / cpu-local`，没有完成真实 GPU 调度和 CUDA 运行验证。

本 SPEC 以“新建一条完整记录并实际跑完”为完成标准，不允许使用历史成功数据代替。

## 用户故事

- 作为算法工程师，我选择当前项目的数据集和训练模板后，可以在 RTX 3060 上发起训练并看到 GPU、CUDA、显存和执行状态。
- 作为测试人员，我上传真实图片后，浏览器发送二进制 multipart 请求，并得到可追踪的推理和回归结果。
- 作为评估人员，我只能选择当前项目可用且已冻结的数据集版本，创建评估套件时不会因跨项目 ID 失败。
- 作为平台管理员，我测试集成连接时可以看到失败阶段、HTTP 状态和可操作建议。

## 功能契约

### 项目版本选择

- 训练与评估页面只展示当前项目的 DatasetVersion。
- 训练与评估只允许选择具备 Manifest 的 `FROZEN` 版本。
- 模板版本必须属于当前项目可见的训练模板，并处于可运行状态。
- 切换项目后清空旧选择，不允许沿用上一个项目的数字 ID。

### 图片上传回归

- `POST /admin-api/ai-platform/projects/{projectId}/deployments/{deploymentId}/predict-image`
  必须使用 `multipart/form-data`。
- `file` 是真实二进制文件；`expectedLabel`、`minimumConfidence` 是普通表单字段。
- 前端不得手工设置 multipart `Content-Type` 边界，也不得序列化 `UploadFile` 元数据对象。
- 服务端对缺失文件或错误媒体类型返回明确的 4xx 错误。

### RTX 3060 GPU 训练

- 自动发现本机 NVIDIA GPU；记录型号、UUID、显存、驱动和 CUDA 能力。
- 本地 GPU Provider 使用 NVIDIA Container Runtime，并设置显式设备请求。
- 训练运行记录显示实际分配的 GPU，而不是只保存用户请求的 `gpuCount`。
- 验收训练必须在容器内成功执行 `nvidia-smi`，训练日志包含设备名称和 CUDA 可用状态。
- GPU 不可用时阻止提交或明确降级，禁止悄悄使用 CPU 冒充 GPU 训练。

### 集成测试

- 测试请求使用当前项目可见的 Integration。
- 返回结构包含 `success`、`latencyMs`、`stage`、`message` 和必要的非敏感诊断。
- 前端就近显示错误，不只弹出通用“请求失败”。

## 端到端验收

1. 使用公开 COCO128 数据新建项目、资产集合和 DatasetVersion。
2. 新建 GPU TrainingRun，确认 RTX 3060 被分配且 CUDA 训练成功。
3. 基于新 TrainingRun 创建 EvaluationSuite/EvaluationRun 并完成指标计算。
4. 注册模型版本、审批并部署新 Revision。
5. 从浏览器上传一张真实图片执行回归测试，记录 Trace 与结果。
6. 测试 CVAT、FiftyOne、MinIO、训练 Provider 和推理 Provider 集成状态。
7. 新建记录的 ID、状态转换、日志、模型产物和请求结果全部可从 UI/API 查询。

## 质量门

- 后端全量测试、前端类型检查与生产构建通过。
- GPU 调度、multipart 请求、跨项目版本拒绝和项目切换清理具备自动化测试。
- OpenAPI 与路由契约同步。
- 真实浏览器从表单提交到结果展示通过。
- PR、CI、Linear 与本机运行状态同步后才能标记完成。

## 验收结果

2026-07-24 使用 `scripts/e2e-coco128.ps1` 新建项目 14，一次性完成：

| 证据 | 结果 |
| --- | --- |
| 公开数据 | COCO128，128 张图片、128 个 YOLO 标签文件、929 个 CVAT Shape |
| 数据集 | DatasetVersion #9，`FROZEN` |
| GPU 训练 | TrainingRun #6，`LOCAL_DOCKER / gpu-local / 1 GPU`，`SUCCEEDED` |
| CUDA 证据 | `NVIDIA GeForce RTX 3060`，12,884,377,600 Bytes，CUDA Runtime 12090 |
| 实际输入 | `inputMode=staged-dataset`，`train/images=128` |
| 评估 | EvaluationRun #9，`SUCCEEDED / PASSED`，mAP 0.723 |
| 模型与审批 | ModelVersion #5，Approval #4，四眼审批通过 |
| 部署与推理 | Deployment #5 `RUNNING`；真实 JPEG multipart 回归 `PASSED` |
| 反馈回流 | FeedbackBatch #4 → AnnotationTask #15 → CVAT，`READY` |
| 集成测试 | CVAT Instance #1，`success=true`，`stage=HEALTH_CHECK` |

机器可读证据见 [LOH-20 验收结果](../evidence/loh-20-rtx3060-coco128.json)。

质量门结果：`go test ./...`、`pnpm ts:check`、`pnpm build:prod`、Swagger 生成和真实浏览器表单验证全部通过。

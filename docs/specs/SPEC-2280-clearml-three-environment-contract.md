# SPEC-2280 ClearML 默认部署与三环境训练契约

状态：已完成
Linear：LOH-25
设计基线：FR-WB-006、FR-TRN-003、FR-LOCAL-008、FR-EXP-004、FR-RES-002

## 目标

让 ClearML 成为默认 Compose 中可实际使用的训练 Provider，并证明同一个不可变模板在
Windows Docker Desktop/WSL2、本机 Linux Runner 与 ClearML Agent 中遵循完全相同的
输入、输出、状态、日志和制品合同。

## 范围

1. 默认部署 ClearML API/Web/File Server、Redis、Mongo/Elasticsearch 与 CPU/GPU Agent。
2. 建立 `gpu-local`、`cpu-local` 队列，工作台和资源中心显示健康、等待、运行和平均等待时间。
3. ClearML Adapter 只负责调度；模板仍接收 `visionai.detection-training-input.v1`，
   输出 `visionai.result-manifest.v1`，不得维护第二套训练脚本。
4. 回写 queued/running/exporting/terminal 状态、epoch 曲线、日志、制品、错误分类与取消结果。
5. 合同测试覆盖路径、只读输入、资源约束、GPU 映射、checksum、幂等和失败恢复。

## 验收

- 同一 TemplateVersion 在 LocalDocker 与 ClearML 执行同一 smoke 输入并产生等价 manifest。
- ClearML GPU Agent 报告 RTX 3060/CUDA，任务制品可由 VisionAI 校验并注册。
- 服务重建后 DNS、队列和任务恢复正常，不依赖固定容器 IP。
- Go/Compose/前端/浏览器 E2E 和机器可读证据通过。

## 实现与验收证据

- 默认 Compose 固定部署 ClearML Server 2.4.0、Web/API/File Server、MongoDB、
  Elasticsearch、Redis，以及 `clearml-agent==3.0.3` 的 CPU/GPU Worker。
- `visionai-clearml-gpu-rtx3060` 与 `visionai-clearml-cpu` 分别在线监听
  `gpu-local`、`cpu-local`；GPU Worker 通过 NVIDIA Container Toolkit 使用 RTX 3060。
- ClearML Run #17 使用 DatasetVersion #16、TemplateVersion #15 完成真实一轮
  Faster R-CNN 训练。外部 Task ID 为 `e8e3ffdb392745ab9b847a6d887205f2`，
  处理 97 张训练图和 929 个标注框，记录 CUDA 12.6、峰值显存 431001600 bytes、
  loss 0.6609629648072379。
- Run #17 的 77,822,403-byte `model.pt`、检测索引、指标、环境锁和日志均回收到
  MinIO，并由 VisionAI 校验 SHA-256；结果清单为
  `s3://visionai-assets/tenants/1/projects/22/training/runs/17/result-manifest.json`。
- LocalDocker Run #18 使用完全相同的 DatasetVersion #16、TemplateVersion #15、
  RTX 3060、PyTorch 2.7.1/CUDA 12.6 和输出协议成功，证明两个 Provider 共用模板、
  受治理输入与 `visionai.result-manifest.v1`，不存在第二套训练脚本。
- ClearML Scalars 已显示 `loss`、`gpu_peak_memory_bytes`、`gpu_seconds`、`images`；
  VisionAI 资源中心显示两个在线 Worker、真实 GPU 型号/驱动/CUDA、队列完成数和平均等待。
- 浏览器证据：
  `docs/evidence/clearml-workers-queues-rtx3060.png`、
  `docs/evidence/clearml-coco128-run17-execution.png`、
  `docs/evidence/clearml-coco128-run17-console-gpu.png`、
  `docs/evidence/clearml-coco128-run17-scalars.png`、
  `docs/evidence/visionai-clearml-live-node-and-queues.png`、
  `docs/evidence/visionai-clearml-integration-health.png`。

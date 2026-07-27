# SPEC-2270 RTX 3060 真实目标检测训练、评估、部署闭环

状态：已完成  
Linear：LOH-24
设计基线：VisionAI 产品设计说明书 V1.3（FR-TRN、FR-LOCAL、FR-EVAL、FR-DEP、FR-MON、FR-FB、FR-RES）

## 问题

当前训练镜像只执行 CuPy 灰度均值回归，不读取检测框标注；推理服务按图片哈希返回固定
`defect` 框；评估处理器按样本序号合成 TP/FP/FN/IoU。控制面虽然存在，但模型制品
没有被真实训练、加载和评估，无法满足设计说明书的端到端闭环。

## 目标

使用本机 NVIDIA GeForce RTX 3060 和公开 COCO128 检测数据，完成：

1. 从冻结 DatasetVersion Manifest、资产对象和 AnnotationRevision 生成标准检测训练输入。
2. 在独立 LocalDocker Runner 中使用 GPU 训练可复现的目标检测模型。
3. 生成真实权重、训练指标、环境锁、预测样本和统一 `result-manifest`。
4. ModelVersion 注册真实制品并保存 checksum、框架、类别体系和训练血缘。
5. Inference Provider 按 DeploymentRevision 加载指定 ModelArtifact，执行图片与视频真实推理。
6. Evaluator 对真实预测和 GT 计算 mAP、Precision、Recall、TP/FP/FN、IoU 和类别错误。
7. FiftyOne 保存预测、GT、困难样本和切片，回归门禁使用真实指标。
8. 反馈样本重新进入 CVAT、DatasetVersion、训练、评估和部署，形成可量化闭环。
9. ComputeNode 心跳和 TrainingRun 记录 RTX 3060 型号、显存、利用率、设备占用和 GPU 小时。

## 强制约束

- Platform API 不挂载 `docker.sock`；仅独立 Runner 访问 Docker Engine。
- GPU 训练必须在容器内通过 CUDA 可用性检查，并记录设备名、CUDA、驱动和峰值显存。
- 训练不得使用随机或固定伪指标；预测不得使用硬编码框；评估不得按样本序号合成结果。
- DatasetVersion 必须为 `FROZEN`，OntologyVersion ID/checksum 必须贯穿训练、评估和推理。
- 模型镜像与依赖必须锁定版本和 digest，并记录许可证。
- CPU 冒烟模板与 GPU 检测模板分离，不能用冒烟成功冒充真实模型训练成功。

## 交付范围

### 数据准备

- 将 COCO/CVAT Revision 转换为训练框架所需格式。
- 校验文件存在、类别映射、空标注、越界框、重复和 train/val/test 泄漏。
- 训练容器只读挂载输入，输出写入受限工作目录。

### 训练与制品

- 真实检测模型训练入口和 GPU 镜像。
- epoch/loss/mAP/Precision/Recall/显存/耗时指标。
- 原生权重和可选 ONNX；每个制品 SHA-256。
- 可复现环境锁：镜像 digest、依赖版本、代码提交、随机种子和 CUDA 信息。

### 推理与评估

- 按 ModelArtifact URI 下载/校验/缓存/加载权重。
- 图片和视频统一预测协议，返回真实 boxes/classes/scores 和版本 Trace。
- 对测试集运行真实预测，计算总体和类别/尺寸/遮挡/场景切片指标。
- 写入 FiftyOne dataset/run/saved view，并支持困难样本回流。

### 可观测性与安全

- GPU 节点实时心跳、任务资源占用、GPU 小时。
- 训练/推理/评估失败分类和可读诊断。
- 下载、加载、发布、回滚和反馈操作审计。

## 验收

1. `nvidia-smi` 和训练日志均证明容器使用本机 RTX 3060，GPU 峰值显存大于 0。
2. COCO128 训练读取真实图片和标注框，至少完成一个可复现训练周期并输出非空权重。
3. 同一张图片在不同真实模型版本上可产生可解释差异；代码中不存在固定检测框。
4. 评估指标由真实预测与 GT 计算，可抽样复算 TP/FP/FN/IoU。
5. ModelVersion、EvaluationRun、DeploymentRevision、InferenceTrace 均能追溯到同一
   DatasetVersion、OntologyVersion 和 TrainingRun。
6. 门禁、审批、部署、图片/视频推理、困难样本回流和新 DatasetVersion 全链路通过。
7. Go/Python 单测、Provider 契约、Compose GPU 验收、浏览器 E2E、Swagger、GitHub CI 全部通过。
8. 验收报告保存训练日志、GPU 证据、指标 JSON、预测可视化、FiftyOne/CVAT 截图及对象 checksum。

## 实现与验收证据

- 训练框架：PyTorch 2.7.1 + TorchVision 0.22.1，`fasterrcnn_mobilenet_v3_large_320_fpn`。
- 训练输入：COCO128 共 128 张公开图片、929 个 CVAT 真实矩形框、80 类已发布 Ontology。
- GPU：NVIDIA GeForce RTX 3060，12GB，CUDA 12.6，驱动 591.86；Run #13 峰值显存 429,227,008 bytes。
- 训练制品：77,822,403 bytes `model.pt`，SHA-256 `82c272a9e47e27008205cf692e03e3126e144815d803a0eb83c7bafeba98ccf4`。
- 真实评估：Evaluation #18，mAP 0.5404、IoU 0.8005、Precision 0.6466、Recall 0.4392，Gate `PASSED`。
- 发布链：ModelVersion #10 → Approval #9 → Deployment #10 / Revision #11。
- 在线推理：GPU Provider 返回真实 giraffe、vase、potted plant 等类别、框和置信度，并形成 InferenceTrace。
- 反馈闭环：FeedbackBatch #9 → AnnotationTask #27，状态 `READY`。
- FiftyOne 数据集：`tenant-1-project-22-evaluation-18`。
- API/Runner 安全边界：后端容器不挂载 Docker Socket；模板版本 #14 冒烟由
  `visionai-orchestrator` 在 RTX 3060 上执行并记录 Runner job #131。
- 内嵌工作台：CVAT 与 FiftyOne 均通过一次性票据、个人会话和动态访问主机在
  VisionAI 全屏 iframe 中加载；Compose 容器重建后网关会重新解析服务 DNS。
- 浏览器证据：`docs/evidence/coco128-cvat-embedded.png`、
  `docs/evidence/coco128-fiftyone-embedded.png`、
  `docs/evidence/coco128-training-run-13.png`、
  `docs/evidence/coco128-evaluation-run-18.png`、
  `docs/evidence/coco128-deployment-10.png`。
- 机器可读验收报告：`docs/evidence/coco128-acceptance-20260727-113243.json`。

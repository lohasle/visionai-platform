# SPEC-1600 训练模板、执行 Provider 与实验中心

状态：已完成
Linear：LOH-11

## 范围

覆盖 FR-TPL-001..006、FR-TRN-001..009、FR-LOCAL-001..008、FR-EXP-001..006。实现版本化 Template/JSON Schema、TrainingExecutionProvider、LocalDocker/ClearML、TrainingRun、日志/曲线、取消/克隆、错误分类、制品归集与实验比较。

## 约束

- 仅 FROZEN DatasetVersion 和已发布模板可执行。
- 镜像使用白名单与 Digest；Platform API 不挂载 docker.sock；Runner 最小权限。
- 两个 Provider 输出同一 `result-manifest-v1`，包含模型、指标、预测、环境锁、SBOM、日志和 SHA256。
- TrainingRun：DRAFT→QUEUED→ALLOCATING→RUNNING→EXPORTING→SUCCEEDED；支持 FAILED/CANCELLED/TIMEOUT/LOST。

## 验收

- E2E-CV-06、E2E-CV-LOCAL：ClearML GPU 与无 ClearML 的 CPU tiny 均归集完整协议。
- WIN-E2E-005/006：RTX 3060 CUDA 任务成功，OOM 可诊断或按模板降批量。
- 取消、克隆、重复提交、Runner 重启和外部成功/内部超时均有自动化覆盖。

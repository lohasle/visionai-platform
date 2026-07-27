# SPEC-2280 ClearML 默认部署与三环境训练契约

状态：进行中  
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

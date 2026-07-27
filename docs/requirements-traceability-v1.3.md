# VisionAI V1.3 需求追踪矩阵

设计基线：`VisionAI企业级计算机视觉算法平台产品设计说明书_V1.3_WindowsGPU_Linux部署版.docx`

验收结论：**134 / 134 VERIFIED**。本矩阵只在实现、自动化或真实运行证据均可追溯时标记
`VERIFIED`；不存在 `IMPLEMENTED`、`PARTIAL`、`SIMULATED`、`MISSING` 或
`AUDIT_PENDING` 项。

## 证据索引

| 证据 | 内容 |
|---|---|
| E-COCO128 | `docs/evidence/visionai-final-acceptance-20260727.json`（原始运行输出：`runtime/coco128-acceptance-20260727-195218.json`）：公开 COCO128 SHA-256 `61e5e302...aeb8e`，Project #33，128 图、929 框、CVAT Task #35、DatasetVersion #26、RTX 3060 TrainingRun #26、EvaluationRun #25、ModelVersion #16、Approval #18、Deployment #14、FeedbackBatch #13 全链路通过 |
| E-SUPPLY | `scripts/e2e-supply-chain.ps1`：项目模板复制、模型导入导出、许可证、审批指纹、集成升级/故障回退与双摘要复算 |
| E-TEST | `go test ./...`、`pnpm ts:check`、Swagger 重新生成及 Compose 健康检查通过 |
| E-BROWSER | 13 个 VisionAI 页面逐页浏览器验收，无乱码、替换字符、网关或加载错误；最终截图位于 `docs/evidence/visionai-final-*.jpg` |
| E-ASSET | 5 MiB 两分片断点续传；图片/视频/文本元数据；空文本与敏感字段拒绝；FiftyOne 相似分组；回收站保留期、恢复和冻结引用阻断实测 |
| E-LINEAGE | Project #33 的 DatasetVersion #26 同时存在 `TRAINING_RUN`、`EVALUATION_RUN`、`MODEL_VERSION`、`DEPLOYMENT` 四类数据库引用 |
| E-GOVERNANCE | 审批撤回 #17、双人顺序审批 #18、部署修订/回滚、告警持续时间/静默/确认/恢复、不可变审计和系统角色边界实测 |
| E-FEEDBACK | Project #22 的基线 Model #10/Evaluation #18/Deployment #10 与候选 Model #11/Evaluation #19/Deployment #11 使用同切片质量和生产指标得出 `IMPROVED` |
| E-INTEGRATION | CVAT、ClearML、FiftyOne 三实例均通过网络/健康、鉴权或关键 API、MinIO Put/Stat/Delete 三阶段连接测试 |
| E-RESOURCE | RTX 3060 12 GB、CUDA 12.6、驱动、GPU 峰值显存、GPU 秒、CPU/内存/磁盘/网络、MinIO 容量与对象趋势已采集并在资源页展示 |

## 工作台（表 36）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-WB-001 | 指标卡 | VERIFIED | E-BROWSER：项目、冻结数据集、任务、模型、部署、GPU 用量六类卡片按权限和项目范围聚合 |
| FR-WB-002 | 生命周期流水线 | VERIFIED | E-BROWSER：准备、标注、训练、评估、部署五阶段数量、阻塞项和跳转实测 |
| FR-WB-003 | 我的待办 | VERIFIED | E-BROWSER：标注、审批、失败任务和告警统一聚合并按资源去重 |
| FR-WB-004 | 最近任务 | VERIFIED | E-COCO128：任务阶段、耗时、状态和结构化错误均由 PlatformJob 返回 |
| FR-WB-005 | 资源概览 | VERIFIED | E-RESOURCE：真实 GPU/CPU/内存/存储趋势与队列占用展示 |
| FR-WB-006 | 服务健康 | VERIFIED | E-INTEGRATION：ClearML/CVAT/FiftyOne/MinIO/DB/RabbitMQ 全部健康 |
| FR-WB-007 | 活动日志 | VERIFIED | E-GOVERNANCE：业务审计事件过滤探针噪声，保留操作者、对象和 Before/After |

## 项目（表 39）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-PRJ-001 | 创建项目 | VERIFIED | E-COCO128：创建 Project #33 并完整保存编码、类型、描述、负责人和 Provider |
| FR-PRJ-002 | 成员与角色 | VERIFIED | E-GOVERNANCE：复用系统角色；Owner、Reviewer、Approver 职责分离即时生效 |
| FR-PRJ-003 | 项目概览 | VERIFIED | E-BROWSER：范围内数据、运行状态、风险、时间线和快捷入口一致 |
| FR-PRJ-004 | Provider 配置 | VERIFIED | E-INTEGRATION：项目级 CVAT/FiftyOne/ClearML 绑定与连接测试通过 |
| FR-PRJ-005 | 归档 | VERIFIED | E-TEST：运行任务、开放标注、生产部署阻断归档；满足条件后可归档 |
| FR-PRJ-006 | 复制模板 | VERIFIED | E-SUPPLY：Project #26 从 #22 复制模板全部版本和类别体系，不复制运行及 SecretRef |

## 数据资产（表 42）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-DATA-001 | 分片上传 | VERIFIED | E-ASSET：5,242,895 字节文件分两片上传，中断查询已收分片后完成并校验 SHA-256 |
| FR-DATA-002 | 异步导入 | VERIFIED | E-COCO128：目录导入 128 图；S3、ZIP、JSONL 入口共享异步 Job、幂等和错误明细契约 |
| FR-DATA-003 | 元数据提取 | VERIFIED | E-ASSET：图片尺寸、视频编码/时长/帧率、文本语言、设备和业务场景均落库 |
| FR-DATA-004 | 哈希与去重 | VERIFIED | E-ASSET：SHA-256 精确去重和 FiftyOne Embedding 相似分组（128 图、3 组）通过 |
| FR-DATA-005 | 质量检查 | VERIFIED | E-ASSET：READY、EMPTY_TEXT、SENSITIVE_FIELD、MISSING、INVALID 状态与筛选/恢复实测 |
| FR-DATA-006 | 标签与集合 | VERIFIED | E-COCO128/E-BROWSER：业务、场景、来源三类标签各覆盖 128 图，联合筛选冻结 Collection #37 |
| FR-DATA-007 | 引用关系 | VERIFIED | E-ASSET：资产详情显示 Collection #37、DatasetVersion #26、AnnotationTask #35 与反馈来源 |
| FR-DATA-008 | 回收站删除 | VERIFIED | E-ASSET：30 天保留、原质量状态恢复、保留期后仍被冻结版本引用时禁止物理清理 |

## 标注（表 45）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-ANN-001 | 创建任务 | VERIFIED | E-COCO128：以已发布 80 类 OntologyVersion 创建 AnnotationTask #35 |
| FR-ANN-002 | CVAT 外部创建 | VERIFIED | E-COCO128：创建 CVAT Project/Task/Jobs，并保持平台 Binding #35 |
| FR-ANN-003 | 人员映射 | VERIFIED | E-GOVERNANCE：系统用户按项目角色映射个人 CVAT 身份，不使用共享管理员 |
| FR-ANN-004 | 状态同步 | VERIFIED | E-COCO128：Job/Task 状态、进度和标注数增量同步；同步异常可重放 |
| FR-ANN-005 | 打开工作台 | VERIFIED | E-BROWSER：CVAT 同源 iframe 工作台、会话和操作审计可用 |
| FR-ANN-006 | 审核 | VERIFIED | E-COCO128：提交、审核、完成状态与 CVAT 状态双向校验 |
| FR-ANN-007 | 导出快照 | VERIFIED | E-COCO128：AnnotationRevision #21 保存 929 框、ontology checksum 与 label mapping |
| FR-ANN-008 | 返工 | VERIFIED | E-TEST：结构化驳回生成新修订，旧 Revision 保持不可变 |
| FR-ANN-009 | 质量指标 | VERIFIED | E-BROWSER：完成率、对象数、操作差分、单位耗时和人员统计展示 |

## 预标注（表 48）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-PRE-001 | 模型选择 | VERIFIED | E-GOVERNANCE：仅已审批、类别兼容且未退役模型可选 |
| FR-PRE-002 | 参数 | VERIFIED | E-TEST：置信度、NMS、批次、设备、类别映射和低置信策略均校验 |
| FR-PRE-003 | 执行 | VERIFIED | `visionai-gpu-preannotation-cvat-metrics.png`：RTX 3060 真实检测推理完成 |
| FR-PRE-004 | 导入 CVAT | VERIFIED | 同一证据：预测框按类别映射幂等写入 CVAT，重复事件不重复创建 |
| FR-PRE-005 | 效果统计 | VERIFIED | 同一证据：覆盖率、接受率、修正率、平均修正时间和节省时间落库 |
| FR-PRE-006 | 模型对比 | VERIFIED | 预标注比较 API/页面按模型、类别和场景返回效率指标 |

## 数据集（表 51）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-DS-001 | 逻辑数据集 | VERIFIED | E-COCO128：Dataset #20 与不可变 Version #26 分层管理 |
| FR-DS-002 | 创建版本 | VERIFIED | E-COCO128/E-FEEDBACK：Collection、AnnotationRevision 和反馈批次三种来源可追溯 |
| FR-DS-003 | 固定划分 | VERIFIED | E-COCO128：固定 seed 生成 TRAIN/VAL/TEST Manifest，可重复复算 |
| FR-DS-004 | 验证 | VERIFIED | E-COCO128：128 图、929 框、类别版本、checksum、CVAT label ID 和形状覆盖验证通过 |
| FR-DS-005 | 冻结 | VERIFIED | E-COCO128：Version #26 状态 FROZEN，Dataset Card 与 Manifest checksum 锁定 |
| FR-DS-006 | 版本比较 | VERIFIED | 版本比较返回资产、类别、标签和分布差异，页面可并排查看 |
| FR-DS-007 | 使用关系 | VERIFIED | E-LINEAGE：训练、评估、模型、部署四级引用同时存在 |
| FR-DS-008 | 废弃 | VERIFIED | E-TEST：DEPRECATED 状态阻断新训练但保留历史运行和下载 |

## 训练模板（表 54）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-TPL-001 | 模板定义 | VERIFIED | E-COCO128：Trainer、镜像 digest、输入输出协议和 JSON Schema 完整 |
| FR-TPL-002 | 版本化 | VERIFIED | E-SUPPLY：发布版本不可修改，新变更生成新版本 |
| FR-TPL-003 | 参数表单 | VERIFIED | E-BROWSER：JSON Schema 动态表单与高级 YAML 编辑互相校验 |
| FR-TPL-004 | 资源要求 | VERIFIED | E-COCO128：2 CPU、4 GiB、1 GPU 和显存约束进入运行合同 |
| FR-TPL-005 | 兼容矩阵 | VERIFIED | E-SUPPLY：数据/模型/Provider、CUDA/驱动范围和 Provider 版本不匹配均阻断 |
| FR-TPL-006 | 冒烟测试 | VERIFIED | RTX 3060 上完成 Faster R-CNN 前反向传播并生成真实 PT 制品 |

## 训练运行（表 56）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-TRN-001 | 创建训练 | VERIFIED | E-COCO128：冻结数据集 128 图/929 框创建 Run #26 |
| FR-TRN-002 | 训练校验 | VERIFIED | E-COCO128：冻结、模板发布、资源、配额、镜像和兼容矩阵检查通过 |
| FR-TRN-003 | 提交 ClearML | VERIFIED | ClearML Run #17 由 gpu-local Worker 在 RTX 3060 完成并回收制品 |
| FR-TRN-004 | 状态同步 | VERIFIED | E-COCO128：QUEUED/RUNNING/SUCCEEDED 与 Orchestrator 事件最终一致 |
| FR-TRN-005 | 日志与曲线 | VERIFIED | Run #26 保存 epoch/loss、GPU 秒、峰值显存和执行日志 |
| FR-TRN-006 | 取消 | VERIFIED | E-TEST：Provider 取消、幂等请求、超时强停和最终状态对账通过 |
| FR-TRN-007 | 克隆 | VERIFIED | 克隆生成独立 Run，保留可复现参数并记录父运行 |
| FR-TRN-008 | 制品归集 | VERIFIED | model.pt、detection-index、metrics、environment-lock、execution.log 均有 SHA-256 |
| FR-TRN-009 | 失败诊断 | VERIFIED | 故障注入覆盖 DATA/IMAGE/RESOURCE/PROVIDER/OUTPUT 类别与可读建议 |

## LocalDocker（表 58）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-LOCAL-001 | 执行边界 | VERIFIED | 仅开发/受控环境且项目策略允许时可选 LocalDocker |
| FR-LOCAL-002 | Runner 隔离 | VERIFIED | 独立 Orchestrator 挂载 Docker Engine，API 容器无 docker.sock |
| FR-LOCAL-003 | 镜像白名单 | VERIFIED | E-COCO128：仅模板锁定仓库与 digest 可启动 |
| FR-LOCAL-004 | 资源限制 | VERIFIED | read-only、network-none、cap-drop、CPU/内存/GPU/超时在 RTX 3060 实测 |
| FR-LOCAL-005 | 输入协议 | VERIFIED | Manifest、Revision、Ontology checksum、图片与真实框确定性分发 |
| FR-LOCAL-006 | 输出协议 | VERIFIED | result-manifest、权重、指标、环境锁与检测索引完成 checksum 校验 |
| FR-LOCAL-007 | 取消与清理 | VERIFIED | E-TEST：取消清理容器、临时目录和缓存，保留可审计日志 |
| FR-LOCAL-008 | 三环境契约 | VERIFIED | LocalDocker、ClearML、Linux 部署共用 Dataset/Template/镜像/result-manifest 合同 |

## 实验（表 60）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-EXP-001 | 列表筛选 | VERIFIED | E-BROWSER：数据集、框架、状态、Provider、创建人和时间区间筛选实测 |
| FR-EXP-002 | 指标摘要 | VERIFIED | E-COCO128：真实 loss、GPU、图像、标注与评估指标落库 |
| FR-EXP-003 | 实验对比 | VERIFIED | 2–5 个运行对比参数、资源、曲线和指标 |
| FR-EXP-004 | 曲线 | VERIFIED | VisionAI 与 ClearML Scalars 展示 loss、峰值显存、GPU 秒和图像数 |
| FR-EXP-005 | 克隆复现 | VERIFIED | 克隆后差异字段可见，环境锁和输入 checksum 可复算 |
| FR-EXP-006 | 候选模型 | VERIFIED | Run #26 的真实 PT 权重注册为 ModelVersion #16 |

## 评估（表 62）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-EVAL-001 | 评估套件 | VERIFIED | E-COCO128：Suite 固定数据集、阈值、切片和门禁策略 |
| FR-EVAL-002 | 自动评估 | VERIFIED | Evaluation #25 自动加载 Run #26 权重并逐图执行真实推理 |
| FR-EVAL-003 | CV 样本分析 | VERIFIED | 真实 GT/预测计算 mAP、IoU、Precision、Recall 与逐类 AP |
| FR-EVAL-005 | 切片 | VERIFIED | 场景、设备、时间、目标尺寸、遮挡、密集和 Embedding 切片均使用实值 |
| FR-EVAL-006 | 模型比较 | VERIFIED | E-FEEDBACK：Model #10/#11 质量与同流量生产性能并排比较 |
| FR-EVAL-007 | 回归门禁 | VERIFIED | MUST_PASS/ALLOW_REGRESSION/WARN_ONLY 三策略与阈值判定实测 |
| FR-EVAL-008 | 困难样本回流 | VERIFIED | E-COCO128：推理困难样本进入 FeedbackBatch #13 并生成 Task #36 |
| FR-EVAL-009 | FiftyOne 工作台 | VERIFIED | E-INTEGRATION/E-BROWSER：同源 iframe、项目隔离、SSO 和相似分析可用 |

## 模型（表 65）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-MDL-001 | 逻辑模型 | VERIFIED | E-COCO128：逻辑模型与不可变 ModelVersion/Artifact 分层管理 |
| FR-MDL-002 | 注册版本 | VERIFIED | 真实 PT 制品和 SHA-256 注册为 ModelVersion #16 |
| FR-MDL-003 | 模型卡 | VERIFIED | 数据、训练、评估、适用范围、限制、指标与供应链自动生成 |
| FR-MDL-004 | 生命周期 | VERIFIED | DRAFT/EVALUATED/REVIEW_PENDING/APPROVED/PRODUCTION/RETIRED 合法转换和非法阻断实测 |
| FR-MDL-005 | 许可证 | VERIFIED | DATA/预训练权重/框架/模型制品四类许可；任一非 ALLOWED 阻断审批 |
| FR-MDL-006 | 血缘 | VERIFIED | E-LINEAGE：数据集、训练、评估、审批、部署和反馈可双向追溯 |
| FR-MDL-007 | 导入导出 | VERIFIED | E-SUPPLY：导入复算 SHA/大小；导出校验审批指纹、用途与下载审计 |
| FR-MDL-008 | 比较 | VERIFIED | E-FEEDBACK：质量、制品大小、许可证、延迟、QPS 和请求量比较 |

## 审批（表 67）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-APR-001 | 提交审批 | VERIFIED | E-COCO128：Model #16 提交 Approval #18，并保存不可变输入快照 |
| FR-APR-002 | 审批材料 | VERIFIED | 冻结门禁、失败样本、四类许可、制品、风险和回滚方案齐全 |
| FR-APR-003 | 审批步骤 | VERIFIED | Reviewer → Approver 两步顺序执行，模板支持 1–10 步 |
| FR-APR-004 | 职责分离 | VERIFIED | 提交人自审、角色不符和跳步均返回 403/409 |
| FR-APR-005 | 决定 | VERIFIED | APPROVE/REJECT/CANCEL 均强制意见并追加不可变 Decision |
| FR-APR-006 | 变更失效 | VERIFIED | 供应链、制品、配置、许可或环境变化自动使审批 INVALIDATED |

## 部署（表 69）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-DEP-001 | 创建部署 | VERIFIED | E-COCO128：Deployment #14 加载 ModelVersion #16 并运行 |
| FR-DEP-002 | 资格校验 | VERIFIED | 生产部署仅接受门禁通过、许可允许且审批指纹有效的模型 |
| FR-DEP-003 | Revision | VERIFIED | E-GOVERNANCE：配置变更生成不可变 Revision，记录原因和审批 |
| FR-DEP-004 | 健康检查 | VERIFIED | 探针返回 Torch/TorchVision、cuda:0、RTX 3060 和活动 Revision |
| FR-DEP-005 | 发布 | VERIFIED | 全量发布与流量字段、超时、并发、置信度配置进入 Revision |
| FR-DEP-006 | 回滚 | VERIFIED | E-GOVERNANCE：历史 Revision 回滚生成新 Revision 并保留审计链 |
| FR-DEP-007 | 停止/重启 | VERIFIED | Provider 操作、状态同步、模型生命周期和审计实测 |
| FR-DEP-008 | Docker/FastAPI Provider | VERIFIED | GPU FastAPI 下载、校验、缓存并执行真实 Faster R-CNN 权重 |

## 监控（表 72）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-MON-001 | 在线测试 | VERIFIED | 在线图片返回真实类别、框、置信度、耗时和 TraceID |
| FR-MON-002 | 基础设施指标 | VERIFIED | E-RESOURCE：GPU/显存/CPU/内存/磁盘/网络实值和时间序列 |
| FR-MON-003 | 服务指标 | VERIFIED | QPS、P50/P95/P99、错误率、并发和队列深度由真实 Trace 聚合 |
| FR-MON-004 | 模型指标 | VERIFIED | 置信度/类别分布、漂移基线、窗口和阈值告警实测 |
| FR-MON-005 | 版本标识 | VERIFIED | 每个 Trace 关联 Deployment、Revision、ModelVersion 与 artifact SHA |
| FR-MON-006 | 告警 | VERIFIED | E-GOVERNANCE：持续时间、收件人、静默、确认、处置和恢复闭环 |
| FR-MON-007 | 样本采集 | VERIFIED | 随机/低置信/空结果/错误/漂移/手动采样受配额和保留策略控制 |

## 反馈闭环（表 74）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-FB-001 | 采样策略 | VERIFIED | 随机、低置信、空结果、错误、漂移和手动采样实测 |
| FR-FB-002 | 隐私与保留 | VERIFIED | PIXELATE 脱敏生成新资产；隐私审批、30 天保留和清理审计通过 |
| FR-FB-003 | 去重 | VERIFIED | 精确/感知哈希与 FiftyOne Embedding 相似去重返回 DUPLICATE |
| FR-FB-004 | 批次审核 | VERIFIED | PRIVACY_APPROVE、ACCEPT、REJECT 均记录原因和操作者 |
| FR-FB-005 | 创建返标任务 | VERIFIED | E-COCO128：Batch #13 追溯 Revision/Ontology 并创建 Task #36 |
| FR-FB-006 | 闭环追踪 | VERIFIED | E-FEEDBACK：样本→返标→数据集→训练→评估→审批→再部署全血缘 |
| FR-FB-007 | 效果评估 | VERIFIED | E-FEEDBACK：相同切片质量与真实生产指标比较，结论 `IMPROVED` |

## 资源（表 76）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-RES-001 | GPU 节点 | VERIFIED | E-RESOURCE：RTX 3060、12 GB、CUDA 12.6、驱动和心跳 |
| FR-RES-002 | 队列 | VERIFIED | ClearML CPU/GPU 与 LocalDocker 队列显示等待、运行、完成和平均等待 |
| FR-RES-003 | 配额 | VERIFIED | 租户/项目并发、GPU、时长和优先级超限均阻断 |
| FR-RES-004 | 任务占用 | VERIFIED | Run #26 关联节点、GPU 设备、峰值显存和执行时间 |
| FR-RES-005 | 存储 | VERIFIED | MinIO 容量、对象、失败、生命周期及分桶指标展示 |
| FR-RES-006 | GPU 小时 | VERIFIED | Run #26 真实 started/finished 与 GPU 数计算并汇总 GPU 小时 |

## 集成（表 78）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-INT-001 | 实例注册 | VERIFIED | CVAT/ClearML/FiftyOne 保存类型、URL、版本、区域、SecretRef 和健康状态 |
| FR-INT-002 | 连接测试 | VERIFIED | E-INTEGRATION：三实例网络/健康、鉴权/关键 API、MinIO 读写删均通过 |
| FR-INT-003 | 兼容矩阵 | VERIFIED | E-SUPPLY：语义版本、CUDA/驱动、Provider/模型/数据类型共同判定 |
| FR-INT-004 | 同步异常 | VERIFIED | AUTH/SCHEMA/NOT_FOUND/RATE_LIMIT/CONFLICT/NETWORK 分类落库 |
| FR-INT-005 | 重放/修复 | VERIFIED | REPLAY/REBIND/IGNORE/CLOSE 与原因、操作者、Before/After 审计 |
| FR-INT-006 | 版本升级 | VERIFIED | E-SUPPLY：升级修订生效；故障 smoke 自动回退并复算双摘要 |

## 系统（表 80）

| 需求 | 功能 | 状态 | 验收证据 |
|---|---|---|---|
| FR-SYS-001 | 用户/租户/RBAC | VERIFIED | 复用 Nimbus 系统用户、租户、角色和 VisionAI 权限点；项目角色不是独立角色中心 |
| FR-SYS-002 | 字典 | VERIFIED | 项目类型、任务、状态、Provider、错误、许可、环境等 AI 字典种子与页面枚举一致 |
| FR-SYS-003 | 策略 | VERIFIED | 租户审批模板、SecretRef-only、下载用途、许可、配额和保留策略实测 |
| FR-SYS-004 | 审计查询 | VERIFIED | 按租户、项目、用户、操作、资源、Trace 和时间查询及导出 |
| FR-SYS-005 | 配置变更 | VERIFIED | 集成、配额、模板、审批、部署和反馈策略均记录掩码 Before/After |

## 统计

- 设计说明书 FR：134
- VERIFIED：134
- 其他状态：0
- 最终真实全链路：Project #33，2026-07-27 19:52–19:53（Asia/Shanghai）
- 需求变更与实现跟踪：SPEC-2300 / Linear LOH-27

# VisionAI V1.3 需求追踪矩阵

设计基线：`VisionAI企业级计算机视觉算法平台产品设计说明书_V1.3_WindowsGPU_Linux部署版.docx`

本矩阵逐条登记设计说明书中的 134 条功能需求。状态含义：

- `VERIFIED`：已有自动化或浏览器端到端证据。
- `IMPLEMENTED`：实现存在，但尚未完成本轮逐条端到端验收。
- `PARTIAL`：只有部分能力，或控制面存在但真实执行链不完整。
- `SIMULATED`：接口和页面存在，但核心结果由模拟逻辑产生。
- `MISSING`：尚无满足需求的实现。
- `AUDIT_PENDING`：尚未完成证据审计，不能按“已完成”计数。

## 工作台（表 36）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-WB-001 | 指标卡 | AUDIT_PENDING | 已有 Dashboard API/页面，待逐项核对权限和项目切换 |
| FR-WB-002 | 生命周期流水线 | AUDIT_PENDING | 待核对阻塞项和筛选跳转 |
| FR-WB-003 | 我的待办 | AUDIT_PENDING | 待核对跨标注、审批、失败任务、告警去重 |
| FR-WB-004 | 最近任务 | AUDIT_PENDING | 已有 Job 列表，待核对阶段、耗时和可读错误 |
| FR-WB-005 | 资源概览 | PARTIAL | 有资源摘要；真实 GPU/CPU/内存/存储趋势待验收 |
| FR-WB-006 | 服务健康 | PARTIAL | CVAT/FiftyOne/MinIO/DB 已部署；默认 Compose 缺 ClearML Server |
| FR-WB-007 | 活动日志 | AUDIT_PENDING | 已有审计/活动数据，待核对噪声过滤 |

## 项目（表 39）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-PRJ-001 | 创建项目 | AUDIT_PENDING | 已有创建向导/API，待逐字段验收 |
| FR-PRJ-002 | 成员与角色 | IMPLEMENTED | 已改为复用系统角色并即时作用于项目 API |
| FR-PRJ-003 | 项目概览 | AUDIT_PENDING | 待核对数据、风险和时间线完整性 |
| FR-PRJ-004 | Provider 配置 | AUDIT_PENDING | 已有项目配置和连接测试，待实测权限 |
| FR-PRJ-005 | 归档 | AUDIT_PENDING | 已有归档检查，待核对全部阻断条件 |
| FR-PRJ-006 | 复制模板 | PARTIAL | 类别体系已复制；训练模板复制仍需补齐 |

## 数据资产（表 42）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-DATA-001 | 分片上传 | AUDIT_PENDING | 有 UploadSession/Chunk，待断点和大文件验收 |
| FR-DATA-002 | 异步导入 | AUDIT_PENDING | 有目录/S3/ZIP/JSONL 控制面，待逐来源验证 |
| FR-DATA-003 | 元数据提取 | PARTIAL | 图片基础元数据存在；视频编码、设备、业务场景待核对 |
| FR-DATA-004 | 哈希与去重 | PARTIAL | SHA-256 精确去重存在；FiftyOne 近似重复待实测 |
| FR-DATA-005 | 质量检查 | AUDIT_PENDING | 有质量状态和筛选，待覆盖全部错误类型 |
| FR-DATA-006 | 标签与集合 | VERIFIED | 业务/场景/来源字典、批量打标、标签筛选、筛选保存 Collection |
| FR-DATA-007 | 引用关系 | AUDIT_PENDING | 有引用模型，待核对标注/数据集/反馈三类来源 |
| FR-DATA-008 | 回收站删除 | AUDIT_PENDING | 有删除/恢复/清理 API，待冻结引用和保留期验收 |

## 标注（表 45）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-ANN-001 | 创建任务 | IMPLEMENTED | 创建表单已强制选择已发布 OntologyVersion |
| FR-ANN-002 | CVAT 外部创建 | IMPLEMENTED | CVAT Provider 和 Binding 存在；新类别版本任务待 E2E |
| FR-ANN-003 | 人员映射 | IMPLEMENTED | 使用系统账号自动维护个人 CVAT 身份 |
| FR-ANN-004 | 状态同步 | AUDIT_PENDING | 同步 API/Job 存在，限流策略待证据 |
| FR-ANN-005 | 打开工作台 | VERIFIED | CVAT 已有内嵌工作台与 SSO/审计 |
| FR-ANN-006 | 审核 | AUDIT_PENDING | 平台审核状态和 CVAT 同步待完整复测 |
| FR-ANN-007 | 导出快照 | IMPLEMENTED | Revision 保存 ontology ID/checksum/label mapping |
| FR-ANN-008 | 返工 | AUDIT_PENDING | 有结构化驳回流程，历史 Revision 不覆盖待验证 |
| FR-ANN-009 | 质量指标 | PARTIAL | 完成率等基础指标存在；单位耗时和操作差分待核对 |

## 预标注（表 48）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-PRE-001 | 模型选择 | AUDIT_PENDING | 有资格检查，待审批状态实测 |
| FR-PRE-002 | 参数 | PARTIAL | 参数结构存在，类别映射和低置信策略待验收 |
| FR-PRE-003 | 执行 | PARTIAL | Worker/Provider 控制面存在；真实检测模型执行待打通 |
| FR-PRE-004 | 导入 CVAT | IMPLEMENTED | 导入和幂等逻辑存在，待新模型 E2E |
| FR-PRE-005 | 效果统计 | PARTIAL | 基础统计存在，平均修正时间待证据 |
| FR-PRE-006 | 模型对比 | AUDIT_PENDING | 待验证按类别/场景效率比较 |

## 数据集（表 51）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-DS-001 | 逻辑数据集 | IMPLEMENTED | Dataset/Version 模型和页面存在 |
| FR-DS-002 | 创建版本 | PARTIAL | Collection/Revision/历史版本存在；反馈批次来源待核对 |
| FR-DS-003 | 固定划分 | IMPLEMENTED | 固定种子和 Manifest 划分存在 |
| FR-DS-004 | 验证 | IMPLEMENTED | 已增加类别版本、checksum 和 CVAT label ID 校验；全形状覆盖待复测 |
| FR-DS-005 | 冻结 | IMPLEMENTED | checksum、Dataset Card、不可变状态存在 |
| FR-DS-006 | 版本比较 | IMPLEMENTED | 已增加类别和资产标签差异；分布精度待 E2E |
| FR-DS-007 | 使用关系 | AUDIT_PENDING | 待验证训练、评估、模型、部署全血缘 |
| FR-DS-008 | 废弃 | IMPLEMENTED | DEPRECATED 状态和新训练阻断存在 |

## 训练模板（表 54）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-TPL-001 | 模板定义 | IMPLEMENTED | 模板/版本/JSON Schema/输出协议模型存在 |
| FR-TPL-002 | 版本化 | IMPLEMENTED | 发布后锁定逻辑存在 |
| FR-TPL-003 | 参数表单 | PARTIAL | JSON Schema 表单存在；高级 YAML 编辑待核对 |
| FR-TPL-004 | 资源要求 | IMPLEMENTED | CPU/内存/GPU/显存字段存在 |
| FR-TPL-005 | 兼容矩阵 | PARTIAL | 兼容规则存在，完整阻断规则待验收 |
| FR-TPL-006 | 冒烟测试 | VERIFIED | RTX 3060 上完成 Faster R-CNN 前反向传播并生成真实 PT 制品 |

## 训练运行（表 56）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-TRN-001 | 创建训练 | VERIFIED | COCO128 冻结数据集 128 图/929 框真实训练通过（Run #13） |
| FR-TRN-002 | 训练校验 | IMPLEMENTED | 冻结、模板、资源和配额检查存在 |
| FR-TRN-003 | 提交 ClearML | PARTIAL | ClearML Adapter 存在；默认部署没有 ClearML Server/Agent |
| FR-TRN-004 | 状态同步 | IMPLEMENTED | 状态映射和 Orchestrator 存在 |
| FR-TRN-005 | 日志与曲线 | VERIFIED | Run #13 归集 epoch/loss/GPU 秒/峰值显存和执行日志 |
| FR-TRN-006 | 取消 | IMPLEMENTED | Provider 取消和最终状态对账逻辑存在 |
| FR-TRN-007 | 克隆 | IMPLEMENTED | 创建独立 TrainingRun 的克隆 API 存在 |
| FR-TRN-008 | 制品归集 | VERIFIED | 77.8MB model.pt、检测索引、指标、环境锁均带 SHA-256 |
| FR-TRN-009 | 失败诊断 | IMPLEMENTED | 标准错误分类存在，待故障注入 |

## LocalDocker（表 58）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-LOCAL-001 | 执行边界 | AUDIT_PENDING | 环境开关待核对 |
| FR-LOCAL-002 | Runner 隔离 | IMPLEMENTED | 独立 Runner 访问 Docker Engine，API 不挂载 docker.sock |
| FR-LOCAL-003 | 镜像白名单 | IMPLEMENTED | 模板版本镜像和 digest 约束存在 |
| FR-LOCAL-004 | 资源限制 | VERIFIED | read-only/network-none/cap-drop/CPU/内存/GPU/超时在 RTX 3060 实测 |
| FR-LOCAL-005 | 输入协议 | VERIFIED | Manifest、AnnotationRevision、Ontology checksum、图片和真实框已确定性分发 |
| FR-LOCAL-006 | 输出协议 | VERIFIED | result-manifest、权重、指标、环境锁和检测索引完成契约与 checksum 验证 |
| FR-LOCAL-007 | 取消与清理 | AUDIT_PENDING | 容器取消存在；临时目录和缓存清理待验证 |
| FR-LOCAL-008 | 三环境契约 | MISSING | 尚无 WSL2/Linux/ClearML 三方同模板契约证据 |

## 实验（表 60）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-EXP-001 | 列表筛选 | AUDIT_PENDING | 页面/API 存在，筛选维度待逐项核对 |
| FR-EXP-002 | 指标摘要 | VERIFIED | 真实训练 loss/GPU/图像/标注统计和评估指标已落库 |
| FR-EXP-003 | 实验对比 | IMPLEMENTED | 2–5 个运行比较 API/页面存在 |
| FR-EXP-004 | 曲线 | PARTIAL | 曲线 UI 存在；ClearML 完整曲线链缺失 |
| FR-EXP-005 | 克隆复现 | IMPLEMENTED | 克隆配置和差异字段存在 |
| FR-EXP-006 | 候选模型 | VERIFIED | Run #13 的真实 PT 权重注册为 ModelVersion #10 |

## 评估（表 62）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-EVAL-001 | 评估套件 | IMPLEMENTED | Suite/Run/阈值/切片/版本模型存在 |
| FR-EVAL-002 | 自动评估 | VERIFIED | Evaluation #18 自动加载 Run #13 权重并逐图执行 128 次真实推理 |
| FR-EVAL-003 | CV 样本分析 | VERIFIED | 真实 GT/预测计算 mAP 0.5404、IoU 0.8005、Precision 0.6466、Recall 0.4392 |
| FR-EVAL-005 | 切片 | PARTIAL | SavedSlice 存在；场景/设备/时间/Embedding 实值待补齐 |
| FR-EVAL-006 | 模型比较 | IMPLEMENTED | 比较控制面使用真实 EvaluationMetric；多版本浏览器对比仍待证据 |
| FR-EVAL-007 | 回归门禁 | IMPLEMENTED | 三类策略字段和判定逻辑存在 |
| FR-EVAL-008 | 困难样本回流 | VERIFIED | 真实推理 Trace 进入 FeedbackBatch #9 并生成回流标注任务 #27 |
| FR-EVAL-009 | FiftyOne 工作台 | VERIFIED | 内嵌 FiftyOne、项目隔离和 SSO 已部署 |

## 模型（表 65）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-MDL-001 | 逻辑模型 | IMPLEMENTED | Model/ModelVersion/Artifact 模型存在 |
| FR-MDL-002 | 注册版本 | VERIFIED | 真实 PT 制品及 SHA-256 注册为 ModelVersion #10 |
| FR-MDL-003 | 模型卡 | IMPLEMENTED | 自动模型卡字段和血缘存在 |
| FR-MDL-004 | 生命周期 | IMPLEMENTED | 状态机存在，待全部非法转换测试 |
| FR-MDL-005 | 许可证 | AUDIT_PENDING | 许可字段/阻断逻辑待逐项验证 |
| FR-MDL-006 | 血缘 | IMPLEMENTED | 数据集、训练、评估、审批、部署关联存在 |
| FR-MDL-007 | 导入导出 | AUDIT_PENDING | 受控 API 存在，权限/用途审计待实测 |
| FR-MDL-008 | 比较 | PARTIAL | 比较 API 存在；真实部署性能待补齐 |

## 审批（表 67）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-APR-001 | 提交审批 | IMPLEMENTED | 前置状态和不可变快照存在 |
| FR-APR-002 | 审批材料 | PARTIAL | 摘要存在；真实失败样本和回滚材料待补齐 |
| FR-APR-003 | 审批步骤 | PARTIAL | 单级流程存在；多级租户模板待核对 |
| FR-APR-004 | 职责分离 | IMPLEMENTED | 提交人自审阻断存在 |
| FR-APR-005 | 决定 | IMPLEMENTED | 决定状态和必填意见存在 |
| FR-APR-006 | 变更失效 | AUDIT_PENDING | 制品/配置/环境变化失效待故障测试 |

## 部署（表 69）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-DEP-001 | 创建部署 | VERIFIED | Deployment #10 / Revision #11 已加载 ModelVersion #10 并运行 |
| FR-DEP-002 | 资格校验 | IMPLEMENTED | 生产审批资格检查存在 |
| FR-DEP-003 | Revision | IMPLEMENTED | 配置变更创建不可变修订 |
| FR-DEP-004 | 健康检查 | VERIFIED | 探针返回 Torch/TorchVision、cuda:0、RTX 3060 和活动 Revision #11 |
| FR-DEP-005 | 发布 | IMPLEMENTED | MVP 全量发布和预留流量字段存在 |
| FR-DEP-006 | 回滚 | IMPLEMENTED | 历史 Revision 回滚和审计存在 |
| FR-DEP-007 | 停止/重启 | IMPLEMENTED | 操作 API 和审计存在 |
| FR-DEP-008 | Docker/FastAPI Provider | VERIFIED | GPU FastAPI Provider 下载、校验、缓存并执行真实 Faster R-CNN 权重 |

## 监控（表 72）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-MON-001 | 在线测试 | VERIFIED | 在线图片返回真实 giraffe/vase/potted plant 等框、类别和置信度 |
| FR-MON-002 | 基础设施指标 | PARTIAL | 摘要字段存在，真实 GPU/显存/网络采集待补齐 |
| FR-MON-003 | 服务指标 | PARTIAL | Trace/延迟存在，QPS/P50/P95/P99/队列需实测 |
| FR-MON-004 | 模型指标 | PARTIAL | 置信度/类别分布已来自真实权重；漂移基线和时间窗口仍待补齐 |
| FR-MON-005 | 版本标识 | IMPLEMENTED | Trace 关联 DeploymentRevision/ModelVersion |
| FR-MON-006 | 告警 | PARTIAL | 规则/确认存在；静默、通知和处置闭环待验证 |
| FR-MON-007 | 样本采集 | IMPLEMENTED | FeedbackPolicy 控制采样和保留 |

## 反馈闭环（表 74）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-FB-001 | 采样策略 | IMPLEMENTED | 随机、低置信、空结果、错误等策略字段存在 |
| FR-FB-002 | 隐私与保留 | PARTIAL | 保留和脱敏策略存在；敏感审批待实测 |
| FR-FB-003 | 去重 | PARTIAL | 精确/感知哈希存在；FiftyOne 相似去重待验证 |
| FR-FB-004 | 批次审核 | IMPLEMENTED | 接受/拒绝和原因流程存在 |
| FR-FB-005 | 创建返标任务 | IMPLEMENTED | 已修复从样本 DeploymentRevision 追溯类别版本 |
| FR-FB-006 | 闭环追踪 | PARTIAL | 血缘字段存在；真实再训练和再部署尚未闭环 |
| FR-FB-007 | 效果评估 | MISSING | 尚无可信真实模型的闭环前后切片/生产指标比较 |

## 资源（表 76）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-RES-001 | GPU 节点 | VERIFIED | 节点心跳与 Run #13 记录 RTX 3060、12GB、CUDA 12.6、驱动 591.86 |
| FR-RES-002 | 队列 | PARTIAL | 队列模型存在；ClearML 等待/运行/平均等待缺真实服务 |
| FR-RES-003 | 配额 | IMPLEMENTED | 租户/项目并发、GPU、时长、优先级字段存在 |
| FR-RES-004 | 任务占用 | PARTIAL | 关联字段存在；真实 GPU 设备占用待采集 |
| FR-RES-005 | 存储 | PARTIAL | MinIO 已部署；容量/对象/失败/生命周期全指标待验证 |
| FR-RES-006 | GPU 小时 | PARTIAL | 用量字段存在；真实训练计量待验证 |

## 集成（表 78）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-INT-001 | 实例注册 | IMPLEMENTED | 类型、URL、版本、区域、Secret Ref、健康字段存在 |
| FR-INT-002 | 连接测试 | AUDIT_PENDING | API 存在；需复测用户报告的实例测试失败 |
| FR-INT-003 | 兼容矩阵 | IMPLEMENTED | Adapter 版本规则和阻断模型存在 |
| FR-INT-004 | 同步异常 | IMPLEMENTED | 标准异常类型和列表存在 |
| FR-INT-005 | 重放/修复 | AUDIT_PENDING | 重放 API 存在，重绑/忽略/人工关闭待核对 |
| FR-INT-006 | 版本升级 | MISSING | 尚无配置备份、升级冒烟和回退的一体化流程证据 |

## 系统（表 80）

| 需求 | 功能 | 状态 | 当前证据或缺口 |
|---|---|---|---|
| FR-SYS-001 | 用户/租户/RBAC | IMPLEMENTED | 复用 Nimbus 系统模块和系统角色，新增 VisionAI 权限点 |
| FR-SYS-002 | 字典 | AUDIT_PENDING | AI 字典种子存在，待逐项对照 |
| FR-SYS-003 | 策略 | PARTIAL | 部分策略字段存在；Secret/下载/导出统一策略待核对 |
| FR-SYS-004 | 审计查询 | IMPLEMENTED | 多维查询和导出 API 存在 |
| FR-SYS-005 | 配置变更 | IMPLEMENTED | Before/After、变更人和敏感掩码审计存在 |

## 已确认的下一批产品缺口

1. ClearML 默认部署与契约：补齐 Server/Agent、队列和 LocalDocker/ClearML 同模板协议验证。
2. 项目模板复制补齐训练模板；补齐集成升级/回退和闭环收益评估。
3. 补齐视频元数据、近似重复、漂移基线、通知静默和完整资源趋势等 PARTIAL 项。

所有新增实现 SPEC 必须在 `docs/specs/README.md` 登记并同步创建 Linear issue；只有状态达到
`VERIFIED` 且证据路径存在时，才能计入产品完成度。

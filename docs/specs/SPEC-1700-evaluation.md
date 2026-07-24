# SPEC-1700 FiftyOne 评估与困难样本分析

状态：已完成
Linear：LOH-12

## 范围

覆盖 FR-EVAL-001..003、005..009：Evaluation Suite/Run、自动评估、FiftyOne 样本分析、SavedSlice、模型比较、回归门禁、困难样本回流入口和受控高级工作台。

## 已实现

- 评估套件固定 FROZEN DatasetVersion、切片、阈值、门禁策略与 Evaluator 版本。
- 异步评估运行输出 mAP、Precision、Recall、IoU、分类 AP、FP/FN 和延迟，并保存逐样本预测证据。
- MUST_PASS、ALLOW_REGRESSION、MANUAL_REVIEW 三种策略；可指定成功 EvaluationRun 作为基线，执行最大允许回退门禁。
- 独立 FiftyOne 1.15.0 服务将实际对象存储图片下载到持久卷，创建按租户/项目/运行隔离的持久数据集，写入 GT、预测、IoU、置信度、错误类型和切片字段。
- FP、FN、低置信度 SavedSlice 与高级工作台访问均经过项目角色授权并写审计。

## 验收证据

- E2E-CV-07：EvaluationRun #5 对 100 个真实唯一图片完成评估并同步 FiftyOne 数据集 `tenant-1-project-1-evaluation-5`。
- 基线 EvaluationRun #1，候选与基线 mAP 差值 0，最大允许回退 0.02，最终门禁 PASSED。
- 数据库保存 100 条样本、8+ 指标、3 个 SavedSlice；FiftyOne 管理 API 和 App 健康检查通过。

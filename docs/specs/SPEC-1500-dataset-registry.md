# SPEC-1500 数据集注册表、Manifest 与不可变版本

状态：已完成
Linear：LOH-10

## 范围

覆盖 FR-DS-001..008：Dataset、DatasetVersion、固定种子划分、验证、冻结、比较、使用关系、废弃，生成 Manifest、Dataset Card、校验和和血缘。

## 状态与不变量

DRAFT→VALIDATING→READY→FROZEN→DEPRECATED；FROZEN 后 Manifest、AnnotationRevision、OntologyVersion 和 Split 不可修改，任何变化创建新版本。

## 验收

- E2E-CV-05：对象存在、标注、类别、重复、泄漏、空标注和越界框验证完整。
- 同一版本可复现相同 Split 与 checksum；冻结后写操作返回 STATE 错误。
- 版本比较覆盖样本、类别分布、来源和标签差异。

# SPEC-1400 CVAT 标注与 AI 预标注

状态：已完成
Linear：LOH-9

## 范围

覆盖 FR-ANN-001..009、FR-PRE-001..006：CVAT Provider、用户映射、创建/同步/审核/返工/导出、不可变 AnnotationRevision，以及经批准模型的预标注、导入幂等和效率统计。

## 状态

DRAFT→PREPARING→PREANNOTATING?→READY→ANNOTATING→REVIEWING→APPROVED/REJECTED→EXPORTING→CLOSED，并支持 FAILED/CANCELLED。

## 验收

- E2E-CV-03/04：真实 CVAT 对象、Binding、用户映射、状态同步、审核与带校验和的快照。
- E2E-FAIL-01：CVAT 中断后可恢复，重试不重复创建任务或标注。
- 打开 CVAT 前校验权限并审计；历史 Revision 不覆盖。
- 预标注接受/删除/修改/新增率和平均修正时间可追溯。

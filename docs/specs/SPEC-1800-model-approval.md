# SPEC-1800 模型注册、供应链与审批

状态：已完成
Linear：LOH-13

## 范围

覆盖 FR-MDL-001..008、FR-APR-001..006：Model/ModelVersion、原生/ONNX/TensorRT 制品、Model Card、血缘、比较、受控导入导出、供应链与许可证，以及数据冻结/模型资格/部署/回滚审批。

## 规则

- ModelVersion 只从成功训练或受控导入创建，注册后制品与配置不可替换。
- 生命周期 DRAFT→EVALUATED→REVIEW_PENDING→APPROVED→STAGING→CANARY→PRODUCTION→RETIRED。
- 审批保存不可变证据快照，默认提交人不可审批自己的生产资格；证据或环境变化使审批失效。

## 验收

- E2E-CV-08：未批准模型不能生产；审批决定不可篡改且意见必填。
- 从部署反向查询 DatasetVersion、TrainingRun、EvaluationRun、Approval 与供应链清单。
- 许可证 REVIEW_REQUIRED/NOT_ALLOWED 正确阻断生产。

## 实现与证据

- 从成功训练和受控导入注册 ModelVersion，保存 4 类制品、Model Card、哈希和完整数据/训练/评估谱系。
- 实现模型比较、Manifest 导出、退役、生命周期与许可证门禁。
- 管理员审批本人提交被 409 阻断；独立 `visionai-reviewer` 审批成功，不可变证据快照以精确字节和哈希保留。

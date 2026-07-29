# SPEC-2290 项目模板、供应链审批与集成升级回退

状态：已完成
Linear：LOH-26  
设计基线：FR-PRJ-006、FR-MDL-005/007、FR-APR-002/003/006、
FR-INT-003/005/006、FR-SYS-003

## 目标

补齐配置复用、模型供应链、审批模板和集成生命周期，使所有复制、导入、导出、审批、
升级和回退动作有明确策略、不可变快照、权限与审计证据。

## 范围

1. 项目模板复制类别体系、训练模板/版本和非敏感 Provider 配置；排除 Secret、资产、
   标注、数据集、运行和部署。
2. 模型许可证策略、用途声明、导入制品校验、导出审批和下载审计。
3. 审批材料包含真实失败样本、回滚方案、制品/配置/环境 checksum；支持租户级多步骤模板。
4. 审批后的任一受控输入变化会使审批失效，禁止继续生产部署。
5. 集成升级前创建配置快照，执行兼容检查与 smoke；失败自动回退并保存差异。
6. 同步异常支持重放、重绑、忽略和人工关闭，所有动作记录 Before/After。

## 验收

- 项目复制后模板/类别可用，敏感和运行数据计数为零。
- 一次集成升级成功、一次故障注入自动回退，配置 checksum 可复算。
- 多步骤审批、职责分离和变更失效均有 API、UI、自动化和浏览器证据。
- Swagger、路由契约、迁移、权限与审计全部通过。

## 实现与验收结果

- `ProjectClone` 同步复制类别体系、训练模板及全部模板版本；Project #22 的验收克隆
  Project #26 得到 2 个训练模板、1 套类别体系、0 个训练运行，`secretRefs={}`。
- 模型版本新增 DATA、PRETRAINED_WEIGHT、FRAMEWORK、MODEL_ARTIFACT 四类许可与用途声明。
  受控导入逐字节读取平台对象存储，复算 SHA-256 和大小，不接受未治理的外部 URI。
- 租户审批模板支持 1–10 个顺序步骤及系统角色；审批冻结真实失败样本、门禁、许可、
  制品、风险、回滚方案、模板和目标环境，并生成输入指纹。审批 #11 在用途声明变化后
  自动变为 `INVALIDATED`；审批 #12 由 Reviewer User #3 和 Approver User #7 顺序完成。
- 模型导出必须处于 `APPROVED`，重新计算审批指纹并记录用途、下载人、制品数和审批 ID；
  生产部署复用同一指纹校验。
- ClearML 集成修订 #5 通过兼容矩阵和 smoke 后生效；修订 #6 使用故障注入，自动恢复
  2.4.1 配置并保存 Before/After SHA-256、diff、smoke 与回退原因。
- 同步事件支持 `REPLAY`、`REBIND`、`IGNORE`、`CLOSE`，事件 #22 已以 `IGNORE`
  完成 Before/After 审计。
- 自动化：`scripts/e2e-supply-chain.ps1` 全部断言通过；`go test ./...`、前端
  `ts:check`、ESLint、Stylelint、Prettier、生产构建和 `docker compose config` 通过。
- 浏览器证据：
  - `docs/evidence/visionai-integration-upgrade-governance.png`
  - `docs/evidence/visionai-integration-revision-rollback-checksums.png`
  - `docs/evidence/visionai-multistep-approval-invalidation-registry.png`
  - `docs/evidence/visionai-model-artifact-license-evidence.png`
  - `docs/evidence/visionai-sequential-approval-trail.png`

# SPEC-2260 标签分类与版本化类别体系治理

状态：已完成
Linear：LOH-23
设计基线：VisionAI 产品设计说明书 V1.3（第 24、28–31、49 页）

## 问题

当前资产标签是无类型自由文本；标注任务和数据集版本只保存手填的
`ontologyVersion` 字符串，类别由创建任务时临时传入。平台不存在设计要求的
`ontology`、`ontology_version` 领域对象，因此无法复用类别、冻结类别版本、
校验 CVAT 类别映射或稳定追溯数据集。

## 覆盖需求

- FR-DATA-006：业务、场景、来源标签及按筛选保存 Collection。
- FR-PRJ-006：项目复制类别体系。
- FR-ANN-001：创建任务必须选择类别体系版本。
- FR-ANN-007：AnnotationRevision 保存类别版本、映射和校验和。
- FR-DS-004：校验标注与类别版本匹配。
- FR-DS-006：数据集版本比较类别分布与标签差异。
- 领域模型与表设计：`ontology`、`ontology_version`。
- 不可变规则：DatasetVersion 冻结后类别版本不可修改。

## 领域模型

1. `Ontology`：项目级逻辑类别体系，保存编码、名称、任务类型和生命周期。
2. `OntologyVersion`：不可变发布版本；草稿可编辑，发布后以 SHA-256 锁定。
3. `OntologyLabel`：类别编码、名称、颜色、形状类型和排序。
4. `OntologyAttribute`：类别属性名称、输入类型、可选值、默认值和可变规则。
5. `AssetTagDefinition`：项目级业务/场景/来源标签定义。
6. `AssetTag`：资产与标签定义的受治理关联。

## API 与交互

- 类别体系列表、创建、详情、草稿版本、类别/属性编辑、发布和废弃。
- 发布版本列表供标注任务与数据集版本选择。
- 标注任务不再接受自由文本类别；CVAT Label/Attribute 由版本快照生成。
- AnnotationRevision、DatasetVersion、Manifest 和 Dataset Card 保存版本 ID 与校验和。
- 标签定义 CRUD、资产批量打标、资产按标签筛选、标签使用统计。
- “标签与类别”统一页面负责类别体系与资产标签字典；数据资产页负责实际打标。

## 兼容与迁移

- 保留历史字符串字段用于只读展示和迁移，不再作为新对象的事实源。
- 启动迁移根据历史 `AnnotationTask.Labels` 创建 Legacy OntologyVersion，并回填引用。
- 历史 DatasetVersion 通过相同项目和版本名匹配回填；无法匹配时建立只读 Legacy 版本。
- 迁移幂等，不修改已冻结 Manifest 或历史 AnnotationRevision 对象。

## 验收

1. 发布后的 OntologyVersion、类别和属性无法修改。
2. 新建 AnnotationTask 必须选择当前项目已发布且任务类型兼容的版本。
3. CVAT Task 的 Label/Attribute 与版本快照一致。
4. DatasetVersion 与 AnnotationRevision 类别版本不一致时验证失败。
5. COCO80 可导入、发布并用于真实 CVAT 任务。
6. 资产可使用业务/场景/来源标签筛选、批量打标并保存为 Collection。
7. 项目复制类别体系但不复制资产、标注和敏感数据。
8. Swagger、路由契约、Go 测试、前端 lint/typecheck/build、Compose 与浏览器验收通过。

## 实现与验收证据

- COCO 2017 Detection Ontology v1 已发布并以 SHA-256 锁定，共 80 类。
- COCO128 标注任务 #26 使用该版本创建 CVAT 标签，导出 Revision #14，共 929 个矩形框。
- DatasetVersion #16 冻结并保存 OntologyVersion ID/checksum，训练 #13、评估 #18 和部署 #10 延续同一语义血缘。
- 项目页面、资产批量标签、Collection、标注任务与数据集页面均改为使用受治理版本，不再接受自由文本类别作为事实源。
- COCO128 项目已建立“公开基准（业务）”“多场景（场景）”“Ultralytics COCO128（来源）”
  三类受治理标签，分别实际应用到全部 128 张资产；三个标签联合筛选固化为已冻结
  Collection #29，资产数 128。
- 浏览器证据：`docs/evidence/coco128-ontology-80.png`、
  `docs/evidence/coco128-cvat-embedded.png`、
  `docs/evidence/coco128-governed-tag-dictionary.png`、
  `docs/evidence/coco128-tag-filtered-frozen-collection.png`。
- 机器可读验收报告：`docs/evidence/coco128-acceptance-20260727-113243.json`。

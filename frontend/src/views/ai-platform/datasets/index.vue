<template>
  <main class="dataset-registry">
    <header class="page-header">
      <div>
        <span class="eyebrow">REPRODUCIBLE INPUTS</span>
        <h1>数据集注册表</h1>
        <p>固定资产、标注快照和切分策略，生成可追溯的 Manifest 与 Dataset Card。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="datasetVisible = true">
        <Icon icon="lucide:plus" :size="16" />新建数据集
      </el-button>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadDatasets">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="`${project.name} · ${project.code}`"
          :value="project.id"
        />
      </el-select>
      <el-button @click="loadDatasets"><Icon icon="lucide:refresh-cw" :size="15" />刷新</el-button>
    </section>

    <section v-loading="loading" class="dataset-layout">
      <aside>
        <button
          v-for="dataset in datasets"
          :key="dataset.id"
          type="button"
          :class="{ active: selectedDataset?.id === dataset.id }"
          @click="selectDataset(dataset)"
        >
          <span>{{ dataset.taskType }}</span>
          <strong>{{ dataset.name }}</strong>
          <small>{{ dataset.versionCount }} 个版本 · {{ dataset.frozenCount }} 已冻结</small>
        </button>
        <div v-if="!datasets.length" class="aside-empty">暂无逻辑数据集</div>
      </aside>

      <section class="versions">
        <header v-if="selectedDataset">
          <div>
            <h2>{{ selectedDataset.name }}</h2>
            <p>{{ selectedDataset.description }}</p>
          </div>
          <el-button type="primary" @click="openVersionDialog">创建版本</el-button>
        </header>
        <article v-for="version in versions" :key="version.id" class="version-card">
          <div class="version-main">
            <div class="version-title">
              <strong>{{ version.semanticVersion }}</strong>
              <el-tag :type="statusType(version.status)">{{ version.status }}</el-tag>
              <span
                >#{{ version.id }} · {{ sourceLabel(version.sourceType) }} #{{
                  version.sourceId
                }}</span
              >
            </div>
            <div class="split">
              <span
                ><b>{{ version.itemCount }}</b> 总样本</span
              >
              <span
                ><b>{{ version.trainCount }}</b> Train</span
              >
              <span
                ><b>{{ version.validationCount }}</b> Val</span
              >
              <span
                ><b>{{ version.testCount }}</b> Test</span
              >
            </div>
            <code v-if="version.checksum">SHA-256 {{ version.checksum }}</code>
          </div>
          <div class="version-actions">
            <el-button v-if="version.status === 'DRAFT'" @click="validateVersion(version)">
              运行验证
            </el-button>
            <el-button
              v-if="version.status === 'READY'"
              type="primary"
              @click="freezeVersion(version)"
            >
              冻结版本
            </el-button>
            <el-button @click="showDetail(version)">质量报告</el-button>
            <el-button
              v-if="version.status === 'FROZEN'"
              type="danger"
              link
              @click="deprecate(version)"
            >
              废弃
            </el-button>
          </div>
        </article>
        <div v-if="selectedDataset && !versions.length" class="empty-state">尚未创建版本</div>
        <div v-if="!selectedDataset" class="empty-state">选择或创建一个逻辑数据集</div>
      </section>
    </section>

    <el-dialog v-model="datasetVisible" title="新建逻辑数据集" width="520">
      <el-form label-position="top">
        <el-form-item label="名称" required><el-input v-model="datasetForm.name" /></el-form-item>
        <el-form-item label="任务类型">
          <el-select v-model="datasetForm.taskType" class="full">
            <el-option label="目标检测" value="CV_DETECTION" />
          </el-select>
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="datasetForm.description" type="textarea" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="datasetVisible = false">取消</el-button>
        <el-button type="primary" @click="submitDataset">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="versionVisible" title="创建可复现版本" width="720">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="数据来源" required>
            <el-select v-model="versionForm.sourceType" class="full" @change="resetSource">
              <el-option label="冻结资产集合" value="COLLECTION" />
              <el-option label="标注修订快照" value="ANNOTATION_REVISION" />
              <el-option label="历史数据集版本" value="PARENT_VERSION" />
              <el-option label="反馈闭环批次" value="FEEDBACK_BATCH" />
            </el-select>
          </el-form-item>
          <el-form-item :label="sourceInputLabel" required>
            <el-select v-model="versionForm.sourceId" class="full" filterable>
              <el-option
                v-for="option in sourceOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </el-form-item>
        </div>
        <el-alert :title="sourceHelp" type="info" :closable="false" show-icon class="source-help" />
        <div class="form-grid">
          <el-form-item label="关联标注修订">
            <el-select
              v-model="versionForm.annotationRevisionId"
              class="full"
              clearable
              placeholder="可选；从标注来源时自动带入"
            >
              <el-option
                v-for="revision in annotationRevisions"
                :key="revision.value"
                :label="revision.label"
                :value="revision.value"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="类别体系版本" required>
            <el-select
              v-model="versionForm.ontologyVersionId"
              class="full"
              placeholder="选择已发布版本"
            >
              <el-option
                v-for="version in ontologyVersions"
                :key="version.id"
                :label="`${version.ontologyName} · ${version.semanticVersion} · ${version.labelCount} 类`"
                :value="version.id"
              />
            </el-select>
          </el-form-item>
        </div>

        <el-divider content-position="left">固定切分</el-divider>
        <div class="form-grid">
          <el-form-item label="切分方式">
            <el-select v-model="versionForm.splitMode" class="full">
              <el-option label="固定种子比例" value="RATIO" />
              <el-option label="资产字段规则" value="RULE" />
              <el-option label="外部分配列表" value="EXTERNAL_LIST" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="versionForm.splitMode === 'RATIO'" label="固定随机种子">
            <el-input-number v-model="versionForm.splitSeed" :min="1" />
          </el-form-item>
        </div>
        <div v-if="versionForm.splitMode === 'RATIO'" class="form-grid three">
          <el-form-item label="Train">
            <el-input-number v-model="versionForm.train" :min="0" :max="1" :step="0.05" />
          </el-form-item>
          <el-form-item label="Val">
            <el-input-number v-model="versionForm.val" :min="0" :max="1" :step="0.05" />
          </el-form-item>
          <el-form-item label="Test">
            <el-input-number v-model="versionForm.test" :min="0" :max="1" :step="0.05" />
          </el-form-item>
        </div>
        <el-form-item v-else-if="versionForm.splitMode === 'RULE'" label="规则 JSON">
          <el-input
            v-model="versionForm.rulesText"
            type="textarea"
            :rows="7"
            placeholder='按顺序匹配，例如 [{"field":"businessScene","operator":"EQ","value":"night","split":"TEST"},{"field":"width","operator":"GTE","value":"0","split":"TRAIN"}]'
          />
          <small class="form-hint">
            字段支持
            filename、contentType、mediaKind、sourceDevice、businessScene、width、height、size 和
            metadata.字段；每个资产必须命中一条规则。
          </small>
        </el-form-item>
        <el-form-item v-else label="外部分配列表">
          <el-input
            v-model="versionForm.externalListText"
            type="textarea"
            :rows="7"
            placeholder="每行一个资产：assetId,TRAIN&#10;1001,TRAIN&#10;1002,VAL&#10;1003,TEST"
          />
          <small class="form-hint">来源中的每个资产都必须且只能分配到 TRAIN、VAL 或 TEST。</small>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="versionVisible = false">取消</el-button>
        <el-button type="primary" @click="submitVersion">创建版本</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="数据集质量与血缘" size="55%">
      <template v-if="detail">
        <section class="detail-summary">
          <article
            ><small>状态</small><strong>{{ detail.version.status }}</strong></article
          >
          <article>
            <small>阻断问题</small>
            <strong>{{ detail.issues.filter((item) => item.severity === 'ERROR').length }}</strong>
          </article>
          <article>
            <small>警告</small>
            <strong>{{
              detail.issues.filter((item) => item.severity === 'WARNING').length
            }}</strong>
          </article>
          <article
            ><small>使用关系</small><strong>{{ detail.usage.length }}</strong></article
          >
        </section>
        <el-descriptions v-if="detail.version.manifestUri" :column="1" border>
          <el-descriptions-item label="Manifest">{{
            detail.version.manifestUri
          }}</el-descriptions-item>
          <el-descriptions-item label="Dataset Card">
            {{ detail.version.datasetCardUri }}
          </el-descriptions-item>
          <el-descriptions-item label="校验和">
            <code>{{ detail.version.checksum }}</code>
          </el-descriptions-item>
        </el-descriptions>
        <el-table :data="detail.issues" class="issue-table">
          <el-table-column label="级别" width="90">
            <template #default="{ row }">
              <el-tag :type="row.severity === 'ERROR' ? 'danger' : 'warning'">
                {{ row.severity }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="assetId" label="资产" width="90" />
          <el-table-column prop="code" label="代码" width="190" />
          <el-table-column prop="message" label="问题" />
          <el-table-column prop="remediation" label="修复建议" />
        </el-table>
      </template>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import {
  getAnnotationTask,
  getAnnotationTasks,
  getAssetCollections,
  type AssetCollection
} from '@/api/ai-platform/annotations'
import {
  createDataset,
  createDatasetVersion,
  deprecateDatasetVersion,
  freezeDatasetVersion,
  getDatasets,
  getDatasetVersion,
  getDatasetVersions,
  validateDatasetVersion,
  type Dataset,
  type DatasetVersion
} from '@/api/ai-platform/datasets'
import { getFeedbackBatches, type FeedbackBatch } from '@/api/ai-platform/feedback'
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  getPublishedOntologyVersions,
  type SelectableOntologyVersion
} from '@/api/ai-platform/ontology'

defineOptions({ name: 'VisionAIDatasets' })

type SourceOption = { value: number; label: string }
type RevisionOption = SourceOption & { ontologyVersionId: number }

const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const datasets = ref<Dataset[]>([])
const selectedDataset = ref<Dataset>()
const versions = ref<DatasetVersion[]>([])
const collections = ref<AssetCollection[]>([])
const annotationRevisions = ref<RevisionOption[]>([])
const feedbackBatches = ref<FeedbackBatch[]>([])
const ontologyVersions = ref<SelectableOntologyVersion[]>([])
const loading = ref(false)
const datasetVisible = ref(false)
const versionVisible = ref(false)
const detailVisible = ref(false)
const detail = ref<Awaited<ReturnType<typeof getDatasetVersion>>>()
const datasetForm = reactive({ name: '', taskType: 'CV_DETECTION', description: '' })
const versionForm = reactive({
  sourceType: 'COLLECTION',
  sourceId: undefined as number | undefined,
  annotationRevisionId: undefined as number | undefined,
  ontologyVersionId: undefined as number | undefined,
  splitMode: 'RATIO',
  splitSeed: 20260721,
  train: 0.8,
  val: 0.1,
  test: 0.1,
  rulesText:
    '[\n  {"field":"businessScene","operator":"EQ","value":"night","split":"TEST"},\n  {"field":"width","operator":"GTE","value":"0","split":"TRAIN"}\n]',
  externalListText: ''
})

const frozenCollections = computed(() => collections.value.filter((item) => item.frozen))
const eligibleFeedbackBatches = computed(() =>
  feedbackBatches.value.filter((item) => ['ANNOTATING', 'COMPLETED'].includes(item.status))
)
const sourceOptions = computed<SourceOption[]>(() => {
  switch (versionForm.sourceType) {
    case 'ANNOTATION_REVISION':
      return annotationRevisions.value
    case 'PARENT_VERSION':
      return versions.value
        .filter((item) => ['FROZEN', 'DEPRECATED'].includes(item.status))
        .map((item) => ({
          value: item.id,
          label: `${item.semanticVersion} · ${item.itemCount} 个资产 · ${item.status}`
        }))
    case 'FEEDBACK_BATCH':
      return eligibleFeedbackBatches.value.map((item) => ({
        value: item.id,
        label: `#${item.id} ${item.name} · ${item.sampleCount} 个样本 · ${item.status}`
      }))
    default:
      return frozenCollections.value.map((item) => ({
        value: item.id,
        label: `${item.name} · ${item.assetCount} 个资产`
      }))
  }
})
const sourceInputLabel = computed(
  () =>
    ({
      COLLECTION: '冻结资产集合',
      ANNOTATION_REVISION: '标注修订',
      PARENT_VERSION: '历史版本',
      FEEDBACK_BATCH: '反馈批次'
    })[versionForm.sourceType] || '来源'
)
const sourceHelp = computed(
  () =>
    ({
      COLLECTION: '从已冻结的资产集合创建独立版本，可选关联一个标注修订。',
      ANNOTATION_REVISION: '使用已审批导出的标注快照及其任务资产，类别体系自动继承。',
      PARENT_VERSION: '从已冻结或已废弃的同一逻辑数据集版本派生，保留父版本血缘。',
      FEEDBACK_BATCH: '仅可使用已审核进入返标流程或已完成收益评估的反馈批次。'
    })[versionForm.sourceType] || ''
)
const sourceLabel = (value: string) =>
  ({
    COLLECTION: '资产集合',
    ANNOTATION_REVISION: '标注修订',
    PARENT_VERSION: '历史版本',
    FEEDBACK_BATCH: '反馈批次'
  })[value] || value
const statusType = (status: string) =>
  status === 'FROZEN'
    ? 'success'
    : status === 'READY'
      ? 'primary'
      : status === 'DEPRECATED'
        ? 'info'
        : status === 'VALIDATING'
          ? 'warning'
          : undefined

const loadAnnotationRevisions = async (targetProjectId: number) => {
  const taskPage = await getAnnotationTasks(targetProjectId, { pageNo: 1, pageSize: 100 })
  const details = await Promise.all(
    taskPage.list
      .filter((task) => task.currentRevisionId)
      .map((task) => getAnnotationTask(targetProjectId, task.id))
  )
  annotationRevisions.value = details.flatMap((detail) =>
    detail.revisions.map((revision) => ({
      value: revision.id,
      ontologyVersionId: detail.task.ontologyVersionId,
      label: `${detail.task.name} · Revision ${revision.revisionNo} · ${revision.annotationCount} 条标注`
    }))
  )
}

const loadDatasets = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    selectedDataset.value = undefined
    const [rows, collectionRows, ontologyRows, batchRows] = await Promise.all([
      getDatasets(projectId.value),
      getAssetCollections(projectId.value),
      getPublishedOntologyVersions(projectId.value),
      getFeedbackBatches(projectId.value),
      loadAnnotationRevisions(projectId.value)
    ])
    datasets.value = rows
    collections.value = collectionRows
    ontologyVersions.value = ontologyRows
    feedbackBatches.value = batchRows
    versionForm.ontologyVersionId = ontologyVersions.value[0]?.id
    if (rows[0]) await selectDataset(rows[0])
    resetSource()
  } finally {
    loading.value = false
  }
}
const selectDataset = async (dataset: Dataset) => {
  selectedDataset.value = dataset
  versions.value = await getDatasetVersions(projectId.value!, dataset.id)
}
const openVersionDialog = () => {
  resetSource()
  versionVisible.value = true
}
const resetSource = () => {
  versionForm.sourceId = sourceOptions.value[0]?.value
  if (versionForm.sourceType === 'ANNOTATION_REVISION') {
    versionForm.annotationRevisionId = versionForm.sourceId
    const revision = annotationRevisions.value.find((item) => item.value === versionForm.sourceId)
    if (revision) versionForm.ontologyVersionId = revision.ontologyVersionId
  }
}
watch(
  () => versionForm.sourceId,
  (sourceId) => {
    if (versionForm.sourceType !== 'ANNOTATION_REVISION') return
    versionForm.annotationRevisionId = sourceId
    const revision = annotationRevisions.value.find((item) => item.value === sourceId)
    if (revision) versionForm.ontologyVersionId = revision.ontologyVersionId
  }
)
const submitDataset = async () => {
  if (!projectId.value || !datasetForm.name.trim()) {
    message.warning('请填写数据集名称')
    return
  }
  const row = await createDataset(projectId.value, { ...datasetForm, ownerUserId: 1 })
  datasetVisible.value = false
  await loadDatasets()
  await selectDataset(row)
}
const parseRules = () => {
  if (versionForm.splitMode !== 'RULE') return []
  const parsed = JSON.parse(versionForm.rulesText)
  if (!Array.isArray(parsed)) throw new Error('规则 JSON 必须是数组')
  return parsed
}
const parseExternalAssignments = () => {
  const assignments: Record<string, string> = {}
  if (versionForm.splitMode !== 'EXTERNAL_LIST') return assignments
  for (const rawLine of versionForm.externalListText.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line) continue
    const [assetId, split] = line.split(/[\s,;\t]+/)
    if (!assetId || !['TRAIN', 'VAL', 'TEST'].includes((split || '').toUpperCase())) {
      throw new Error(`外部分配格式错误：${line}`)
    }
    assignments[assetId] = split.toUpperCase()
  }
  return assignments
}
const submitVersion = async () => {
  if (
    !projectId.value ||
    !selectedDataset.value ||
    !versionForm.sourceId ||
    !versionForm.ontologyVersionId
  ) {
    message.warning('请选择完整的数据来源与类别体系版本')
    return
  }
  try {
    await createDatasetVersion(projectId.value, selectedDataset.value.id, {
      sourceType: versionForm.sourceType,
      sourceId: versionForm.sourceId,
      parentId: versionForm.sourceType === 'PARENT_VERSION' ? versionForm.sourceId : 0,
      annotationRevisionId: versionForm.annotationRevisionId || 0,
      ontologyVersionId: versionForm.ontologyVersionId,
      splitMode: versionForm.splitMode,
      splitSeed: versionForm.splitSeed,
      split: { TRAIN: versionForm.train, VAL: versionForm.val, TEST: versionForm.test },
      splitRules: parseRules(),
      externalAssignments: parseExternalAssignments()
    })
    versionVisible.value = false
    message.success('数据来源、切分策略与固定血缘已写入新版本')
    await selectDataset(selectedDataset.value)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '创建版本失败')
  }
}
const validateVersion = async (version: DatasetVersion) => {
  await validateDatasetVersion(projectId.value!, version.id)
  message.success('七类质量检查已进入队列')
  setTimeout(() => selectDataset(selectedDataset.value!), 1800)
}
const freezeVersion = async (version: DatasetVersion) => {
  await freezeDatasetVersion(projectId.value!, version.id)
  message.success('Manifest 与 Dataset Card 正在生成')
  setTimeout(() => selectDataset(selectedDataset.value!), 1800)
}
const showDetail = async (version: DatasetVersion) => {
  detail.value = await getDatasetVersion(projectId.value!, version.id)
  detailVisible.value = true
}
const deprecate = async (version: DatasetVersion) => {
  const result = await message.prompt('请输入废弃原因', '废弃数据集版本')
  await deprecateDatasetVersion(projectId.value!, version.id, result.value)
  await selectDataset(selectedDataset.value!)
}

onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadDatasets()
})
</script>

<style scoped lang="scss">
.dataset-registry {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: linear-gradient(145deg, rgb(22 119 255 / 5%), transparent 42%);
}

.page-header,
.toolbar,
.versions header,
.version-card,
.version-title,
.split,
.version-actions {
  display: flex;
  align-items: center;
}

.page-header,
.versions header,
.version-card {
  justify-content: space-between;
}

.page-header {
  align-items: flex-start;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 5px 0;
  font-size: 30px;
  letter-spacing: -1px;
}

.page-header p,
.versions header p {
  margin: 0;
  color: var(--text-secondary);
}

.eyebrow {
  color: var(--el-color-primary);
  font-size: 12px;
  font-weight: 750;
  letter-spacing: 1.4px;
}

.toolbar {
  gap: 10px;
  padding: 14px;
  margin-bottom: 16px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
}

.toolbar .el-select {
  width: 320px;
}

.dataset-layout {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  gap: 18px;
  min-height: 520px;
}

aside {
  padding: 10px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
}

aside button {
  display: grid;
  width: 100%;
  padding: 14px;
  margin-bottom: 8px;
  color: inherit;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 11px;
  cursor: pointer;
}

aside button:hover,
aside button.active {
  background: var(--el-color-primary-light-9);
  border-color: var(--el-color-primary-light-7);
}

aside span,
aside small,
.version-title span,
.form-hint {
  color: var(--text-secondary);
  font-size: 12px;
}

aside strong {
  margin: 4px 0;
}

.aside-empty,
.empty-state {
  padding: 48px 12px;
  color: var(--text-secondary);
  text-align: center;
}

.versions {
  min-width: 0;
}

.versions header {
  margin-bottom: 12px;
}

.versions h2 {
  margin: 0 0 4px;
}

.version-card {
  gap: 18px;
  padding: 18px;
  margin-bottom: 12px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
}

.version-main {
  min-width: 0;
}

.version-title,
.split,
.version-actions {
  gap: 10px;
}

.version-title strong {
  font-size: 18px;
}

.split {
  margin: 13px 0;
}

.split span {
  padding: 7px 10px;
  color: var(--text-secondary);
  background: var(--el-fill-color-light);
  border-radius: 8px;
}

.split b {
  margin-right: 3px;
  color: var(--text-primary);
}

code {
  overflow-wrap: anywhere;
  color: var(--el-color-primary);
  font-size: 12px;
}

.full {
  width: 100%;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.form-grid.three {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.source-help {
  margin: -4px 0 18px;
}

.form-hint {
  display: block;
  margin-top: 7px;
  line-height: 1.6;
}

.detail-summary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 20px;
}

.detail-summary article {
  display: grid;
  gap: 7px;
  padding: 16px;
  background: var(--el-fill-color-light);
  border-radius: 12px;
}

.detail-summary small {
  color: var(--text-secondary);
}

.detail-summary strong {
  font-size: 24px;
}

.issue-table {
  margin-top: 18px;
}

@media (max-width: 980px) {
  .dataset-layout {
    grid-template-columns: 1fr;
  }

  .version-card,
  .page-header {
    align-items: stretch;
    flex-direction: column;
  }

  .form-grid,
  .form-grid.three,
  .detail-summary {
    grid-template-columns: 1fr;
  }
}
</style>

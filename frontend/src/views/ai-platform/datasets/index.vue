<template>
  <main class="dataset-registry">
    <header class="page-header">
      <div>
        <span class="eyebrow">REPRODUCIBLE INPUTS</span>
        <h1>数据集注册表</h1>
        <p>固定资产、标注快照、划分、Manifest 和 Dataset Card，形成可复现训练输入。</p>
      </div>
      <div class="actions">
        <el-button type="primary" :disabled="!projectId" @click="datasetVisible = true">
          <Icon icon="lucide:plus" :size="16" />新建数据集
        </el-button>
      </div>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadDatasets">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="project.name"
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
          <div
            ><h2>{{ selectedDataset.name }}</h2
            ><p>{{ selectedDataset.description }}</p></div
          >
          <el-button type="primary" @click="versionVisible = true">创建版本</el-button>
        </header>
        <article v-for="version in versions" :key="version.id" class="version-card">
          <div class="version-main">
            <div class="version-title">
              <strong>{{ version.semanticVersion }}</strong>
              <el-tag :type="statusType(version.status)">{{ version.status }}</el-tag>
              <span>#{{ version.id }} · {{ version.sourceType }} #{{ version.sourceId }}</span>
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
            <el-button v-if="version.status === 'DRAFT'" @click="validateVersion(version)"
              >运行验证</el-button
            >
            <el-button
              v-if="version.status === 'READY'"
              type="primary"
              @click="freezeVersion(version)"
              >冻结版本</el-button
            >
            <el-button @click="showDetail(version)">质量报告</el-button>
            <el-button
              v-if="version.status === 'FROZEN'"
              type="danger"
              link
              @click="deprecate(version)"
              >废弃</el-button
            >
          </div>
        </article>
        <div v-if="selectedDataset && !versions.length" class="empty-state">尚未创建版本</div>
        <div v-if="!selectedDataset" class="empty-state">选择或创建一个逻辑数据集</div>
      </section>
    </section>

    <el-dialog v-model="datasetVisible" title="新建逻辑数据集" width="520">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="datasetForm.name" /></el-form-item>
        <el-form-item label="任务类型"
          ><el-select v-model="datasetForm.taskType" class="full"
            ><el-option label="目标检测" value="CV_DETECTION" /></el-select
        ></el-form-item>
        <el-form-item label="说明"
          ><el-input v-model="datasetForm.description" type="textarea"
        /></el-form-item>
      </el-form>
      <template #footer
        ><el-button @click="datasetVisible = false">取消</el-button
        ><el-button type="primary" @click="submitDataset">创建</el-button></template
      >
    </el-dialog>

    <el-dialog v-model="versionVisible" title="创建可复现版本" width="600">
      <el-form label-position="top">
        <el-form-item label="冻结资产集合">
          <el-select v-model="versionForm.sourceId" class="full">
            <el-option
              v-for="collection in frozenCollections"
              :key="collection.id"
              :label="`${collection.name} · ${collection.assetCount} 张`"
              :value="collection.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="AnnotationRevision ID"
          ><el-input-number v-model="versionForm.annotationRevisionId" :min="0"
        /></el-form-item>
        <div class="form-grid">
          <el-form-item label="类别体系"
            ><el-input v-model="versionForm.ontologyVersion"
          /></el-form-item>
          <el-form-item label="固定随机种子"
            ><el-input-number v-model="versionForm.splitSeed" :min="1"
          /></el-form-item>
        </div>
        <div class="form-grid three">
          <el-form-item label="Train"
            ><el-input-number v-model="versionForm.train" :min="0" :max="1" :step="0.05"
          /></el-form-item>
          <el-form-item label="Val"
            ><el-input-number v-model="versionForm.val" :min="0" :max="1" :step="0.05"
          /></el-form-item>
          <el-form-item label="Test"
            ><el-input-number v-model="versionForm.test" :min="0" :max="1" :step="0.05"
          /></el-form-item>
        </div>
      </el-form>
      <template #footer
        ><el-button @click="versionVisible = false">取消</el-button
        ><el-button type="primary" @click="submitVersion">创建版本</el-button></template
      >
    </el-dialog>

    <el-drawer v-model="detailVisible" title="数据集质量与血缘" size="55%">
      <template v-if="detail">
        <section class="detail-summary">
          <article
            ><small>状态</small><strong>{{ detail.version.status }}</strong></article
          >
          <article
            ><small>阻断问题</small
            ><strong>{{
              detail.issues.filter((i) => i.severity === 'ERROR').length
            }}</strong></article
          >
          <article
            ><small>警告</small
            ><strong>{{
              detail.issues.filter((i) => i.severity === 'WARNING').length
            }}</strong></article
          >
          <article
            ><small>使用关系</small><strong>{{ detail.usage.length }}</strong></article
          >
        </section>
        <el-descriptions v-if="detail.version.manifestUri" :column="1" border>
          <el-descriptions-item label="Manifest">{{
            detail.version.manifestUri
          }}</el-descriptions-item>
          <el-descriptions-item label="Dataset Card">{{
            detail.version.datasetCardUri
          }}</el-descriptions-item>
          <el-descriptions-item label="校验和"
            ><code>{{ detail.version.checksum }}</code></el-descriptions-item
          >
        </el-descriptions>
        <el-table :data="detail.issues" class="issue-table">
          <el-table-column label="级别" width="90"
            ><template #default="{ row }"
              ><el-tag :type="row.severity === 'ERROR' ? 'danger' : 'warning'">{{
                row.severity
              }}</el-tag></template
            ></el-table-column
          >
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
import { getAssetCollections, type AssetCollection } from '@/api/ai-platform/annotations'
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
import { getProjectPage, type Project } from '@/api/ai-platform/projects'

defineOptions({ name: 'VisionAIDatasets' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const datasets = ref<Dataset[]>([])
const selectedDataset = ref<Dataset>()
const versions = ref<DatasetVersion[]>([])
const collections = ref<AssetCollection[]>([])
const loading = ref(false)
const datasetVisible = ref(false)
const versionVisible = ref(false)
const detailVisible = ref(false)
const detail = ref<Awaited<ReturnType<typeof getDatasetVersion>>>()
const datasetForm = reactive({ name: '', taskType: 'CV_DETECTION', description: '' })
const versionForm = reactive({
  sourceId: undefined as number | undefined,
  annotationRevisionId: 0,
  ontologyVersion: 'defect-v1',
  splitSeed: 20260721,
  train: 0.8,
  val: 0.1,
  test: 0.1
})
const frozenCollections = computed(() => collections.value.filter((item) => item.frozen))
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
const loadDatasets = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    const [rows, collectionRows] = await Promise.all([
      getDatasets(projectId.value),
      getAssetCollections(projectId.value)
    ])
    datasets.value = rows
    collections.value = collectionRows
    versionForm.sourceId = frozenCollections.value[0]?.id
    if (!selectedDataset.value && rows[0]) await selectDataset(rows[0])
  } finally {
    loading.value = false
  }
}
const selectDataset = async (dataset: Dataset) => {
  selectedDataset.value = dataset
  versions.value = await getDatasetVersions(projectId.value!, dataset.id)
}
const submitDataset = async () => {
  if (!projectId.value || !datasetForm.name.trim()) return
  const row = await createDataset(projectId.value, { ...datasetForm, ownerUserId: 1 })
  datasetVisible.value = false
  await loadDatasets()
  await selectDataset(row)
}
const submitVersion = async () => {
  if (!projectId.value || !selectedDataset.value || !versionForm.sourceId) return
  await createDatasetVersion(projectId.value, selectedDataset.value.id, {
    sourceType: 'COLLECTION',
    sourceId: versionForm.sourceId,
    annotationRevisionId: versionForm.annotationRevisionId,
    ontologyVersion: versionForm.ontologyVersion,
    splitSeed: versionForm.splitSeed,
    split: { TRAIN: versionForm.train, VAL: versionForm.val, TEST: versionForm.test }
  })
  versionVisible.value = false
  message.success('固定种子划分已写入新版本')
  await selectDataset(selectedDataset.value)
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

.page-header {
  display: flex;
  justify-content: space-between;
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
  font-size: 12px;
  font-weight: 750;
  letter-spacing: 1.8px;
  color: var(--el-color-primary);
}

.toolbar,
.actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.toolbar .el-select {
  width: 260px;
}

.dataset-layout {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: 20px;
}

aside {
  display: flex;
  flex-direction: column;
  gap: 9px;
}

aside button {
  display: flex;
  padding: 16px;
  text-align: left;
  cursor: pointer;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  flex-direction: column;
  gap: 5px;
}

aside button.active {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 3px rgb(22 119 255 / 9%);
}

aside button span,
aside button small {
  font-size: 12px;
  color: var(--text-secondary);
}

.versions {
  display: flex;
  flex-direction: column;
  gap: 13px;
}

.versions > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 4px 10px;
}

.versions h2 {
  margin: 0 0 3px;
}

.version-card {
  display: grid;
  padding: 20px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 13px;
  grid-template-columns: minmax(0, 1fr) 160px;
  gap: 20px;
}

.version-title {
  display: flex;
  gap: 10px;
  align-items: center;
}

.version-title strong {
  font-size: 20px;
}

.version-title span {
  font-size: 12px;
  color: var(--text-secondary);
}

.split {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin: 18px 0;
}

.split span {
  padding: 10px;
  color: var(--text-secondary);
  background: var(--el-fill-color-light);
  border-radius: 8px;
}

.split b {
  display: block;
  font-size: 18px;
  color: var(--text-primary);
}

.version-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.version-actions .el-button {
  margin-left: 0;
}

.empty-state,
.aside-empty {
  padding: 70px 20px;
  color: var(--text-secondary);
  text-align: center;
  border: 1px dashed var(--el-border-color);
  border-radius: 12px;
}

.full {
  width: 100%;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.form-grid.three {
  grid-template-columns: repeat(3, 1fr);
}

.detail-summary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin-bottom: 18px;
}

.detail-summary article {
  padding: 14px;
  background: var(--el-fill-color-light);
  border-radius: 10px;
}

.detail-summary small,
.detail-summary strong {
  display: block;
}

.detail-summary strong {
  margin-top: 4px;
  font-size: 20px;
}

.issue-table {
  margin-top: 18px;
}

code {
  overflow-wrap: anywhere;
  color: var(--text-secondary);
}

@media (width <= 900px) {
  .dataset-layout {
    grid-template-columns: 1fr;
  }

  .version-card {
    grid-template-columns: 1fr;
  }
}
</style>

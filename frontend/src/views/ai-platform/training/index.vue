<template>
  <main class="training-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">REPRODUCIBLE TRAINING</span>
        <h1>训练与实验</h1>
        <p>不可变模板、冻结数据集、资源约束、实时状态、指标与产物形成完整可追溯链路。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="runVisible = true">
        <Icon icon="lucide:play" :size="16" />发起训练
      </el-button>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="() => loadAll()">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="`${project.name} · ${project.code}`"
          :value="project.id"
        />
      </el-select>
      <el-button :loading="loading" @click="() => loadAll()"
        ><Icon icon="lucide:refresh-cw" :size="15" />刷新</el-button
      >
      <el-button @click="templateVisible = true">新建训练模板</el-button>
    </section>
    <section class="experiment-filters" aria-label="实验筛选">
      <el-select v-model="runFilters.datasetVersionId" clearable placeholder="数据集版本">
        <el-option
          v-for="version in frozenDatasetVersions"
          :key="version.id"
          :label="`${version.datasetName} · ${version.semanticVersion}`"
          :value="version.id"
        />
      </el-select>
      <el-input
        v-model="runFilters.framework"
        clearable
        placeholder="框架 / Trainer"
        @keyup.enter="loadAll()"
      />
      <el-select v-model="runFilters.status" clearable placeholder="运行状态">
        <el-option
          v-for="status in trainingStatuses"
          :key="status"
          :label="statusLabel(status)"
          :value="status"
        />
      </el-select>
      <el-select v-model="runFilters.provider" clearable placeholder="Provider">
        <el-option label="LocalDocker" value="LOCAL_DOCKER" />
        <el-option label="ClearML" value="CLEARML" />
      </el-select>
      <el-input-number
        v-model="runFilters.createdBy"
        :min="1"
        :controls="false"
        placeholder="创建人 ID"
      />
      <el-date-picker
        v-model="runFilters.createdRange"
        type="datetimerange"
        value-format="YYYY-MM-DD HH:mm:ss"
        start-placeholder="开始时间"
        end-placeholder="结束时间"
      />
      <el-button type="primary" @click="loadAll()">筛选实验</el-button>
      <el-button @click="resetRunFilters">重置</el-button>
    </section>

    <section class="summary-grid">
      <article
        ><small>模板</small><strong>{{ templates.length }}</strong
        ><span>版本化 Trainer 契约</span></article
      >
      <article
        ><small>运行中</small><strong>{{ activeRuns }}</strong
        ><span>队列与资源状态可见</span></article
      >
      <article
        ><small>成功</small><strong>{{ succeededRuns }}</strong
        ><span>模型、日志与环境锁已归档</span></article
      >
      <article
        ><small>失败</small><strong>{{ failedRuns }}</strong
        ><span>结构化错误可重试</span></article
      >
    </section>

    <section v-loading="loading" class="workspace">
      <aside>
        <header><b>训练模板</b><span>先冒烟，再发布</span></header>
        <button
          v-for="template in templates"
          :key="template.id"
          type="button"
          :class="{ active: selectedTemplate?.id === template.id }"
          @click="selectTemplate(template)"
        >
          <span>{{ template.aiType }}</span>
          <strong>{{ template.name }}</strong>
          <small>{{ template.versionCount }} 个版本 · {{ template.publishedCount }} 已发布</small>
        </button>
        <el-button v-if="selectedTemplate" class="new-version" @click="versionVisible = true"
          >创建不可变版本</el-button
        >
      </aside>

      <section class="runs">
        <header
          ><div><h2>实验运行</h2><p>Provider、队列、优先级和执行阶段统一观测。</p></div></header
        >
        <article v-for="run in runs" :key="run.id" class="run-card">
          <div class="run-head">
            <div
              ><b>{{ run.name }}</b
              ><span>#{{ run.id }} · {{ run.provider }} / {{ run.queue }}</span></div
            >
            <el-tag :type="statusType(run.status)">{{ statusLabel(run.status) }}</el-tag>
          </div>
          <el-progress
            :percentage="run.progress"
            :status="
              run.status === 'FAILED'
                ? 'exception'
                : run.status === 'SUCCEEDED'
                  ? 'success'
                  : undefined
            "
          />
          <div class="run-meta">
            <span>DatasetVersion #{{ run.datasetVersionId }}</span>
            <span>TemplateVersion #{{ run.templateVersionId }}</span>
            <span>{{ run.gpuCount ? `${run.gpuCount} GPU` : 'CPU' }}</span>
          </div>
          <p v-if="run.errorCode" class="error"
            >{{ run.errorCategory }} / {{ run.errorCode }} · {{ run.errorMessage }}</p
          >
          <div class="run-actions">
            <el-button link type="primary" @click="showRun(run)">指标与产物</el-button>
            <el-button
              v-if="run.status === 'SUCCEEDED'"
              link
              :loading="exportingRunId === run.id"
              @click="exportRun(run)"
            >
              导出训练包
            </el-button>
            <el-button v-if="isActive(run.status)" link type="danger" @click="cancelRun(run)"
              >取消</el-button
            >
            <el-button
              v-if="run.status === 'SUCCEEDED' || run.status === 'FAILED'"
              link
              @click="cloneRun(run)"
              >克隆配置</el-button
            >
          </div>
        </article>
        <div v-if="!runs.length" class="empty-state"
          >暂无训练运行，先发布模板并选择冻结数据集。</div
        >
      </section>
    </section>

    <el-dialog v-model="templateVisible" title="新建训练模板" width="520">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="templateForm.name" /></el-form-item>
        <el-form-item label="AI 类型"
          ><el-select v-model="templateForm.aiType" class="full"
            ><el-option label="目标检测" value="CV_DETECTION" /></el-select
        ></el-form-item>
        <el-form-item label="说明"
          ><el-input v-model="templateForm.description" type="textarea"
        /></el-form-item>
      </el-form>
      <template #footer
        ><el-button @click="templateVisible = false">取消</el-button
        ><el-button type="primary" :loading="submitting" @click="submitTemplate"
          >创建</el-button
        ></template
      >
    </el-dialog>

    <el-dialog v-model="versionVisible" title="创建不可变模板版本" width="650">
      <el-alert
        type="info"
        :closable="false"
        title="镜像必须固定到 sha256；版本通过最小训练冒烟后才能发布。"
      />
      <el-radio-group v-model="versionEditorMode" class="editor-mode">
        <el-radio-button value="FORM">结构化表单</el-radio-button>
        <el-radio-button value="YAML">高级 YAML</el-radio-button>
      </el-radio-group>
      <el-form label-position="top" class="dialog-form">
        <template v-if="versionEditorMode === 'FORM'">
          <el-form-item label="Trainer"><el-input v-model="versionForm.trainer" /></el-form-item>
          <el-form-item label="镜像 sha256"
            ><el-input v-model="versionForm.imageRef"
          /></el-form-item>
          <el-form-item label="Entrypoint（留空使用镜像默认值）"
            ><el-input v-model="versionForm.entrypoint"
          /></el-form-item>
          <el-form-item label="参数 JSON Schema">
            <el-input v-model="versionForm.parameterSchemaText" type="textarea" :rows="7" />
          </el-form-item>
          <el-form-item label="兼容矩阵（JSON）">
            <el-input v-model="versionForm.compatibilityText" type="textarea" :rows="7" />
          </el-form-item>
        </template>
        <el-form-item v-else label="完整模板 YAML">
          <el-input
            v-model="versionForm.advancedYaml"
            class="yaml-editor"
            type="textarea"
            :rows="25"
            spellcheck="false"
          />
        </el-form-item>
      </el-form>
      <template #footer
        ><el-button @click="versionVisible = false">取消</el-button
        ><el-button type="primary" :loading="submitting" @click="submitVersion"
          >创建并冒烟</el-button
        ></template
      >
    </el-dialog>

    <el-dialog v-model="runVisible" title="发起可复现训练" width="620">
      <el-form label-position="top">
        <el-form-item label="实验名称"><el-input v-model="runForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="冻结数据集版本">
            <el-select
              v-model="runForm.datasetVersionId"
              class="full"
              placeholder="选择当前项目的 FROZEN 版本"
            >
              <el-option
                v-for="version in frozenDatasetVersions"
                :key="version.id"
                :label="`${version.datasetName} · ${version.semanticVersion} · ${version.itemCount} 张`"
                :value="version.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="已发布 TemplateVersion">
            <el-select v-model="runForm.templateVersionId" class="full">
              <el-option
                v-for="version in publishedVersions"
                :key="version.id"
                :label="`${version.semanticVersion} · ${version.trainer}`"
                :value="version.id"
              />
            </el-select>
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="Provider"
            ><el-select v-model="runForm.provider" class="full"
              ><el-option label="LocalDocker" value="LOCAL_DOCKER" /><el-option
                label="ClearML"
                value="CLEARML" /></el-select
          ></el-form-item>
          <el-form-item label="队列"><el-input v-model="runForm.queue" /></el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="代码提交 / 版本">
            <el-input v-model.trim="runForm.codeCommit" placeholder="Git SHA、标签或受控代码版本" />
          </el-form-item>
          <el-form-item label="预训练模型引用">
            <el-input
              v-model.trim="runForm.pretrainedRef"
              placeholder="可选：s3://、ModelVersion 或受控权重引用"
            />
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="GPU 数量">
            <el-input-number
              v-model="runForm.gpuCount"
              :min="0"
              :max="availableGPUCount"
              :disabled="runForm.provider !== 'LOCAL_DOCKER'"
            />
          </el-form-item>
          <el-form-item label="可用加速器">
            <el-input :model-value="gpuSummary" readonly placeholder="未发现在线 GPU 节点" />
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="优先级">
            <el-slider v-model="runForm.priority" :min="0" :max="100" show-input />
          </el-form-item>
          <el-form-item label="运行资源">
            <el-input
              :model-value="`${runForm.cpu} CPU · ${runForm.memoryGiB} GiB 内存`"
              readonly
            />
          </el-form-item>
        </div>
        <section v-if="schemaFields.length" class="schema-fields">
          <div class="schema-heading">
            <strong>模板参数</strong>
            <small>由 TemplateVersion 的 JSON Schema 动态生成</small>
          </div>
          <div class="form-grid">
            <el-form-item
              v-for="field in schemaFields"
              :key="field.key"
              :label="field.title || field.key"
              :required="field.required"
            >
              <el-select
                v-if="field.enum?.length"
                v-model="runForm.parameters[field.key]"
                class="full"
              >
                <el-option
                  v-for="option in field.enum"
                  :key="String(option)"
                  :label="String(option)"
                  :value="option"
                />
              </el-select>
              <el-switch
                v-else-if="field.type === 'boolean'"
                v-model="runForm.parameters[field.key]"
              />
              <el-input-number
                v-else-if="field.type === 'number' || field.type === 'integer'"
                v-model="runForm.parameters[field.key]"
                :min="field.minimum"
                :max="field.maximum"
                :step="field.type === 'integer' ? 1 : field.multipleOf || 0.01"
              />
              <el-input
                v-else
                v-model="runForm.parameters[field.key]"
                :placeholder="field.description"
              />
            </el-form-item>
          </div>
        </section>
      </el-form>
      <template #footer
        ><el-button @click="runVisible = false">取消</el-button
        ><el-button type="primary" :loading="submitting" @click="submitRun"
          >进入队列</el-button
        ></template
      >
    </el-dialog>

    <el-drawer v-model="detailVisible" size="55%">
      <template #header>
        <div class="drawer-header">
          <div
            ><strong>指标、日志与产物</strong
            ><span v-if="runDetail">训练运行 #{{ runDetail.run.id }}</span></div
          >
          <el-button
            v-if="runDetail?.run.status === 'SUCCEEDED'"
            type="primary"
            :loading="exportingRunId === runDetail.run.id"
            @click="exportRun(runDetail.run)"
          >
            <Icon icon="lucide:package-down" :size="16" />导出全部
          </el-button>
        </div>
      </template>
      <template v-if="runDetail">
        <section class="detail-summary">
          <article
            ><small>状态</small><strong>{{ statusLabel(runDetail.run.status) }}</strong></article
          >
          <article
            ><small>指标点</small><strong>{{ runDetail.metrics.length }}</strong></article
          >
          <article
            ><small>归档产物</small><strong>{{ runDetail.artifacts.length }}</strong></article
          >
        </section>
        <el-table :data="runDetail.metrics" empty-text="该运行暂无指标记录">
          <el-table-column prop="name" label="指标" />
          <el-table-column prop="step" label="Step" width="100" />
          <el-table-column prop="value" label="值" width="140" />
        </el-table>
        <el-table
          :data="runDetail.artifacts"
          class="artifact-table"
          empty-text="该运行暂无归档产物"
        >
          <el-table-column prop="kind" label="类型" width="160" />
          <el-table-column prop="name" label="名称" />
          <el-table-column label="大小" width="110"
            ><template #default="{ row }">{{ formatBytes(row.size) }}</template></el-table-column
          >
          <el-table-column prop="sha256" label="SHA-256"
            ><template #default="{ row }"
              ><code>{{ row.sha256.slice(0, 12) }}…</code></template
            ></el-table-column
          >
          <el-table-column label="操作" width="100" align="right">
            <template #default="{ row }">
              <el-tooltip
                :disabled="isDownloadable(row.uri)"
                content="外部 Provider 产物需在对应训练服务中下载"
              >
                <span>
                  <el-button
                    link
                    type="primary"
                    :disabled="!isDownloadable(row.uri)"
                    :loading="downloadingArtifactId === row.id"
                    @click="downloadArtifact(runDetail!.run, row)"
                  >
                    下载
                  </el-button>
                </span>
              </el-tooltip>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import { getDatasets, getDatasetVersions, type DatasetVersion } from '@/api/ai-platform/datasets'
import { getResourceOverview } from '@/api/ai-platform/operations'
import {
  cancelTrainingRun,
  cloneTrainingRun,
  createTrainingRun,
  createTrainingTemplate,
  createTrainingTemplateVersion,
  downloadTrainingArtifact,
  exportTrainingRun,
  getTrainingRun,
  getTrainingRuns,
  getTrainingTemplates,
  getTrainingTemplateVersions,
  publishTrainingTemplateVersion,
  smokeTrainingTemplateVersion,
  type TrainingRun,
  type TrainingArtifact,
  type TrainingStatus,
  type TrainingTemplate,
  type TrainingTemplateVersion
} from '@/api/ai-platform/training'
import download from '@/utils/download'
import { downloadByData } from '@/utils/filt'

defineOptions({ name: 'VisionAITraining' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const templates = ref<TrainingTemplate[]>([])
const selectedTemplate = ref<TrainingTemplate>()
const versions = ref<TrainingTemplateVersion[]>([])
const runs = ref<TrainingRun[]>([])
const frozenDatasetVersions = ref<Array<DatasetVersion & { datasetName: string }>>([])
const computeNodes = ref<Array<Record<string, any>>>([])
const templateVisible = ref(false)
const versionVisible = ref(false)
const runVisible = ref(false)
const detailVisible = ref(false)
const runDetail = ref<Awaited<ReturnType<typeof getTrainingRun>>>()
const loading = ref(false)
const submitting = ref(false)
const exportingRunId = ref<number>()
const downloadingArtifactId = ref<number>()
let pollTimer: number | undefined
const trainingStatuses = [
  'QUEUED',
  'ALLOCATING',
  'RUNNING',
  'EXPORTING',
  'SUCCEEDED',
  'FAILED',
  'CANCELLED',
  'TIMEOUT'
] as const
const runFilters = reactive({
  datasetVersionId: undefined as number | undefined,
  framework: '',
  status: '',
  provider: '',
  createdBy: undefined as number | undefined,
  createdRange: [] as string[]
})
const templateForm = reactive({ name: '', aiType: 'CV_DETECTION', description: '' })
const versionEditorMode = ref<'FORM' | 'YAML'>('FORM')
const defaultParameterSchema = {
  type: 'object',
  additionalProperties: false,
  properties: {
    epochs: { type: 'integer', title: '训练轮数', minimum: 1, maximum: 300, default: 10 },
    learningRate: {
      type: 'number',
      title: '学习率',
      minimum: 0.000001,
      maximum: 1,
      default: 0.001
    },
    batchSize: { type: 'integer', title: '批大小', minimum: 1, maximum: 128, default: 8 }
  },
  required: ['epochs', 'learningRate', 'batchSize']
}
const defaultCompatibility = {
  datasetTypes: ['CV_DETECTION'],
  modelTypes: ['CV_DETECTION'],
  providers: ['LOCAL_DOCKER', 'CLEARML'],
  cudaRange: '>=12.0 <13.0',
  driverRange: '>=550',
  minCUDA: '12.0',
  minDriver: '550',
  providerVersions: {
    LOCAL_DOCKER: 'Docker Engine 27+',
    CLEARML: '2.x'
  }
}
const versionForm = reactive({
  trainer: 'TorchVisionDetection',
  imageRef: '',
  entrypoint: '',
  parameterSchemaText: JSON.stringify(defaultParameterSchema, null, 2),
  compatibilityText: JSON.stringify(defaultCompatibility, null, 2),
  advancedYaml: `trainer: TorchVisionDetection
imageRef: ""
entrypoint: ""
outputProtocol: visionai.result-manifest.v1
parameterSchema:
  type: object
  additionalProperties: false
  properties:
    epochs: {type: integer, title: 训练轮数, minimum: 1, maximum: 300, default: 10}
    learningRate: {type: number, title: 学习率, minimum: 0.000001, maximum: 1, default: 0.001}
    batchSize: {type: integer, title: 批大小, minimum: 1, maximum: 128, default: 8}
  required: [epochs, learningRate, batchSize]
resourceRequirements:
  cpu: 2
  memoryBytes: 4294967296
  gpuMin: 1
  gpuMax: 1
compatibility:
  datasetTypes: [CV_DETECTION]
  modelTypes: [CV_DETECTION]
  providers: [LOCAL_DOCKER, CLEARML]
  cudaRange: ">=12.0 <13.0"
  driverRange: ">=550"
  minCUDA: "12.0"
  minDriver: "550"
  providerVersions:
    LOCAL_DOCKER: "Docker Engine 27+"
    CLEARML: "2.x"
licensePolicy:
  allowed: true
`
})
const runForm = reactive({
  name: '',
  datasetVersionId: undefined as number | undefined,
  templateVersionId: undefined as number | undefined,
  provider: 'LOCAL_DOCKER',
  queue: 'gpu-local',
  gpuCount: 1,
  priority: 50,
  codeCommit: '',
  pretrainedRef: '',
  cpu: 2,
  memoryGiB: 4,
  parameters: {} as Record<string, any>
})
const onlineGPUNodes = computed(() =>
  computeNodes.value.filter((node) => node.status === 'ONLINE' && Number(node.gpuCount) > 0)
)
const availableGPUCount = computed(() =>
  onlineGPUNodes.value.reduce((total, node) => total + Number(node.gpuCount || 0), 0)
)
const gpuSummary = computed(() =>
  onlineGPUNodes.value.length
    ? onlineGPUNodes.value.map((node) => `${node.gpuModel} × ${node.gpuCount}`).join('，')
    : ''
)
const publishedVersions = computed(() => versions.value.filter((item) => item.published))
type SchemaField = {
  key: string
  type: string
  title?: string
  description?: string
  enum?: Array<string | number>
  minimum?: number
  maximum?: number
  multipleOf?: number
  default?: string | number | boolean
  required: boolean
}
const selectedRunTemplate = computed(() =>
  versions.value.find((item) => item.id === runForm.templateVersionId)
)
const schemaFields = computed<SchemaField[]>(() => {
  try {
    const schema = JSON.parse(selectedRunTemplate.value?.parameterSchema || '{}')
    const required = new Set<string>(schema.required || [])
    return Object.entries<Record<string, any>>(schema.properties || {}).map(([key, value]) => ({
      key,
      type: value.type || 'string',
      title: value.title,
      description: value.description,
      enum: value.enum,
      minimum: value.minimum,
      maximum: value.maximum,
      multipleOf: value.multipleOf,
      default: value.default,
      required: required.has(key)
    }))
  } catch {
    return []
  }
})
watch(
  schemaFields,
  (fields) => {
    const next: Record<string, any> = {}
    for (const field of fields) {
      next[field.key] =
        runForm.parameters[field.key] ?? field.default ?? (field.type === 'boolean' ? false : '')
    }
    runForm.parameters = next
  },
  { immediate: true }
)
const activeRuns = computed(() => runs.value.filter((item) => isActive(item.status)).length)
const succeededRuns = computed(
  () => runs.value.filter((item) => item.status === 'SUCCEEDED').length
)
const failedRuns = computed(
  () => runs.value.filter((item) => item.status === 'FAILED' || item.status === 'TIMEOUT').length
)
const isActive = (status: TrainingStatus) =>
  ['QUEUED', 'ALLOCATING', 'RUNNING', 'EXPORTING'].includes(status)
const statusType = (status: TrainingStatus) =>
  status === 'SUCCEEDED'
    ? 'success'
    : status === 'FAILED' || status === 'TIMEOUT'
      ? 'danger'
      : status === 'CANCELLED'
        ? 'info'
        : 'warning'
const statusLabel = (status: TrainingStatus) =>
  ({
    DRAFT: '草稿',
    QUEUED: '排队中',
    ALLOCATING: '分配资源',
    RUNNING: '训练中',
    EXPORTING: '归集产物',
    SUCCEEDED: '已完成',
    FAILED: '失败',
    CANCELLED: '已取消',
    TIMEOUT: '已超时'
  })[status]
const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}
const isDownloadable = (uri: string) => uri.startsWith('s3://')
const loadAll = async (background = false) => {
  if (!projectId.value) return
  if (!background) loading.value = true
  try {
    const selectedId = selectedTemplate.value?.id
    const [templateRows, runRows, datasetRows, resources] = await Promise.all([
      getTrainingTemplates(projectId.value),
      getTrainingRuns(projectId.value, {
        datasetVersionId: runFilters.datasetVersionId,
        framework: runFilters.framework || undefined,
        status: runFilters.status || undefined,
        provider: runFilters.provider || undefined,
        createdBy: runFilters.createdBy,
        createdFrom: runFilters.createdRange[0],
        createdTo: runFilters.createdRange[1]
      }),
      getDatasets(projectId.value),
      getResourceOverview()
    ])
    templates.value = templateRows
    runs.value = runRows.list
    computeNodes.value = resources.nodes
    const versionGroups = await Promise.all(
      datasetRows.map(async (dataset) =>
        (await getDatasetVersions(projectId.value!, dataset.id))
          .filter((version) => version.status === 'FROZEN')
          .map((version) => ({ ...version, datasetName: dataset.name }))
      )
    )
    frozenDatasetVersions.value = versionGroups.flat()
    if (!frozenDatasetVersions.value.some((version) => version.id === runForm.datasetVersionId))
      runForm.datasetVersionId = frozenDatasetVersions.value[0]?.id
    if (availableGPUCount.value > 0 && runForm.provider === 'LOCAL_DOCKER') {
      runForm.gpuCount = Math.max(1, Math.min(runForm.gpuCount, availableGPUCount.value))
      runForm.queue = 'gpu-local'
    }
    const nextTemplate =
      templates.value.find((item) => item.id === selectedId) || templates.value[0]
    if (nextTemplate && nextTemplate.id !== selectedTemplate.value?.id)
      await selectTemplate(nextTemplate)
    if (!nextTemplate) {
      selectedTemplate.value = undefined
      versions.value = []
    }
  } finally {
    if (!background) loading.value = false
  }
}
const resetRunFilters = async () => {
  runFilters.datasetVersionId = undefined
  runFilters.framework = ''
  runFilters.status = ''
  runFilters.provider = ''
  runFilters.createdBy = undefined
  runFilters.createdRange = []
  await loadAll()
}
const selectTemplate = async (template: TrainingTemplate) => {
  selectedTemplate.value = template
  versions.value = await getTrainingTemplateVersions(projectId.value!, template.id)
  runForm.templateVersionId = publishedVersions.value[0]?.id
}
const submitTemplate = async () => {
  if (!projectId.value || !templateForm.name.trim()) {
    message.warning('请填写训练模板名称')
    return
  }
  submitting.value = true
  try {
    const template = await createTrainingTemplate(projectId.value, templateForm)
    templateVisible.value = false
    await loadAll()
    await selectTemplate(template)
    message.success('训练模板已创建')
  } finally {
    submitting.value = false
  }
}
const submitVersion = async () => {
  if (!projectId.value || !selectedTemplate.value) return
  let parameterSchema: Record<string, unknown> = {}
  let compatibility: Record<string, unknown> = {}
  try {
    parameterSchema = JSON.parse(versionForm.parameterSchemaText)
    compatibility = JSON.parse(versionForm.compatibilityText)
  } catch {
    if (versionEditorMode.value === 'FORM') {
      message.warning('参数 Schema 与兼容矩阵必须是有效 JSON')
      return
    }
  }
  if (versionEditorMode.value === 'FORM' && !versionForm.imageRef.startsWith('sha256:')) {
    message.warning('请输入完整 sha256 镜像引用')
    return
  }
  submitting.value = true
  try {
    const version = await createTrainingTemplateVersion(
      projectId.value,
      selectedTemplate.value.id,
      {
        trainer: versionForm.trainer,
        imageRef: versionForm.imageRef,
        entrypoint: versionForm.entrypoint,
        advancedYaml: versionEditorMode.value === 'YAML' ? versionForm.advancedYaml : '',
        outputProtocol: 'visionai.result-manifest.v1',
        parameterSchema,
        resourceRequirements: { cpu: 2, memoryBytes: 4294967296, gpuMin: 1, gpuMax: 1 },
        compatibility,
        licensePolicy: { allowed: true }
      }
    )
    message.info('正在使用本机 GPU 运行隔离的目标检测最小训练冒烟…')
    await smokeTrainingTemplateVersion(projectId.value, version.id)
    await publishTrainingTemplateVersion(projectId.value, version.id)
    versionVisible.value = false
    message.success('模板冒烟通过并已发布')
    await loadAll()
  } finally {
    submitting.value = false
  }
}
const submitRun = async () => {
  if (
    !projectId.value ||
    !runForm.name.trim() ||
    !runForm.templateVersionId ||
    !runForm.datasetVersionId
  ) {
    message.warning('请填写实验名称，并选择冻结数据集与已发布模板版本')
    return
  }
  if (runForm.provider === 'LOCAL_DOCKER' && runForm.gpuCount < 1) {
    message.warning('本机已配置 GPU 训练，请至少分配 1 块 GPU')
    return
  }
  submitting.value = true
  try {
    await createTrainingRun(projectId.value, {
      ...runForm,
      parameters: runForm.parameters,
      runtimeSpec: { cpus: runForm.cpu, memoryBytes: runForm.memoryGiB * 1024 ** 3 }
    })
    runVisible.value = false
    message.success('训练已进入统一任务队列')
    await loadAll()
  } finally {
    submitting.value = false
  }
}
const showRun = async (run: TrainingRun) => {
  runDetail.value = await getTrainingRun(projectId.value!, run.id)
  detailVisible.value = true
}
const cancelRun = async (run: TrainingRun) => {
  await cancelTrainingRun(projectId.value!, run.id)
  await loadAll()
}
const cloneRun = async (run: TrainingRun) => {
  await cloneTrainingRun(projectId.value!, run.id)
  message.success('已克隆为 DRAFT 配置')
  await loadAll()
}
const downloadArtifact = async (run: TrainingRun, artifact: TrainingArtifact) => {
  if (!projectId.value || !isDownloadable(artifact.uri)) return
  downloadingArtifactId.value = artifact.id
  try {
    const data = await downloadTrainingArtifact(projectId.value, run.id, artifact.id)
    downloadByData(data, artifact.name, artifact.mediaType || 'application/octet-stream')
    message.success(`已下载 ${artifact.name}`)
  } finally {
    downloadingArtifactId.value = undefined
  }
}
const exportRun = async (run: TrainingRun) => {
  if (!projectId.value) return
  exportingRunId.value = run.id
  try {
    const data = await exportTrainingRun(projectId.value, run.id)
    download.zip(data, `training-run-${run.id}-export.zip`)
    message.success('训练运行导出完成')
  } finally {
    exportingRunId.value = undefined
  }
}
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
  pollTimer = window.setInterval(() => projectId.value && loadAll(true), 5000)
})
onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer)
})
</script>

<style scoped lang="scss">
.training-page {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--el-fill-color-extra-light);
}

.page-header,
.toolbar,
.run-head,
.runs > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-header {
  margin-bottom: 22px;
}

.page-header h1 {
  margin: 5px 0;
  font-size: 30px;
  letter-spacing: -1px;
}

.page-header p,
.runs header p {
  margin: 0;
  color: var(--text-secondary);
}

.eyebrow {
  font-size: 12px;
  font-weight: 750;
  letter-spacing: 1.8px;
  color: var(--el-color-primary);
}

.toolbar {
  justify-content: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}

.toolbar .el-select {
  width: 260px;
}

.experiment-filters {
  display: grid;
  grid-template-columns: repeat(4, minmax(150px, 1fr)) auto auto;
  gap: 10px;
  padding: 14px;
  margin: -6px 0 18px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}

.experiment-filters > * {
  width: 100%;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 18px;
}

.summary-grid article,
.run-card,
aside {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}

.summary-grid article {
  display: flex;
  padding: 17px;
  flex-direction: column;
  gap: 3px;
}

.summary-grid small,
.summary-grid span,
aside span,
aside small,
.run-head span,
.run-meta {
  font-size: 12px;
  color: var(--text-secondary);
}

.summary-grid strong {
  font-size: 27px;
}

.workspace {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  gap: 18px;
}

aside {
  display: flex;
  padding: 14px;
  flex-direction: column;
  gap: 8px;
}

aside header {
  display: flex;
  padding: 5px 4px 10px;
  flex-direction: column;
}

aside button {
  display: flex;
  padding: 14px;
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  flex-direction: column;
  gap: 4px;
}

aside button.active {
  background: rgb(22 119 255 / 6%);
  border-color: var(--el-color-primary);
}

.new-version {
  margin-top: 5px;
}

.runs {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.runs h2 {
  margin: 0;
}

.run-card {
  padding: 17px;
}

.run-head > div {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.run-card .el-progress {
  margin: 15px 0 10px;
}

.run-meta,
.run-actions {
  display: flex;
  gap: 18px;
}

.run-actions {
  margin-top: 8px;
}

.error {
  padding: 8px;
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
  border-radius: 7px;
}

.full {
  width: 100%;
}

.dialog-form {
  margin-top: 16px;
}

.editor-mode {
  margin-top: 16px;
}

.yaml-editor :deep(textarea) {
  font-family: 'Cascadia Code', 'JetBrains Mono', Consolas, monospace;
  line-height: 1.55;
}

.schema-fields {
  padding-top: 14px;
  margin-top: 8px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.schema-heading {
  display: flex;
  margin-bottom: 14px;
  flex-direction: column;
  gap: 3px;
}

.schema-heading small {
  color: var(--text-secondary);
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.detail-summary {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-bottom: 18px;
}

.detail-summary article {
  display: flex;
  padding: 15px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  flex-direction: column;
}

.detail-summary strong {
  font-size: 24px;
}

.artifact-table {
  margin-top: 18px;
}

.drawer-header {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  padding-right: 16px;
}

.drawer-header > div {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.drawer-header span {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: var(--text-secondary);
}

.artifact-table code {
  font-size: 12px;
  color: var(--text-secondary);
}

.empty-state {
  padding: 70px;
  color: var(--text-secondary);
  text-align: center;
  border: 1px dashed var(--el-border-color);
  border-radius: 12px;
}

@media (width <= 1100px) {
  .summary-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .workspace {
    grid-template-columns: 1fr;
  }
}
</style>

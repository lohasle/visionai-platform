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
      <el-select v-model="projectId" placeholder="选择项目" @change="loadAll">
        <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
      </el-select>
      <el-button @click="loadAll"><Icon icon="lucide:refresh-cw" :size="15" />刷新</el-button>
      <el-button @click="templateVisible = true">新建训练模板</el-button>
    </section>

    <section class="summary-grid">
      <article><small>模板</small><strong>{{ templates.length }}</strong><span>版本化 Trainer 契约</span></article>
      <article><small>运行中</small><strong>{{ activeRuns }}</strong><span>队列与资源状态可见</span></article>
      <article><small>成功</small><strong>{{ succeededRuns }}</strong><span>模型、日志与环境锁已归档</span></article>
      <article><small>失败</small><strong>{{ failedRuns }}</strong><span>结构化错误可重试</span></article>
    </section>

    <section class="workspace">
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
        <el-button v-if="selectedTemplate" class="new-version" @click="versionVisible = true">创建不可变版本</el-button>
      </aside>

      <section class="runs">
        <header><div><h2>实验运行</h2><p>Provider、队列、优先级和执行阶段统一观测。</p></div></header>
        <article v-for="run in runs" :key="run.id" class="run-card">
          <div class="run-head">
            <div><b>{{ run.name }}</b><span>#{{ run.id }} · {{ run.provider }} / {{ run.queue }}</span></div>
            <el-tag :type="statusType(run.status)">{{ run.status }}</el-tag>
          </div>
          <el-progress :percentage="run.progress" :status="run.status === 'FAILED' ? 'exception' : run.status === 'SUCCEEDED' ? 'success' : undefined" />
          <div class="run-meta">
            <span>DatasetVersion #{{ run.datasetVersionId }}</span>
            <span>TemplateVersion #{{ run.templateVersionId }}</span>
            <span>{{ run.gpuCount ? `${run.gpuCount} GPU` : 'CPU' }}</span>
          </div>
          <p v-if="run.errorCode" class="error">{{ run.errorCategory }} / {{ run.errorCode }} · {{ run.errorMessage }}</p>
          <div class="run-actions">
            <el-button link type="primary" @click="showRun(run)">指标与产物</el-button>
            <el-button v-if="isActive(run.status)" link type="danger" @click="cancelRun(run)">取消</el-button>
            <el-button v-if="run.status === 'SUCCEEDED' || run.status === 'FAILED'" link @click="cloneRun(run)">克隆配置</el-button>
          </div>
        </article>
        <div v-if="!runs.length" class="empty-state">暂无训练运行，先发布模板并选择冻结数据集。</div>
      </section>
    </section>

    <el-dialog v-model="templateVisible" title="新建训练模板" width="520">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="templateForm.name" /></el-form-item>
        <el-form-item label="AI 类型"><el-select v-model="templateForm.aiType" class="full"><el-option label="目标检测" value="CV_DETECTION" /></el-select></el-form-item>
        <el-form-item label="说明"><el-input v-model="templateForm.description" type="textarea" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="templateVisible = false">取消</el-button><el-button type="primary" @click="submitTemplate">创建</el-button></template>
    </el-dialog>

    <el-dialog v-model="versionVisible" title="创建不可变模板版本" width="650">
      <el-alert type="info" :closable="false" title="镜像必须固定到 sha256；版本通过最小训练冒烟后才能发布。" />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="Trainer"><el-input v-model="versionForm.trainer" /></el-form-item>
        <el-form-item label="镜像 sha256"><el-input v-model="versionForm.imageRef" /></el-form-item>
        <el-form-item label="Entrypoint（留空使用镜像默认值）"><el-input v-model="versionForm.entrypoint" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="versionVisible = false">取消</el-button><el-button type="primary" @click="submitVersion">创建并冒烟</el-button></template>
    </el-dialog>

    <el-dialog v-model="runVisible" title="发起可复现训练" width="620">
      <el-form label-position="top">
        <el-form-item label="实验名称"><el-input v-model="runForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="冻结 DatasetVersion ID"><el-input-number v-model="runForm.datasetVersionId" :min="1" /></el-form-item>
          <el-form-item label="已发布 TemplateVersion">
            <el-select v-model="runForm.templateVersionId" class="full">
              <el-option v-for="version in publishedVersions" :key="version.id" :label="`${version.semanticVersion} · ${version.trainer}`" :value="version.id" />
            </el-select>
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="Provider"><el-select v-model="runForm.provider" class="full"><el-option label="LocalDocker" value="LOCAL_DOCKER" /><el-option label="ClearML" value="CLEARML" /></el-select></el-form-item>
          <el-form-item label="队列"><el-input v-model="runForm.queue" /></el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="runVisible = false">取消</el-button><el-button type="primary" @click="submitRun">进入队列</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="指标、日志与产物" size="55%">
      <template v-if="runDetail">
        <section class="detail-summary">
          <article><small>状态</small><strong>{{ runDetail.run.status }}</strong></article>
          <article><small>指标点</small><strong>{{ runDetail.metrics.length }}</strong></article>
          <article><small>归档产物</small><strong>{{ runDetail.artifacts.length }}</strong></article>
        </section>
        <el-table :data="runDetail.metrics">
          <el-table-column prop="name" label="指标" />
          <el-table-column prop="step" label="Step" width="100" />
          <el-table-column prop="value" label="值" width="140" />
        </el-table>
        <el-table :data="runDetail.artifacts" class="artifact-table">
          <el-table-column prop="kind" label="类型" width="160" />
          <el-table-column prop="name" label="名称" />
          <el-table-column prop="sha256" label="SHA-256" />
        </el-table>
      </template>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  cancelTrainingRun,
  cloneTrainingRun,
  createTrainingRun,
  createTrainingTemplate,
  createTrainingTemplateVersion,
  getTrainingRun,
  getTrainingRuns,
  getTrainingTemplates,
  getTrainingTemplateVersions,
  publishTrainingTemplateVersion,
  smokeTrainingTemplateVersion,
  type TrainingRun,
  type TrainingStatus,
  type TrainingTemplate,
  type TrainingTemplateVersion
} from '@/api/ai-platform/training'

defineOptions({ name: 'VisionAITraining' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const templates = ref<TrainingTemplate[]>([])
const selectedTemplate = ref<TrainingTemplate>()
const versions = ref<TrainingTemplateVersion[]>([])
const runs = ref<TrainingRun[]>([])
const templateVisible = ref(false)
const versionVisible = ref(false)
const runVisible = ref(false)
const detailVisible = ref(false)
const runDetail = ref<Awaited<ReturnType<typeof getTrainingRun>>>()
const templateForm = reactive({ name: '', aiType: 'CV_DETECTION', description: '' })
const versionForm = reactive({ trainer: 'LocalDockerSmoke', imageRef: '', entrypoint: '' })
const runForm = reactive({ name: '', datasetVersionId: 1, templateVersionId: undefined as number | undefined, provider: 'LOCAL_DOCKER', queue: 'cpu-local' })
const publishedVersions = computed(() => versions.value.filter((item) => item.published))
const activeRuns = computed(() => runs.value.filter((item) => isActive(item.status)).length)
const succeededRuns = computed(() => runs.value.filter((item) => item.status === 'SUCCEEDED').length)
const failedRuns = computed(() => runs.value.filter((item) => item.status === 'FAILED' || item.status === 'TIMEOUT').length)
const isActive = (status: TrainingStatus) => ['QUEUED', 'ALLOCATING', 'RUNNING', 'EXPORTING'].includes(status)
const statusType = (status: TrainingStatus) => status === 'SUCCEEDED' ? 'success' : status === 'FAILED' || status === 'TIMEOUT' ? 'danger' : status === 'CANCELLED' ? 'info' : 'warning'
const loadAll = async () => {
  if (!projectId.value) return
  const [templateRows, runRows] = await Promise.all([getTrainingTemplates(projectId.value), getTrainingRuns(projectId.value)])
  templates.value = templateRows
  runs.value = runRows.list
  if (templates.value[0]) await selectTemplate(templates.value[0])
}
const selectTemplate = async (template: TrainingTemplate) => {
  selectedTemplate.value = template
  versions.value = await getTrainingTemplateVersions(projectId.value!, template.id)
  runForm.templateVersionId = publishedVersions.value[0]?.id
}
const submitTemplate = async () => {
  if (!projectId.value || !templateForm.name.trim()) return
  const template = await createTrainingTemplate(projectId.value, templateForm)
  templateVisible.value = false
  await loadAll()
  await selectTemplate(template)
}
const submitVersion = async () => {
  if (!projectId.value || !selectedTemplate.value || !versionForm.imageRef.startsWith('sha256:')) {
    message.warning('请输入完整 sha256 镜像引用')
    return
  }
  const version = await createTrainingTemplateVersion(projectId.value, selectedTemplate.value.id, {
    ...versionForm,
    outputProtocol: 'visionai.result-manifest.v1',
    parameterSchema: { type: 'object', additionalProperties: true },
    resourceRequirements: { cpu: 1, memoryBytes: 536870912, gpuMax: 8 },
    compatibility: { taskTypes: ['CV_DETECTION'] },
    licensePolicy: { allowed: true }
  })
  message.info('正在运行隔离的 CPU 最小训练冒烟…')
  await smokeTrainingTemplateVersion(projectId.value, version.id)
  await publishTrainingTemplateVersion(projectId.value, version.id)
  versionVisible.value = false
  message.success('模板冒烟通过并已发布')
  await loadAll()
}
const submitRun = async () => {
  if (!projectId.value || !runForm.name.trim() || !runForm.templateVersionId) return
  await createTrainingRun(projectId.value, {
    ...runForm,
    gpuCount: 0,
    priority: 50,
    parameters: {},
    runtimeSpec: { cpu: 1, memoryBytes: 536870912 },
    codeCommit: 'local-acceptance'
  })
  runVisible.value = false
  message.success('训练已进入统一任务队列')
  await loadAll()
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
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
  window.setInterval(() => projectId.value && loadAll(), 5000)
})
</script>

<style scoped lang="scss">
.training-page { min-height: 100%; padding: var(--app-content-padding); color: var(--text-primary); background: radial-gradient(circle at 85% 0, rgb(104 86 255 / 10%), transparent 35%); }
.page-header, .toolbar, .run-head, .runs > header { display: flex; align-items: center; justify-content: space-between; }
.page-header { margin-bottom: 22px; }
.page-header h1 { margin: 5px 0; font-size: 30px; letter-spacing: -1px; }
.page-header p, .runs header p { margin: 0; color: var(--text-secondary); }
.eyebrow { font-size: 12px; font-weight: 750; letter-spacing: 1.8px; color: var(--el-color-primary); }
.toolbar { justify-content: flex-start; gap: 12px; margin-bottom: 18px; }
.toolbar .el-select { width: 260px; }
.summary-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 18px; }
.summary-grid article, .run-card, aside { background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter); border-radius: 14px; }
.summary-grid article { display: flex; padding: 17px; flex-direction: column; gap: 3px; }
.summary-grid small, .summary-grid span, aside span, aside small, .run-head span, .run-meta { font-size: 12px; color: var(--text-secondary); }
.summary-grid strong { font-size: 27px; }
.workspace { display: grid; grid-template-columns: 300px minmax(0, 1fr); gap: 18px; }
aside { display: flex; padding: 14px; flex-direction: column; gap: 8px; }
aside header { display: flex; padding: 5px 4px 10px; flex-direction: column; }
aside button { display: flex; padding: 14px; text-align: left; cursor: pointer; background: transparent; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; flex-direction: column; gap: 4px; }
aside button.active { border-color: var(--el-color-primary); background: rgb(22 119 255 / 6%); }
.new-version { margin-top: 5px; }
.runs { display: flex; flex-direction: column; gap: 12px; }
.runs h2 { margin: 0; }
.run-card { padding: 17px; }
.run-head > div { display: flex; flex-direction: column; gap: 3px; }
.run-card .el-progress { margin: 15px 0 10px; }
.run-meta, .run-actions { display: flex; gap: 18px; }
.run-actions { margin-top: 8px; }
.error { padding: 8px; color: var(--el-color-danger); background: var(--el-color-danger-light-9); border-radius: 7px; }
.full { width: 100%; }
.dialog-form { margin-top: 16px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.detail-summary { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 18px; }
.detail-summary article { display: flex; padding: 15px; background: var(--el-fill-color-light); border-radius: 10px; flex-direction: column; }
.detail-summary strong { font-size: 24px; }
.artifact-table { margin-top: 18px; }
.empty-state { padding: 70px; text-align: center; color: var(--text-secondary); border: 1px dashed var(--el-border-color); border-radius: 12px; }
@media (max-width: 1100px) { .summary-grid { grid-template-columns: repeat(2, 1fr); } .workspace { grid-template-columns: 1fr; } }
</style>

<template>
  <main class="evaluation-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">MODEL QUALITY GATE</span>
        <h1>评估与困难样本</h1>
        <p>从总体指标下钻到 FP/FN、置信度、IoU 与场景切片，并保留可复查的门禁证据。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="suiteVisible = true">新建评估套件</el-button>
    </header>
    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadAll">
        <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
      </el-select>
      <el-button @click="loadAll">刷新</el-button>
    </section>
    <section class="summary-grid">
      <article><small>评估运行</small><strong>{{ runs.length }}</strong><span>数据集与训练血缘固定</span></article>
      <article><small>已通过</small><strong>{{ passedCount }}</strong><span>MUST_PASS 门禁</span></article>
      <article><small>FP / FN</small><strong>{{ fpCount }} / {{ fnCount }}</strong><span>困难样本可检索</span></article>
      <article><small>mAP</small><strong>{{ primaryMAP }}</strong><span>Evaluator 版本可追溯</span></article>
    </section>
    <section class="content-grid">
      <aside>
        <header><b>Evaluation Suite</b><span>阈值、切片、门禁策略</span></header>
        <button v-for="suite in suites" :key="suite.id" :class="{ active: selectedSuite?.id === suite.id }" @click="selectedSuite = suite">
          <span>DatasetVersion #{{ suite.datasetVersionId }}</span><strong>{{ suite.name }}</strong><small>{{ suite.gatePolicy }} · {{ suite.evaluatorVersion }}</small>
        </button>
        <el-button v-if="selectedSuite" @click="runVisible = true">运行评估</el-button>
      </aside>
      <section class="run-list">
        <article v-for="run in runs" :key="run.id" class="run-card" @click="showRun(run)">
          <div><b>EvaluationRun #{{ run.id }}</b><span>TrainingRun #{{ run.trainingRunId }}</span></div>
          <el-tag :type="run.gateDecision === 'PASSED' ? 'success' : run.gateDecision === 'BLOCKED' ? 'danger' : 'warning'">{{ run.gateDecision }}</el-tag>
          <p>{{ run.fiftyOneDataset || run.status }}</p>
        </article>
        <div v-if="!runs.length" class="empty-state">创建 Suite 并选择成功训练运行开始评估。</div>
      </section>
    </section>

    <el-dialog v-model="suiteVisible" title="新建评估套件" width="580">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="suiteForm.name" /></el-form-item>
        <el-form-item label="冻结 DatasetVersion ID"><el-input-number v-model="suiteForm.datasetVersionId" :min="1" /></el-form-item>
        <el-form-item label="门禁策略"><el-select v-model="suiteForm.gatePolicy" class="full"><el-option label="必须通过" value="MUST_PASS" /><el-option label="允许回归" value="ALLOW_REGRESSION" /><el-option label="人工审核" value="MANUAL_REVIEW" /></el-select></el-form-item>
        <div class="form-grid"><el-form-item label="mAP 下限"><el-input-number v-model="suiteForm.map" :min="0" :max="1" :step="0.05" /></el-form-item><el-form-item label="Recall 下限"><el-input-number v-model="suiteForm.recall" :min="0" :max="1" :step="0.05" /></el-form-item></div>
      </el-form>
      <template #footer><el-button @click="suiteVisible = false">取消</el-button><el-button type="primary" @click="submitSuite">创建</el-button></template>
    </el-dialog>
    <el-dialog v-model="runVisible" title="运行评估" width="500">
      <el-form label-position="top"><el-form-item label="成功 TrainingRun ID"><el-input-number v-model="runForm.trainingRunId" :min="1" /></el-form-item><el-form-item label="Baseline Run（可选）"><el-input-number v-model="runForm.baselineRunId" :min="0" /></el-form-item></el-form>
      <template #footer><el-button @click="runVisible = false">取消</el-button><el-button type="primary" @click="submitRun">进入队列</el-button></template>
    </el-dialog>
    <el-drawer v-model="detailVisible" title="评估证据与困难样本" size="65%">
      <template v-if="detail">
        <section class="metric-grid"><article v-for="metric in detail.metrics" :key="metric.id"><small>{{ metric.name }}</small><strong>{{ formatMetric(metric) }}</strong></article></section>
        <div class="slice-row"><el-button v-for="slice in detail.savedSlices" :key="slice.id" @click="filterSamples(slice.name)">{{ slice.name }} · {{ slice.sampleCount }}</el-button><el-button type="primary" plain @click="workbench">受控工作台</el-button></div>
        <el-table :data="detail.samples">
          <el-table-column prop="assetId" label="Asset" width="90" />
          <el-table-column prop="split" label="Split" width="85" />
          <el-table-column prop="slice" label="切片" />
          <el-table-column prop="errorType" label="错误" width="90"><template #default="{ row }"><el-tag :type="row.errorType === 'TP' ? 'success' : 'danger'">{{ row.errorType }}</el-tag></template></el-table-column>
          <el-table-column prop="confidence" label="Confidence" />
          <el-table-column prop="iou" label="IoU" />
          <el-table-column prop="latencyMs" label="Latency(ms)" />
        </el-table>
      </template>
    </el-drawer>
    <EmbeddedWorkbench
      v-model="workbenchVisible"
      provider="FiftyOne"
      :title="workbenchTitle"
      :context="workbenchContext"
      :url="workbenchUrl"
    />
  </main>
</template>

<script lang="ts" setup>
import EmbeddedWorkbench from '@/views/ai-platform/components/EmbeddedWorkbench.vue'
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  createEvaluationRun, createEvaluationSuite, getEvaluationRun, getEvaluationRuns,
  getEvaluationSuites, openEvaluationWorkbench, type EvaluationMetric, type EvaluationRun,
  type EvaluationSuite
} from '@/api/ai-platform/evaluation'

defineOptions({ name: 'VisionAIEvaluation' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const suites = ref<EvaluationSuite[]>([])
const selectedSuite = ref<EvaluationSuite>()
const runs = ref<EvaluationRun[]>([])
const detail = ref<Awaited<ReturnType<typeof getEvaluationRun>>>()
const suiteVisible = ref(false)
const runVisible = ref(false)
const detailVisible = ref(false)
const workbenchVisible = ref(false)
const workbenchUrl = ref('')
const workbenchTitle = ref('FiftyOne 评估工作台')
const workbenchContext = ref('')
const suiteForm = reactive({ name: '', datasetVersionId: 3, gatePolicy: 'MUST_PASS', map: 0.5, recall: 0.5 })
const runForm = reactive({ trainingRunId: 1, baselineRunId: 0 })
const passedCount = computed(() => runs.value.filter((run) => run.gateDecision === 'PASSED').length)
const metricValue = (name: string) => detail.value?.metrics.find((m) => m.name === name)?.value || 0
const fpCount = computed(() => metricValue('FP'))
const fnCount = computed(() => metricValue('FN'))
const primaryMAP = computed(() => metricValue('mAP').toFixed(3))
const loadAll = async () => {
  if (!projectId.value) return
  ;[suites.value, runs.value] = await Promise.all([getEvaluationSuites(projectId.value), getEvaluationRuns(projectId.value)])
  selectedSuite.value = suites.value[0]
  if (runs.value[0]?.status === 'SUCCEEDED') detail.value = await getEvaluationRun(projectId.value, runs.value[0].id)
}
const submitSuite = async () => {
  if (!projectId.value || !suiteForm.name.trim()) return
  selectedSuite.value = await createEvaluationSuite(projectId.value, {
    name: suiteForm.name, datasetVersionId: suiteForm.datasetVersionId, gatePolicy: suiteForm.gatePolicy,
    slices: ['all', 'small-object', 'occluded', 'dense', 'night', 'low-confidence'],
    thresholds: { mAP: suiteForm.map, precision: 0.5, recall: suiteForm.recall }
  })
  suiteVisible.value = false
  await loadAll()
}
const submitRun = async () => {
  if (!projectId.value || !selectedSuite.value) return
  await createEvaluationRun(projectId.value, selectedSuite.value.id, runForm)
  runVisible.value = false
  message.success('评估已进入统一任务队列')
  setTimeout(loadAll, 1500)
}
const showRun = async (run: EvaluationRun) => {
  detail.value = await getEvaluationRun(projectId.value!, run.id)
  detailVisible.value = true
}
const filterSamples = async (name: string) => {
  if (!detail.value) return
  const type = name.includes('Positive') ? 'FP' : name.includes('Negative') ? 'FN' : undefined
  detail.value = await getEvaluationRun(projectId.value!, detail.value.run.id, type)
}
const workbench = async () => {
  if (!detail.value) return
  const result = await openEvaluationWorkbench(projectId.value!, detail.value.run.id)
  workbenchUrl.value = result.url
  workbenchTitle.value = `EvaluationRun #${detail.value.run.id}`
  workbenchContext.value = `${result.dataset} · TrainingRun #${detail.value.run.trainingRunId} · ${detail.value.run.gateDecision}`
  workbenchVisible.value = true
  message.success(`已授权并载入数据集 ${result.dataset}`)
}
const formatMetric = (metric: EvaluationMetric) => ['FP', 'FN'].includes(metric.name) ? metric.value : metric.value.toFixed(3)
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.evaluation-page { min-height: 100%; padding: var(--app-content-padding); color: var(--text-primary); background: linear-gradient(145deg, rgb(22 119 255 / 6%), transparent 38%); }
.page-header, .toolbar, .run-card { display: flex; align-items: center; justify-content: space-between; }
.page-header { margin-bottom: 22px; }
.page-header h1 { margin: 5px 0; font-size: 30px; }
.page-header p { margin: 0; color: var(--text-secondary); }
.eyebrow { font-size: 12px; font-weight: 750; letter-spacing: 1.8px; color: var(--el-color-primary); }
.toolbar { justify-content: flex-start; gap: 12px; margin-bottom: 18px; }
.toolbar .el-select { width: 260px; }
.summary-grid, .metric-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 18px; }
.summary-grid article, .metric-grid article, aside, .run-card { padding: 16px; background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter); border-radius: 13px; }
.summary-grid article, .metric-grid article { display: flex; flex-direction: column; gap: 3px; }
.summary-grid strong, .metric-grid strong { font-size: 25px; }
.summary-grid span, .summary-grid small, .metric-grid small, aside span, aside small, .run-card span, .run-card p { font-size: 12px; color: var(--text-secondary); }
.content-grid { display: grid; grid-template-columns: 310px minmax(0, 1fr); gap: 18px; }
aside { display: flex; flex-direction: column; gap: 8px; }
aside header, aside button, .run-card div { display: flex; flex-direction: column; gap: 4px; }
aside button { padding: 14px; text-align: left; cursor: pointer; background: transparent; border: 1px solid var(--el-border-color-lighter); border-radius: 9px; }
aside button.active { border-color: var(--el-color-primary); }
.run-list { display: flex; flex-direction: column; gap: 10px; }
.run-card { cursor: pointer; }
.run-card p { margin: 0; }
.full { width: 100%; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.slice-row { display: flex; gap: 9px; margin-bottom: 15px; flex-wrap: wrap; }
.empty-state { padding: 70px; text-align: center; color: var(--text-secondary); border: 1px dashed var(--el-border-color); border-radius: 12px; }
</style>

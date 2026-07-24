<template>
  <main class="deployment-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">MODEL SERVING</span>
        <h1>部署、推理与监控</h1>
        <p>查看当前服务是否就绪，上传图片完成回归测试，并管理修订、告警与回滚。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="createVisible = true">
        <Icon icon="lucide:rocket" :size="16" />创建部署
      </el-button>
    </header>

    <section class="toolbar" aria-label="部署筛选与刷新">
      <el-select v-model="projectId" placeholder="选择项目" @change="() => loadAll()">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="project.name"
          :value="project.id"
        />
      </el-select>
      <el-button :loading="loading" @click="() => loadAll()">
        <Icon icon="lucide:refresh-cw" :size="15" />刷新
      </el-button>
      <span v-if="detail" class="refresh-note"
        >每 5 秒自动刷新 · {{ formatTime(detail.inferenceStatus.checkedAt) }}</span
      >
    </section>

    <section v-loading="loading" class="deployment-grid" aria-label="部署列表">
      <button
        v-for="item in deployments"
        :key="item.id"
        type="button"
        class="deployment-card"
        :class="{ selected: selected?.id === item.id }"
        :aria-pressed="selected?.id === item.id"
        @click="selectDeployment(item)"
      >
        <span class="deployment-card__head">
          <span>{{ environmentLabel(item.environment) }}</span>
          <el-tag :type="deploymentStatusType(item.status)">{{
            deploymentStatusLabel(item.status)
          }}</el-tag>
        </span>
        <strong>{{ item.name }}</strong>
        <span>Revision {{ item.currentRevisionId ? `#${item.currentRevisionId}` : '未激活' }}</span>
        <code>{{ item.endpointUrl || '尚未分配 Endpoint' }}</code>
      </button>
      <el-empty v-if="!deployments.length && !loading" description="当前项目还没有部署">
        <el-button type="primary" @click="createVisible = true">创建第一个部署</el-button>
      </el-empty>
    </section>

    <template v-if="detail">
      <section
        class="inference-status"
        :class="{ ready: detail.inferenceStatus.ready }"
        aria-live="polite"
      >
        <div class="status-summary">
          <span class="status-dot" aria-hidden="true"></span>
          <div>
            <strong>{{
              detail.inferenceStatus.ready ? '推理服务已就绪' : '推理服务暂不可用'
            }}</strong>
            <span>
              {{
                detail.inferenceStatus.ready ? '当前修订可接受在线请求' : inferenceUnavailableReason
              }}
            </span>
          </div>
        </div>
        <dl>
          <div
            ><dt>部署</dt
            ><dd>{{ deploymentStatusLabel(detail.inferenceStatus.deploymentStatus) }}</dd></div
          >
          <div
            ><dt>当前修订</dt
            ><dd>{{
              detail.inferenceStatus.revisionNo ? `R${detail.inferenceStatus.revisionNo}` : '—'
            }}</dd></div
          >
          <div
            ><dt>模型版本</dt
            ><dd>{{
              detail.inferenceStatus.modelVersionId
                ? `#${detail.inferenceStatus.modelVersionId}`
                : '—'
            }}</dd></div
          >
          <div
            ><dt>最近请求</dt
            ><dd>{{
              detail.inferenceStatus.lastRequestAt
                ? formatTime(detail.inferenceStatus.lastRequestAt)
                : '暂无'
            }}</dd></div
          >
        </dl>
        <div class="service-actions">
          <el-button
            v-if="detail.deployment.status === 'RUNNING'"
            :loading="controlling"
            @click="stopCurrent"
          >
            <Icon icon="lucide:square" :size="15" />停止
          </el-button>
          <el-button v-else type="primary" :loading="controlling" @click="restartCurrent">
            <Icon icon="lucide:rotate-cw" :size="15" />重新启动
          </el-button>
        </div>
      </section>

      <section class="metric-grid" aria-label="最近一百次请求指标">
        <article
          ><small>请求</small><strong>{{ detail.metrics.requests || 0 }}</strong
          ><span>{{ format(detail.metrics.qps) }} QPS</span></article
        >
        <article
          ><small>错误率</small><strong>{{ percent(detail.metrics.errorRate) }}</strong
          ><span>最近 100 次请求</span></article
        >
        <article
          ><small>P95 / P99</small
          ><strong>{{ format(detail.metrics.p95) }} / {{ format(detail.metrics.p99) }}</strong
          ><span>毫秒</span></article
        >
        <article
          ><small>平均置信度</small><strong>{{ format(detail.metrics.meanConfidence) }}</strong
          ><span>空结果率 {{ percent(detail.metrics.emptyRate) }}</span></article
        >
      </section>

      <section class="workspace">
        <section class="panel inference-panel">
          <div class="section-title">
            <div><h2>在线图片测试</h2><p>直接上传图片，或选择项目中的已有资产。</p></div>
          </div>

          <el-tabs v-model="testMode" class="test-tabs">
            <el-tab-pane label="上传图片" name="upload">
              <el-upload
                ref="uploadRef"
                class="regression-upload"
                drag
                :auto-upload="false"
                :limit="1"
                accept="image/jpeg,image/png,image/webp"
                :on-change="handleImageChange"
                :on-remove="handleImageRemove"
              >
                <Icon icon="lucide:image-up" :size="28" />
                <div class="el-upload__text">拖入图片，或 <em>点击选择</em></div>
                <template #tip>
                  <div class="el-upload__tip">JPEG、PNG、WebP · 最大 20 MiB · 不会写入数据资产</div>
                </template>
              </el-upload>
              <div class="assertion-grid">
                <el-form-item label="预期标签（可选）">
                  <el-input v-model="expectedLabel" placeholder="例如 person；留空仅执行在线测试" />
                </el-form-item>
                <el-form-item label="最低置信度">
                  <el-input-number
                    v-model="minimumConfidence"
                    :min="0"
                    :max="1"
                    :step="0.05"
                    :precision="2"
                  />
                </el-form-item>
              </div>
              <div class="test-actions">
                <el-button
                  type="primary"
                  :loading="predicting"
                  :disabled="!selectedFile || !detail.inferenceStatus.ready"
                  @click="runImageRegression"
                >
                  <Icon icon="lucide:scan-search" :size="16" />
                  {{ expectedLabel.trim() ? '运行回归测试' : '运行图片测试' }}
                </el-button>
                <span v-if="!detail.inferenceStatus.ready">服务就绪后可测试</span>
              </div>
            </el-tab-pane>

            <el-tab-pane label="已有资产" name="asset">
              <div class="asset-test">
                <el-form-item label="Asset ID">
                  <el-input-number v-model="assetId" :min="1" />
                </el-form-item>
                <el-button
                  type="primary"
                  :loading="predicting"
                  :disabled="!detail.inferenceStatus.ready"
                  @click="runAssetPrediction"
                >
                  推理已有资产
                </el-button>
              </div>
            </el-tab-pane>
          </el-tabs>

          <article v-if="activePrediction" class="prediction" aria-live="polite">
            <header>
              <div>
                <span>TRACE {{ activePrediction.traceId }}</span>
                <strong
                  >Model #{{ activePrediction.modelVersionId }} · Revision #{{
                    activePrediction.deploymentRevisionId
                  }}</strong
                >
              </div>
              <el-tag
                v-if="regressionPrediction"
                :type="regressionTagType(regressionPrediction.regression.status)"
              >
                {{ regressionLabel(regressionPrediction.regression.status) }}
              </el-tag>
            </header>
            <p>
              {{ activePrediction.result.detections.length }} 个检测结果 · Provider
              {{ format(activePrediction.result.latencyMs) }} ms · 平台
              {{ format(activePrediction.platformLatencyMs) }} ms
            </p>
            <p v-if="regressionPrediction?.regression.expectedLabel" class="assertion-result">
              预期 {{ regressionPrediction.regression.expectedLabel }} ≥
              {{ percent(regressionPrediction.regression.minimumConfidence) }}，命中
              {{ regressionPrediction.regression.matchedDetectionCount }} 个
            </p>
            <div
              v-for="(detection, index) in activePrediction.result.detections"
              :key="`${detection.label}-${index}`"
              class="detection"
            >
              <b>{{ detection.label }}</b>
              <el-progress :percentage="Math.round(detection.confidence * 100)" />
            </div>
            <el-empty
              v-if="!activePrediction.result.detections.length"
              :image-size="52"
              description="模型未返回检测框"
            />
          </article>

          <div class="trace-title"><h3>最近推理记录</h3><span>保留最近 100 次请求</span></div>
          <el-table :data="detail.traces" max-height="360" empty-text="暂无推理记录">
            <el-table-column label="输入" min-width="150">
              <template #default="{ row }">
                <div class="trace-source">
                  <strong>{{ sourceLabel(row.sourceType) }}</strong>
                  <span>{{ row.sourceName || (row.assetId ? `Asset #${row.assetId}` : '—') }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="Trace" min-width="135">
              <template #default="{ row }"
                ><code>{{ row.traceId.slice(0, 12) }}…</code></template
              >
            </el-table-column>
            <el-table-column label="回归" width="95">
              <template #default="{ row }">
                <el-tag
                  v-if="row.testMode === 'REGRESSION'"
                  size="small"
                  :type="regressionTagType(row.regressionStatus)"
                >
                  {{ regressionLabel(row.regressionStatus) }}
                </el-tag>
                <span v-else>—</span>
              </template>
            </el-table-column>
            <el-table-column label="延迟" width="90"
              ><template #default="{ row }"
                >{{ format(row.latencyMs) }} ms</template
              ></el-table-column
            >
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tooltip :disabled="!row.errorMessage" :content="row.errorMessage">
                  <el-tag size="small" :type="row.status === 'SUCCEEDED' ? 'success' : 'danger'">{{
                    row.status === 'SUCCEEDED' ? '成功' : '失败'
                  }}</el-tag>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="时间" width="150"
              ><template #default="{ row }">{{
                formatTime(row.createTime)
              }}</template></el-table-column
            >
          </el-table>
        </section>

        <section class="panel revision-panel">
          <div class="section-title">
            <div><h2>Revision 与告警</h2><p>回滚会创建新 Revision，历史记录不会被覆盖。</p></div>
            <el-button @click="alertVisible = true">告警规则</el-button>
          </div>
          <el-timeline v-if="detail.revisions.length">
            <el-timeline-item
              v-for="revision in detail.revisions"
              :key="revision.id"
              :type="revision.status === 'RUNNING' ? 'success' : 'primary'"
              :timestamp="revision.activatedAt ? formatTime(revision.activatedAt) : '待激活'"
            >
              <b
                >Revision {{ revision.revisionNo }} ·
                {{ deploymentStatusLabel(revision.status) }}</b
              >
              <p
                >ModelVersion #{{ revision.modelVersionId }}
                <span v-if="revision.sourceRevisionId"
                  >· 回滚自 #{{ revision.sourceRevisionId }}</span
                ></p
              >
              <el-button
                v-if="revision.id !== detail.deployment.currentRevisionId"
                link
                type="primary"
                :loading="rollingBackId === revision.id"
                @click="rollback(revision.id)"
              >
                回滚到此修订
              </el-button>
            </el-timeline-item>
          </el-timeline>
          <el-empty v-else :image-size="70" description="暂无修订记录" />
          <div class="alerts-title"
            ><h3>告警事件</h3><span>{{ detail.alerts.length }} 条</span></div
          >
          <div v-for="alert in detail.alerts" :key="alert.id" class="alert-row">
            <div
              ><b>{{ alert.message }}</b
              ><span>{{ alert.status }} · {{ formatTime(alert.createTime) }}</span></div
            >
            <el-button v-if="alert.status === 'OPEN'" link @click="ack(alert.id)">确认</el-button>
          </div>
          <el-empty v-if="!detail.alerts.length" :image-size="60" description="当前没有告警事件" />
        </section>
      </section>
    </template>

    <el-dialog v-model="createVisible" title="创建受控部署" width="570">
      <el-alert
        type="warning"
        :closable="false"
        title="生产环境仅允许已批准模型；测试放宽必须显式开启。"
      />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="名称"><el-input v-model="createForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="环境">
            <el-select v-model="createForm.environment" class="full">
              <el-option label="生产" value="PRODUCTION" />
              <el-option label="金丝雀" value="CANARY" />
              <el-option label="预发" value="STAGING" />
            </el-select>
          </el-form-item>
          <el-form-item label="ModelVersion ID"
            ><el-input-number v-model="createForm.modelVersionId" :min="1"
          /></el-form-item>
        </div>
        <el-form-item label="置信度阈值">
          <el-slider
            v-model="createForm.confidenceThreshold"
            :min="0"
            :max="1"
            :step="0.05"
            show-input
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="doCreate">创建并发布</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="alertVisible" title="创建告警规则" width="500">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="alertForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="指标">
            <el-select v-model="alertForm.metric" class="full">
              <el-option label="延迟(ms)" value="LATENCY_MS" />
              <el-option label="错误" value="ERROR" />
              <el-option label="空结果" value="EMPTY_RESULT" />
              <el-option label="置信度" value="CONFIDENCE" />
            </el-select>
          </el-form-item>
          <el-form-item label="条件">
            <div class="condition">
              <el-select v-model="alertForm.operator"
                ><el-option label=">" value=">" /><el-option label="<" value="<"
              /></el-select>
              <el-input-number v-model="alertForm.threshold" />
            </div>
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="alertVisible = false">取消</el-button>
        <el-button type="primary" :loading="creatingAlert" @click="doCreateAlert">创建</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  acknowledgeAlert,
  createAlertRule,
  createDeployment,
  getDeployment,
  getDeployments,
  predict,
  predictImage,
  restartDeployment,
  rollbackDeployment,
  stopDeployment,
  type Deployment,
  type ImageRegressionResponse,
  type PredictionResponse
} from '@/api/ai-platform/deployments'

defineOptions({ name: 'VisionAIDeployments' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const deployments = ref<Deployment[]>([])
const selected = ref<Deployment>()
const detail = ref<Awaited<ReturnType<typeof getDeployment>>>()
const assetPrediction = ref<PredictionResponse>()
const regressionPrediction = ref<ImageRegressionResponse>()
const activePrediction = computed(() => regressionPrediction.value || assetPrediction.value)
const createVisible = ref(false)
const alertVisible = ref(false)
const loading = ref(false)
const predicting = ref(false)
const creating = ref(false)
const creatingAlert = ref(false)
const controlling = ref(false)
const rollingBackId = ref<number>()
const testMode = ref<'upload' | 'asset'>('upload')
const selectedFile = ref<File>()
const uploadRef = ref<{ clearFiles: () => void }>()
const expectedLabel = ref('')
const minimumConfidence = ref(0.5)
const assetId = ref(204)
let pollTimer: number | undefined
const createForm = reactive({
  name: '生产缺陷检测',
  environment: 'PRODUCTION',
  modelVersionId: 2,
  confidenceThreshold: 0.5
})
const alertForm = reactive({
  name: 'P95 延迟过高',
  metric: 'LATENCY_MS',
  operator: '>',
  threshold: 100
})

const format = (value = 0) => Number(value).toFixed(2)
const percent = (value = 0) => `${(Number(value) * 100).toFixed(1)}%`
const formatTime = (value?: string) => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '—'
    : new Intl.DateTimeFormat('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
      }).format(date)
}
const environmentLabel = (value: string) =>
  ({
    PRODUCTION: '生产',
    CANARY: '金丝雀',
    STAGING: '预发'
  })[value] || value
const deploymentStatusLabel = (value?: string) =>
  ({
    DRAFT: '草稿',
    QUEUED: '排队中',
    DEPLOYING: '发布中',
    RUNNING: '运行中',
    STOPPED: '已停止',
    FAILED: '失败',
    ROLLED_BACK: '已回滚'
  })[value || ''] ||
  value ||
  '未知'
const deploymentStatusType = (value: string) =>
  value === 'RUNNING'
    ? 'success'
    : value === 'FAILED'
      ? 'danger'
      : value === 'DEPLOYING'
        ? 'warning'
        : 'info'
const regressionLabel = (value: string) =>
  ({
    PASSED: '通过',
    FAILED: '未通过',
    NOT_ASSERTED: '未设置断言'
  })[value] || value
const regressionTagType = (value: string) =>
  value === 'PASSED' ? 'success' : value === 'FAILED' ? 'danger' : 'info'
const sourceLabel = (value: string) =>
  ({
    ASSET: '已有资产',
    UPLOAD: '上传图片',
    VIDEO_UPLOAD: '上传视频'
  })[value] || '在线请求'
const inferenceUnavailableReason = computed(() => {
  if (!detail.value) return ''
  if (detail.value.deployment.status !== 'RUNNING') {
    return `部署状态为“${deploymentStatusLabel(detail.value.deployment.status)}”`
  }
  if (!detail.value.inferenceStatus.currentRevisionId) return '还没有激活的修订'
  return `当前修订状态为“${deploymentStatusLabel(detail.value.inferenceStatus.revisionStatus)}”`
})

const refreshDetail = async (deploymentId: number) => {
  if (!projectId.value) return
  detail.value = await getDeployment(projectId.value, deploymentId)
}
const loadAll = async (background = false) => {
  if (!projectId.value) return
  if (!background) loading.value = true
  try {
    const data = await getDeployments(projectId.value)
    deployments.value = data.deployments
    selected.value =
      deployments.value.find((row) => row.id === selected.value?.id) || deployments.value[0]
    detail.value = selected.value
      ? await getDeployment(projectId.value, selected.value.id)
      : undefined
  } finally {
    if (!background) loading.value = false
  }
}
const selectDeployment = async (item: Deployment) => {
  selected.value = item
  assetPrediction.value = undefined
  regressionPrediction.value = undefined
  await refreshDetail(item.id)
}
const doCreate = async () => {
  if (!projectId.value || !createForm.name.trim()) {
    message.warning('请填写部署名称')
    return
  }
  creating.value = true
  try {
    await createDeployment(projectId.value, {
      name: createForm.name,
      environment: createForm.environment,
      modelVersionId: createForm.modelVersionId,
      config: { replicas: 1, confidenceThreshold: createForm.confidenceThreshold, maxBatchSize: 8 }
    })
    createVisible.value = false
    message.success('部署已进入统一任务队列')
    await loadAll()
  } finally {
    creating.value = false
  }
}
const handleImageChange = (uploadFile: { raw?: File; size?: number }) => {
  if (!uploadFile.raw) return
  if ((uploadFile.size || uploadFile.raw.size) > 20 * 1024 * 1024) {
    message.warning('图片不能超过 20 MiB')
    selectedFile.value = undefined
    uploadRef.value?.clearFiles()
    return
  }
  selectedFile.value = uploadFile.raw
}
const handleImageRemove = () => {
  selectedFile.value = undefined
}
const runImageRegression = async () => {
  if (!selected.value || !projectId.value || !selectedFile.value) {
    message.warning('请先选择一张图片')
    return
  }
  predicting.value = true
  try {
    regressionPrediction.value = await predictImage(
      projectId.value,
      selected.value.id,
      selectedFile.value,
      expectedLabel.value,
      minimumConfidence.value
    )
    assetPrediction.value = undefined
    const status = regressionPrediction.value.regression.status
    if (status === 'PASSED') message.success('回归测试通过')
    else if (status === 'FAILED') message.error('回归测试未通过，请检查检测结果和断言')
    else message.success('图片推理完成')
    await refreshDetail(selected.value.id)
  } finally {
    predicting.value = false
  }
}
const runAssetPrediction = async () => {
  if (!selected.value || !projectId.value) return
  predicting.value = true
  try {
    assetPrediction.value = await predict(projectId.value, selected.value.id, assetId.value)
    regressionPrediction.value = undefined
    message.success(`推理完成：${assetPrediction.value.traceId}`)
    await refreshDetail(selected.value.id)
  } finally {
    predicting.value = false
  }
}
const rollback = async (revisionId: number) => {
  if (!selected.value || !projectId.value) return
  await message.confirm('回滚会基于目标模型创建新的不可变修订，是否继续？')
  rollingBackId.value = revisionId
  try {
    await rollbackDeployment(projectId.value, selected.value.id, revisionId)
    message.success('回滚修订已创建')
    await loadAll()
  } finally {
    rollingBackId.value = undefined
  }
}
const stopCurrent = async () => {
  if (!selected.value || !projectId.value) return
  await message.confirm('停止后 Endpoint 将拒绝新的推理请求，是否继续？')
  controlling.value = true
  try {
    await stopDeployment(projectId.value, selected.value.id)
    message.success('部署已停止')
    await loadAll()
  } finally {
    controlling.value = false
  }
}
const restartCurrent = async () => {
  if (!selected.value || !projectId.value) return
  controlling.value = true
  try {
    await restartDeployment(projectId.value, selected.value.id)
    message.success('部署已进入重新启动流程')
    await loadAll()
  } finally {
    controlling.value = false
  }
}
const doCreateAlert = async () => {
  if (!selected.value || !projectId.value || !alertForm.name.trim()) {
    message.warning('请填写告警规则名称')
    return
  }
  creatingAlert.value = true
  try {
    await createAlertRule(projectId.value, selected.value.id, alertForm)
    alertVisible.value = false
    message.success('告警规则已启用')
    await refreshDetail(selected.value.id)
  } finally {
    creatingAlert.value = false
  }
}
const ack = async (alertId: number) => {
  if (!selected.value || !projectId.value) return
  await acknowledgeAlert(projectId.value, alertId)
  await refreshDetail(selected.value.id)
}

onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
  pollTimer = window.setInterval(() => selected.value && loadAll(true), 5000)
})
onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer)
})
</script>

<style scoped lang="scss">
.deployment-page {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--el-fill-color-extra-light);
}

.page-header,
.toolbar,
.section-title,
.alert-row,
.trace-title,
.alerts-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-header h1 {
  margin: 5px 0;
  font-size: 30px;
  letter-spacing: -1px;
}

.page-header p,
.section-title p {
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
  margin: 20px 0;
}

.toolbar .el-select {
  width: 260px;
}

.refresh-note {
  font-size: 12px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

.deployment-grid {
  display: grid;
  min-height: 80px;
  margin-bottom: 18px;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.deployment-grid > .el-empty {
  grid-column: 1 / -1;
}

.deployment-card {
  display: flex;
  min-height: 156px;
  padding: 16px;
  color: inherit;
  text-align: left;
  cursor: pointer;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  flex-direction: column;
  gap: 9px;
  transition:
    border-color 180ms ease,
    box-shadow 180ms ease;
}

.deployment-card:hover {
  border-color: var(--el-color-primary-light-3);
}

.deployment-card:focus-visible {
  outline: 3px solid var(--el-color-primary-light-5);
  outline-offset: 2px;
}

.deployment-card.selected {
  border-color: var(--el-color-primary);
  box-shadow: 0 4px 14px rgb(21 94 239 / 10%);
}

.deployment-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.deployment-card__head > span {
  font-size: 12px;
  font-weight: 700;
  color: var(--el-color-primary);
}

.deployment-card > strong {
  font-size: 18px;
}

.deployment-card > span:not(.deployment-card__head) {
  font-size: 13px;
  color: var(--text-secondary);
}

code {
  overflow: hidden;
  font-size: 12px;
  color: var(--el-color-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inference-status {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) minmax(460px, 1.7fr) auto;
  align-items: center;
  gap: 24px;
  padding: 18px;
  margin-bottom: 14px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-color-warning-light-5);
  border-left: 4px solid var(--el-color-warning);
  border-radius: 4px;
}

.inference-status.ready {
  border-color: var(--el-color-success-light-5);
  border-left-color: var(--el-color-success);
}

.status-summary {
  display: flex;
  align-items: center;
  gap: 12px;
}

.status-summary > div {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.status-summary span {
  font-size: 12px;
  color: var(--text-secondary);
}

.status-dot {
  width: 12px;
  height: 12px;
  background: var(--el-color-warning);
  border-radius: 50%;
  box-shadow: 0 0 0 5px var(--el-color-warning-light-8);
}

.ready .status-dot {
  background: var(--el-color-success);
  box-shadow: 0 0 0 5px var(--el-color-success-light-8);
}

.inference-status dl {
  display: grid;
  grid-template-columns: repeat(4, minmax(80px, 1fr));
  gap: 8px;
  margin: 0;
}

.inference-status dl div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.inference-status dt {
  font-size: 11px;
  color: var(--text-secondary);
}

.inference-status dd {
  margin: 0;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.service-actions {
  display: flex;
  justify-content: flex-end;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 18px;
}

.metric-grid article,
.panel {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}

.metric-grid article {
  display: flex;
  padding: 15px;
  flex-direction: column;
  gap: 3px;
}

.metric-grid strong {
  font-size: 23px;
  font-variant-numeric: tabular-nums;
}

.metric-grid small,
.metric-grid span {
  color: var(--text-secondary);
}

.workspace {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(360px, 0.65fr);
  gap: 18px;
}

.panel {
  padding: 18px;
}

.section-title h2 {
  margin: 0 0 3px;
}

.test-tabs {
  margin-top: 12px;
}

.regression-upload :deep(.el-upload-dragger) {
  padding: 25px 16px;
  border-radius: 4px;
}

.regression-upload :deep(.el-upload-dragger svg) {
  color: var(--el-color-primary);
}

.assertion-grid {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 190px;
  gap: 14px;
  margin-top: 18px;
}

.assertion-grid .el-form-item {
  margin-bottom: 8px;
}

.test-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 4px 0 18px;
}

.test-actions span {
  font-size: 12px;
  color: var(--text-secondary);
}

.asset-test {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  min-height: 105px;
}

.asset-test .el-form-item {
  margin-bottom: 0;
}

.prediction {
  padding: 16px;
  margin: 2px 0 18px;
  background: var(--el-color-primary-light-9);
  border: 1px solid var(--el-color-primary-light-7);
  border-radius: 4px;
}

.prediction header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.prediction header > div,
.prediction header span,
.prediction header strong {
  display: block;
}

.prediction header span {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  color: var(--el-color-primary);
}

.prediction p {
  color: var(--text-secondary);
}

.assertion-result {
  padding: 8px 10px;
  background: var(--el-bg-color);
  border-left: 3px solid var(--el-color-primary);
}

.detection {
  display: grid;
  grid-template-columns: 100px 1fr;
  align-items: center;
  gap: 10px;
}

.trace-title,
.alerts-title {
  margin-top: 14px;
}

.trace-title h3,
.alerts-title h3 {
  margin: 0;
}

.trace-title span,
.alerts-title span {
  font-size: 12px;
  color: var(--text-secondary);
}

.trace-source {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.trace-source span {
  overflow: hidden;
  font-size: 11px;
  color: var(--text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.revision-panel :deep(.el-timeline) {
  padding-left: 6px;
  margin-top: 20px;
}

.revision-panel p {
  color: var(--text-secondary);
}

.alert-row {
  padding: 11px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.alert-row div {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.alert-row span {
  font-size: 12px;
  color: var(--text-secondary);
}

.dialog-form {
  margin-top: 16px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.full {
  width: 100%;
}

.condition {
  display: flex;
  gap: 8px;
}

@media (width <= 1250px) {
  .inference-status {
    grid-template-columns: 1fr 1.4fr;
  }

  .service-actions {
    grid-column: 1 / -1;
    justify-content: flex-start;
  }

  .workspace {
    grid-template-columns: 1fr;
  }
}

@media (width <= 900px) {
  .deployment-grid,
  .metric-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .inference-status {
    grid-template-columns: 1fr;
  }

  .inference-status dl {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (width <= 600px) {
  .page-header {
    align-items: flex-start;
    flex-direction: column;
    gap: 14px;
  }

  .toolbar {
    align-items: stretch;
    flex-wrap: wrap;
  }

  .toolbar .el-select {
    width: 100%;
  }

  .deployment-grid,
  .metric-grid,
  .assertion-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .inference-status dl {
    grid-template-columns: repeat(2, 1fr);
  }

  .asset-test {
    align-items: stretch;
    flex-direction: column;
  }
}

@media (prefers-reduced-motion: reduce) {
  .deployment-card {
    transition: none;
  }
}
</style>

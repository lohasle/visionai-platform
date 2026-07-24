<template>
  <main class="deployment-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">SAFE MODEL SERVING</span>
        <h1>部署、推理与监控</h1>
        <p>以不可变 Revision 发布模型，统一查看 Endpoint、推理 Trace、性能指标、告警和回滚历史。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="createVisible = true">创建部署</el-button>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadAll">
        <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
      </el-select>
      <el-button @click="loadAll">刷新</el-button>
    </section>

    <section class="deployment-grid">
      <article v-for="item in deployments" :key="item.id" :class="{ selected: selected?.id === item.id }" @click="selectDeployment(item)">
        <header><span>{{ item.environment }}</span><el-tag :type="item.status === 'RUNNING' ? 'success' : item.status === 'FAILED' ? 'danger' : 'info'">{{ item.status }}</el-tag></header>
        <h2>{{ item.name }}</h2>
        <p>Revision #{{ item.currentRevisionId || '—' }}</p>
        <code>{{ item.endpointUrl || '尚未分配 Endpoint' }}</code>
      </article>
    </section>

    <template v-if="detail">
      <section class="metric-grid">
        <article><small>请求</small><strong>{{ detail.metrics.requests || 0 }}</strong><span>{{ format(detail.metrics.qps) }} QPS</span></article>
        <article><small>错误率</small><strong>{{ percent(detail.metrics.errorRate) }}</strong><span>服务可用性</span></article>
        <article><small>P95 / P99</small><strong>{{ format(detail.metrics.p95) }} / {{ format(detail.metrics.p99) }}</strong><span>毫秒</span></article>
        <article><small>平均置信度</small><strong>{{ format(detail.metrics.meanConfidence) }}</strong><span>空结果率 {{ percent(detail.metrics.emptyRate) }}</span></article>
      </section>

      <section class="workspace">
        <section class="panel inference-panel">
          <div class="section-title"><div><h2>在线图片测试</h2><p>通过平台 Endpoint 调用真实 FastAPI Provider。</p></div></div>
          <div class="test-row"><el-input-number v-model="assetId" :min="1" /><el-button type="primary" @click="runPrediction">推理 Asset</el-button></div>
          <article v-if="prediction" class="prediction">
            <span>TRACE {{ prediction.traceId }}</span>
            <strong>Model #{{ prediction.modelVersionId }} · Revision #{{ prediction.deploymentRevisionId }}</strong>
            <p>{{ prediction.result.detections.length }} 个检测结果 · Provider {{ prediction.result.latencyMs }} ms</p>
            <div v-for="detection in prediction.result.detections" :key="detection.label" class="detection">
              <b>{{ detection.label }}</b><el-progress :percentage="Math.round(detection.confidence * 100)" />
            </div>
          </article>
          <el-table :data="detail.traces" max-height="310">
            <el-table-column prop="traceId" label="Trace"><template #default="{ row }"><code>{{ row.traceId.slice(0, 12) }}…</code></template></el-table-column>
            <el-table-column prop="deploymentRevisionId" label="Revision" width="90" />
            <el-table-column prop="modelVersionId" label="Model" width="80" />
            <el-table-column prop="latencyMs" label="Latency" width="100" />
            <el-table-column prop="status" label="状态" width="100" />
          </el-table>
        </section>

        <section class="panel">
          <div class="section-title"><div><h2>Revision 与告警</h2><p>回滚会创建新 Revision，历史记录不被改写。</p></div><el-button @click="alertVisible = true">告警规则</el-button></div>
          <el-timeline>
            <el-timeline-item v-for="revision in detail.revisions" :key="revision.id" :type="revision.status === 'RUNNING' ? 'success' : 'primary'" :timestamp="revision.activatedAt">
              <b>Revision {{ revision.revisionNo }} · {{ revision.status }}</b>
              <p>ModelVersion #{{ revision.modelVersionId }} <span v-if="revision.sourceRevisionId">· rollback from #{{ revision.sourceRevisionId }}</span></p>
              <el-button v-if="revision.id !== detail.deployment.currentRevisionId" link type="primary" @click="rollback(revision.id)">回滚到此修订</el-button>
            </el-timeline-item>
          </el-timeline>
          <h3>告警事件</h3>
          <div v-for="alert in detail.alerts" :key="alert.id" class="alert-row">
            <div><b>{{ alert.message }}</b><span>{{ alert.status }}</span></div>
            <el-button v-if="alert.status === 'OPEN'" link @click="ack(alert.id)">确认</el-button>
          </div>
        </section>
      </section>
    </template>

    <el-dialog v-model="createVisible" title="创建受控部署" width="570">
      <el-alert type="warning" :closable="false" title="PRODUCTION 环境仅允许已批准模型；测试放宽必须显式开启。" />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="名称"><el-input v-model="createForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="环境"><el-select v-model="createForm.environment" class="full"><el-option label="生产" value="PRODUCTION" /><el-option label="金丝雀" value="CANARY" /><el-option label="预发" value="STAGING" /></el-select></el-form-item>
          <el-form-item label="ModelVersion ID"><el-input-number v-model="createForm.modelVersionId" :min="1" /></el-form-item>
        </div>
        <el-form-item label="置信度阈值"><el-slider v-model="createForm.confidenceThreshold" :min="0" :max="1" :step="0.05" show-input /></el-form-item>
      </el-form>
      <template #footer><el-button @click="createVisible = false">取消</el-button><el-button type="primary" @click="doCreate">创建并发布</el-button></template>
    </el-dialog>

    <el-dialog v-model="alertVisible" title="创建告警规则" width="500">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="alertForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="指标"><el-select v-model="alertForm.metric" class="full"><el-option label="延迟(ms)" value="LATENCY_MS" /><el-option label="错误" value="ERROR" /><el-option label="空结果" value="EMPTY_RESULT" /><el-option label="置信度" value="CONFIDENCE" /></el-select></el-form-item>
          <el-form-item label="条件"><div class="condition"><el-select v-model="alertForm.operator"><el-option label=">" value=">" /><el-option label="<" value="<" /></el-select><el-input-number v-model="alertForm.threshold" /></div></el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="alertVisible = false">取消</el-button><el-button type="primary" @click="doCreateAlert">创建</el-button></template>
    </el-dialog>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  acknowledgeAlert, createAlertRule, createDeployment, getDeployment, getDeployments, predict,
  rollbackDeployment, type Deployment
} from '@/api/ai-platform/deployments'

defineOptions({ name: 'VisionAIDeployments' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const deployments = ref<Deployment[]>([])
const selected = ref<Deployment>()
const detail = ref<Awaited<ReturnType<typeof getDeployment>>>()
const prediction = ref<Awaited<ReturnType<typeof predict>>>()
const createVisible = ref(false)
const alertVisible = ref(false)
const assetId = ref(204)
const createForm = reactive({ name: '生产缺陷检测', environment: 'PRODUCTION', modelVersionId: 2, confidenceThreshold: 0.5 })
const alertForm = reactive({ name: 'P95 延迟过高', metric: 'LATENCY_MS', operator: '>', threshold: 100 })
const format = (value = 0) => Number(value).toFixed(2)
const percent = (value = 0) => `${(Number(value) * 100).toFixed(1)}%`
const loadAll = async () => {
  if (!projectId.value) return
  const data = await getDeployments(projectId.value)
  deployments.value = data.deployments
  if (selected.value) selected.value = deployments.value.find((row) => row.id === selected.value?.id)
  else selected.value = deployments.value[0]
  if (selected.value) detail.value = await getDeployment(projectId.value, selected.value.id)
}
const selectDeployment = async (item: Deployment) => {
  selected.value = item
  detail.value = await getDeployment(projectId.value!, item.id)
}
const doCreate = async () => {
  if (!projectId.value || !createForm.name.trim()) return
  await createDeployment(projectId.value, {
    name: createForm.name, environment: createForm.environment, modelVersionId: createForm.modelVersionId,
    config: { replicas: 1, confidenceThreshold: createForm.confidenceThreshold, maxBatchSize: 8 }
  })
  createVisible.value = false
  message.success('部署已进入统一任务队列')
  setTimeout(loadAll, 1500)
}
const runPrediction = async () => {
  if (!selected.value) return
  prediction.value = await predict(projectId.value!, selected.value.id, assetId.value)
  message.success(`推理完成：${prediction.value.traceId}`)
  await selectDeployment(selected.value)
}
const rollback = async (revisionId: number) => {
  if (!selected.value) return
  await rollbackDeployment(projectId.value!, selected.value.id, revisionId)
  message.success('回滚修订已创建')
  setTimeout(() => selectDeployment(selected.value!), 1500)
}
const doCreateAlert = async () => {
  if (!selected.value) return
  await createAlertRule(projectId.value!, selected.value.id, alertForm)
  alertVisible.value = false
  message.success('告警规则已启用')
  await selectDeployment(selected.value)
}
const ack = async (alertId: number) => {
  await acknowledgeAlert(projectId.value!, alertId)
  await selectDeployment(selected.value!)
}
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value.at(-1)?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.deployment-page { min-height: 100%; padding: var(--app-content-padding); color: var(--text-primary); background: radial-gradient(circle at 5% 0, rgb(5 150 105 / 9%), transparent 32%); }
.page-header, .toolbar, .deployment-grid header, .section-title, .alert-row { display: flex; align-items: center; justify-content: space-between; }
.page-header h1 { margin: 5px 0; font-size: 30px; }
.page-header p, .section-title p { margin: 0; color: var(--text-secondary); }
.eyebrow { color: #059669; font-size: 12px; font-weight: 750; letter-spacing: 1.8px; }
.toolbar { justify-content: flex-start; gap: 12px; margin: 20px 0; }
.toolbar .el-select { width: 260px; }
.deployment-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 13px; margin-bottom: 18px; }
.deployment-grid article, .metric-grid article, .panel { background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter); border-radius: 14px; }
.deployment-grid article { padding: 16px; cursor: pointer; }
.deployment-grid article.selected { border-color: #10b981; box-shadow: 0 8px 24px rgb(5 150 105 / 10%); }
.deployment-grid h2 { margin: 13px 0 4px; font-size: 18px; }
.deployment-grid p { margin: 0 0 10px; color: var(--text-secondary); }
.deployment-grid header span { color: #059669; font-size: 12px; font-weight: 700; }
code { color: #047857; font-size: 12px; }
.metric-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 18px; }
.metric-grid article { display: flex; flex-direction: column; gap: 3px; padding: 15px; }
.metric-grid strong { font-size: 23px; }
.metric-grid small, .metric-grid span { color: var(--text-secondary); }
.workspace { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(350px, .7fr); gap: 18px; }
.panel { padding: 18px; }
.section-title h2 { margin: 0 0 3px; }
.test-row { display: flex; gap: 10px; margin: 16px 0; }
.prediction { padding: 15px; margin-bottom: 15px; background: rgb(5 150 105 / 7%); border-radius: 11px; }
.prediction > span, .prediction > strong { display: block; }
.prediction > span { color: #047857; font-size: 11px; }
.prediction p { color: var(--text-secondary); }
.detection { display: grid; grid-template-columns: 100px 1fr; align-items: center; gap: 10px; }
.alert-row { padding: 10px 0; border-bottom: 1px solid var(--el-border-color-lighter); }
.alert-row div { display: flex; flex-direction: column; }
.alert-row span { color: var(--text-secondary); font-size: 12px; }
.dialog-form { margin-top: 16px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.full { width: 100%; }
.condition { display: flex; gap: 8px; }
</style>

<template>
  <main class="operations-page">
    <header class="page-header">
      <div><span class="eyebrow">PLATFORM CONTROL PLANE</span><h1>资源、集成与审计</h1><p>统一查看 GPU 心跳、计算队列、项目配额、外部实例兼容性、同步异常和不可变审计记录。</p></div>
      <div><el-button @click="integrationVisible = true">接入外部实例</el-button><el-button type="primary" @click="nodeVisible = true">模拟节点心跳</el-button></div>
    </header>
    <section class="summary-grid">
      <article><small>GPU 节点</small><strong>{{ resources.nodes.length }}</strong><span>{{ onlineNodes }} ONLINE</span></article>
      <article><small>运行 / 排队任务</small><strong>{{ resources.activeJobs }} / {{ resources.queuedJobs }}</strong><span>统一 Job 视图</span></article>
      <article><small>对象存储</small><strong>{{ formatBytes(resources.storageBytes) }}</strong><span>项目资产用量</span></article>
      <article><small>外部实例</small><strong>{{ integrations.instances.length }}</strong><span>{{ healthyInstances }} HEALTHY</span></article>
    </section>
    <el-tabs v-model="tab">
      <el-tab-pane label="计算资源" name="resources">
        <section class="two-columns">
          <section class="panel"><div class="section-title"><div><h2>GPU 节点</h2><p>显存、驱动、CUDA、标签和最近心跳。</p></div></div>
            <el-table :data="resources.nodes"><el-table-column prop="name" label="节点" /><el-table-column prop="gpuModel" label="GPU" /><el-table-column prop="gpuCount" label="数量" width="70" /><el-table-column prop="driverVersion" label="驱动" /><el-table-column prop="cudaVersion" label="CUDA" /><el-table-column prop="status" label="状态" /></el-table>
          </section>
          <section class="panel"><div class="section-title"><div><h2>计算队列</h2><p>平台队列映射 ClearML Queue。</p></div><el-button @click="queueVisible = true">配置</el-button></div>
            <article v-for="queue in resources.queues" :key="queue.id" class="compact-card"><div><b>{{ queue.name }}</b><span>{{ queue.provider }} / {{ queue.externalQueue }}</span></div><el-tag :type="queue.enabled ? 'success' : 'info'">{{ queue.enabled ? 'ENABLED' : 'DISABLED' }}</el-tag></article>
          </section>
        </section>
      </el-tab-pane>
      <el-tab-pane label="外部集成" name="integrations">
        <section class="panel">
          <el-table :data="integrations.instances"><el-table-column prop="name" label="实例" /><el-table-column prop="providerType" label="Provider" /><el-table-column prop="baseUrl" label="Base URL" /><el-table-column prop="secretRef" label="Secret Ref" /><el-table-column prop="version" label="版本" /><el-table-column prop="status" label="状态"><template #default="{ row }"><el-tag :type="row.status === 'HEALTHY' ? 'success' : 'warning'">{{ row.status }}</el-tag></template></el-table-column><el-table-column label="操作"><template #default="{ row }"><el-button link type="primary" @click="test(row.id)">连接测试</el-button></template></el-table-column></el-table>
        </section>
        <section class="panel incidents"><div class="section-title"><div><h2>同步异常</h2><p>SYNC_WARNING、ORPHANED、MAPPING_MISSING、AUTH_FAILED 可诊断重放。</p></div></div>
          <article v-for="incident in integrations.incidents" :key="incident.id" class="compact-card"><div><b>{{ incident.code }}</b><span>{{ incident.message }}</span></div><el-button v-if="incident.status !== 'RESOLVED'" @click="replay(incident.id)">重放</el-button></article>
        </section>
      </el-tab-pane>
      <el-tab-pane label="系统审计" name="audit">
        <section class="panel"><div class="section-title"><div><h2>不可变审计日志</h2><p>按用户、项目、动作、资源、Trace 与时间追溯。</p></div><a class="export-link" href="/admin-api/ai-platform/audit-events/export">导出 CSV</a></div>
          <el-table :data="audits" max-height="520"><el-table-column prop="createTime" label="时间" width="180" /><el-table-column prop="actorUserId" label="用户" width="75" /><el-table-column prop="projectId" label="项目" width="75" /><el-table-column prop="action" label="动作" min-width="210" /><el-table-column prop="resourceType" label="资源" width="150" /><el-table-column prop="resourceId" label="ID" width="75" /><el-table-column prop="traceId" label="Trace"><template #default="{ row }"><code>{{ row.traceId?.slice(0, 14) }}</code></template></el-table-column></el-table>
        </section>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="nodeVisible" title="GPU 节点心跳" width="540"><el-form label-position="top"><el-form-item label="节点标识"><el-input v-model="nodeForm.nodeKey" /></el-form-item><el-form-item label="名称"><el-input v-model="nodeForm.name" /></el-form-item><div class="form-grid"><el-form-item label="GPU 型号"><el-input v-model="nodeForm.gpuModel" /></el-form-item><el-form-item label="GPU 数量"><el-input-number v-model="nodeForm.gpuCount" :min="0" /></el-form-item></div><div class="form-grid"><el-form-item label="驱动"><el-input v-model="nodeForm.driverVersion" /></el-form-item><el-form-item label="CUDA"><el-input v-model="nodeForm.cudaVersion" /></el-form-item></div></el-form><template #footer><el-button @click="nodeVisible = false">取消</el-button><el-button type="primary" @click="saveNode">发送心跳</el-button></template></el-dialog>
    <el-dialog v-model="queueVisible" title="映射计算队列" width="500"><el-form label-position="top"><el-form-item label="名称"><el-input v-model="queueForm.name" /></el-form-item><el-form-item label="Provider"><el-select v-model="queueForm.provider" class="full"><el-option label="ClearML" value="CLEARML" /><el-option label="本地 Docker" value="LOCAL_DOCKER" /></el-select></el-form-item><el-form-item label="外部队列"><el-input v-model="queueForm.externalQueue" /></el-form-item></el-form><template #footer><el-button @click="queueVisible = false">取消</el-button><el-button type="primary" @click="doSaveQueue">保存</el-button></template></el-dialog>
    <el-dialog v-model="integrationVisible" title="接入外部实例" width="560"><el-alert type="info" :closable="false" title="平台只保存 Secret Ref，不保存凭据明文。" /><el-form label-position="top" class="dialog-form"><el-form-item label="实例名称"><el-input v-model="integrationForm.name" /></el-form-item><div class="form-grid"><el-form-item label="Provider"><el-select v-model="integrationForm.providerType" class="full"><el-option label="CVAT" value="CVAT" /><el-option label="FiftyOne" value="FIFTYONE" /><el-option label="Inference" value="INFERENCE" /><el-option label="ClearML" value="CLEARML" /></el-select></el-form-item><el-form-item label="版本"><el-input v-model="integrationForm.version" /></el-form-item></div><el-form-item label="Base URL"><el-input v-model="integrationForm.baseUrl" /></el-form-item><el-form-item label="Secret Ref"><el-input v-model="integrationForm.secretRef" placeholder="env://CVAT_CREDENTIALS" /></el-form-item></el-form><template #footer><el-button @click="integrationVisible = false">取消</el-button><el-button type="primary" @click="doCreateIntegration">创建</el-button></template></el-dialog>
  </main>
</template>

<script lang="ts" setup>
import {
  createIntegration, getAuditEvents, getIntegrations, getResourceOverview, heartbeatNode,
  replayIncident, saveQueue, testIntegration
} from '@/api/ai-platform/operations'

defineOptions({ name: 'VisionAIOperations' })
const message = useMessage()
const tab = ref('resources')
const resources = ref<Awaited<ReturnType<typeof getResourceOverview>>>({ nodes: [], queues: [], activeJobs: 0, queuedJobs: 0, storageBytes: 0 })
const integrations = ref<Awaited<ReturnType<typeof getIntegrations>>>({ instances: [], compatibilityRules: [], incidents: [] })
const audits = ref<Array<Record<string, any>>>([])
const nodeVisible = ref(false)
const queueVisible = ref(false)
const integrationVisible = ref(false)
const nodeForm = reactive({ nodeKey: 'windows-gpu-01', name: 'Windows GPU 工作站', gpuModel: 'NVIDIA RTX 4090', gpuCount: 1, gpuMemoryBytes: 25769803776, gpuUsedBytes: 4294967296, driverVersion: '576.88', cudaVersion: '12.8', labels: { os: 'windows', zone: 'local' } })
const queueForm = reactive({ name: 'gpu-default', provider: 'CLEARML', externalQueue: 'visionai-gpu', priority: 50, enabled: true })
const integrationForm = reactive({ name: '本地 CVAT', providerType: 'CVAT', baseUrl: 'http://127.0.0.1:28080', secretRef: 'env_CVAT_CREDENTIALS', version: '2.61.0', capabilities: { annotation: true } })
const onlineNodes = computed(() => resources.value.nodes.filter((row) => row.status === 'ONLINE').length)
const healthyInstances = computed(() => integrations.value.instances.filter((row) => row.status === 'HEALTHY').length)
const formatBytes = (value: number) => value > 1e9 ? `${(value / 1e9).toFixed(2)} GB` : `${(value / 1e6).toFixed(2)} MB`
const loadAll = async () => {
  ;[resources.value, integrations.value, audits.value] = await Promise.all([getResourceOverview(), getIntegrations(), getAuditEvents()])
}
const saveNode = async () => { await heartbeatNode(nodeForm); nodeVisible.value = false; message.success('节点心跳已登记'); await loadAll() }
const doSaveQueue = async () => { await saveQueue(queueForm); queueVisible.value = false; message.success('队列映射已保存'); await loadAll() }
const doCreateIntegration = async () => { await createIntegration(integrationForm); integrationVisible.value = false; message.success('外部实例已创建'); await loadAll() }
const test = async (id: number) => { await testIntegration(id); message.success('连接测试通过'); await loadAll() }
const replay = async (id: number) => { await replayIncident(id); await loadAll() }
onMounted(loadAll)
</script>

<style scoped lang="scss">
.operations-page { min-height: 100%; padding: var(--app-content-padding); color: var(--text-primary); background: radial-gradient(circle at 5% 0, rgb(37 99 235 / 8%), transparent 32%); }
.page-header, .section-title, .compact-card { display: flex; align-items: center; justify-content: space-between; }
.page-header h1 { margin: 5px 0; font-size: 30px; }
.page-header p, .section-title p { margin: 0; color: var(--text-secondary); }
.eyebrow { color: #2563eb; font-size: 12px; font-weight: 750; letter-spacing: 1.8px; }
.summary-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin: 20px 0; }
.summary-grid article, .panel { background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter); border-radius: 14px; }
.summary-grid article { display: flex; flex-direction: column; gap: 3px; padding: 15px; }
.summary-grid strong { font-size: 24px; }
.summary-grid small, .summary-grid span { color: var(--text-secondary); }
.two-columns { display: grid; grid-template-columns: 1.25fr .75fr; gap: 18px; }
.panel { padding: 18px; margin-bottom: 18px; }
.section-title h2 { margin: 0 0 3px; }
.compact-card { padding: 12px 0; border-bottom: 1px solid var(--el-border-color-lighter); }
.compact-card div { display: flex; flex-direction: column; gap: 2px; }
.compact-card span { color: var(--text-secondary); font-size: 12px; }
.incidents { margin-top: 18px; }
.export-link { color: #2563eb; }
code { color: #1d4ed8; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.full { width: 100%; }
.dialog-form { margin-top: 14px; }
</style>

<template>
  <main class="operations-page">
    <header class="page-header">
      <div
        ><span class="eyebrow">PLATFORM CONTROL PLANE</span><h1>资源、集成与审计</h1
        ><p
          >统一查看 GPU 心跳、计算队列、项目配额、外部实例兼容性、同步异常和不可变审计记录。</p
        ></div
      >
      <div
        ><el-button @click="compatibilityVisible = true">兼容性规则</el-button
        ><el-button @click="integrationVisible = true">接入外部实例</el-button
        ><el-button type="primary" @click="nodeVisible = true">模拟节点心跳</el-button></div
      >
    </header>
    <section class="summary-grid">
      <article
        ><small>GPU 节点</small><strong>{{ resources.nodes.length }}</strong
        ><span>{{ onlineNodes }} ONLINE</span></article
      >
      <article
        ><small>运行 / 排队任务</small
        ><strong>{{ resources.activeJobs }} / {{ resources.queuedJobs }}</strong
        ><span>统一 Job 视图</span></article
      >
      <article
        ><small>对象存储</small><strong>{{ formatBytes(resources.storageBytes) }}</strong
        ><span
          >配额 {{ formatBytes(resources.storageCapacityBytes) }} ·
          {{ (resources.storageUtilization * 100).toFixed(2) }}% ·
          {{ resources.storageObjects }} 对象 · {{ resources.storageFailures }} 失败 ·
          {{ resources.lifecyclePolicies }} 策略 / {{ resources.lifecyclePurged }} 已清理</span
        ></article
      >
      <article
        ><small>真实 GPU 小时</small><strong>{{ resources.gpuHours.toFixed(3) }}</strong
        ><span
          >{{ integrations.instances.length }} 个外部实例 · {{ healthyInstances }} HEALTHY</span
        ></article
      >
    </section>
    <el-tabs v-model="tab">
      <el-tab-pane label="计算资源" name="resources">
        <section class="two-columns">
          <section class="panel"
            ><div class="section-title"
              ><div><h2>GPU 节点</h2><p>显存、驱动、CUDA、标签和最近心跳。</p></div></div
            >
            <el-table :data="resources.nodes"
              ><el-table-column prop="name" label="节点" /><el-table-column
                prop="gpuModel"
                label="GPU" /><el-table-column label="GPU / 显存" min-width="135"
                ><template #default="{ row }"
                  >{{ Number(row.gpuUtilization || 0).toFixed(0) }}% ·
                  {{ formatBytes(row.gpuUsedBytes || 0) }} /
                  {{ formatBytes(row.gpuMemoryBytes || 0) }}</template
                ></el-table-column
              ><el-table-column label="CPU / 内存" min-width="130"
                ><template #default="{ row }"
                  >{{ Number(row.cpuUtilization || 0).toFixed(1) }}% ·
                  {{ formatBytes(row.memoryUsedBytes || 0) }}</template
                ></el-table-column
              ><el-table-column prop="driverVersion" label="驱动" /><el-table-column
                label="占用任务"
                min-width="140"
                ><template #default="{ row }">{{
                  activeRunsFor(row.nodeKey) || '空闲'
                }}</template></el-table-column
              ><el-table-column prop="status" label="状态"
            /></el-table>
          </section>
          <section class="panel"
            ><div class="section-title"
              ><div><h2>计算队列</h2><p>平台队列映射 ClearML Queue。</p></div
              ><el-button @click="queueVisible = true">配置</el-button></div
            >
            <article v-for="queue in resources.queues" :key="queue.id" class="compact-card"
              ><div
                ><b>{{ queue.name }}</b
                ><span>{{ queue.provider }} / {{ queue.externalQueue }}</span
                ><span class="queue-metrics"
                  >等待 {{ queue.queuedJobs }} · 运行 {{ queue.runningJobs }} · 已完成
                  {{ queue.completedJobs }} · 平均等待
                  {{ Number(queue.averageWaitSeconds || 0).toFixed(1) }}s</span
                ></div
              ><el-tag :type="queue.enabled ? 'success' : 'info'">{{
                queue.enabled ? 'ENABLED' : 'DISABLED'
              }}</el-tag></article
            >
          </section>
        </section>
        <section class="panel trend-panel">
          <div class="section-title">
            <div>
              <h2>24 小时资源趋势</h2>
              <p>节点心跳和本机 Docker / NVIDIA 探针形成可追溯时间序列。</p>
            </div>
            <span>{{ resourceHistory.length }} 个采样点</span>
          </div>
          <div v-if="resourceHistory.length" class="trend-grid">
            <article v-for="trend in resourceTrends" :key="trend.key">
              <header
                ><span>{{ trend.label }}</span
                ><strong>{{ trend.latest }}</strong></header
              >
              <svg viewBox="0 0 320 90" role="img" :aria-label="`${trend.label}趋势`">
                <polyline
                  :points="trend.points"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="3"
                />
              </svg>
              <small>{{ trend.min }} 最低 · {{ trend.max }} 最高</small>
            </article>
          </div>
          <el-empty v-else :image-size="60" description="等待节点生成资源采样点" />
        </section>
      </el-tab-pane>
      <el-tab-pane label="外部集成" name="integrations">
        <section class="panel">
          <el-table :data="integrations.instances"
            ><el-table-column prop="name" label="实例" /><el-table-column
              prop="providerType"
              label="Provider"
            /><el-table-column prop="baseUrl" label="Base URL" /><el-table-column
              prop="networkRegion"
              label="网络区域"
              width="110"
            /><el-table-column prop="secretRef" label="Secret Ref" /><el-table-column
              prop="version"
              label="版本"
            /><el-table-column prop="status" label="状态"
              ><template #default="{ row }"
                ><el-tag :type="row.status === 'HEALTHY' ? 'success' : 'warning'">{{
                  row.status
                }}</el-tag></template
              ></el-table-column
            ><el-table-column label="操作" width="220"
              ><template #default="{ row }"
                ><el-button link type="primary" @click="test(row.id)">连接测试</el-button
                ><el-button link type="primary" @click="startUpgrade(row)">升级</el-button
                ><el-button link @click="showRevisions(row)">修订</el-button></template
              ></el-table-column
            ></el-table
          >
        </section>
        <section class="panel matrix-panel"
          ><div class="section-title"
            ><div
              ><h2>兼容性矩阵</h2><p>平台与 Provider 版本必须命中 ALLOWED 规则才可升级。</p></div
            ></div
          ><el-table :data="latestCompatibilityRules"
            ><el-table-column prop="providerType" label="Provider" width="130" /><el-table-column
              prop="platformRange"
              label="平台版本范围" /><el-table-column
              prop="providerRange"
              label="Provider 版本范围" /><el-table-column prop="decision" label="决策" width="150"
              ><template #default="{ row }"
                ><el-tag :type="row.decision === 'ALLOWED' ? 'success' : 'danger'">{{
                  row.decision
                }}</el-tag></template
              ></el-table-column
            ><el-table-column prop="notes" label="说明" /></el-table
        ></section>
        <section class="panel incidents"
          ><div class="section-title"
            ><div
              ><h2>同步异常</h2
              ><p>SYNC_WARNING、ORPHANED、MAPPING_MISSING、AUTH_FAILED 可诊断重放。</p></div
            ></div
          >
          <article v-for="incident in visibleIncidents" :key="incident.id" class="compact-card"
            ><div
              ><b>{{ incident.code }}</b
              ><span>{{ incident.message }}</span></div
            ><div class="incident-actions"
              ><el-tag>{{ incident.status }}</el-tag
              ><el-button
                v-if="!['RESOLVED', 'IGNORED', 'CLOSED'].includes(incident.status)"
                @click="startIncidentAction(incident)"
                >处置</el-button
              ></div
            ></article
          >
        </section>
      </el-tab-pane>
      <el-tab-pane label="系统审计" name="audit">
        <section class="panel">
          <div class="section-title">
            <div><h2>不可变审计日志</h2><p>按用户、项目、动作、资源、结果与时间追溯。</p></div>
            <el-button class="export-link" link type="primary" @click="exportAudits">
              导出 CSV
            </el-button>
          </div>
          <div class="audit-filters">
            <el-input-number
              v-model="auditFilters.userId"
              :min="0"
              :controls="false"
              placeholder="用户 ID"
            />
            <el-input-number
              v-model="auditFilters.projectId"
              :min="0"
              :controls="false"
              placeholder="项目 ID"
            />
            <el-input v-model="auditFilters.action" clearable placeholder="动作" />
            <el-input v-model="auditFilters.resourceType" clearable placeholder="资源类型" />
            <el-input-number
              v-model="auditFilters.resourceId"
              :min="0"
              :controls="false"
              placeholder="资源 ID"
            />
            <el-select v-model="auditFilters.result" clearable placeholder="结果">
              <el-option label="成功" value="SUCCESS" />
              <el-option label="失败" value="FAILED" />
            </el-select>
            <el-date-picker
              v-model="auditFilters.createdRange"
              type="datetimerange"
              value-format="YYYY-MM-DD HH:mm:ss"
              start-placeholder="开始时间"
              end-placeholder="结束时间"
            />
            <el-button type="primary" @click="loadAudits">查询</el-button>
          </div>
          <el-table :data="audits" max-height="520">
            <el-table-column prop="createTime" label="时间" width="180" />
            <el-table-column prop="actorUserId" label="用户" width="75" />
            <el-table-column prop="projectId" label="项目" width="75" />
            <el-table-column prop="action" label="动作" min-width="210" />
            <el-table-column prop="resourceType" label="资源" width="150" />
            <el-table-column prop="resourceId" label="ID" width="75" />
            <el-table-column prop="result" label="结果" width="90">
              <template #default="{ row }">
                <el-tag :type="row.result === 'SUCCESS' ? 'success' : 'danger'">
                  {{ row.result }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="traceId" label="Trace">
              <template #default="{ row }"
                ><code>{{ row.traceId?.slice(0, 14) }}</code></template
              >
            </el-table-column>
          </el-table>
        </section>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="nodeVisible" title="GPU 节点心跳" width="540"
      ><el-form label-position="top"
        ><el-form-item label="节点标识"><el-input v-model="nodeForm.nodeKey" /></el-form-item
        ><el-form-item label="名称"><el-input v-model="nodeForm.name" /></el-form-item
        ><div class="form-grid"
          ><el-form-item label="GPU 型号"><el-input v-model="nodeForm.gpuModel" /></el-form-item
          ><el-form-item label="GPU 数量"
            ><el-input-number v-model="nodeForm.gpuCount" :min="0" /></el-form-item></div
        ><div class="form-grid"
          ><el-form-item label="驱动"><el-input v-model="nodeForm.driverVersion" /></el-form-item
          ><el-form-item label="CUDA"
            ><el-input v-model="nodeForm.cudaVersion" /></el-form-item></div></el-form
      ><template #footer
        ><el-button @click="nodeVisible = false">取消</el-button
        ><el-button type="primary" @click="saveNode">发送心跳</el-button></template
      ></el-dialog
    >
    <el-dialog v-model="queueVisible" title="映射计算队列" width="500"
      ><el-form label-position="top"
        ><el-form-item label="名称"><el-input v-model="queueForm.name" /></el-form-item
        ><el-form-item label="Provider"
          ><el-select v-model="queueForm.provider" class="full"
            ><el-option label="ClearML" value="CLEARML" /><el-option
              label="本地 Docker"
              value="LOCAL_DOCKER" /></el-select></el-form-item
        ><el-form-item label="外部队列"
          ><el-input v-model="queueForm.externalQueue" /></el-form-item></el-form
      ><template #footer
        ><el-button @click="queueVisible = false">取消</el-button
        ><el-button type="primary" @click="doSaveQueue">保存</el-button></template
      ></el-dialog
    >
    <el-dialog v-model="integrationVisible" title="接入外部实例" width="560"
      ><el-alert
        type="info"
        :closable="false"
        title="平台只保存 Secret Ref，不保存凭据明文。"
      /><el-form label-position="top" class="dialog-form"
        ><el-form-item label="实例名称"><el-input v-model="integrationForm.name" /></el-form-item
        ><div class="form-grid"
          ><el-form-item label="Provider"
            ><el-select v-model="integrationForm.providerType" class="full"
              ><el-option label="CVAT" value="CVAT" /><el-option
                label="FiftyOne"
                value="FIFTYONE" /><el-option label="Inference" value="INFERENCE" /><el-option
                label="ClearML"
                value="CLEARML" /></el-select></el-form-item
          ><el-form-item label="版本"
            ><el-input v-model="integrationForm.version" /></el-form-item></div
        ><div class="form-grid">
          <el-form-item label="网络区域">
            <el-select v-model="integrationForm.networkRegion" class="full">
              <el-option label="本机 / 内网" value="LOCAL" />
              <el-option label="办公局域网" value="LAN" />
              <el-option label="云上私网" value="VPC" />
              <el-option label="公网" value="PUBLIC" />
            </el-select>
          </el-form-item>
          <el-form-item label="Base URL"
            ><el-input v-model="integrationForm.baseUrl"
          /></el-form-item> </div
        ><el-form-item label="Secret Ref"
          ><el-input
            v-model="integrationForm.secretRef"
            placeholder="env://CVAT_CREDENTIALS" /></el-form-item></el-form
      ><template #footer
        ><el-button @click="integrationVisible = false">取消</el-button
        ><el-button type="primary" @click="doCreateIntegration">创建</el-button></template
      ></el-dialog
    >
    <el-dialog v-model="compatibilityVisible" title="兼容性矩阵规则" width="560">
      <el-alert
        type="info"
        :closable="false"
        title="未命中规则的版本默认 REVIEW_REQUIRED，并阻止自动升级。"
      />
      <el-form label-position="top" class="dialog-form">
        <div class="form-grid">
          <el-form-item label="Provider">
            <el-select v-model="compatibilityForm.providerType" class="full">
              <el-option label="CVAT" value="CVAT" />
              <el-option label="FiftyOne" value="FIFTYONE" />
              <el-option label="ClearML" value="CLEARML" />
            </el-select>
          </el-form-item>
          <el-form-item label="决策">
            <el-select v-model="compatibilityForm.decision" class="full">
              <el-option label="允许" value="ALLOWED" />
              <el-option label="需人工复核" value="REVIEW_REQUIRED" />
              <el-option label="阻止" value="BLOCKED" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="平台版本范围">
          <el-input v-model.trim="compatibilityForm.platformRange" placeholder=">=1.3.0 <2.0.0" />
        </el-form-item>
        <el-form-item label="Provider 版本范围">
          <el-input v-model.trim="compatibilityForm.providerRange" placeholder=">=2.0.0 <3.0.0" />
        </el-form-item>
        <el-form-item label="规则说明">
          <el-input v-model.trim="compatibilityForm.notes" type="textarea" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="compatibilityVisible = false">取消</el-button>
        <el-button type="primary" @click="saveCompatibility">保存规则</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="upgradeVisible" title="受控集成升级" width="610">
      <el-alert
        type="warning"
        :closable="false"
        title="系统会先备份配置并校验兼容性；冒烟失败时自动恢复原配置。"
      />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="实例">
          <el-input :model-value="selectedIntegration?.name" disabled />
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="当前版本">
            <el-input :model-value="selectedIntegration?.version" disabled />
          </el-form-item>
          <el-form-item label="目标版本">
            <el-input v-model.trim="upgradeForm.version" placeholder="2.4.1" />
          </el-form-item>
        </div>
        <el-form-item label="新 Base URL（留空沿用当前地址）">
          <el-input v-model.trim="upgradeForm.baseUrl" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="upgradeForm.faultInjection"
            >故障注入：验证冒烟失败后的自动回滚</el-checkbox
          >
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="upgradeVisible = false">取消</el-button>
        <el-button type="primary" :loading="upgrading" @click="doUpgrade"
          >执行备份、校验与升级</el-button
        >
      </template>
    </el-dialog>

    <el-dialog v-model="incidentVisible" title="同步事件处置" width="540">
      <el-form label-position="top">
        <el-form-item label="事件">
          <el-input
            :model-value="`${selectedIncident?.code || ''} · ${selectedIncident?.message || ''}`"
            disabled
          />
        </el-form-item>
        <el-form-item label="处置动作">
          <el-radio-group v-model="incidentForm.action">
            <el-radio-button value="REPLAY">重放</el-radio-button>
            <el-radio-button value="REBIND">重新绑定</el-radio-button>
            <el-radio-button value="IGNORE">忽略</el-radio-button>
            <el-radio-button value="CLOSE">人工关闭</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="incidentForm.action === 'REBIND'" label="新的外部 ID">
          <el-input v-model.trim="incidentForm.newExternalId" />
        </el-form-item>
        <el-form-item label="处置原因">
          <el-input v-model.trim="incidentForm.reason" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="incidentVisible = false">取消</el-button>
        <el-button type="primary" @click="doIncidentAction">保存 Before / After 审计</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="revisionVisible" title="集成配置修订" size="72%">
      <el-table :data="revisions">
        <el-table-column prop="revisionNo" label="修订" width="80" />
        <el-table-column label="版本" width="150">
          <template #default="{ row }">{{ row.fromVersion }} → {{ row.toVersion }}</template>
        </el-table-column>
        <el-table-column prop="compatibility" label="兼容性" width="130" />
        <el-table-column prop="status" label="结果" width="150">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ACTIVE' ? 'success' : 'warning'">{{
              row.status
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="beforeSha256" label="备份摘要" min-width="180">
          <template #default="{ row }"
            ><code>{{ shortHash(row.beforeSha256) }}</code></template
          >
        </el-table-column>
        <el-table-column prop="afterSha256" label="候选摘要" min-width="180">
          <template #default="{ row }"
            ><code>{{ shortHash(row.afterSha256) }}</code></template
          >
        </el-table-column>
      </el-table>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import {
  actOnIncident,
  createCompatibilityRule,
  createIntegration,
  exportAuditEvents,
  getAuditEvents,
  getIntegrationRevisions,
  getIntegrations,
  getResourceHistory,
  getResourceOverview,
  heartbeatNode,
  saveQueue,
  testIntegration,
  upgradeIntegration
} from '@/api/ai-platform/operations'
import { ElMessageBox } from 'element-plus'

defineOptions({ name: 'VisionAIOperations' })
const message = useMessage()
const tab = ref('resources')
const resources = ref<Awaited<ReturnType<typeof getResourceOverview>>>({
  nodes: [],
  queues: [],
  activeJobs: 0,
  queuedJobs: 0,
  storageBytes: 0,
  storageObjects: 0,
  storageFailures: 0,
  storageCapacityBytes: 0,
  storageUtilization: 0,
  lifecyclePolicies: 0,
  lifecyclePurged: 0,
  gpuHours: 0
})
const resourceHistory = ref<Array<Record<string, any>>>([])
const integrations = ref<Awaited<ReturnType<typeof getIntegrations>>>({
  instances: [],
  compatibilityRules: [],
  incidents: []
})
const audits = ref<Array<Record<string, any>>>([])
const auditFilters = reactive({
  userId: undefined as number | undefined,
  projectId: undefined as number | undefined,
  action: '',
  resourceType: '',
  resourceId: undefined as number | undefined,
  result: '',
  createdRange: [] as string[]
})
const nodeVisible = ref(false)
const queueVisible = ref(false)
const integrationVisible = ref(false)
const compatibilityVisible = ref(false)
const upgradeVisible = ref(false)
const incidentVisible = ref(false)
const revisionVisible = ref(false)
const upgrading = ref(false)
const selectedIntegration = ref<Record<string, any>>()
const selectedIncident = ref<Record<string, any>>()
const revisions = ref<Array<Record<string, any>>>([])
const nodeForm = reactive({
  nodeKey: 'windows-gpu-01',
  name: 'Windows GPU 工作站',
  gpuModel: 'NVIDIA RTX 4090',
  gpuCount: 1,
  gpuMemoryBytes: 25769803776,
  gpuUsedBytes: 4294967296,
  driverVersion: '576.88',
  cudaVersion: '12.8',
  labels: { os: 'windows', zone: 'local' }
})
const queueForm = reactive({
  name: 'gpu-default',
  provider: 'CLEARML',
  externalQueue: 'visionai-gpu',
  priority: 50,
  enabled: true
})
const integrationForm = reactive({
  name: '本地 CVAT',
  providerType: 'CVAT',
  baseUrl: 'http://127.0.0.1:28080',
  networkRegion: 'LOCAL',
  secretRef: 'env_CVAT_CREDENTIALS',
  version: '2.61.0',
  capabilities: { annotation: true }
})
const compatibilityForm = reactive({
  providerType: 'CLEARML',
  platformRange: '>=1.3.0 <2.0.0',
  providerRange: '>=2.0.0 <3.0.0',
  decision: 'ALLOWED',
  notes: 'VisionAI V1.3 官方验证范围'
})
const upgradeForm = reactive({
  version: '',
  baseUrl: '',
  faultInjection: false
})
const incidentForm = reactive<{
  action: 'REPLAY' | 'REBIND' | 'IGNORE' | 'CLOSE'
  newExternalId: string
  reason: string
}>({
  action: 'REPLAY',
  newExternalId: '',
  reason: '修复集成状态后重新同步'
})
const onlineNodes = computed(
  () => resources.value.nodes.filter((row) => row.status === 'ONLINE').length
)
const healthyInstances = computed(
  () => integrations.value.instances.filter((row) => row.status === 'HEALTHY').length
)
const latestCompatibilityRules = computed(() => {
  const seen = new Set<string>()
  return integrations.value.compatibilityRules.filter((row) => {
    const key = `${row.providerType}:${row.platformRange}:${row.providerRange}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
})
const visibleIncidents = computed(() =>
  [...integrations.value.incidents]
    .sort((a, b) => {
      const aOpen = ['OPEN', 'RETRYING'].includes(a.status) ? 1 : 0
      const bOpen = ['OPEN', 'RETRYING'].includes(b.status) ? 1 : 0
      return bOpen - aOpen || Number(b.id) - Number(a.id)
    })
    .slice(0, 12)
)
const formatBytes = (value: number) =>
  value > 1e9 ? `${(value / 1e9).toFixed(2)} GB` : `${(value / 1e6).toFixed(2)} MB`
const activeRunsFor = (nodeKey: string) => {
  const sample = [...resourceHistory.value].reverse().find((row) => row.nodeKey === nodeKey)
  if (!sample) return ''
  try {
    const ids = Array.isArray(sample.activeRunIds)
      ? sample.activeRunIds
      : JSON.parse(sample.activeRunIds || '[]')
    return ids.length ? ids.map((id: number) => `Run #${id}`).join('、') : ''
  } catch {
    return ''
  }
}
const resourceTrends = computed(() => {
  const definitions: Array<{
    key: string
    label: string
    value?: (row: Record<string, any>) => number
    format: (value: number) => string
  }> = [
    {
      key: 'gpuUtilization',
      label: 'GPU 使用率',
      format: (value: number) => `${value.toFixed(1)}%`
    },
    {
      key: 'gpuMemoryPercent',
      label: 'GPU 显存',
      value: (row: Record<string, any>) =>
        row.gpuMemoryBytes ? (Number(row.gpuUsedBytes) / Number(row.gpuMemoryBytes)) * 100 : 0,
      format: (value: number) => `${value.toFixed(1)}%`
    },
    {
      key: 'cpuUtilization',
      label: 'CPU 使用率',
      format: (value: number) => `${value.toFixed(1)}%`
    },
    {
      key: 'memoryPercent',
      label: '内存使用率',
      value: (row: Record<string, any>) =>
        row.memoryBytes ? (Number(row.memoryUsedBytes) / Number(row.memoryBytes)) * 100 : 0,
      format: (value: number) => `${value.toFixed(1)}%`
    },
    { key: 'diskUsedBytes', label: '磁盘使用', format: formatBytes },
    {
      key: 'networkTotal',
      label: '网络 I/O',
      value: (row: Record<string, any>) =>
        Number(row.networkRxBytes || 0) + Number(row.networkTxBytes || 0),
      format: formatBytes
    }
  ]
  return definitions.map((definition) => {
    const values = resourceHistory.value.map((row) =>
      Number(definition.value ? definition.value(row) : row[definition.key] || 0)
    )
    const min = Math.min(...values)
    const max = Math.max(...values)
    const span = Math.max(max - min, 1)
    const points = values
      .map((value, index) => {
        const x = values.length === 1 ? 160 : (index / (values.length - 1)) * 320
        const y = 82 - ((value - min) / span) * 72
        return `${x.toFixed(1)},${y.toFixed(1)}`
      })
      .join(' ')
    return {
      key: definition.key,
      label: definition.label,
      points,
      latest: definition.format(values.at(-1) || 0),
      min: definition.format(min),
      max: definition.format(max)
    }
  })
})
const shortHash = (value = '') => (value ? `${value.slice(0, 12)}…${value.slice(-8)}` : '—')
const loadAll = async () => {
  ;[resources.value, resourceHistory.value, integrations.value, audits.value] = await Promise.all([
    getResourceOverview(),
    getResourceHistory(24),
    getIntegrations(),
    getAuditEvents()
  ])
}
const auditParams = () => ({
  userId: auditFilters.userId,
  projectId: auditFilters.projectId,
  action: auditFilters.action || undefined,
  resourceType: auditFilters.resourceType || undefined,
  resourceId: auditFilters.resourceId,
  result: auditFilters.result || undefined,
  createdFrom: auditFilters.createdRange[0],
  createdTo: auditFilters.createdRange[1]
})
const loadAudits = async () => {
  audits.value = await getAuditEvents(auditParams())
}
const exportAudits = async () => {
  const response = (await exportAuditEvents(auditParams())) as any
  const href = URL.createObjectURL(response.data as Blob)
  const link = document.createElement('a')
  link.href = href
  link.download = `visionai-audit-${new Date().toISOString().slice(0, 10)}.csv`
  link.click()
  URL.revokeObjectURL(href)
}
const saveNode = async () => {
  await heartbeatNode(nodeForm)
  nodeVisible.value = false
  message.success('节点心跳已登记')
  await loadAll()
}
const doSaveQueue = async () => {
  await saveQueue(queueForm)
  queueVisible.value = false
  message.success('队列映射已保存')
  await loadAll()
}
const doCreateIntegration = async () => {
  await createIntegration(integrationForm)
  integrationVisible.value = false
  message.success('外部实例已创建')
  await loadAll()
}
const test = async (id: number) => {
  const result = await testIntegration(id)
  await ElMessageBox.alert(
    result.checks
      .map(
        (check) =>
          `${check.success ? '✓' : '✕'} ${check.name} · ${check.latencyMs.toFixed(1)} ms · ${check.detail}`
      )
      .join('\n'),
    result.message,
    { confirmButtonText: '知道了' }
  )
  await loadAll()
}
const saveCompatibility = async () => {
  await createCompatibilityRule(compatibilityForm)
  compatibilityVisible.value = false
  message.success('兼容性矩阵规则已保存')
  await loadAll()
}
const startUpgrade = (row: Record<string, any>) => {
  selectedIntegration.value = row
  upgradeForm.version = row.version
  upgradeForm.baseUrl = ''
  upgradeForm.faultInjection = false
  upgradeVisible.value = true
}
const doUpgrade = async () => {
  if (!selectedIntegration.value || !upgradeForm.version) return message.warning('目标版本必填')
  upgrading.value = true
  try {
    const result = await upgradeIntegration(selectedIntegration.value.id, upgradeForm)
    upgradeVisible.value = false
    if (result.rolledBack) {
      message.warning('冒烟失败，系统已自动恢复升级前配置')
    } else {
      message.success('兼容性校验与冒烟通过，升级修订已生效')
    }
    await loadAll()
  } finally {
    upgrading.value = false
  }
}
const showRevisions = async (row: Record<string, any>) => {
  revisions.value = await getIntegrationRevisions(row.id)
  revisionVisible.value = true
}
const startIncidentAction = (row: Record<string, any>) => {
  selectedIncident.value = row
  incidentForm.action = 'REPLAY'
  incidentForm.newExternalId = ''
  incidentForm.reason = '修复集成状态后重新同步'
  incidentVisible.value = true
}
const doIncidentAction = async () => {
  if (!selectedIncident.value || !incidentForm.reason) return message.warning('处置原因必填')
  if (incidentForm.action === 'REBIND' && !incidentForm.newExternalId)
    return message.warning('重新绑定必须填写新的外部 ID')
  await actOnIncident(selectedIncident.value.id, incidentForm)
  incidentVisible.value = false
  message.success('同步事件已处置，Before / After 已写入审计')
  await loadAll()
}
onMounted(loadAll)
</script>

<style scoped lang="scss">
.operations-page {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--el-fill-color-lighter);
}

.page-header,
.section-title,
.compact-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-header h1 {
  margin: 5px 0;
  font-size: 30px;
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
  color: #2563eb;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin: 20px 0;
}

.summary-grid article,
.panel {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
}

.summary-grid article {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 15px;
}

.summary-grid strong {
  font-size: 24px;
}

.summary-grid small,
.summary-grid span {
  color: var(--text-secondary);
}

.two-columns {
  display: grid;
  grid-template-columns: 1.25fr 0.75fr;
  gap: 18px;
}

.trend-panel {
  margin-top: 18px;
}

.trend-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.trend-grid article {
  padding: 14px;
  color: var(--el-color-primary);
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
}

.trend-grid header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  color: var(--text-primary);
}

.trend-grid svg {
  width: 100%;
  height: 90px;
  margin: 8px 0;
}

.trend-grid small {
  color: var(--text-secondary);
}

.panel {
  padding: 18px;
  margin-bottom: 18px;
}

.section-title h2 {
  margin: 0 0 3px;
}

.compact-card {
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.compact-card div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.compact-card span {
  font-size: 12px;
  color: var(--text-secondary);
}

.incidents {
  margin-top: 18px;
}

.matrix-panel {
  margin-top: 18px;
}

.incident-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.export-link {
  color: #2563eb;
}

.audit-filters {
  display: grid;
  grid-template-columns: repeat(6, minmax(120px, 1fr)) minmax(280px, 2fr) auto;
  gap: 10px;
  margin-bottom: 16px;
}

.audit-filters > * {
  width: 100%;
}

code {
  color: #1d4ed8;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.full {
  width: 100%;
}

@media (width <= 980px) {
  .summary-grid,
  .two-columns,
  .trend-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 620px) {
  .page-header,
  .section-title,
  .compact-card {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
  }

  .summary-grid,
  .two-columns,
  .trend-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }
}

.dialog-form {
  margin-top: 14px;
}
</style>

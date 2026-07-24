<template>
  <main class="model-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">TRUSTED MODEL REGISTRY</span>
        <h1>模型注册与审批</h1>
        <p>从训练、数据与评估证据形成不可变模型版本，经职责分离审批后获得生产资格。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="registerVisible = true">
        注册模型版本
      </el-button>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadAll">
        <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
      </el-select>
      <el-button @click="loadAll">刷新</el-button>
    </section>

    <section class="summary-grid">
      <article><small>模型</small><strong>{{ models.length }}</strong><span>按项目隔离</span></article>
      <article><small>不可变版本</small><strong>{{ versions.length }}</strong><span>制品摘要已锁定</span></article>
      <article><small>待审批</small><strong>{{ pending.length }}</strong><span>提交人与审批人分离</span></article>
      <article><small>生产合格</small><strong>{{ approved.length }}</strong><span>许可证与门禁通过</span></article>
    </section>

    <section class="registry">
      <div class="section-title"><div><h2>模型版本</h2><p>版本状态、血缘、许可证和供应链校验一览。</p></div></div>
      <article v-for="version in versions" :key="version.id" class="version-card" @click="openDetail(version)">
        <div class="version-identity">
          <span>MODEL VERSION #{{ version.id }}</span>
          <strong>{{ modelName(version.modelId) }} · {{ version.semanticVersion }}</strong>
          <small>Dataset #{{ version.datasetVersionId }} → Training #{{ version.trainingRunId }} → Evaluation #{{ version.evaluationRunId }}</small>
        </div>
        <div class="evidence">
          <span>Supply Chain</span>
          <code>{{ shortHash(version.supplyChainSha256) }}</code>
        </div>
        <el-tag :type="statusType(version.status)">{{ version.status }}</el-tag>
      </article>
      <div v-if="!versions.length" class="empty">尚无模型版本，请从成功训练与通过门禁的评估运行注册。</div>
    </section>

    <section class="approval-section">
      <div class="section-title"><div><h2>审批队列</h2><p>决定为追加式证据，意见必填且不可篡改。</p></div></div>
      <el-table :data="approvals">
        <el-table-column prop="id" label="审批" width="90" />
        <el-table-column prop="modelVersionId" label="模型版本" width="120" />
        <el-table-column prop="approvalType" label="类型" min-width="190" />
        <el-table-column prop="targetEnvironment" label="环境" width="130" />
        <el-table-column prop="evidenceSha256" label="证据摘要" min-width="180">
          <template #default="{ row }"><code>{{ shortHash(row.evidenceSha256) }}</code></template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }"><el-tag :type="row.status === 'APPROVED' ? 'success' : row.status === 'REJECTED' ? 'danger' : 'warning'">{{ row.status }}</el-tag></template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }"><el-button v-if="row.status === 'PENDING'" link type="primary" @click.stop="startDecision(row)">审批</el-button></template>
        </el-table-column>
      </el-table>
    </section>

    <el-dialog v-model="registerVisible" title="从已验证运行注册模型" width="590">
      <el-alert type="info" :closable="false" title="训练运行必须成功，且对应评估运行必须通过 MUST_PASS 门禁。" />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="模型名称"><el-input v-model="registerForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="TrainingRun ID"><el-input-number v-model="registerForm.trainingRunId" :min="1" /></el-form-item>
          <el-form-item label="EvaluationRun ID"><el-input-number v-model="registerForm.evaluationRunId" :min="1" /></el-form-item>
        </div>
        <el-form-item label="语义版本"><el-input v-model="registerForm.semanticVersion" placeholder="1.0.0" /></el-form-item>
        <el-form-item label="用途与限制"><el-input v-model="registerForm.description" type="textarea" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="registerVisible = false">取消</el-button><el-button type="primary" @click="doRegister">注册</el-button></template>
    </el-dialog>

    <el-dialog v-model="decisionVisible" title="生产资格审批" width="520">
      <el-form label-position="top">
        <el-form-item label="决定"><el-radio-group v-model="decisionForm.decision"><el-radio-button value="APPROVE">批准</el-radio-button><el-radio-button value="REJECT">拒绝</el-radio-button></el-radio-group></el-form-item>
        <el-form-item label="审批意见（必填）"><el-input v-model="decisionForm.comment" type="textarea" :rows="4" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="decisionVisible = false">取消</el-button><el-button type="primary" @click="doDecision">保存不可变决定</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="模型版本证据" size="58%">
      <template v-if="detail">
        <section class="detail-grid">
          <article><small>状态</small><strong>{{ detail.version.status }}</strong></article>
          <article><small>许可证</small><strong>{{ detail.version.licenseDecision }}</strong></article>
          <article><small>制品</small><strong>{{ detail.artifacts.length }}</strong></article>
        </section>
        <h3>不可变制品</h3>
        <el-table :data="detail.artifacts">
          <el-table-column prop="format" label="格式" width="130" />
          <el-table-column prop="uri" label="URI" min-width="280" />
          <el-table-column prop="sha256" label="SHA-256" min-width="210"><template #default="{ row }"><code>{{ shortHash(row.sha256) }}</code></template></el-table-column>
        </el-table>
        <div class="drawer-actions">
          <el-button v-if="detail.version.status === 'EVALUATED'" type="primary" @click="doSubmitApproval">提交生产审批</el-button>
          <el-button @click="downloadManifest">导出受控清单</el-button>
        </div>
      </template>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  decideApproval, exportModelManifest, getApprovals, getModels, getModelVersion, registerModel,
  submitApproval, type ApprovalRequest, type ModelRecord, type ModelVersion
} from '@/api/ai-platform/models'

defineOptions({ name: 'VisionAIModelRegistry' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const models = ref<ModelRecord[]>([])
const versions = ref<ModelVersion[]>([])
const approvals = ref<ApprovalRequest[]>([])
const registerVisible = ref(false)
const decisionVisible = ref(false)
const detailVisible = ref(false)
const detail = ref<Awaited<ReturnType<typeof getModelVersion>>>()
const selectedApproval = ref<ApprovalRequest>()
const registerForm = reactive({ name: '缺陷检测模型', description: '制造缺陷检测', trainingRunId: 1, evaluationRunId: 1, semanticVersion: '1.0.0' })
const decisionForm = reactive<{ decision: 'APPROVE' | 'REJECT'; comment: string }>({ decision: 'APPROVE', comment: '' })
const pending = computed(() => approvals.value.filter((row) => row.status === 'PENDING'))
const approved = computed(() => versions.value.filter((row) => row.status === 'APPROVED'))
const modelName = (id: number) => models.value.find((row) => row.id === id)?.name || `Model #${id}`
const shortHash = (value = '') => value ? `${value.slice(0, 12)}…${value.slice(-8)}` : '—'
const statusType = (status: string) => status === 'APPROVED' ? 'success' : status === 'REVIEW_PENDING' ? 'warning' : 'info'
const loadAll = async () => {
  if (!projectId.value) return
  const [registry, queue] = await Promise.all([getModels(projectId.value), getApprovals(projectId.value)])
  models.value = registry.models
  versions.value = registry.versions
  approvals.value = queue
}
const doRegister = async () => {
  if (!projectId.value || !registerForm.name.trim()) return
  await registerModel(projectId.value, registerForm)
  registerVisible.value = false
  message.success('模型版本、Model Card 与供应链清单已锁定')
  await loadAll()
}
const openDetail = async (version: ModelVersion) => {
  detail.value = await getModelVersion(projectId.value!, version.id)
  detailVisible.value = true
}
const doSubmitApproval = async () => {
  if (!detail.value) return
  await submitApproval(projectId.value!, detail.value.version.id, { approvalType: 'PRODUCTION_QUALIFICATION', targetEnvironment: 'PRODUCTION' })
  message.success('已提交生产资格审批')
  detailVisible.value = false
  await loadAll()
}
const startDecision = (row: ApprovalRequest) => {
  selectedApproval.value = row
  decisionForm.comment = ''
  decisionVisible.value = true
}
const doDecision = async () => {
  if (!selectedApproval.value || !decisionForm.comment.trim()) return message.warning('审批意见必填')
  await decideApproval(projectId.value!, selectedApproval.value.id, decisionForm)
  decisionVisible.value = false
  message.success('审批决定已保存并写入审计')
  await loadAll()
}
const downloadManifest = async () => {
  if (!detail.value) return
  const value = await exportModelManifest(projectId.value!, detail.value.version.id)
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `model-version-${detail.value.version.id}-manifest.json`
  link.click()
  URL.revokeObjectURL(link.href)
}
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value.at(-1)?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.model-page { min-height: 100%; padding: var(--app-content-padding); background: radial-gradient(circle at 90% 0, rgb(79 70 229 / 9%), transparent 34%); color: var(--text-primary); }
.page-header, .toolbar, .version-card, .section-title { display: flex; align-items: center; justify-content: space-between; }
.page-header { margin-bottom: 20px; }
.page-header h1 { margin: 5px 0; font-size: 30px; }
.page-header p, .section-title p { margin: 0; color: var(--text-secondary); }
.eyebrow { color: #4f46e5; font-size: 12px; font-weight: 750; letter-spacing: 1.8px; }
.toolbar { justify-content: flex-start; gap: 12px; margin-bottom: 18px; }
.toolbar .el-select { width: 260px; }
.summary-grid, .detail-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 20px; }
.summary-grid article, .detail-grid article, .registry, .approval-section { background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter); border-radius: 14px; }
.summary-grid article, .detail-grid article { display: flex; flex-direction: column; gap: 4px; padding: 16px; }
.summary-grid strong, .detail-grid strong { font-size: 25px; }
.summary-grid small, .summary-grid span, .detail-grid small { color: var(--text-secondary); }
.registry, .approval-section { padding: 18px; margin-bottom: 18px; }
.section-title h2 { margin: 0 0 3px; }
.version-card { margin-top: 12px; padding: 15px; border: 1px solid var(--el-border-color-lighter); border-radius: 11px; cursor: pointer; }
.version-card:hover { border-color: #6366f1; box-shadow: 0 8px 22px rgb(79 70 229 / 8%); }
.version-identity { display: flex; flex: 1; flex-direction: column; gap: 3px; }
.version-identity > span, .version-identity small, .evidence span { color: var(--text-secondary); font-size: 12px; }
.version-identity strong { font-size: 16px; }
.evidence { display: flex; flex-direction: column; width: 220px; }
code { color: #4f46e5; font-size: 12px; }
.empty { padding: 55px; text-align: center; color: var(--text-secondary); }
.dialog-form { margin-top: 16px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.detail-grid { grid-template-columns: repeat(3, 1fr); }
.drawer-actions { display: flex; gap: 10px; margin-top: 18px; }
</style>

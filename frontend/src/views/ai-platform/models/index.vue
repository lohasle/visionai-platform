<template>
  <main class="model-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">TRUSTED MODEL REGISTRY</span>
        <h1>模型注册与审批</h1>
        <p>从训练、数据与评估证据形成不可变模型版本，经职责分离审批后获得生产资格。</p>
      </div>
      <div class="header-actions">
        <el-button @click="templateVisible = true">审批模板</el-button>
        <el-button :disabled="comparisonIds.length < 2" @click="showComparison">
          比较模型（{{ comparisonIds.length }}）
        </el-button>
        <el-button type="primary" :disabled="!projectId" @click="registerVisible = true">
          注册模型版本
        </el-button>
      </div>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadAll">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="`${project.name} · ${project.code}`"
          :value="project.id"
        />
      </el-select>
      <el-button @click="loadAll">刷新</el-button>
    </section>

    <section class="summary-grid">
      <article
        ><small>模型</small><strong>{{ models.length }}</strong
        ><span>按项目隔离</span></article
      >
      <article
        ><small>不可变版本</small><strong>{{ versions.length }}</strong
        ><span>制品摘要已锁定</span></article
      >
      <article
        ><small>待审批</small><strong>{{ pending.length }}</strong
        ><span>提交人与审批人分离</span></article
      >
      <article
        ><small>生产合格</small><strong>{{ approved.length }}</strong
        ><span>许可证与门禁通过</span></article
      >
    </section>

    <section class="registry">
      <div class="section-title"
        ><div><h2>模型版本</h2><p>版本状态、血缘、许可证和供应链校验一览。</p></div></div
      >
      <article
        v-for="version in versions"
        :key="version.id"
        class="version-card"
        @click="openDetail(version)"
      >
        <el-checkbox
          class="compare-selector"
          :model-value="comparisonIds.includes(version.id)"
          :aria-label="`选择模型版本 ${version.semanticVersion} 进行比较`"
          @click.stop
          @change="toggleComparison(version.id)"
        />
        <div class="version-identity">
          <span>MODEL VERSION #{{ version.id }}</span>
          <strong>{{ modelName(version.modelId) }} · {{ version.semanticVersion }}</strong>
          <small
            >Dataset #{{ version.datasetVersionId }} → Training #{{ version.trainingRunId }} →
            Evaluation #{{ version.evaluationRunId }}</small
          >
        </div>
        <div class="evidence">
          <span>Supply Chain</span>
          <code>{{ shortHash(version.supplyChainSha256) }}</code>
        </div>
        <el-tag :type="statusType(version.status)">{{ version.status }}</el-tag>
      </article>
      <div v-if="!versions.length" class="empty"
        >尚无模型版本，请从成功训练与通过门禁的评估运行注册。</div
      >
    </section>

    <section class="approval-section">
      <div class="section-title"
        ><div><h2>审批队列</h2><p>决定为追加式证据，意见必填且不可篡改。</p></div></div
      >
      <el-table :data="approvals">
        <el-table-column prop="id" label="审批" width="90" />
        <el-table-column prop="modelVersionId" label="模型版本" width="120" />
        <el-table-column prop="approvalType" label="类型" min-width="190" />
        <el-table-column prop="targetEnvironment" label="环境" width="130" />
        <el-table-column label="步骤" width="110">
          <template #default="{ row }">{{ row.currentStep }} / {{ row.totalSteps }}</template>
        </el-table-column>
        <el-table-column prop="evidenceSha256" label="证据摘要" min-width="180">
          <template #default="{ row }"
            ><code>{{ shortHash(row.evidenceSha256) }}</code></template
          >
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }"
            ><el-tag
              :type="
                row.status === 'APPROVED'
                  ? 'success'
                  : row.status === 'REJECTED'
                    ? 'danger'
                    : 'warning'
              "
              >{{ row.status }}</el-tag
            ></template
          >
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="{ row }"
            ><el-button
              v-if="row.status === 'PENDING' || row.status === 'IN_PROGRESS'"
              link
              type="primary"
              @click.stop="startDecision(row)"
              >审批</el-button
            ><el-button
              v-if="row.status === 'PENDING' || row.status === 'IN_PROGRESS'"
              link
              type="danger"
              @click.stop="doCancelApproval(row)"
              >取消</el-button
            ></template
          >
        </el-table-column>
      </el-table>
    </section>

    <el-dialog v-model="registerVisible" title="从已验证运行注册模型" width="590">
      <el-alert
        type="info"
        :closable="false"
        title="训练运行必须成功，且对应评估运行必须通过 MUST_PASS 门禁。"
      />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="模型名称"><el-input v-model="registerForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="TrainingRun ID"
            ><el-input-number v-model="registerForm.trainingRunId" :min="1"
          /></el-form-item>
          <el-form-item label="EvaluationRun ID"
            ><el-input-number v-model="registerForm.evaluationRunId" :min="1"
          /></el-form-item>
        </div>
        <el-form-item label="语义版本"
          ><el-input v-model="registerForm.semanticVersion" placeholder="1.0.0"
        /></el-form-item>
        <el-form-item label="用途与限制"
          ><el-input v-model="registerForm.description" type="textarea"
        /></el-form-item>
      </el-form>
      <template #footer
        ><el-button @click="registerVisible = false">取消</el-button
        ><el-button type="primary" @click="doRegister">注册</el-button></template
      >
    </el-dialog>

    <el-dialog v-model="decisionVisible" title="生产资格审批" width="520">
      <el-form label-position="top">
        <el-form-item label="决定"
          ><el-radio-group v-model="decisionForm.decision"
            ><el-radio-button value="APPROVE">批准</el-radio-button
            ><el-radio-button value="REJECT">拒绝</el-radio-button></el-radio-group
          ></el-form-item
        >
        <el-form-item label="审批意见（必填）"
          ><el-input v-model="decisionForm.comment" type="textarea" :rows="4"
        /></el-form-item>
      </el-form>
      <template #footer
        ><el-button @click="decisionVisible = false">取消</el-button
        ><el-button type="primary" @click="doDecision">保存不可变决定</el-button></template
      >
    </el-dialog>

    <el-dialog v-model="approvalVisible" title="提交生产资格审批" width="620">
      <el-alert
        type="info"
        :closable="false"
        title="提交后将冻结模型制品、供应链、目标环境、许可结论、失败样本与审批模板快照。"
      />
      <el-form label-position="top" class="dialog-form">
        <el-form-item label="租户审批模板">
          <el-select v-model="approvalForm.templateId" class="full" placeholder="选择审批模板">
            <el-option
              v-for="item in templates"
              :key="item.id"
              :label="`${item.name} · ${templateStepCount(item.steps)} 步`"
              :value="item.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="风险说明">
          <el-input v-model.trim="approvalForm.riskSummary" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="回滚方案">
          <el-input v-model.trim="approvalForm.rollbackPlan" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="approvalVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="doSubmitApproval"
          >冻结证据并提交</el-button
        >
      </template>
    </el-dialog>

    <el-dialog v-model="templateVisible" title="租户审批模板" width="660">
      <p class="dialog-intro">模板由系统角色驱动，支持单步或顺序多级审批。</p>
      <el-table :data="templates" max-height="240">
        <el-table-column prop="name" label="模板" />
        <el-table-column prop="targetEnvironment" label="目标环境" width="130" />
        <el-table-column label="步骤" width="90">
          <template #default="{ row }">{{ templateStepCount(row.steps) }}</template>
        </el-table-column>
        <el-table-column label="职责分离" width="120">
          <template #default="{ row }">
            <el-tag :type="row.allowSelfApproval ? 'danger' : 'success'">
              {{ row.allowSelfApproval ? '管理员例外' : '强制分离' }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
      <el-divider content-position="left">新建模板</el-divider>
      <el-form label-position="top">
        <el-form-item label="模板名称"><el-input v-model.trim="templateForm.name" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="第一步">
            <el-select v-model="templateForm.firstRole" class="full">
              <el-option label="技术复核 · REVIEWER" value="REVIEWER" />
              <el-option label="生产批准 · APPROVER" value="APPROVER" />
            </el-select>
          </el-form-item>
          <el-form-item label="第二步（可选）">
            <el-select v-model="templateForm.secondRole" clearable class="full">
              <el-option label="生产批准 · APPROVER" value="APPROVER" />
              <el-option label="运维放行 · OPS" value="OPS" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item>
          <el-checkbox v-model="templateForm.allowSelfApproval">
            允许提交人审批自己的申请（仅超级管理员可创建此例外）
          </el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="templateVisible = false">关闭</el-button>
        <el-button type="primary" @click="doCreateTemplate">创建模板</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="licenseVisible" title="复核许可与用途声明" width="780">
      <el-alert
        type="warning"
        :closable="false"
        title="修改任一许可受控输入都会使待审批或已批准资格自动失效。"
      />
      <div v-for="item in licenseForm" :key="item.componentType" class="license-row">
        <b>{{ item.componentType }}</b>
        <el-input v-model.trim="item.componentName" placeholder="组件名称" />
        <el-input v-model.trim="item.licenseId" placeholder="SPDX 或组织策略标识" />
        <el-input v-model.trim="item.useDeclaration" placeholder="用途声明" />
        <el-select v-model="item.decision">
          <el-option label="允许" value="ALLOWED" />
          <el-option label="需复核" value="REVIEW_REQUIRED" />
          <el-option label="禁止" value="NOT_ALLOWED" />
        </el-select>
      </div>
      <template #footer>
        <el-button @click="licenseVisible = false">取消</el-button>
        <el-button type="primary" @click="saveLicenses">保存并重新计算资格</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="exportVisible" title="受控模型导出" width="520">
      <el-form label-position="top">
        <el-form-item label="导出用途（写入审计）">
          <el-input v-model.trim="exportForm.purpose" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="exportForm.includeArtifacts">在响应中包含制品引用</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="exportVisible = false">取消</el-button>
        <el-button type="primary" @click="downloadManifest">校验审批并导出</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="comparisonVisible" title="模型版本横向比较" width="1120">
      <el-table :data="comparisonRows" border>
        <el-table-column label="模型版本" min-width="180">
          <template #default="{ row }">
            <b>{{ modelName(row.version.modelId) }} · {{ row.version.semanticVersion }}</b>
            <small class="table-note">{{ row.version.status }}</small>
          </template>
        </el-table-column>
        <el-table-column label="数据集" min-width="150">
          <template #default="{ row }">
            {{ row.datasetVersion.semanticVersion || `#${row.datasetVersion.id}` }}
          </template>
        </el-table-column>
        <el-table-column label="评估指标" min-width="180">
          <template #default="{ row }">{{ metricSummary(row.evaluationSummary) }}</template>
        </el-table-column>
        <el-table-column label="制品" width="140">
          <template #default="{ row }">
            {{ row.artifactCount }} 个 · {{ formatBytes(row.artifactBytes) }}
          </template>
        </el-table-column>
        <el-table-column label="生产请求 / QPS" width="145">
          <template #default="{ row }">
            {{ row.deploymentPerformance.requests }} /
            {{ Number(row.deploymentPerformance.qps || 0).toFixed(2) }}
          </template>
        </el-table-column>
        <el-table-column label="P95 / P99" width="150">
          <template #default="{ row }">
            {{ Number(row.deploymentPerformance.p95 || 0).toFixed(1) }} /
            {{ Number(row.deploymentPerformance.p99 || 0).toFixed(1) }} ms
          </template>
        </el-table-column>
        <el-table-column label="错误率" width="110">
          <template #default="{ row }">
            {{ (Number(row.deploymentPerformance.errorRate || 0) * 100).toFixed(2) }}%
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="模型版本证据" size="58%">
      <template v-if="detail">
        <section class="detail-grid">
          <article
            ><small>状态</small><strong>{{ detail.version.status }}</strong></article
          >
          <article
            ><small>许可证</small><strong>{{ detail.version.licenseDecision }}</strong></article
          >
          <article
            ><small>制品</small><strong>{{ detail.artifacts.length }}</strong></article
          >
        </section>
        <h3>不可变制品</h3>
        <el-table :data="detail.artifacts">
          <el-table-column prop="format" label="格式" width="130" />
          <el-table-column prop="uri" label="URI" min-width="280" />
          <el-table-column prop="sha256" label="SHA-256" min-width="210"
            ><template #default="{ row }"
              ><code>{{ shortHash(row.sha256) }}</code></template
            ></el-table-column
          >
        </el-table>
        <div class="subsection-heading">
          <div>
            <h3>许可与用途声明</h3>
            <p>数据、预训练权重、训练框架和模型制品必须逐项通过。</p>
          </div>
          <el-button @click="openLicenseEditor">复核许可</el-button>
        </div>
        <el-table :data="detail.licenses">
          <el-table-column prop="componentType" label="组件" width="170" />
          <el-table-column prop="componentName" label="名称" min-width="180" />
          <el-table-column prop="licenseId" label="许可证" width="150" />
          <el-table-column prop="useDeclaration" label="用途声明" min-width="240" />
          <el-table-column prop="decision" label="结论" width="150">
            <template #default="{ row }">
              <el-tag :type="row.decision === 'ALLOWED' ? 'success' : 'danger'">{{
                row.decision
              }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <template v-if="detail.approvals.length">
          <div class="subsection-heading">
            <div><h3>审批轨迹</h3><p>模板快照、步骤决定与输入指纹均不可变。</p></div>
          </div>
          <el-timeline>
            <el-timeline-item
              v-for="step in detail.stepDecisions"
              :key="step.id"
              :type="step.decision === 'APPROVE' ? 'success' : 'danger'"
              :timestamp="`${step.requiredRole} · User #${step.decidedBy}`"
            >
              <b
                >审批 #{{ step.approvalRequestId }} · 第 {{ step.stepNo }} 步 ·
                {{ step.stepName }}</b
              >
              <p>{{ step.comment }}</p>
            </el-timeline-item>
          </el-timeline>
        </template>
        <div class="drawer-actions">
          <el-button
            v-if="detail.version.status === 'EVALUATED'"
            type="primary"
            @click="approvalVisible = true"
            >提交生产审批</el-button
          >
          <el-button
            :disabled="!isProductionQualified(detail.version)"
            @click="exportVisible = true"
            >导出受控清单</el-button
          >
        </div>
      </template>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import { ElMessageBox } from 'element-plus'
import {
  cancelApproval,
  compareModelVersions,
  createApprovalTemplate,
  decideApproval,
  exportModel,
  getApprovalTemplates,
  getApprovals,
  getModels,
  getModelVersion,
  registerModel,
  replaceModelLicenses,
  submitApproval,
  type ApprovalTemplate,
  type ApprovalRequest,
  type ModelLicenseDeclaration,
  type ModelRecord,
  type ModelVersion
} from '@/api/ai-platform/models'

defineOptions({ name: 'VisionAIModelRegistry' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const models = ref<ModelRecord[]>([])
const versions = ref<ModelVersion[]>([])
const comparisonIds = ref<number[]>([])
const comparisonVisible = ref(false)
const comparisonRows = ref<Awaited<ReturnType<typeof compareModelVersions>>>([])
const approvals = ref<ApprovalRequest[]>([])
const registerVisible = ref(false)
const decisionVisible = ref(false)
const approvalVisible = ref(false)
const templateVisible = ref(false)
const licenseVisible = ref(false)
const exportVisible = ref(false)
const detailVisible = ref(false)
const submitting = ref(false)
const detail = ref<Awaited<ReturnType<typeof getModelVersion>>>()
const selectedApproval = ref<ApprovalRequest>()
const templates = ref<ApprovalTemplate[]>([])
const licenseForm = ref<ModelLicenseDeclaration[]>([])
const registerForm = reactive({
  name: '缺陷检测模型',
  description: '制造缺陷检测',
  trainingRunId: 1,
  evaluationRunId: 1,
  semanticVersion: '1.0.0'
})
const decisionForm = reactive<{ decision: 'APPROVE' | 'REJECT'; comment: string }>({
  decision: 'APPROVE',
  comment: ''
})
const approvalForm = reactive({
  templateId: undefined as number | undefined,
  riskSummary: '已知风险为小目标与遮挡场景精度下降，门禁结果与失败样本已随审批快照固化。',
  rollbackPlan: '生产异常时切回上一已验证模型版本，保留推理追踪并停止当前修订。'
})
const templateForm = reactive({
  name: '生产双人审批',
  firstRole: 'REVIEWER',
  secondRole: 'APPROVER',
  allowSelfApproval: false
})
const exportForm = reactive({ purpose: '生产部署受控制品交付', includeArtifacts: true })
const pending = computed(() =>
  approvals.value.filter((row) => row.status === 'PENDING' || row.status === 'IN_PROGRESS')
)
const isProductionQualified = (version: ModelVersion) =>
  Boolean(version.approvedAt) &&
  ['APPROVED', 'STAGING', 'CANARY', 'PRODUCTION'].includes(version.status)
const approved = computed(() => versions.value.filter(isProductionQualified))
const modelName = (id: number) => models.value.find((row) => row.id === id)?.name || `Model #${id}`
const shortHash = (value = '') => (value ? `${value.slice(0, 12)}…${value.slice(-8)}` : '—')
const statusType = (status: string) =>
  ['APPROVED', 'STAGING', 'CANARY', 'PRODUCTION'].includes(status)
    ? 'success'
    : status === 'REVIEW_PENDING'
      ? 'warning'
      : status === 'INVALIDATED'
        ? 'danger'
        : 'info'
const templateStepCount = (raw = '[]') => {
  try {
    return JSON.parse(raw).length
  } catch {
    return 0
  }
}
const loadAll = async () => {
  if (!projectId.value) return
  const [registry, queue, templateRows] = await Promise.all([
    getModels(projectId.value),
    getApprovals(projectId.value),
    getApprovalTemplates()
  ])
  models.value = registry.models
  versions.value = registry.versions
  approvals.value = queue
  templates.value = templateRows
  if (!approvalForm.templateId) approvalForm.templateId = templates.value[0]?.id
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
  if (!approvalForm.riskSummary || !approvalForm.rollbackPlan)
    return message.warning('风险说明和回滚方案必填')
  submitting.value = true
  try {
    await submitApproval(projectId.value!, detail.value.version.id, {
      approvalType: 'PRODUCTION_QUALIFICATION',
      targetEnvironment: 'PRODUCTION',
      templateId: approvalForm.templateId,
      riskSummary: approvalForm.riskSummary,
      rollbackPlan: approvalForm.rollbackPlan
    })
    approvalVisible.value = false
    detailVisible.value = false
    message.success('审批证据、输入指纹与模板快照已冻结')
    await loadAll()
  } finally {
    submitting.value = false
  }
}
const startDecision = (row: ApprovalRequest) => {
  selectedApproval.value = row
  decisionForm.comment = ''
  decisionVisible.value = true
}
const doDecision = async () => {
  if (!selectedApproval.value || !decisionForm.comment.trim())
    return message.warning('审批意见必填')
  await decideApproval(projectId.value!, selectedApproval.value.id, decisionForm)
  decisionVisible.value = false
  message.success('当前审批步骤已保存并写入审计')
  await loadAll()
}
const doCancelApproval = async (row: ApprovalRequest) => {
  const { value } = await ElMessageBox.prompt(
    '请填写取消原因；取消决定会写入不可变审计。',
    '取消审批',
    {
      confirmButtonText: '确认取消',
      cancelButtonText: '返回',
      inputValidator: (input) => Boolean(input?.trim()) || '取消原因必填'
    }
  )
  await cancelApproval(projectId.value!, row.id, value.trim())
  message.success('审批已取消')
  await loadAll()
}
const doCreateTemplate = async () => {
  if (!templateForm.name) return message.warning('模板名称必填')
  const steps = [
    { name: '技术与合规复核', requiredRole: templateForm.firstRole },
    ...(templateForm.secondRole
      ? [{ name: '生产发布批准', requiredRole: templateForm.secondRole }]
      : [])
  ]
  const value = await createApprovalTemplate({
    name: templateForm.name,
    approvalType: 'PRODUCTION_QUALIFICATION',
    targetEnvironment: 'PRODUCTION',
    steps,
    allowSelfApproval: templateForm.allowSelfApproval,
    enabled: true
  })
  templates.value.unshift(value)
  approvalForm.templateId = value.id
  message.success('租户审批模板已创建')
}
const openLicenseEditor = () => {
  if (!detail.value) return
  licenseForm.value = detail.value.licenses.length
    ? detail.value.licenses.map((row) => ({ ...row }))
    : (['DATA', 'PRETRAINED_WEIGHT', 'FRAMEWORK', 'MODEL_ARTIFACT'] as const).map(
        (componentType) =>
          ({
            id: 0,
            componentType,
            componentName:
              componentType === 'DATA'
                ? `DatasetVersion-${detail.value!.version.datasetVersionId}`
                : componentType === 'MODEL_ARTIFACT'
                  ? `ModelVersion-${detail.value!.version.id}`
                  : '请填写来源组件',
            licenseId: 'PROJECT_POLICY',
            sourceUri: '',
            useDeclaration: '仅用于本项目受控训练、评估与审批后的生产部署',
            decision: 'ALLOWED',
            reviewedBy: 0
          }) satisfies ModelLicenseDeclaration
      )
  licenseVisible.value = true
}
const saveLicenses = async () => {
  if (!detail.value || licenseForm.value.length !== 4)
    return message.warning('四类许可声明必须完整')
  await replaceModelLicenses(projectId.value!, detail.value.version.id, licenseForm.value)
  licenseVisible.value = false
  message.success('许可结论已保存，相关审批资格已重新计算')
  detail.value = await getModelVersion(projectId.value!, detail.value.version.id)
  await loadAll()
}
const downloadManifest = async () => {
  if (!detail.value) return
  if (!exportForm.purpose) return message.warning('导出用途必填')
  const value = await exportModel(projectId.value!, detail.value.version.id, exportForm)
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `model-version-${detail.value.version.id}-manifest.json`
  link.click()
  URL.revokeObjectURL(link.href)
  exportVisible.value = false
  message.success('审批指纹校验通过，导出行为已写入审计')
}
const toggleComparison = (versionId: number) => {
  if (comparisonIds.value.includes(versionId)) {
    comparisonIds.value = comparisonIds.value.filter((id) => id !== versionId)
    return
  }
  if (comparisonIds.value.length >= 5) {
    message.warning('一次最多比较 5 个模型版本')
    return
  }
  comparisonIds.value.push(versionId)
}
const showComparison = async () => {
  if (!projectId.value || comparisonIds.value.length < 2) {
    message.warning('请选择 2–5 个模型版本')
    return
  }
  comparisonRows.value = await compareModelVersions(projectId.value, comparisonIds.value)
  comparisonVisible.value = true
}
const formatBytes = (value: number) => {
  if (!value) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}
const metricSummary = (summary: Record<string, unknown>) => {
  const metrics =
    summary && typeof summary.metrics === 'object'
      ? (summary.metrics as Record<string, unknown>)
      : summary
  const ordered = ['mAP', 'accuracy', 'f1', 'precision', 'recall', 'iou']
  const entries = ordered
    .filter((key) => typeof metrics?.[key] === 'number')
    .slice(0, 3)
    .map((key) => `${key} ${Number(metrics[key]).toFixed(4)}`)
  return entries.join(' · ') || '暂无评估摘要'
}
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.model-page {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--el-fill-color-lighter);
}

.page-header,
.toolbar,
.version-card,
.section-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-header {
  margin-bottom: 20px;
}

.header-actions,
.subsection-heading {
  display: flex;
  gap: 8px;
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
  color: #4f46e5;
}

.toolbar {
  justify-content: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}

.toolbar .el-select {
  width: 260px;
}

.summary-grid,
.detail-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 20px;
}

.summary-grid article,
.detail-grid article,
.registry,
.approval-section {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
}

.summary-grid article,
.detail-grid article {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 16px;
}

.summary-grid strong,
.detail-grid strong {
  font-size: 25px;
}

.summary-grid small,
.summary-grid span,
.detail-grid small {
  color: var(--text-secondary);
}

.registry,
.approval-section {
  padding: 18px;
  margin-bottom: 18px;
}

.section-title h2 {
  margin: 0 0 3px;
}

.version-card {
  position: relative;
  padding: 15px;
  margin-top: 12px;
  cursor: pointer;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 11px;
}

.compare-selector {
  flex: 0 0 auto;
  margin-right: 6px;
}

.table-note {
  display: block;
  margin-top: 4px;
  color: var(--text-secondary);
}

.version-card:hover {
  border-color: #6366f1;
  box-shadow: 0 8px 22px rgb(79 70 229 / 8%);
}

.version-identity {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
}

.version-identity > span,
.version-identity small,
.evidence span {
  font-size: 12px;
  color: var(--text-secondary);
}

.version-identity strong {
  font-size: 16px;
}

.evidence {
  display: flex;
  flex-direction: column;
  width: 220px;
}

code {
  font-size: 12px;
  color: #4f46e5;
}

.empty {
  padding: 55px;
  color: var(--text-secondary);
  text-align: center;
}

.dialog-form {
  margin-top: 16px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.detail-grid {
  grid-template-columns: repeat(3, 1fr);
}

.drawer-actions {
  display: flex;
  gap: 10px;
  margin-top: 18px;
}

.subsection-heading {
  margin-top: 24px;
}

.subsection-heading h3 {
  margin: 0;
}

.subsection-heading p,
.dialog-intro {
  margin: 4px 0 12px;
  color: var(--text-secondary);
}

.license-row {
  display: grid;
  grid-template-columns: 140px 1fr 160px 1.3fr 140px;
  gap: 8px;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.full {
  width: 100%;
}

@media (width <= 900px) {
  .summary-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .license-row {
    grid-template-columns: 1fr;
  }

  .evidence {
    display: none;
  }
}

@media (width <= 560px) {
  .page-header,
  .version-card,
  .subsection-heading {
    align-items: flex-start;
    flex-direction: column;
  }

  .summary-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>

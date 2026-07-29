<template>
  <main class="feedback-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">PRODUCTION LEARNING LOOP</span>
        <h1>生产反馈与再训练闭环</h1>
        <p>按策略采样推理难例，经脱敏、去重和人工审核后进入真实 CVAT 返标链路。</p>
      </div>
      <el-button type="primary" :disabled="!projectId" @click="policyVisible = true"
        >配置采样策略</el-button
      >
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
      <el-button @click="runCleanup">执行保留期清理</el-button>
      <el-button type="primary" :disabled="!annotationBatches" @click="benefitVisible = true"
        >评估闭环收益</el-button
      >
    </section>
    <section class="summary-grid">
      <article
        ><small>策略</small><strong>{{ policy?.enabled ? 'ON' : 'OFF' }}</strong
        ><span>低置信阈值 {{ policy?.confidenceBelow || 0 }}</span></article
      >
      <article
        ><small>待审核样本</small><strong>{{ pending.length }}</strong
        ><span>每日上限 {{ policy?.dailyLimit || 0 }}</span></article
      >
      <article
        ><small>反馈批次</small><strong>{{ batches.length }}</strong
        ><span>去重后成批审核</span></article
      >
      <article
        ><small>真实返标任务</small><strong>{{ annotationBatches }}</strong
        ><span>保留 DeploymentRevision 血缘</span></article
      >
    </section>
    <section class="workspace">
      <section class="panel">
        <div class="section-title"
          ><div><h2>候选样本</h2><p>仅保存策略允许的样本引用，不保存完整生产请求体。</p></div
          ><el-button :disabled="!selectedIds.length" type="primary" @click="batchVisible = true"
            >创建批次 ({{ selectedIds.length }})</el-button
          ></div
        >
        <el-table :data="samples" @selection-change="selectionChanged">
          <el-table-column
            type="selection"
            width="46"
            :selectable="(row: FeedbackSample) => row.status === 'PENDING'"
          />
          <el-table-column prop="assetId" label="Asset" width="85" />
          <el-table-column prop="reason" label="采样原因" width="150"
            ><template #default="{ row }"
              ><el-tag>{{ row.reason }}</el-tag></template
            ></el-table-column
          >
          <el-table-column prop="deploymentRevisionId" label="Revision" width="95" />
          <el-table-column prop="modelVersionId" label="Model" width="80" />
          <el-table-column prop="perceptualHash" label="去重摘要"
            ><template #default="{ row }"
              ><code>{{ row.perceptualHash.slice(0, 16) }}…</code></template
            ></el-table-column
          >
          <el-table-column prop="status" label="状态" width="110" />
          <el-table-column prop="expiresAt" label="保留至" width="180" />
        </el-table>
      </section>
      <section class="panel batches">
        <div class="section-title"
          ><div><h2>审核与返标</h2><p>接受后自动冻结集合并创建 CVAT Task。</p></div></div
        >
        <article v-for="batch in batches" :key="batch.id">
          <header
            ><div
              ><span>BATCH #{{ batch.id }}</span
              ><strong>{{ batch.name }}</strong></div
            ><el-tag
              :type="
                batch.status === 'ANNOTATING'
                  ? 'success'
                  : batch.status === 'REJECTED'
                    ? 'danger'
                    : 'warning'
              "
              >{{ batch.status }}</el-tag
            ></header
          >
          <p>{{ batch.sampleCount }} 个样本</p>
          <div v-if="batch.annotationTaskId" class="lineage"
            >Feedback → CVAT Task #{{ batch.annotationTaskId }} → Dataset → Model → Deployment</div
          >
          <div v-if="batch.status === 'READY'" class="actions"
            ><el-button type="success" @click="startReview(batch, 'ACCEPT')">接受返标</el-button
            ><el-button type="danger" plain @click="startReview(batch, 'REJECT')"
              >拒绝</el-button
            ></div
          >
          <div v-if="batch.status === 'PRIVACY_REVIEW'" class="actions">
            <el-button type="primary" @click="startReview(batch, 'PRIVACY_APPROVE')"
              >隐私审批</el-button
            >
            <el-button type="danger" plain @click="startReview(batch, 'REJECT')">拒绝</el-button>
          </div>
        </article>
      </section>
    </section>
    <section class="panel benefit-panel">
      <div class="section-title">
        <div>
          <h2>闭环收益证据</h2>
          <p>在相同切片上比较基线与返标再训练版本，同时约束真实生产 QPS、错误率和 P95。</p>
        </div>
        <span>{{ benefits.length }} 份</span>
      </div>
      <el-table :data="benefits" empty-text="完成返标、再训练、评估和部署后创建第一份收益证据">
        <el-table-column prop="feedbackBatchId" label="反馈批次" width="110">
          <template #default="{ row }">#{{ row.feedbackBatchId }}</template>
        </el-table-column>
        <el-table-column label="模型版本">
          <template #default="{ row }"
            >#{{ row.baselineModelVersionId }} → #{{ row.candidateModelVersionId }}</template
          >
        </el-table-column>
        <el-table-column label="评估运行">
          <template #default="{ row }"
            >#{{ row.baselineEvaluationRunId }} → #{{ row.candidateEvaluationRunId }}</template
          >
        </el-table-column>
        <el-table-column prop="conclusion" label="结论" width="130">
          <template #default="{ row }">
            <el-tag :type="row.conclusion === 'IMPROVED' ? 'success' : 'warning'">{{
              row.conclusion
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="不可变证据摘要" min-width="210">
          <template #default="{ row }"
            ><code>{{ row.evidenceSha256 }}</code></template
          >
        </el-table-column>
      </el-table>
    </section>

    <el-dialog v-model="policyVisible" title="反馈采样与隐私策略" width="600">
      <el-form label-position="top">
        <el-form-item
          ><el-switch v-model="policyForm.enabled" active-text="启用生产采样"
        /></el-form-item>
        <div class="form-grid"
          ><el-form-item label="低置信度阈值"
            ><el-input-number
              v-model="policyForm.confidenceBelow"
              :min="0"
              :max="1"
              :step="0.05" /></el-form-item
          ><el-form-item label="随机采样比例"
            ><el-input-number
              v-model="policyForm.randomRate"
              :min="0"
              :max="1"
              :step="0.01" /></el-form-item
        ></div>
        <el-form-item label="采集前图像脱敏">
          <el-select v-model="policyForm.redactionMode" class="full">
            <el-option label="不修改图像（仍剥离 EXIF）" value="NONE" />
            <el-option label="遮挡画面中心敏感区域" value="MASK_CENTER" />
            <el-option label="整图像素化" value="PIXELATE" />
          </el-select>
        </el-form-item>
        <div class="form-grid"
          ><el-form-item label="每日上限"
            ><el-input-number v-model="policyForm.dailyLimit" :min="1" /></el-form-item
          ><el-form-item label="保留天数"
            ><el-input-number v-model="policyForm.retentionDays" :min="1" /></el-form-item
        ></div>
        <el-form-item
          ><el-checkbox v-model="policyForm.captureEmpty">采集空结果</el-checkbox
          ><el-checkbox v-model="policyForm.captureErrors">采集错误请求</el-checkbox
          ><el-checkbox v-model="policyForm.captureManual">允许人工反馈采样</el-checkbox
          ><el-checkbox v-model="policyForm.captureDrift">漂移触发采样</el-checkbox
          ><el-checkbox v-model="policyForm.sensitiveReview"
            >敏感项目需二次审核</el-checkbox
          ></el-form-item
        >
      </el-form>
      <template #footer
        ><el-button @click="policyVisible = false">取消</el-button
        ><el-button type="primary" @click="doSavePolicy">保存策略</el-button></template
      >
    </el-dialog>
    <el-dialog v-model="batchVisible" title="创建反馈审核批次" width="480">
      <el-form label-position="top"
        ><el-form-item label="批次名称"><el-input v-model="batchName" /></el-form-item
      ></el-form>
      <template #footer
        ><el-button @click="batchVisible = false">取消</el-button
        ><el-button type="primary" @click="doCreateBatch">创建</el-button></template
      >
    </el-dialog>
    <el-dialog v-model="reviewVisible" title="审核反馈批次" width="500">
      <el-alert
        :type="reviewDecision === 'ACCEPT' ? 'success' : 'warning'"
        :closable="false"
        :title="
          reviewDecision === 'ACCEPT'
            ? '接受后将创建真实 CVAT 返标任务'
            : reviewDecision === 'PRIVACY_APPROVE'
              ? '隐私审批通过后才能进入业务审核'
              : '拒绝后样本不进入训练链路'
        "
      />
      <el-form label-position="top" class="review-form"
        ><el-form-item label="审核意见（必填）"
          ><el-input v-model="reviewComment" type="textarea" :rows="4" /></el-form-item
      ></el-form>
      <template #footer
        ><el-button @click="reviewVisible = false">取消</el-button
        ><el-button type="primary" @click="doReview">确认决定</el-button></template
      >
    </el-dialog>
    <el-dialog v-model="benefitVisible" title="创建闭环收益评估" width="680">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="反馈批次">
            <el-select v-model="benefitForm.feedbackBatchId" class="full">
              <el-option
                v-for="batch in batches.filter((row) => row.annotationTaskId > 0)"
                :key="batch.id"
                :label="`#${batch.id} · ${batch.name}`"
                :value="batch.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="切片">
            <el-select v-model="benefitForm.slices" multiple allow-create filterable class="full">
              <el-option label="all" value="all" />
              <el-option label="small-object" value="small-object" />
              <el-option label="occluded" value="occluded" />
              <el-option label="dense" value="dense" />
            </el-select>
          </el-form-item>
          <el-form-item label="基线评估">
            <el-select v-model="benefitForm.baselineEvaluationRunId" class="full">
              <el-option
                v-for="run in successfulEvaluations"
                :key="run.id"
                :label="`Evaluation #${run.id}`"
                :value="run.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="候选评估">
            <el-select v-model="benefitForm.candidateEvaluationRunId" class="full">
              <el-option
                v-for="run in successfulEvaluations"
                :key="run.id"
                :label="`Evaluation #${run.id}`"
                :value="run.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="基线部署">
            <el-select v-model="benefitForm.baselineDeploymentId" class="full">
              <el-option
                v-for="deployment in deployments"
                :key="deployment.id"
                :label="`#${deployment.id} · ${deployment.name}`"
                :value="deployment.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="候选部署">
            <el-select v-model="benefitForm.candidateDeploymentId" class="full">
              <el-option
                v-for="deployment in deployments"
                :key="deployment.id"
                :label="`#${deployment.id} · ${deployment.name}`"
                :value="deployment.id"
              />
            </el-select>
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="benefitVisible = false">取消</el-button>
        <el-button type="primary" @click="doCreateBenefit">生成收益证据</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<script lang="ts" setup>
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import { getEvaluationRuns, type EvaluationRun } from '@/api/ai-platform/evaluation'
import { getDeployments, type Deployment } from '@/api/ai-platform/deployments'
import {
  cleanupFeedback,
  createFeedbackBenefit,
  createFeedbackBatch,
  getFeedbackBenefits,
  getFeedbackBatches,
  getFeedbackPolicy,
  getFeedbackSamples,
  reviewFeedbackBatch,
  saveFeedbackPolicy,
  type FeedbackBatch,
  type FeedbackBenefit,
  type FeedbackPolicy,
  type FeedbackSample
} from '@/api/ai-platform/feedback'

defineOptions({ name: 'VisionAIFeedback' })
const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const policy = ref<FeedbackPolicy | null>()
const samples = ref<FeedbackSample[]>([])
const batches = ref<FeedbackBatch[]>([])
const benefits = ref<FeedbackBenefit[]>([])
const evaluations = ref<EvaluationRun[]>([])
const deployments = ref<Deployment[]>([])
const selectedIds = ref<number[]>([])
const policyVisible = ref(false)
const batchVisible = ref(false)
const reviewVisible = ref(false)
const benefitVisible = ref(false)
const batchName = ref('生产困难样本')
const reviewBatch = ref<FeedbackBatch>()
const reviewDecision = ref('ACCEPT')
const reviewComment = ref('')
const benefitForm = reactive({
  feedbackBatchId: undefined as number | undefined,
  baselineEvaluationRunId: undefined as number | undefined,
  candidateEvaluationRunId: undefined as number | undefined,
  baselineDeploymentId: undefined as number | undefined,
  candidateDeploymentId: undefined as number | undefined,
  slices: ['all']
})
const policyForm = reactive({
  enabled: true,
  randomRate: 0,
  confidenceBelow: 0.75,
  captureEmpty: true,
  captureErrors: true,
  captureManual: true,
  captureDrift: true,
  dailyLimit: 1000,
  retentionDays: 30,
  redactionMode: 'NONE',
  sensitiveReview: false
})
const pending = computed(() => samples.value.filter((row) => row.status === 'PENDING'))
const annotationBatches = computed(
  () => batches.value.filter((row) => row.annotationTaskId > 0).length
)
const successfulEvaluations = computed(() =>
  evaluations.value.filter((row) => row.status === 'SUCCEEDED')
)
const loadAll = async () => {
  if (!projectId.value) return
  const deploymentPage = await getDeployments(projectId.value)
  ;[policy.value, samples.value, batches.value, benefits.value, evaluations.value] =
    await Promise.all([
      getFeedbackPolicy(projectId.value),
      getFeedbackSamples(projectId.value),
      getFeedbackBatches(projectId.value),
      getFeedbackBenefits(projectId.value),
      getEvaluationRuns(projectId.value)
    ])
  deployments.value = deploymentPage.deployments
  if (policy.value) {
    Object.assign(policyForm, policy.value)
    try {
      const redaction = JSON.parse(policy.value.redactionPolicy || '{}')
      policyForm.redactionMode = redaction.mode || 'NONE'
    } catch {
      policyForm.redactionMode = 'NONE'
    }
  }
}
const selectionChanged = (rows: FeedbackSample[]) => {
  selectedIds.value = rows.map((row) => row.id)
}
const doSavePolicy = async () => {
  policy.value = await saveFeedbackPolicy(projectId.value!, {
    ...policyForm,
    redactionPolicy: {
      mode: policyForm.redactionMode,
      stripExif: true,
      storeRequestBody: false,
      centerWidthRatio: 0.45,
      centerHeightRatio: 0.3
    }
  })
  policyVisible.value = false
  message.success('生产反馈策略已生效')
}
const doCreateBatch = async () => {
  await createFeedbackBatch(projectId.value!, {
    name: batchName.value,
    sampleIds: selectedIds.value
  })
  batchVisible.value = false
  message.success('反馈批次已进入审核')
  await loadAll()
}
const startReview = (batch: FeedbackBatch, decision: string) => {
  reviewBatch.value = batch
  reviewDecision.value = decision
  reviewComment.value = ''
  reviewVisible.value = true
}
const doReview = async () => {
  if (!reviewBatch.value || !reviewComment.value.trim()) return message.warning('审核意见必填')
  await reviewFeedbackBatch(projectId.value!, reviewBatch.value.id, {
    decision: reviewDecision.value,
    comment: reviewComment.value
  })
  reviewVisible.value = false
  message.success(
    reviewDecision.value === 'ACCEPT'
      ? '已创建真实 CVAT 返标任务'
      : reviewDecision.value === 'PRIVACY_APPROVE'
        ? '隐私审批已通过，批次进入业务审核'
        : '批次已拒绝'
  )
  await loadAll()
}
const doCreateBenefit = async () => {
  if (Object.entries(benefitForm).some(([key, value]) => key !== 'slices' && !value)) {
    return message.warning('请选择批次、两次评估和两次部署')
  }
  await createFeedbackBenefit(projectId.value!, benefitForm)
  benefitVisible.value = false
  message.success('闭环收益证据已生成')
  await loadAll()
}
const runCleanup = async () => {
  const result = await cleanupFeedback(projectId.value!)
  message.success(`已按保留策略过期 ${result.expired} 个样本`)
  await loadAll()
}
onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.feedback-page {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--el-fill-color-extra-light);
}

.benefit-panel {
  margin-top: 18px;
}

.benefit-panel code {
  display: block;
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.page-header,
.toolbar,
.section-title,
.batches header {
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
  color: #ea580c;
}

.toolbar {
  justify-content: flex-start;
  gap: 12px;
  margin: 20px 0;
}

.toolbar .el-select {
  width: 260px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 18px;
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

.workspace {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(350px, 0.65fr);
  gap: 18px;
}

.panel {
  padding: 18px;
}

.section-title h2 {
  margin: 0 0 3px;
}

code {
  color: #c2410c;
}

.batches article {
  padding: 14px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.batches header div {
  display: flex;
  flex-direction: column;
}

.batches header span {
  font-size: 11px;
  color: #ea580c;
}

.batches p {
  color: var(--text-secondary);
}

.lineage {
  padding: 8px;
  font-size: 12px;
  color: #9a3412;
  background: rgb(234 88 12 / 7%);
  border-radius: 7px;
}

.actions {
  margin-top: 10px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.review-form {
  margin-top: 14px;
}
</style>

<template>
  <main class="annotation-center">
    <header class="page-header">
      <div>
        <span class="eyebrow">HUMAN IN THE LOOP</span>
        <h1>标注任务</h1>
        <p>底座账号自动进入 CVAT，多名标注员并行处理分片，审核与快照全程可追溯。</p>
      </div>
      <div class="actions">
        <el-button @click="mappingVisible = true">
          <Icon icon="lucide:users-round" :size="16" />协作身份
        </el-button>
        <el-button :disabled="!projectId" @click="showPreannotationComparison">
          <Icon icon="lucide:chart-no-axes-combined" :size="16" />预标注效果
        </el-button>
        <el-button type="primary" :disabled="!projectId" @click="createVisible = true">
          <Icon icon="lucide:plus" :size="16" />创建任务
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
      <el-select v-model="query.status" clearable placeholder="全部状态" @change="loadTasks">
        <el-option
          v-for="status in statuses"
          :key="status"
          :label="statusText(status)"
          :value="status"
        />
      </el-select>
      <el-button @click="loadTasks"><Icon icon="lucide:refresh-cw" :size="15" />刷新</el-button>
    </section>

    <section class="metric-strip">
      <article
        ><small>全部任务</small><strong>{{ total }}</strong></article
      >
      <article
        ><small>标注中</small><strong>{{ countStatus('ANNOTATING') }}</strong></article
      >
      <article
        ><small>待审核</small><strong>{{ countStatus('REVIEWING') }}</strong></article
      >
      <article
        ><small>已关闭快照</small><strong>{{ countStatus('CLOSED') }}</strong></article
      >
    </section>

    <section v-loading="loading" class="task-list">
      <article v-for="task in tasks" :key="task.id" class="task-card">
        <div class="task-main">
          <div class="task-title">
            <el-tag :type="statusType(task.status)" effect="light">{{
              statusText(task.status)
            }}</el-tag>
            <span>#{{ task.id }} · {{ task.taskType }}</span>
          </div>
          <h2>{{ task.name }}</h2>
          <p>资产集合 #{{ task.collectionId }} · 类别体系 {{ task.ontologyVersion }}</p>
          <el-progress :percentage="task.progress" :stroke-width="7" />
          <p v-if="task.errorMessage" class="error"
            >{{ task.errorCode }} · {{ task.errorMessage }}</p
          >
          <p v-if="task.rejectionReason" class="warning">
            {{ task.rejectionCode }} · {{ task.rejectionReason }}
          </p>
        </div>
        <div class="task-actions">
          <el-button
            v-if="task.status === 'DRAFT' || task.status === 'FAILED'"
            @click="prepare(task)"
          >
            创建/恢复 CVAT
          </el-button>
          <el-button v-if="task.externalBindingId" @click="sync(task)">同步状态</el-button>
          <el-button
            v-if="task.status === 'READY' || task.status === 'REJECTED'"
            @click="advance(task, 'ANNOTATING')"
          >
            开始标注
          </el-button>
          <el-button
            v-if="task.status === 'READY' && task.externalBindingId"
            type="primary"
            plain
            @click="openPreannotation(task)"
          >
            GPU 预标注
          </el-button>
          <el-button v-if="task.status === 'ANNOTATING'" @click="advance(task, 'REVIEWING')">
            提交审核
          </el-button>
          <el-button
            v-if="task.status === 'REVIEWING'"
            type="success"
            @click="review(task, 'APPROVE')"
          >
            审核通过
          </el-button>
          <el-button v-if="task.status === 'REVIEWING'" type="danger" @click="reject(task)"
            >驳回</el-button
          >
          <el-button v-if="task.status === 'APPROVED'" type="primary" @click="exportSnapshot(task)">
            导出快照
          </el-button>
          <el-button v-if="task.externalBindingId" link type="primary" @click="openWorkbench(task)">
            内嵌 CVAT
          </el-button>
          <el-button
            v-if="task.currentRevisionId || task.preannotationRunId"
            link
            @click="showDetail(task)"
            >查看证据</el-button
          >
        </div>
      </article>
      <div v-if="!loading && !tasks.length" class="empty-state">
        <Icon icon="lucide:scan-search" :size="34" />
        <strong>暂无标注任务</strong>
        <span>先冻结一个资产集合，再创建 CVAT 标注任务。</span>
      </div>
    </section>

    <el-dialog v-model="createVisible" title="创建标注任务" width="600">
      <el-form label-position="top">
        <el-form-item label="任务名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="冻结资产集合">
          <el-select v-model="form.collectionId" class="full-width">
            <el-option
              v-for="collection in frozenCollections"
              :key="collection.id"
              :label="`${collection.name} · ${collection.assetCount} 张 · v${collection.version}`"
              :value="collection.id"
            />
          </el-select>
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="任务类型">
            <el-select v-model="form.taskType"
              ><el-option label="目标检测" value="CV_DETECTION"
            /></el-select>
          </el-form-item>
          <el-form-item label="类别体系版本" required>
            <el-select
              v-model="form.ontologyVersionId"
              class="full-width"
              placeholder="选择已发布版本"
            >
              <el-option
                v-for="version in ontologyVersions"
                :key="version.id"
                :label="`${version.ontologyName} · ${version.semanticVersion} · ${version.labelCount} 类`"
                :value="version.id"
              />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="计划日期">
          <el-date-picker
            v-model="form.planRange"
            class="full-width"
            type="datetimerange"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            start-placeholder="计划开始"
            end-placeholder="计划完成"
          />
        </el-form-item>
        <el-alert
          v-if="!ontologyVersions.length"
          :closable="false"
          type="warning"
          show-icon
          title="当前项目没有已发布的目标检测类别版本，请先到“标签与类别”发布。"
        />
        <div class="form-grid">
          <el-form-item label="标注员">
            <el-select
              v-model="form.annotatorIds"
              class="full-width"
              multiple
              filterable
              collapse-tags
              placeholder="选择项目标注员"
            >
              <el-option
                v-for="user in annotatorOptions"
                :key="user.id"
                :label="`${user.nickname || user.username} · ${user.username}`"
                :value="user.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="审核员">
            <el-select
              v-model="form.reviewerIds"
              class="full-width"
              multiple
              filterable
              collapse-tags
              placeholder="选择项目审核员"
            >
              <el-option
                v-for="user in reviewerOptions"
                :key="user.id"
                :label="`${user.nickname || user.username} · ${user.username}`"
                :value="user.id"
              />
            </el-select>
          </el-form-item>
        </div>
        <el-alert
          :closable="false"
          type="info"
          show-icon
          title="账号与权限来自底座；创建任务时自动同步个人 CVAT 身份，并按人员分配 Job。"
        />
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitCreate">创建草稿</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="mappingVisible" title="CVAT 协作身份" width="680" @open="loadMappings">
      <el-alert
        class="identity-tip"
        :closable="false"
        type="success"
        show-icon
        title="无需维护第二套账号。外部身份由底座用户自动生成，密码不会显示或下发到浏览器。"
      />
      <el-table :data="mappings">
        <el-table-column label="底座用户" min-width="170">
          <template #default="{ row }">{{ userLabel(row.platformUserId) }}</template>
        </el-table-column>
        <el-table-column prop="cvatUserId" label="CVAT ID" />
        <el-table-column prop="cvatUsername" label="CVAT 用户名" />
        <el-table-column label="状态"
          ><template #default="{ row }"
            ><el-tag type="success">{{ row.active ? '已验证' : '停用' }}</el-tag></template
          ></el-table-column
        >
      </el-table>
      <el-divider />
      <div class="mapping-form">
        <el-select
          v-model="mappingUserId"
          class="identity-user-select"
          filterable
          placeholder="选择底座用户"
        >
          <el-option
            v-for="user in projectUsers"
            :key="user.id"
            :label="`${user.nickname || user.username} · ${user.username}`"
            :value="user.id"
          />
        </el-select>
        <el-button type="primary" :loading="mappingSaving" @click="saveMapping">
          同步个人身份
        </el-button>
      </div>
    </el-dialog>

    <el-dialog v-model="detailVisible" title="不可变 AnnotationRevision" width="680">
      <el-descriptions v-if="detail?.revisions[0]" :column="1" border>
        <el-descriptions-item label="Revision"
          >v{{ detail.revisions[0].revisionNo }}</el-descriptions-item
        >
        <el-descriptions-item label="格式">{{ detail.revisions[0].format }}</el-descriptions-item>
        <el-descriptions-item label="标注数">{{
          detail.revisions[0].annotationCount
        }}</el-descriptions-item>
        <el-descriptions-item label="SHA-256"
          ><code>{{ detail.revisions[0].checksum }}</code></el-descriptions-item
        >
        <el-descriptions-item label="快照 URI">{{
          detail.revisions[0].snapshotUri
        }}</el-descriptions-item>
      </el-descriptions>
      <template v-if="detail?.preannotationRuns?.length">
        <el-divider content-position="left">预标注运行与人工修正</el-divider>
        <article
          v-for="run in detail.preannotationRuns"
          :key="run.id"
          class="preannotation-evidence"
        >
          <div>
            <strong>运行 #{{ run.id }} · 模型版本 #{{ run.modelVersionId }}</strong>
            <el-tag size="small" :type="run.status === 'FAILED' ? 'danger' : 'success'">{{
              run.status
            }}</el-tag>
          </div>
          <p>
            建议 {{ run.proposedCount }} · 接受 {{ run.acceptedCount }} · 删除
            {{ run.deletedCount }} · 修改 {{ run.modifiedCount }} · 新增 {{ run.addedCount }} · 修正
            {{ run.correctionSeconds }} 秒
          </p>
          <el-button
            v-if="run.status === 'IMPORTED' || run.status === 'SUCCEEDED'"
            size="small"
            @click="openMetrics(detail!.task, run)"
            >登记修正指标</el-button
          >
          <p v-if="run.errorMessage" class="error">{{ run.errorCode }} · {{ run.errorMessage }}</p>
        </article>
      </template>
    </el-dialog>

    <el-dialog v-model="preannotationVisible" title="运行预标注" width="720">
      <el-alert
        :closable="false"
        show-icon
        type="info"
        title="仅显示已批准用于预标注的模型；任务由独立编排器执行真实推理并幂等写入 CVAT。"
      />
      <el-form class="preannotation-form" label-position="top">
        <el-form-item label="模型版本">
          <el-select v-model="preannotationForm.modelVersionId" class="full-width">
            <el-option
              v-for="version in eligibleModels"
              :key="version.id"
              :label="`${modelName(version.modelId)} · ${version.semanticVersion} · #${version.id}`"
              :value="version.id"
            />
          </el-select>
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="最低置信度">
            <el-input-number
              v-model="preannotationForm.confidence"
              :min="0"
              :max="1"
              :step="0.05"
            />
          </el-form-item>
          <el-form-item label="NMS IoU">
            <el-input-number v-model="preannotationForm.nms" :min="0.05" :max="1" :step="0.05" />
          </el-form-item>
          <el-form-item label="批大小">
            <el-input-number v-model="preannotationForm.batchSize" :min="1" :max="128" />
          </el-form-item>
          <el-form-item label="设备">
            <el-radio-group v-model="preannotationForm.device">
              <el-radio-button value="GPU">GPU</el-radio-button>
              <el-radio-button value="CPU">CPU</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="低置信结果">
            <el-select v-model="preannotationForm.lowConfidencePolicy">
              <el-option label="丢弃" value="DROP" />
              <el-option label="保留待审核" value="KEEP_REVIEW" />
            </el-select>
          </el-form-item>
          <el-form-item label="保留下限">
            <el-input-number
              v-model="preannotationForm.lowConfidenceFloor"
              :min="0"
              :max="preannotationForm.confidence"
              :step="0.05"
            />
          </el-form-item>
        </div>
        <el-form-item label="类别映射（JSON，可选）">
          <el-input
            v-model="preannotationForm.classMappingText"
            type="textarea"
            :rows="4"
            placeholder='{"source_label_code":"目标类别名称"}'
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="preannotationVisible = false">取消</el-button>
        <el-button type="primary" :loading="preannotationSaving" @click="submitPreannotation"
          >进入 GPU 队列</el-button
        >
      </template>
    </el-dialog>

    <el-dialog v-model="metricsVisible" title="登记人工修正指标" width="620">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="模型建议"
            ><el-input-number v-model="metricsForm.proposedCount" :min="0"
          /></el-form-item>
          <el-form-item label="直接接受"
            ><el-input-number v-model="metricsForm.acceptedCount" :min="0"
          /></el-form-item>
          <el-form-item label="删除"
            ><el-input-number v-model="metricsForm.deletedCount" :min="0"
          /></el-form-item>
          <el-form-item label="修改"
            ><el-input-number v-model="metricsForm.modifiedCount" :min="0"
          /></el-form-item>
          <el-form-item label="人工新增"
            ><el-input-number v-model="metricsForm.addedCount" :min="0"
          /></el-form-item>
          <el-form-item label="修正耗时（秒）"
            ><el-input-number v-model="metricsForm.correctionSeconds" :min="0"
          /></el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="metricsVisible = false">取消</el-button>
        <el-button type="primary" @click="saveMetrics">保存可审计指标</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="comparisonVisible" title="预标注模型效果对比" width="980">
      <el-table :data="comparisons">
        <el-table-column label="运行 / 模型" min-width="160">
          <template #default="{ row }">#{{ row.run.id }} / #{{ row.run.modelVersionId }}</template>
        </el-table-column>
        <el-table-column label="接受率"
          ><template #default="{ row }">{{
            percent(row.acceptanceRate)
          }}</template></el-table-column
        >
        <el-table-column label="删除率"
          ><template #default="{ row }">{{ percent(row.deletionRate) }}</template></el-table-column
        >
        <el-table-column label="修改率"
          ><template #default="{ row }">{{
            percent(row.modificationRate)
          }}</template></el-table-column
        >
        <el-table-column label="新增率"
          ><template #default="{ row }">{{ percent(row.additionRate) }}</template></el-table-column
        >
        <el-table-column label="单位修正"
          ><template #default="{ row }"
            >{{ row.unitCorrectionSeconds.toFixed(1) }}s</template
          ></el-table-column
        >
        <el-table-column label="类别 / 场景" min-width="230">
          <template #default="{ row }">
            <span>{{ breakdownText(row.breakdown?.byClass) }}</span>
            <small>{{ breakdownText(row.breakdown?.byScene) }}</small>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <EmbeddedWorkbench
      v-model="workbenchVisible"
      provider="CVAT"
      :title="workbenchTitle"
      :context="workbenchContext"
      :url="workbenchUrl"
      @refresh="refreshWorkbench"
      @open-external="openWorkbenchExternal"
    />
  </main>
</template>

<script lang="ts" setup>
import EmbeddedWorkbench from '@/views/ai-platform/components/EmbeddedWorkbench.vue'
import {
  createAnnotationTask,
  createPreannotation,
  exportAnnotationTask,
  getAnnotationTask,
  getAnnotationTasks,
  getAssetCollections,
  getCVATUserMappings,
  getPreannotationComparisons,
  openAnnotationWorkbench,
  prepareAnnotationTask,
  reviewAnnotationTask,
  saveCVATUserMapping,
  syncAnnotationTask,
  updateAnnotationStatus,
  updatePreannotationMetrics,
  type AnnotationStatus,
  type AnnotationTask,
  type AssetCollection,
  type CVATUserMapping,
  type PreannotationComparison,
  type PreannotationRun
} from '@/api/ai-platform/annotations'
import { getModels, type ModelRecord, type ModelVersion } from '@/api/ai-platform/models'
import {
  getProjectPage,
  getProjectMembers,
  type Project,
  type ProjectMember
} from '@/api/ai-platform/projects'
import { getSimpleUserList, type UserVO } from '@/api/system/user'
import {
  getPublishedOntologyVersions,
  type SelectableOntologyVersion
} from '@/api/ai-platform/ontology'

defineOptions({ name: 'VisionAIAnnotations' })
const message = useMessage()
const statuses: AnnotationStatus[] = [
  'DRAFT',
  'PREPARING',
  'PREANNOTATING',
  'READY',
  'ANNOTATING',
  'REVIEWING',
  'APPROVED',
  'REJECTED',
  'EXPORTING',
  'CLOSED',
  'FAILED',
  'CANCELLED'
]
const projects = ref<Project[]>([])
const projectId = ref<number>()
const tasks = ref<AnnotationTask[]>([])
const collections = ref<AssetCollection[]>([])
const mappings = ref<CVATUserMapping[]>([])
const members = ref<ProjectMember[]>([])
const users = ref<UserVO[]>([])
const ontologyVersions = ref<SelectableOntologyVersion[]>([])
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const mappingSaving = ref(false)
const createVisible = ref(false)
const mappingVisible = ref(false)
const detailVisible = ref(false)
const workbenchVisible = ref(false)
const preannotationVisible = ref(false)
const preannotationSaving = ref(false)
const metricsVisible = ref(false)
const comparisonVisible = ref(false)
const workbenchUrl = ref('')
const workbenchTitle = ref('CVAT 标注工作台')
const workbenchContext = ref('')
const activeWorkbenchTask = ref<AnnotationTask>()
const detail = ref<Awaited<ReturnType<typeof getAnnotationTask>>>()
const modelRecords = ref<ModelRecord[]>([])
const modelVersions = ref<ModelVersion[]>([])
const comparisons = ref<PreannotationComparison[]>([])
const activePreannotationTask = ref<AnnotationTask>()
const activeMetricsTask = ref<AnnotationTask>()
const activeMetricsRun = ref<PreannotationRun>()
const query = reactive({ pageNo: 1, pageSize: 50, status: '' })
const form = reactive({
  name: '',
  taskType: 'CV_DETECTION',
  collectionId: undefined as number | undefined,
  ontologyVersionId: undefined as number | undefined,
  annotatorIds: [] as number[],
  reviewerIds: [] as number[],
  planRange: [] as string[]
})
const preannotationForm = reactive({
  modelVersionId: undefined as number | undefined,
  confidence: 0.5,
  nms: 0.5,
  batchSize: 8,
  device: 'GPU' as 'GPU' | 'CPU',
  lowConfidencePolicy: 'KEEP_REVIEW' as 'DROP' | 'KEEP_REVIEW',
  lowConfidenceFloor: 0.1,
  classMappingText: '{}'
})
const metricsForm = reactive({
  proposedCount: 0,
  acceptedCount: 0,
  deletedCount: 0,
  modifiedCount: 0,
  addedCount: 0,
  correctionSeconds: 0
})
const mappingUserId = ref<number>()
const frozenCollections = computed(() => collections.value.filter((item) => item.frozen))
const eligibleModels = computed(() =>
  modelVersions.value.filter(
    (version) => version.status === 'APPROVED' && version.preannotationApproved
  )
)
const modelName = (modelId: number) =>
  modelRecords.value.find((model) => model.id === modelId)?.name || `模型 #${modelId}`
const memberByUser = computed(() => new Map(members.value.map((member) => [member.userId, member])))
const projectUsers = computed(() => users.value.filter((user) => memberByUser.value.has(user.id)))
const annotatorOptions = computed(() =>
  projectUsers.value.filter(
    (user) =>
      user.id === projects.value.find((project) => project.id === projectId.value)?.ownerUserId ||
      memberByUser.value.get(user.id)?.roles.some((role) => role.code === 'ANNOTATOR')
  )
)
const reviewerOptions = computed(() =>
  projectUsers.value.filter(
    (user) =>
      user.id === projects.value.find((project) => project.id === projectId.value)?.ownerUserId ||
      memberByUser.value.get(user.id)?.roles.some((role) => role.code === 'REVIEWER')
  )
)
const userLabel = (userId: number) => {
  const user = users.value.find((item) => item.id === userId)
  return user ? `${user.nickname || user.username} · ${user.username}` : `用户 #${userId}`
}
const countStatus = (status: AnnotationStatus) =>
  tasks.value.filter((task) => task.status === status).length
const statusText = (status: AnnotationStatus) =>
  ({
    DRAFT: '草稿',
    PREPARING: '准备中',
    PREANNOTATING: '预标注',
    READY: '待开始',
    ANNOTATING: '标注中',
    REVIEWING: '审核中',
    APPROVED: '已批准',
    REJECTED: '待返工',
    EXPORTING: '导出中',
    CLOSED: '已关闭',
    FAILED: '失败',
    CANCELLED: '已取消'
  })[status]
const statusType = (status: AnnotationStatus) =>
  status === 'CLOSED' || status === 'APPROVED'
    ? 'success'
    : status === 'FAILED' || status === 'REJECTED'
      ? 'danger'
      : status === 'REVIEWING'
        ? 'warning'
        : 'primary'
const loadTasks = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    const data = await getAnnotationTasks(projectId.value, query)
    tasks.value = data.list
    total.value = data.total
  } finally {
    loading.value = false
  }
}
const loadAll = async () => {
  if (!projectId.value) return
  const [, collectionRows, memberRows, versionRows, modelData] = await Promise.all([
    loadTasks(),
    getAssetCollections(projectId.value),
    getProjectMembers(projectId.value),
    getPublishedOntologyVersions(projectId.value),
    getModels(projectId.value)
  ])
  collections.value = collectionRows
  members.value = memberRows
  ontologyVersions.value = versionRows
  modelRecords.value = modelData.models
  modelVersions.value = modelData.versions
  preannotationForm.modelVersionId = eligibleModels.value[0]?.id
  form.collectionId = frozenCollections.value[0]?.id
  form.ontologyVersionId = ontologyVersions.value[0]?.id
  form.annotatorIds = annotatorOptions.value.slice(0, 1).map((user) => user.id)
  form.reviewerIds = reviewerOptions.value.slice(0, 1).map((user) => user.id)
  mappingUserId.value = projectUsers.value[0]?.id
}
const loadMappings = async () => (mappings.value = await getCVATUserMappings())
const submitCreate = async () => {
  if (
    !projectId.value ||
    !form.collectionId ||
    !form.ontologyVersionId ||
    !form.name.trim() ||
    !form.annotatorIds.length ||
    !form.reviewerIds.length
  )
    return message.warning('请填写任务名称，选择冻结集合、已发布类别版本、标注员和审核员')
  saving.value = true
  try {
    await createAnnotationTask(projectId.value, {
      name: form.name,
      taskType: form.taskType,
      collectionId: form.collectionId,
      ontologyVersionId: form.ontologyVersionId,
      annotatorIds: form.annotatorIds,
      reviewerIds: form.reviewerIds,
      planStartAt: form.planRange[0] || undefined,
      planEndAt: form.planRange[1] || undefined
    })
    createVisible.value = false
    message.success('标注任务草稿已创建')
    await loadTasks()
  } finally {
    saving.value = false
  }
}
const prepare = async (task: AnnotationTask) => {
  await prepareAnnotationTask(projectId.value!, task.id)
  message.success('CVAT 编排已进入队列')
  await loadTasks()
}
const sync = async (task: AnnotationTask) => {
  await syncAnnotationTask(projectId.value!, task.id)
  await loadTasks()
}
const advance = async (task: AnnotationTask, status: AnnotationStatus) => {
  await updateAnnotationStatus(projectId.value!, task.id, status)
  await loadTasks()
}
const review = async (task: AnnotationTask, decision: 'APPROVE' | 'REJECT') => {
  await reviewAnnotationTask(projectId.value!, task.id, { decision })
  await loadTasks()
}
const reject = async (task: AnnotationTask) => {
  const reason = await message.prompt('请输入返工原因', '驳回标注')
  await reviewAnnotationTask(projectId.value!, task.id, {
    decision: 'REJECT',
    code: 'QUALITY_REWORK',
    reason: reason.value
  })
  await loadTasks()
}
const exportSnapshot = async (task: AnnotationTask) => {
  await exportAnnotationTask(projectId.value!, task.id)
  message.success('不可变快照导出已进入队列')
  await loadTasks()
}
const openWorkbench = async (task: AnnotationTask) => {
  activeWorkbenchTask.value = task
  const data = await openAnnotationWorkbench(projectId.value!, task.id)
  workbenchUrl.value = data.url
  workbenchTitle.value = task.name
  workbenchContext.value = `VisionAI Task #${task.id} · ${task.taskType} · ${statusText(task.status)}`
  workbenchVisible.value = true
}
const refreshWorkbench = async () => {
  if (!activeWorkbenchTask.value) return
  const data = await openAnnotationWorkbench(projectId.value!, activeWorkbenchTask.value.id)
  workbenchUrl.value = data.url
}
const openWorkbenchExternal = async () => {
  if (!activeWorkbenchTask.value) return
  const popup = window.open('about:blank', '_blank')
  try {
    const data = await openAnnotationWorkbench(projectId.value!, activeWorkbenchTask.value.id)
    if (popup) popup.location.href = data.url
    else window.open(data.url, '_blank', 'noopener,noreferrer')
  } catch (error) {
    popup?.close()
    throw error
  }
}
const showDetail = async (task: AnnotationTask) => {
  detail.value = await getAnnotationTask(projectId.value!, task.id)
  detailVisible.value = true
}
const openPreannotation = (task: AnnotationTask) => {
  activePreannotationTask.value = task
  preannotationForm.modelVersionId = eligibleModels.value[0]?.id
  preannotationVisible.value = true
}
const submitPreannotation = async () => {
  if (!projectId.value || !activePreannotationTask.value || !preannotationForm.modelVersionId)
    return message.warning('当前项目没有已批准用于预标注的模型')
  let classMapping: Record<string, string>
  try {
    classMapping = JSON.parse(preannotationForm.classMappingText || '{}')
  } catch {
    return message.warning('类别映射必须是 JSON 对象')
  }
  preannotationSaving.value = true
  try {
    await createPreannotation(projectId.value, activePreannotationTask.value.id, {
      modelVersionId: preannotationForm.modelVersionId,
      parameters: {
        confidence: preannotationForm.confidence,
        nms: preannotationForm.nms,
        classMapping,
        batchSize: preannotationForm.batchSize,
        device: preannotationForm.device,
        lowConfidencePolicy: preannotationForm.lowConfidencePolicy,
        lowConfidenceFloor: preannotationForm.lowConfidenceFloor
      }
    })
    preannotationVisible.value = false
    message.success('真实模型预标注已进入独立编排器队列')
    await loadTasks()
  } finally {
    preannotationSaving.value = false
  }
}
const openMetrics = (task: AnnotationTask, run: PreannotationRun) => {
  activeMetricsTask.value = task
  activeMetricsRun.value = run
  Object.assign(metricsForm, {
    proposedCount: run.proposedCount,
    acceptedCount: run.acceptedCount,
    deletedCount: run.deletedCount,
    modifiedCount: run.modifiedCount,
    addedCount: run.addedCount,
    correctionSeconds: run.correctionSeconds
  })
  metricsVisible.value = true
}
const saveMetrics = async () => {
  if (!projectId.value || !activeMetricsTask.value || !activeMetricsRun.value) return
  await updatePreannotationMetrics(
    projectId.value,
    activeMetricsTask.value.id,
    activeMetricsRun.value.id,
    { ...metricsForm }
  )
  metricsVisible.value = false
  message.success('预标注人工修正指标已保存')
  detail.value = await getAnnotationTask(projectId.value, activeMetricsTask.value.id)
}
const showPreannotationComparison = async () => {
  if (!projectId.value) return
  comparisons.value = await getPreannotationComparisons(projectId.value)
  comparisonVisible.value = true
}
const percent = (value: number) => `${(Number(value || 0) * 100).toFixed(1)}%`
const breakdownText = (value?: Record<string, number>) => {
  if (!value || !Object.keys(value).length) return '暂无分项'
  return Object.entries(value)
    .slice(0, 4)
    .map(([key, count]) => `${key} ${count}`)
    .join(' · ')
}
const saveMapping = async () => {
  if (!mappingUserId.value) return message.warning('请选择底座用户')
  mappingSaving.value = true
  try {
    await saveCVATUserMapping({ platformUserId: mappingUserId.value })
    message.success('个人 CVAT 身份已同步')
    await loadMappings()
  } finally {
    mappingSaving.value = false
  }
}
onMounted(async () => {
  const [data, userRows] = await Promise.all([
    getProjectPage({ pageNo: 1, pageSize: 100 }),
    getSimpleUserList()
  ])
  users.value = userRows
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.annotation-center {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--el-bg-color-page);
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 5px 0;
  font-size: 30px;
  letter-spacing: -1px;
}

.page-header p {
  margin: 0;
  color: var(--text-secondary);
}

.eyebrow {
  font-size: 12px;
  font-weight: 750;
  letter-spacing: 1.8px;
  color: var(--el-color-primary);
}

.actions,
.toolbar,
.mapping-form {
  display: flex;
  gap: 12px;
  align-items: center;
}

.toolbar {
  margin-bottom: 18px;
}

.toolbar .el-select {
  width: 250px;
}

.metric-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(130px, 1fr));
  gap: 14px;
  margin-bottom: 18px;
}

.metric-strip article {
  padding: 17px 20px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
}

.metric-strip small {
  display: block;
  color: var(--text-secondary);
}

.metric-strip strong {
  display: block;
  margin-top: 5px;
  font-size: 25px;
}

.task-list {
  display: grid;
  gap: 14px;
}

.task-card {
  display: grid;
  padding: 22px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  box-shadow: 0 8px 28px rgb(15 23 42 / 4%);
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 24px;
}

.task-title {
  display: flex;
  font-size: 13px;
  color: var(--text-secondary);
  align-items: center;
  gap: 10px;
}

.task-card h2 {
  margin: 12px 0 5px;
  font-size: 20px;
}

.task-card p {
  color: var(--text-secondary);
}

.task-actions {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
}

.task-actions .el-button {
  margin-left: 0;
}

.error {
  color: var(--el-color-danger) !important;
}

.warning {
  color: var(--el-color-warning) !important;
}

.empty-state {
  display: flex;
  min-height: 260px;
  color: var(--text-secondary);
  border: 1px dashed var(--el-border-color);
  border-radius: 14px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.full-width {
  width: 100%;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.mapping-form > * {
  flex: 1;
}

.identity-tip {
  margin-bottom: 16px;
}

.identity-user-select {
  min-width: 320px;
}

.preannotation-form {
  margin-top: 18px;
}

.preannotation-evidence {
  padding: 14px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.preannotation-evidence > div {
  display: flex;
  align-items: center;
  gap: 10px;
}

.preannotation-evidence p {
  margin: 8px 0;
  color: var(--text-secondary);
}

code {
  overflow-wrap: anywhere;
}

@media (width <= 900px) {
  .page-header,
  .toolbar,
  .actions {
    flex-wrap: wrap;
  }

  .metric-strip {
    grid-template-columns: repeat(2, 1fr);
  }

  .task-card {
    grid-template-columns: 1fr;
  }
}
</style>

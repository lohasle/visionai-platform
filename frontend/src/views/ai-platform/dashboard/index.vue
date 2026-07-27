<template>
  <main class="vision-dashboard">
    <header class="page-header">
      <div>
        <span class="eyebrow">VISIONAI CONTROL PLANE</span>
        <h1>计算机视觉生命周期工作台</h1>
        <p>从数据准备到生产反馈，集中查看需要你处理的事项与平台运行状态。</p>
      </div>
      <div class="page-header__actions">
        <el-select
          v-model="selectedProjectId"
          class="scope-select"
          placeholder="全部可访问项目"
          clearable
          @change="loadSummary"
        >
          <el-option
            v-for="project in summary.scope.projects"
            :key="project.id"
            :label="`${project.name} · ${project.code}`"
            :value="project.id"
          />
        </el-select>
        <el-button :loading="loading" @click="loadSummary">
          <Icon icon="lucide:refresh-cw" :size="16" />
          刷新
        </el-button>
        <el-button type="primary" @click="router.push('/ai-platform/projects')">
          <Icon icon="lucide:plus" :size="16" />
          新建项目
        </el-button>
      </div>
    </header>

    <el-alert
      v-if="errorMessage"
      class="dashboard-alert"
      type="error"
      :title="errorMessage"
      show-icon
      :closable="false"
    />

    <section class="metrics" aria-label="平台指标">
      <article v-for="metric in metrics" :key="metric.label" class="metric-card">
        <span class="metric-card__icon" :class="metric.tone">
          <Icon :icon="metric.icon" :size="20" />
        </span>
        <div>
          <small>{{ metric.label }}</small>
          <strong>{{ metric.value }}</strong>
          <p>{{ metric.note }}</p>
        </div>
      </article>
    </section>

    <section class="panel lifecycle-panel">
      <header class="panel__header">
        <div>
          <h2>生命周期总览</h2>
          <p>数量来自当前项目范围；阻塞项优先显示，点击阶段进入对应列表。</p>
        </div>
        <span class="scope-note">{{ selectedProjectName }}</span>
      </header>
      <div v-if="loading" class="skeleton-list">
        <el-skeleton v-for="item in 5" :key="item" animated :rows="1" />
      </div>
      <ol v-else class="pipeline">
        <li
          v-for="(stage, index) in summary.lifecycle"
          :key="stage.key"
          tabindex="0"
          @click="router.push(stage.route)"
          @keyup.enter="router.push(stage.route)"
        >
          <span class="pipeline__index">{{ index + 1 }}</span>
          <div>
            <strong>{{ stage.label }}</strong>
            <small>{{ stage.count }} 个业务对象</small>
          </div>
          <span v-if="stage.blocked" class="blocked">{{ stage.blocked }} 阻塞</span>
          <Icon icon="lucide:chevron-right" :size="17" />
        </li>
      </ol>
    </section>

    <section class="dashboard-grid dashboard-grid--primary">
      <article class="panel">
        <header class="panel__header">
          <div>
            <h2>我的待办</h2>
            <p>仅显示未完成的标注、审核、审批、失败任务和生产告警。</p>
          </div>
          <span class="count-badge">{{ summary.todos.length }}</span>
        </header>
        <el-empty v-if="!loading && !summary.todos.length" description="当前没有待处理事项" />
        <ul v-else class="todo-list">
          <li v-for="todo in summary.todos" :key="todo.key" @click="router.push(todo.route)">
            <span class="todo-icon" :class="todo.priority.toLowerCase()">
              <Icon :icon="todoIcon(todo.kind)" :size="17" />
            </span>
            <div>
              <strong>{{ todo.title }}</strong>
              <small>{{ todo.reason }}</small>
            </div>
            <span class="todo-kind">{{ todoKind(todo.kind) }}</span>
            <Icon icon="lucide:arrow-up-right" :size="15" />
          </li>
        </ul>
      </article>

      <article class="panel">
        <header class="panel__header">
          <div>
            <h2>资源状态</h2>
            <p>
              {{
                summary.resources.visibility === 'DETAIL'
                  ? '运维角色可查看计算节点实时状态。'
                  : '按权限仅展示当前项目配额和用量。'
              }}
            </p>
          </div>
          <span class="scope-note">{{ summary.resources.gpuHours.toFixed(2) }} GPUh</span>
        </header>
        <div v-if="summary.resources.visibility === 'DETAIL'" class="resource-list">
          <div v-for="node in summary.resources.nodes || []" :key="node.id" class="resource-node">
            <div class="resource-node__title">
              <span>
                <strong>{{ node.name }}</strong>
                <small>{{ node.gpuModel || 'CPU 节点' }}</small>
              </span>
              <span class="status-dot" :class="serviceTone(node.status)">{{ node.status }}</span>
            </div>
            <label
              >GPU 利用率 <b>{{ node.gpuUtilization.toFixed(0) }}%</b></label
            >
            <el-progress :percentage="Math.min(100, node.gpuUtilization)" :stroke-width="6" />
            <label
              >显存
              <b
                >{{ formatBytes(node.gpuUsedBytes) }} / {{ formatBytes(node.gpuMemoryBytes) }}</b
              ></label
            >
            <el-progress
              :percentage="percentage(node.gpuUsedBytes, node.gpuMemoryBytes)"
              :stroke-width="6"
              color="#7c3aed"
            />
          </div>
        </div>
        <div class="quota-summary">
          <div>
            <small>对象存储用量</small>
            <strong>{{ formatBytes(summary.resources.storageBytes) }}</strong>
          </div>
          <div>
            <small>项目配额记录</small>
            <strong>{{ summary.resources.quotas.length }}</strong>
          </div>
        </div>
      </article>
    </section>

    <section class="panel jobs-panel">
      <header class="panel__header">
        <div>
          <h2>最近任务</h2>
          <p>展示执行阶段、进度、耗时、可读失败原因和当前可用操作。</p>
        </div>
        <el-button text @click="router.push('/ai-platform/operations')">查看全部</el-button>
      </header>
      <el-table :data="summary.recentJobs" stripe empty-text="当前范围没有任务">
        <el-table-column prop="id" label="Job" width="82">
          <template #default="{ row }">#{{ row.id }}</template>
        </el-table-column>
        <el-table-column prop="jobType" label="类型" min-width="160" show-overflow-tooltip />
        <el-table-column label="状态 / 阶段" min-width="190">
          <template #default="{ row }">
            <div class="job-state">
              <span class="status-dot" :class="serviceTone(row.status)">{{ row.status }}</span>
              <small>{{ row.stage || '等待调度' }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="160">
          <template #default="{ row }">
            <el-progress :percentage="row.progress" :stroke-width="6" />
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="100">
          <template #default="{ row }">{{ formatDuration(row.durationSeconds) }}</template>
        </el-table-column>
        <el-table-column label="失败原因" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="{ 'error-copy': row.errorMessage }">{{
              row.errorMessage || row.remediation || '—'
            }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right" width="190">
          <template #default="{ row }">
            <el-button v-if="row.actions.includes('VIEW')" link @click="router.push(row.route)"
              >查看</el-button
            >
            <el-button
              v-if="row.actions.includes('CANCEL')"
              link
              type="danger"
              @click="handleJob(row, 'CANCEL')"
              >取消</el-button
            >
            <el-button
              v-if="row.actions.includes('RETRY')"
              link
              type="primary"
              @click="handleJob(row, 'RETRY')"
              >重试</el-button
            >
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="dashboard-grid">
      <article class="panel">
        <header class="panel__header">
          <div>
            <h2>服务健康</h2>
            <p>数据库、对象存储、本地 GPU Runner 与外部工作台连接状态。</p>
          </div>
        </header>
        <ul class="health-list">
          <li v-for="service in summary.services" :key="`${service.provider}-${service.name}`">
            <span class="service-icon"
              ><Icon :icon="serviceIcon(service.provider)" :size="18"
            /></span>
            <div>
              <strong>{{ service.name }}</strong>
              <small>{{ service.detail || '尚无版本信息' }}</small>
            </div>
            <span class="status-dot" :class="serviceTone(service.status)">{{
              service.status
            }}</span>
          </li>
        </ul>
      </article>

      <article class="panel">
        <header class="panel__header">
          <div>
            <h2>关键活动</h2>
            <p>只保留业务变更事件，过滤读取、打开页面等低价值噪声。</p>
          </div>
        </header>
        <el-empty v-if="!summary.activities.length" description="暂无关键活动" />
        <ol v-else class="activity-list">
          <li v-for="activity in summary.activities.slice(0, 12)" :key="activity.id">
            <span class="activity-mark"></span>
            <div>
              <strong>{{ actionLabel(activity.action) }}</strong>
              <small
                >{{ activity.resourceType }} #{{ activity.resourceId }} · 用户 #{{
                  activity.actorUserId
                }}</small
              >
            </div>
            <time>{{ formatTime(activity.createTime) }}</time>
          </li>
        </ol>
      </article>
    </section>
  </main>
</template>

<script lang="ts" setup>
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  cancelDashboardJob,
  getDashboardSummary,
  retryDashboardJob,
  type DashboardJob,
  type DashboardSummary,
  type DashboardTodo
} from '@/api/ai-platform/dashboard'

defineOptions({ name: 'VisionAIDashboard' })

const router = useRouter()
const loading = ref(false)
const errorMessage = ref('')
const selectedProjectId = ref<number>()
const emptySummary = (): DashboardSummary => ({
  scope: { projectId: 0, projects: [] },
  metrics: {
    projects: 0,
    datasetVersions: 0,
    activeJobs: 0,
    models: 0,
    deployments: 0,
    gpuHours: 0
  },
  jobs: { running: 0, failed: 0, pending: 0 },
  lifecycle: [],
  todos: [],
  recentJobs: [],
  services: [],
  activities: [],
  resources: { visibility: 'QUOTA_ONLY', quotas: [], storageBytes: 0, gpuHours: 0 }
})
const summary = reactive<DashboardSummary>(emptySummary())

const selectedProjectName = computed(
  () =>
    (() => {
      const project = summary.scope.projects.find(
        (project) => project.id === selectedProjectId.value
      )
      return project ? `${project.name} · ${project.code}` : ''
    })() || '全部可访问项目'
)

const metrics = computed(() => [
  {
    label: '项目',
    value: summary.metrics.projects,
    note: '当前可访问范围',
    icon: 'lucide:folder-kanban',
    tone: 'primary'
  },
  {
    label: '冻结数据集',
    value: summary.metrics.datasetVersions,
    note: '可复现训练输入',
    icon: 'lucide:database',
    tone: 'violet'
  },
  {
    label: '运行中任务',
    value: summary.metrics.activeJobs,
    note: `${summary.jobs.pending} 个等待调度`,
    icon: 'lucide:activity',
    tone: 'warning'
  },
  {
    label: '模型版本',
    value: summary.metrics.models,
    note: '含审批前版本',
    icon: 'lucide:box',
    tone: 'primary'
  },
  {
    label: '在线部署',
    value: summary.metrics.deployments,
    note: '当前运行中端点',
    icon: 'lucide:rocket',
    tone: 'success'
  },
  {
    label: 'GPU 用量',
    value: summary.metrics.gpuHours.toFixed(2),
    note: '累计 GPU 小时',
    icon: 'lucide:cpu',
    tone: 'violet'
  }
])

const loadSummary = async () => {
  loading.value = true
  errorMessage.value = ''
  try {
    const projectOptions = summary.scope.projects
    const data = await getDashboardSummary(selectedProjectId.value)
    Object.assign(summary, emptySummary(), data)
    if (!summary.scope.projects.length && projectOptions.length)
      summary.scope.projects = projectOptions
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载工作台数据'
  } finally {
    loading.value = false
  }
}

const handleJob = async (job: DashboardJob, action: 'CANCEL' | 'RETRY') => {
  if (action === 'CANCEL') {
    await ElMessageBox.confirm(`确认取消 Job #${job.id}？`, '取消任务', { type: 'warning' })
    await cancelDashboardJob(job.id)
    ElMessage.success('已提交取消请求')
  } else {
    await retryDashboardJob(job.id)
    ElMessage.success('任务已重新入队')
  }
  await loadSummary()
}

const todoIcon = (kind: DashboardTodo['kind']) =>
  ({
    ANNOTATION: 'lucide:pen-tool',
    REVIEW: 'lucide:scan-search',
    APPROVAL: 'lucide:badge-check',
    FAILED_JOB: 'lucide:triangle-alert',
    ALERT: 'lucide:siren'
  })[kind]

const todoKind = (kind: DashboardTodo['kind']) =>
  ({
    ANNOTATION: '标注',
    REVIEW: '审核',
    APPROVAL: '审批',
    FAILED_JOB: '失败任务',
    ALERT: '告警'
  })[kind]

const serviceIcon = (provider: string) =>
  ({
    DATABASE: 'lucide:database',
    MINIO: 'lucide:hard-drive',
    LOCAL_DOCKER: 'lucide:cpu',
    CVAT: 'lucide:pen-tool',
    FIFTYONE: 'lucide:gallery-horizontal',
    CLEARML: 'lucide:brain-circuit'
  })[provider] || 'lucide:server'

const serviceTone = (status: string) => {
  const value = status.toUpperCase()
  if (['UP', 'RUNNING', 'SUCCEEDED', 'ONLINE', 'HEALTHY'].includes(value)) return 'success'
  if (['FAILED', 'DOWN', 'OFFLINE', 'ERROR'].includes(value)) return 'danger'
  if (['QUEUED', 'PENDING', 'DEGRADED', 'DEPLOYING'].includes(value)) return 'warning'
  return 'neutral'
}

const percentage = (used: number, total: number) =>
  total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  return `${(bytes / 1024 ** index).toFixed(index > 1 ? 1 : 0)} ${units[index]}`
}

const formatDuration = (seconds: number) => {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
}

const formatTime = (value: string) =>
  new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))

const actionLabel = (action: string) =>
  action
    .split('_')
    .filter(Boolean)
    .map((part) => part.toLowerCase())
    .join(' · ')

onMounted(loadSummary)
</script>

<style scoped lang="scss">
.vision-dashboard {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.page-header,
.panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-5);
}

.page-header {
  min-height: 76px;
  align-items: center;

  h1 {
    margin: var(--space-2) 0 var(--space-1);
    font-size: 24px;
    line-height: 32px;
  }

  p {
    color: var(--text-tertiary);
  }

  &__actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
}

.scope-select {
  width: 210px;
}

.eyebrow {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  color: var(--primary);
}

.dashboard-alert {
  margin-top: var(--space-4);
}

.metrics {
  display: grid;
  margin: var(--space-6) 0;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: var(--space-3);
}

.metric-card,
.panel {
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
}

.metric-card {
  display: flex;
  min-width: 0;
  padding: var(--space-4);
  gap: var(--space-3);

  &__icon,
  .service-icon,
  .todo-icon {
    display: grid;
    flex: none;
    place-items: center;
  }

  &__icon {
    width: 38px;
    height: 38px;
    color: var(--primary);
    background: var(--primary-soft);
    border-radius: var(--radius-md);

    &.warning {
      color: var(--warning);
      background: var(--warning-soft);
    }

    &.success {
      color: var(--success);
      background: var(--success-soft);
    }

    &.violet {
      color: #7c3aed;
      background: #f3e8ff;
    }
  }

  div {
    display: flex;
    min-width: 0;
    flex-direction: column;
  }

  small,
  p {
    font-size: 11px;
    color: var(--text-tertiary);
  }

  strong {
    margin: 1px 0;
    font-size: 21px;
    font-variant-numeric: tabular-nums;
  }
}

.panel {
  padding: var(--space-5);

  &__header {
    margin-bottom: var(--space-4);

    h2 {
      margin: 0 0 var(--space-1);
      font-size: 16px;
    }

    p {
      font-size: 12px;
      color: var(--text-tertiary);
    }
  }
}

.lifecycle-panel,
.jobs-panel {
  margin-bottom: var(--space-6);
}

.scope-note,
.count-badge {
  padding: 4px 9px;
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--bg-subtle);
  border-radius: var(--radius-sm);
}

.pipeline,
.todo-list,
.health-list,
.activity-list {
  padding: 0;
  margin: 0;
  list-style: none;
}

.pipeline {
  display: grid;
  grid-template-columns: repeat(5, minmax(140px, 1fr));

  li {
    display: flex;
    min-width: 0;
    padding: var(--space-4);
    cursor: pointer;
    align-items: center;
    gap: var(--space-3);
    border: 1px solid var(--divider);
    border-right: 0;
    transition:
      border-color 0.16s ease,
      background-color 0.16s ease;

    &:hover,
    &:focus-visible {
      background: var(--bg-subtle);
      border-color: var(--primary);
      outline: none;
    }

    &:last-child {
      border-right: 1px solid var(--divider);
    }

    > svg {
      margin-left: auto;
      color: var(--text-tertiary);
    }

    div {
      display: flex;
      min-width: 0;
      flex-direction: column;
    }

    strong {
      font-size: 13px;
    }

    small {
      font-size: 11px;
      color: var(--text-tertiary);
    }
  }

  &__index {
    display: grid;
    width: 26px;
    height: 26px;
    flex: none;
    color: var(--primary);
    background: var(--primary-soft);
    border-radius: 50%;
    place-items: center;
  }
}

.blocked {
  padding: 2px 5px;
  font-size: 10px;
  color: var(--danger);
  background: var(--danger-soft);
  border-radius: var(--radius-sm);
}

.dashboard-grid {
  display: grid;
  margin-bottom: var(--space-6);
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-6);

  &--primary {
    grid-template-columns: minmax(0, 1.35fr) minmax(340px, 0.65fr);
  }
}

.todo-list,
.health-list {
  max-height: 360px;
  overflow: auto;
}

.todo-list li,
.health-list li {
  display: grid;
  min-height: 58px;
  border-bottom: 1px solid var(--divider);
  gap: var(--space-3);
  align-items: center;

  &:last-child {
    border-bottom: 0;
  }

  div {
    display: flex;
    min-width: 0;
    flex-direction: column;
  }

  strong {
    font-size: 13px;
  }

  small {
    overflow: hidden;
    font-size: 11px;
    color: var(--text-tertiary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.todo-list li {
  cursor: pointer;
  grid-template-columns: 34px minmax(0, 1fr) auto 18px;

  &:hover {
    background: var(--bg-subtle);
  }
}

.todo-icon,
.service-icon {
  width: 32px;
  height: 32px;
  color: var(--primary);
  background: var(--primary-soft);
  border-radius: var(--radius-md);
}

.todo-icon.high {
  color: var(--warning);
  background: var(--warning-soft);
}

.todo-icon.urgent {
  color: var(--danger);
  background: var(--danger-soft);
}

.todo-kind {
  font-size: 11px;
  color: var(--text-secondary);
}

.health-list li {
  grid-template-columns: 36px minmax(0, 1fr) auto;
}

.status-dot {
  display: inline-flex;
  padding: 3px 7px;
  font-size: 10px;
  font-weight: 600;
  border-radius: var(--radius-sm);

  &.success {
    color: var(--success);
    background: var(--success-soft);
  }

  &.danger {
    color: var(--danger);
    background: var(--danger-soft);
  }

  &.warning {
    color: var(--warning);
    background: var(--warning-soft);
  }

  &.neutral {
    color: var(--text-secondary);
    background: var(--bg-subtle);
  }
}

.resource-list {
  display: grid;
  gap: var(--space-4);
}

.resource-node {
  padding: var(--space-4);
  background: var(--bg-subtle);
  border: 1px solid var(--divider);
  border-radius: var(--radius-md);

  &__title {
    display: flex;
    margin-bottom: var(--space-3);
    align-items: flex-start;
    justify-content: space-between;

    > span:first-child {
      display: flex;
      flex-direction: column;
    }

    small {
      font-size: 11px;
      color: var(--text-tertiary);
    }
  }

  label {
    display: flex;
    margin: var(--space-2) 0 3px;
    font-size: 11px;
    color: var(--text-tertiary);
    justify-content: space-between;
  }
}

.quota-summary {
  display: grid;
  margin-top: var(--space-4);
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-3);

  div {
    display: flex;
    padding: var(--space-3);
    flex-direction: column;
    background: var(--bg-subtle);
    border-radius: var(--radius-md);
  }

  small {
    font-size: 11px;
    color: var(--text-tertiary);
  }
}

.job-state {
  display: flex;
  align-items: center;
  gap: var(--space-2);

  small {
    color: var(--text-tertiary);
  }
}

.error-copy {
  color: var(--danger);
}

.activity-list {
  max-height: 360px;
  overflow: auto;

  li {
    display: grid;
    min-height: 48px;
    grid-template-columns: 10px minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: center;
  }

  div {
    display: flex;
    min-width: 0;
    flex-direction: column;
  }

  strong {
    font-size: 12px;
  }

  small,
  time {
    font-size: 10px;
    color: var(--text-tertiary);
  }
}

.activity-mark {
  width: 7px;
  height: 7px;
  background: var(--primary);
  border-radius: 50%;
}

.skeleton-list {
  display: grid;
  gap: var(--space-3);
}

@media (width <= 1320px) {
  .metrics {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (width <= 1000px) {
  .page-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .dashboard-grid,
  .dashboard-grid--primary {
    grid-template-columns: 1fr;
  }

  .pipeline {
    grid-template-columns: 1fr;
  }

  .pipeline li {
    border-right: 1px solid var(--divider);
    border-bottom: 0;
  }

  .pipeline li:last-child {
    border-bottom: 1px solid var(--divider);
  }
}

@media (width <= 720px) {
  .page-header__actions {
    width: 100%;
    flex-wrap: wrap;
  }

  .scope-select {
    width: 100%;
  }

  .metrics {
    grid-template-columns: 1fr;
  }
}
</style>

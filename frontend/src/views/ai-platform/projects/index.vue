<template>
  <main class="project-center">
    <header class="page-header">
      <div>
        <span class="eyebrow">GOVERNED WORKSPACES</span>
        <h1>项目中心</h1>
        <p>按租户与成员边界管理视觉项目、Provider 配置和生命周期。</p>
      </div>
      <el-button type="primary" @click="openCreate()">
        <Icon icon="lucide:plus" :size="16" />
        新建项目
      </el-button>
    </header>

    <section class="toolbar">
      <el-input
        v-model="query.keyword"
        clearable
        placeholder="搜索项目名称或编码"
        @keyup.enter="loadProjects"
      >
        <template #prefix><Icon icon="lucide:search" :size="16" /></template>
      </el-input>
      <el-select v-model="query.status" clearable placeholder="全部状态">
        <el-option v-for="item in statusOptions" :key="item.value" v-bind="item" />
      </el-select>
      <el-button :loading="loading" @click="loadProjects">查询</el-button>
    </section>

    <el-alert v-if="errorMessage" type="error" :title="errorMessage" :closable="false" show-icon />

    <section v-loading="loading" class="project-grid" aria-live="polite">
      <article
        v-for="project in projects"
        :key="project.id"
        class="project-card"
        @click="openProject(project)"
      >
        <header>
          <span class="project-icon"><Icon icon="lucide:scan-line" :size="19" /></span>
          <el-tag :type="statusMeta[project.status].type" effect="light">
            {{ statusMeta[project.status].label }}
          </el-tag>
        </header>
        <div>
          <small>{{ project.code }}</small>
          <h2>{{ project.name }}</h2>
          <p>{{ project.description || '暂无项目说明' }}</p>
        </div>
        <footer>
          <span><Icon icon="lucide:user-round" :size="14" />负责人 #{{ project.ownerUserId }}</span>
          <span>更新于 {{ formatDate(project.updateTime) }}</span>
        </footer>
      </article>
      <button v-if="!loading && !projects.length" class="empty-card" @click="openCreate()">
        <Icon icon="lucide:folder-plus" :size="28" />
        <strong>创建第一个视觉项目</strong>
        <span>从数据准备开始建立可追溯的模型生命周期</span>
      </button>
    </section>

    <el-pagination
      v-if="total > query.pageSize"
      v-model:current-page="query.pageNo"
      :page-size="query.pageSize"
      :total="total"
      layout="prev, pager, next"
      @current-change="loadProjects"
    />

    <el-dialog
      v-model="createVisible"
      :title="cloneSource ? '复制项目配置' : '创建项目'"
      width="520"
    >
      <el-form label-position="top">
        <el-form-item label="项目编码" required>
          <el-input
            v-model="projectForm.code"
            maxlength="64"
            placeholder="例如 defect-inspection"
          />
        </el-form-item>
        <el-form-item label="项目名称" required>
          <el-input v-model="projectForm.name" maxlength="128" />
        </el-form-item>
        <el-form-item label="项目说明">
          <el-input v-model="projectForm.description" type="textarea" :rows="3" maxlength="1024" />
        </el-form-item>
        <template v-if="!cloneSource">
          <div class="form-grid">
            <el-form-item label="AI 领域" required>
              <el-select v-model="projectForm.aiDomain" class="full-width">
                <el-option label="计算机视觉" value="CV" />
              </el-select>
            </el-form-item>
            <el-form-item label="任务类型" required>
              <el-select v-model="projectForm.taskType" class="full-width">
                <el-option label="目标检测" value="CV_DETECTION" />
                <el-option label="实例分割" value="CV_SEGMENTATION" />
                <el-option label="关键点" value="CV_KEYPOINT" />
                <el-option label="视频跟踪" value="CV_TRACKING" />
              </el-select>
            </el-form-item>
          </div>
          <el-form-item label="项目负责人" required>
            <el-select
              v-model="projectForm.ownerUserId"
              filterable
              class="full-width"
              placeholder="选择租户用户"
            >
              <el-option
                v-for="user in users.filter((item) => item.status === 0)"
                :key="user.id"
                :label="`${user.nickname || user.username} · ${user.username}`"
                :value="user.id"
              />
            </el-select>
          </el-form-item>
          <div class="form-grid">
            <el-form-item label="默认训练 Provider" required>
              <el-select v-model="projectForm.defaultProvider" class="full-width">
                <el-option label="本机 Docker" value="LOCAL_DOCKER" />
                <el-option label="ClearML" value="CLEARML" />
              </el-select>
            </el-form-item>
            <el-form-item label="对象存储空间" required>
              <el-input v-model.trim="projectForm.storageBucket" placeholder="visionai-assets" />
            </el-form-item>
          </div>
          <div class="form-grid quota-grid">
            <el-form-item label="最大并发任务">
              <el-input-number v-model="projectForm.maxConcurrentJobs" :min="1" :max="128" />
            </el-form-item>
            <el-form-item label="月度 GPU 小时">
              <el-input-number v-model="projectForm.monthlyGpuHours" :min="1" :max="100000" />
            </el-form-item>
          </div>
          <el-form-item label="存储配额（GiB）">
            <el-input-number
              :model-value="Math.round(projectForm.storageBytes / 1073741824)"
              :min="1"
              :max="1048576"
              @update:model-value="projectForm.storageBytes = Number($event) * 1073741824"
            />
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitProject">
          {{ cloneSource ? '创建副本' : '创建草稿' }}
        </el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="drawerVisible" size="720" destroy-on-close>
      <template #header>
        <div v-if="selected" class="drawer-title">
          <span class="project-icon"><Icon icon="lucide:scan-line" :size="19" /></span>
          <div
            ><small>{{ selected.code }}</small
            ><strong>{{ selected.name }}</strong></div
          >
          <el-tag :type="statusMeta[selected.status].type">
            {{ statusMeta[selected.status].label }}
          </el-tag>
        </div>
      </template>
      <template v-if="selected">
        <div class="lifecycle-actions">
          <el-button
            v-if="selected.status === 'DRAFT' || selected.status === 'SUSPENDED'"
            type="primary"
            @click="changeStatus('ACTIVE')"
          >
            {{ selected.status === 'DRAFT' ? '激活项目' : '恢复项目' }}
          </el-button>
          <el-button v-if="selected.status === 'ACTIVE'" @click="changeStatus('SUSPENDED')">
            暂停项目
          </el-button>
          <el-button :disabled="selected.status === 'ARCHIVED'" @click="openCreate(selected)">
            复制配置
          </el-button>
          <el-button
            type="danger"
            plain
            :disabled="selected.status === 'ARCHIVED'"
            @click="doArchive"
          >
            归档
          </el-button>
        </div>
        <el-tabs v-model="activeTab" @tab-change="loadTab">
          <el-tab-pane label="概览" name="overview">
            <div v-loading="tabLoading" class="project-overview">
              <dl class="overview-list">
                <div
                  ><dt>项目编码</dt><dd>{{ selected.code }}</dd></div
                >
                <div
                  ><dt>AI / 任务类型</dt
                  ><dd>{{ selected.aiDomain }} · {{ selected.taskType }}</dd></div
                >
                <div
                  ><dt>负责人</dt><dd>#{{ selected.ownerUserId }}</dd></div
                >
                <div
                  ><dt>默认 Provider</dt><dd>{{ selected.defaultProvider }}</dd></div
                >
                <div
                  ><dt>对象存储空间</dt><dd>{{ selected.storageBucket }}</dd></div
                >
                <div
                  ><dt>创建时间</dt><dd>{{ formatDate(selected.createTime) }}</dd></div
                >
                <div
                  ><dt>项目说明</dt><dd>{{ selected.description || '暂无' }}</dd></div
                >
              </dl>
              <section v-if="overview" class="overview-metrics" aria-label="生命周期资产统计">
                <article v-for="item in overviewMetricCards" :key="item.label">
                  <span>{{ item.label }}</span>
                  <strong>{{ item.value }}</strong>
                </article>
              </section>
              <section v-if="overview" class="overview-section">
                <header>
                  <div><h3>项目风险</h3><p>失败任务、生产告警和待审批事项。</p></div>
                  <el-tag :type="overview.risks.length ? 'danger' : 'success'">
                    {{ overview.risks.length ? `${overview.risks.length} 项` : '无未处理风险' }}
                  </el-tag>
                </header>
                <button
                  v-for="risk in overview.risks"
                  :key="risk.code"
                  type="button"
                  class="risk-row"
                  @click="router.push(risk.route)"
                >
                  <el-tag :type="risk.severity === 'HIGH' ? 'danger' : 'warning'">
                    {{ risk.severity }}
                  </el-tag>
                  <span>{{ risk.message }}</span>
                  <strong>{{ risk.count }}</strong>
                </button>
                <p v-if="!overview.risks.length" class="empty-copy"
                  >当前没有未处理的生命周期风险。</p
                >
              </section>
              <section v-if="overview" class="overview-section">
                <header>
                  <div><h3>业务时间线</h3><p>仅展示项目级关键业务变更。</p></div>
                </header>
                <ol class="timeline-list">
                  <li v-for="event in overview.timeline.slice(0, 12)" :key="event.id">
                    <span class="timeline-dot"></span>
                    <div>
                      <strong>{{ event.action }}</strong>
                      <p
                        >{{ event.resourceType }} #{{ event.resourceId }} · 用户 #{{
                          event.actorUserId
                        }}</p
                      >
                    </div>
                    <time>{{ formatDate(event.createTime) }}</time>
                  </li>
                </ol>
              </section>
            </div>
          </el-tab-pane>
          <el-tab-pane label="项目成员" name="members">
            <div class="tab-header">
              <div>
                <p>项目只维护成员范围，权限实时继承系统角色。</p>
                <el-button link type="primary" @click="router.push('/system/role')">
                  前往系统角色管理
                </el-button>
              </div>
              <el-button type="primary" plain @click="openMemberDialog">添加成员</el-button>
            </div>
            <el-table :data="members" v-loading="tabLoading">
              <el-table-column label="成员" min-width="180">
                <template #default="{ row }">
                  <strong>{{ row.nickname || row.username }}</strong>
                  <small class="member-account">{{ row.username }} · #{{ row.userId }}</small>
                </template>
              </el-table-column>
              <el-table-column label="系统角色" min-width="220">
                <template #default="{ row }">
                  <el-tag
                    v-for="role in row.roles"
                    :key="role.id"
                    class="role-tag"
                    :type="role.status === 0 ? 'primary' : 'info'"
                  >
                    {{ role.name }}
                  </el-tag>
                  <span v-if="!row.roles.length" class="empty-role">未分配系统角色</span>
                </template>
              </el-table-column>
              <el-table-column width="90" align="right">
                <template #default="{ row }">
                  <el-button link type="danger" @click="deleteMember(row.userId)">移除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
          <el-tab-pane label="Provider 配置" name="config">
            <el-alert
              title="凭据不会明文入库；请填写外部 Secret 引用。"
              type="info"
              :closable="false"
              show-icon
            />
            <el-form class="config-form" label-position="top" v-loading="tabLoading">
              <div class="form-grid">
                <el-form-item label="CVAT 实例">
                  <el-select v-model="configForm.cvatInstanceId" clearable class="full-width">
                    <el-option
                      v-for="item in providerInstances('CVAT')"
                      :key="item.id"
                      :label="`${item.name} · ${item.status}`"
                      :value="item.id"
                    />
                  </el-select>
                </el-form-item>
                <el-form-item label="ClearML 实例">
                  <el-select v-model="configForm.clearmlInstanceId" clearable class="full-width">
                    <el-option
                      v-for="item in providerInstances('CLEARML')"
                      :key="item.id"
                      :label="`${item.name} · ${item.status}`"
                      :value="item.id"
                    />
                  </el-select>
                </el-form-item>
              </div>
              <div class="form-grid">
                <el-form-item label="FiftyOne 实例">
                  <el-select v-model="configForm.fiftyOneInstanceId" clearable class="full-width">
                    <el-option
                      v-for="item in providerInstances('FIFTYONE')"
                      :key="item.id"
                      :label="`${item.name} · ${item.status}`"
                      :value="item.id"
                    />
                  </el-select>
                </el-form-item>
                <el-form-item label="默认训练队列">
                  <el-input v-model.trim="configForm.defaultQueue" placeholder="gpu-local" />
                </el-form-item>
              </div>
              <el-form-item label="对象存储 Endpoint">
                <el-input
                  v-model="configForm.storageEndpoint"
                  placeholder="https://s3.example.com"
                />
              </el-form-item>
              <el-form-item label="默认 Bucket">
                <el-input v-model="configForm.bucket" />
              </el-form-item>
              <el-form-item label="训练 Provider 地址">
                <el-input v-model="configForm.trainingEndpoint" />
              </el-form-item>
              <el-form-item label="凭据 Secret 引用">
                <el-input
                  v-model="configForm.credentialRef"
                  placeholder="vault://visionai/provider"
                />
              </el-form-item>
              <el-form-item label="变更原因" required>
                <el-input
                  v-model.trim="configForm.changeReason"
                  type="textarea"
                  :rows="2"
                  placeholder="说明本次 Provider、对象存储或队列配置变更的原因"
                />
              </el-form-item>
              <div class="config-actions">
                <el-button :loading="saving" @click="testSelectedProviders">测试所选连接</el-button>
                <el-button type="primary" :loading="saving" @click="saveConfig">保存配置</el-button>
              </div>
            </el-form>
          </el-tab-pane>
        </el-tabs>
      </template>
    </el-drawer>

    <el-dialog v-model="memberVisible" title="添加项目成员" width="520">
      <el-alert
        title="角色请在“系统管理 → 用户管理 → 分配角色”中统一设置"
        type="info"
        :closable="false"
        show-icon
      />
      <el-form label-position="top">
        <el-form-item label="租户用户" required class="member-user-field">
          <el-select
            v-model="memberForm.userId"
            filterable
            class="full-width"
            placeholder="选择系统用户"
          >
            <el-option
              v-for="user in availableMemberUsers"
              :key="user.id"
              :label="`${user.nickname || user.username} · ${user.username}`"
              :value="user.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="memberVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveMember">保存</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<script lang="ts" setup>
import {
  archiveProject,
  cloneProject,
  createProject,
  getProjectConfig,
  getProjectMembers,
  getProjectOverview,
  getProjectPage,
  removeProjectMember,
  updateProjectConfig,
  updateProjectStatus,
  upsertProjectMember,
  type Project,
  type ProjectOverview,
  type ProjectMember,
  type ProjectStatus
} from '@/api/ai-platform/projects'
import { getIntegrations, testIntegration } from '@/api/ai-platform/operations'
import { getSimpleUserList, type UserVO } from '@/api/system/user'

defineOptions({ name: 'VisionAIProjects' })

const message = useMessage()
const router = useRouter()
const statusOptions = [
  { label: '草稿', value: 'DRAFT' },
  { label: '运行中', value: 'ACTIVE' },
  { label: '已暂停', value: 'SUSPENDED' },
  { label: '已归档', value: 'ARCHIVED' }
]
const statusMeta: Record<ProjectStatus, { label: string; type: 'info' | 'success' | 'warning' }> = {
  DRAFT: { label: '草稿', type: 'info' },
  ACTIVE: { label: '运行中', type: 'success' },
  SUSPENDED: { label: '已暂停', type: 'warning' },
  ARCHIVED: { label: '已归档', type: 'info' }
}
const query = reactive({ pageNo: 1, pageSize: 12, keyword: '', status: '' })
const projects = ref<Project[]>([])
const members = ref<ProjectMember[]>([])
const users = ref<UserVO[]>([])
const overview = ref<ProjectOverview>()
const integrations = ref<Array<Record<string, any>>>([])
const total = ref(0)
const loading = ref(false)
const tabLoading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const createVisible = ref(false)
const drawerVisible = ref(false)
const memberVisible = ref(false)
const selected = ref<Project>()
const cloneSource = ref<Project>()
const activeTab = ref('overview')
const projectForm = reactive({
  code: '',
  name: '',
  description: '',
  aiDomain: 'CV',
  taskType: 'CV_DETECTION',
  ownerUserId: undefined as number | undefined,
  defaultProvider: 'LOCAL_DOCKER',
  storageBucket: 'visionai-assets',
  maxConcurrentJobs: 2,
  monthlyGpuHours: 100,
  storageBytes: 100 * 1024 * 1024 * 1024
})
const memberForm = reactive<{ userId?: number }>({ userId: undefined })
const availableMemberUsers = computed(() => {
  const existing = new Set(members.value.map((member) => member.userId))
  return users.value.filter((user) => user.status === 0 && !existing.has(user.id))
})
const configForm = reactive({
  storageEndpoint: '',
  bucket: '',
  trainingEndpoint: '',
  credentialRef: '',
  cvatInstanceId: undefined as number | undefined,
  clearmlInstanceId: undefined as number | undefined,
  fiftyOneInstanceId: undefined as number | undefined,
  defaultQueue: 'gpu-local',
  changeReason: ''
})
const providerInstances = (providerType: string) =>
  integrations.value.filter((item) => item.providerType === providerType)
const overviewMetricCards = computed(() => {
  const metrics = overview.value?.metrics
  if (!metrics) return []
  return [
    { label: '数据资产', value: metrics.assets },
    { label: '标注任务', value: metrics.annotations },
    { label: '数据集版本', value: metrics.datasetVersions },
    { label: '训练运行', value: metrics.trainingRuns },
    { label: '模型版本', value: metrics.modelVersions },
    { label: '部署', value: metrics.deployments }
  ]
})

const formatDate = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })

const loadProjects = async () => {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await getProjectPage(query)
    projects.value = data.list
    total.value = data.total
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '项目列表加载失败'
  } finally {
    loading.value = false
  }
}

const openCreate = (source?: Project) => {
  cloneSource.value = source
  Object.assign(projectForm, {
    code: source ? `${source.code}-copy` : '',
    name: source ? `${source.name} 副本` : '',
    description: source?.description || '',
    aiDomain: source?.aiDomain || 'CV',
    taskType: source?.taskType || 'CV_DETECTION',
    ownerUserId: source?.ownerUserId || users.value.find((item) => item.username === 'admin')?.id,
    defaultProvider: source?.defaultProvider || 'LOCAL_DOCKER',
    storageBucket: source?.storageBucket || 'visionai-assets',
    maxConcurrentJobs: 2,
    monthlyGpuHours: 100,
    storageBytes: 100 * 1024 * 1024 * 1024
  })
  createVisible.value = true
}

const submitProject = async () => {
  if (
    !projectForm.code.trim() ||
    !projectForm.name.trim() ||
    (!cloneSource.value &&
      (!projectForm.ownerUserId ||
        !projectForm.aiDomain ||
        !projectForm.taskType ||
        !projectForm.defaultProvider ||
        !projectForm.storageBucket.trim()))
  ) {
    message.warning('请填写项目编码、名称、AI/任务类型、负责人、Provider 和对象存储空间')
    return
  }
  saving.value = true
  try {
    if (cloneSource.value) await cloneProject(cloneSource.value.id, projectForm)
    else await createProject(projectForm)
    message.success('项目已创建')
    createVisible.value = false
    await loadProjects()
  } finally {
    saving.value = false
  }
}

const openProject = async (project: Project) => {
  selected.value = project
  activeTab.value = 'overview'
  drawerVisible.value = true
  await loadTab('overview')
}

const changeStatus = async (status: ProjectStatus) => {
  if (!selected.value) return
  selected.value = await updateProjectStatus(selected.value.id, status)
  message.success('项目状态已更新')
  await loadProjects()
}

const doArchive = async () => {
  if (!selected.value) return
  await message.confirm('归档后项目只读且不能恢复，是否继续？', '归档项目')
  await archiveProject(selected.value.id)
  message.success('项目已归档')
  drawerVisible.value = false
  await loadProjects()
}

const loadTab = async (name: string | number) => {
  if (!selected.value) return
  tabLoading.value = true
  try {
    if (name === 'overview') overview.value = await getProjectOverview(selected.value.id)
    if (name === 'members') members.value = await getProjectMembers(selected.value.id)
    if (name === 'config') {
      const data = await getProjectConfig(selected.value.id)
      Object.assign(configForm, {
        storageEndpoint: String(data.storageConfig.endpoint || ''),
        bucket: String(data.storageConfig.bucket || ''),
        trainingEndpoint: String(data.providerConfig.trainingEndpoint || ''),
        credentialRef: data.secretRefs.providerCredential || '',
        cvatInstanceId: Number(data.providerConfig.cvatInstanceId) || undefined,
        clearmlInstanceId: Number(data.providerConfig.clearmlInstanceId) || undefined,
        fiftyOneInstanceId: Number(data.providerConfig.fiftyOneInstanceId) || undefined,
        defaultQueue: String(data.providerConfig.defaultQueue || 'gpu-local')
      })
    }
  } finally {
    tabLoading.value = false
  }
}

const openMemberDialog = () => {
  memberForm.userId = availableMemberUsers.value[0]?.id
  memberVisible.value = true
}

const saveMember = async () => {
  if (!selected.value || !memberForm.userId) {
    message.warning('请选择项目成员')
    return
  }
  saving.value = true
  try {
    await upsertProjectMember(selected.value.id, { userId: memberForm.userId })
    memberVisible.value = false
    await loadTab('members')
    message.success('项目成员已添加，权限继承系统角色')
  } finally {
    saving.value = false
  }
}

const deleteMember = async (userId: number) => {
  if (!selected.value) return
  await removeProjectMember(selected.value.id, userId)
  await loadTab('members')
  message.success('成员已移除')
}

const saveConfig = async () => {
  if (!selected.value) return
  if (!configForm.changeReason) return message.warning('配置变更原因必填')
  saving.value = true
  try {
    await updateProjectConfig(selected.value.id, {
      storageConfig: { endpoint: configForm.storageEndpoint, bucket: configForm.bucket },
      providerConfig: {
        trainingEndpoint: configForm.trainingEndpoint,
        cvatInstanceId: configForm.cvatInstanceId,
        clearmlInstanceId: configForm.clearmlInstanceId,
        fiftyOneInstanceId: configForm.fiftyOneInstanceId,
        defaultQueue: configForm.defaultQueue
      },
      secretRefs: { providerCredential: configForm.credentialRef },
      changeReason: configForm.changeReason
    })
    configForm.changeReason = ''
    message.success('Provider 配置已保存')
  } finally {
    saving.value = false
  }
}

const testSelectedProviders = async () => {
  const ids = [
    configForm.cvatInstanceId,
    configForm.clearmlInstanceId,
    configForm.fiftyOneInstanceId
  ].filter((id): id is number => Boolean(id))
  if (!ids.length) {
    message.warning('请至少选择一个外部实例')
    return
  }
  saving.value = true
  try {
    await Promise.all(ids.map((id) => testIntegration(id)))
    message.success(`${ids.length} 个 Provider 的网络、健康 API 与认证响应均通过`)
    const data = await getIntegrations()
    integrations.value = data.instances
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  const [, systemUsers, operationData] = await Promise.all([
    loadProjects(),
    getSimpleUserList(),
    getIntegrations()
  ])
  users.value = systemUsers
  integrations.value = operationData.instances
})
</script>

<style scoped lang="scss">
.project-center {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.page-header,
.toolbar,
.project-card header,
.project-card footer,
.drawer-title,
.lifecycle-actions,
.tab-header {
  display: flex;
  align-items: center;
}

.page-header {
  justify-content: space-between;
  gap: var(--space-5);

  h1 {
    margin: var(--space-2) 0 var(--space-1);
    font-size: 24px;
  }

  p {
    color: var(--text-tertiary);
  }
}

.eyebrow {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  color: var(--primary);
}

.toolbar {
  max-width: 720px;
  margin: var(--space-6) 0;
  gap: var(--space-3);

  .el-input {
    width: 360px;
  }

  .el-select {
    width: 160px;
  }
}

.project-grid {
  display: grid;
  min-height: 260px;
  margin-top: var(--space-5);
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-4);
}

.project-card,
.empty-card {
  min-height: 212px;
  padding: var(--space-5);
  text-align: left;
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
}

.project-card {
  display: flex;
  cursor: pointer;
  flex-direction: column;
  transition:
    border-color 0.16s,
    transform 0.16s;

  &:hover {
    border-color: var(--primary);
    transform: translateY(-2px);
  }

  header,
  footer {
    justify-content: space-between;
  }

  > div {
    flex: 1;
    padding-top: var(--space-4);
  }

  small {
    font-size: 11px;
    color: var(--text-tertiary);
  }

  h2 {
    margin: var(--space-1) 0 var(--space-2);
    font-size: 17px;
  }

  p {
    display: -webkit-box;
    overflow: hidden;
    font-size: 12px;
    color: var(--text-secondary);
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  footer {
    padding-top: var(--space-4);
    font-size: 11px;
    color: var(--text-tertiary);
    border-top: 1px solid var(--divider);

    span {
      display: flex;
      align-items: center;
      gap: 4px;
    }
  }
}

.project-icon {
  display: grid;
  width: 38px;
  height: 38px;
  color: var(--primary);
  background: var(--primary-soft);
  border-radius: var(--radius-md);
  place-items: center;
}

.member-account {
  display: block;
  margin-top: 2px;
  color: var(--text-tertiary);
}

.empty-role {
  font-size: 12px;
  color: var(--text-tertiary);
}

.member-user-field {
  margin-top: var(--space-4);
}

.empty-card {
  display: grid;
  color: var(--text-tertiary);
  cursor: pointer;
  border-style: dashed;
  place-items: center;
  align-content: center;
  gap: var(--space-2);

  strong {
    color: var(--text-primary);
  }
}

.drawer-title {
  min-width: 0;
  gap: var(--space-3);

  div {
    display: flex;
    min-width: 0;
    margin-right: var(--space-3);
    flex-direction: column;
  }

  small {
    color: var(--text-tertiary);
  }
}

.lifecycle-actions {
  padding-bottom: var(--space-5);
  gap: var(--space-2);
}

.overview-list {
  margin: 0;

  div {
    display: grid;
    min-height: 54px;
    border-bottom: 1px solid var(--divider);
    grid-template-columns: 140px 1fr;
    align-items: center;
  }

  dt {
    color: var(--text-tertiary);
  }

  dd {
    margin: 0;
  }
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
}

.quota-grid :deep(.el-input-number) {
  width: 100%;
}

.project-overview {
  display: grid;
  gap: var(--space-5);
}

.overview-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-3);

  article {
    display: grid;
    min-height: 88px;
    padding: var(--space-4);
    background: var(--bg-canvas);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    align-content: center;
    gap: var(--space-1);
  }

  span {
    font-size: 12px;
    color: var(--text-tertiary);
  }

  strong {
    font-size: 24px;
  }
}

.overview-section {
  padding: var(--space-4);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);

  > header {
    display: flex;
    margin-bottom: var(--space-3);
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  h3 {
    margin: 0 0 2px;
    font-size: 16px;
  }

  p {
    margin: 0;
    font-size: 12px;
    color: var(--text-tertiary);
  }
}

.risk-row {
  display: grid;
  width: 100%;
  padding: 10px 0;
  text-align: left;
  background: transparent;
  border: 0;
  border-top: 1px solid var(--divider);
  cursor: pointer;
  grid-template-columns: 72px 1fr auto;
  align-items: center;
  gap: var(--space-2);
}

.empty-copy {
  padding: var(--space-3) 0;
}

.timeline-list {
  display: grid;
  margin: 0;
  padding: 0;
  list-style: none;
  gap: var(--space-3);

  li {
    display: grid;
    grid-template-columns: 12px 1fr auto;
    align-items: start;
    gap: var(--space-2);
  }

  time {
    font-size: 11px;
    color: var(--text-tertiary);
  }
}

.timeline-dot {
  width: 8px;
  height: 8px;
  margin-top: 5px;
  background: var(--primary);
  border-radius: 50%;
}

.tab-header {
  justify-content: space-between;
  margin-bottom: var(--space-3);

  p {
    color: var(--text-tertiary);
  }
}

.role-tag {
  margin: 2px 4px 2px 0;
}

.config-form {
  max-width: 560px;
  margin-top: var(--space-5);
}

.config-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}

.full-width {
  width: 100%;
}

@media (width <= 1100px) {
  .project-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 720px) {
  .page-header,
  .toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .toolbar .el-input,
  .toolbar .el-select {
    width: 100%;
  }

  .project-grid {
    grid-template-columns: 1fr;
  }

  .form-grid,
  .overview-metrics {
    grid-template-columns: 1fr;
  }
}
</style>

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
            <dl class="overview-list">
              <div
                ><dt>项目编码</dt><dd>{{ selected.code }}</dd></div
              >
              <div
                ><dt>负责人</dt><dd>#{{ selected.ownerUserId }}</dd></div
              >
              <div
                ><dt>创建时间</dt><dd>{{ formatDate(selected.createTime) }}</dd></div
              >
              <div
                ><dt>项目说明</dt><dd>{{ selected.description || '暂无' }}</dd></div
              >
            </dl>
          </el-tab-pane>
          <el-tab-pane label="成员与角色" name="members">
            <div class="tab-header">
              <p>角色变更由后端即时鉴权。</p>
              <el-button type="primary" plain @click="memberVisible = true">添加成员</el-button>
            </div>
            <el-table :data="members" v-loading="tabLoading">
              <el-table-column prop="userId" label="用户 ID" width="110" />
              <el-table-column label="项目角色">
                <template #default="{ row }">
                  <el-tag v-for="role in row.roles" :key="role" class="role-tag">{{ role }}</el-tag>
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
              <el-button type="primary" :loading="saving" @click="saveConfig">保存配置</el-button>
            </el-form>
          </el-tab-pane>
        </el-tabs>
      </template>
    </el-drawer>

    <el-dialog v-model="memberVisible" title="添加或更新成员" width="520">
      <el-form label-position="top">
        <el-form-item label="租户用户 ID" required>
          <el-input-number v-model="memberForm.userId" :min="1" controls-position="right" />
        </el-form-item>
        <el-form-item label="项目角色" required>
          <el-select v-model="memberForm.roles" multiple class="full-width">
            <el-option v-for="role in roleOptions" :key="role" :label="role" :value="role" />
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
  getProjectPage,
  removeProjectMember,
  updateProjectConfig,
  updateProjectStatus,
  upsertProjectMember,
  type Project,
  type ProjectMember,
  type ProjectStatus
} from '@/api/ai-platform/projects'

defineOptions({ name: 'VisionAIProjects' })

const message = useMessage()
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
const roleOptions = [
  'DATA_MANAGER',
  'ANNOTATOR',
  'REVIEWER',
  'ALGORITHM_ENGINEER',
  'APPROVER',
  'OPS',
  'AUDITOR'
]
const query = reactive({ pageNo: 1, pageSize: 12, keyword: '', status: '' })
const projects = ref<Project[]>([])
const members = ref<ProjectMember[]>([])
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
const projectForm = reactive({ code: '', name: '', description: '' })
const memberForm = reactive<{ userId: number; roles: string[] }>({ userId: 1, roles: [] })
const configForm = reactive({
  storageEndpoint: '',
  bucket: '',
  trainingEndpoint: '',
  credentialRef: ''
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
    description: source?.description || ''
  })
  createVisible.value = true
}

const submitProject = async () => {
  if (!projectForm.code.trim() || !projectForm.name.trim()) {
    message.warning('请填写项目编码和名称')
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

const openProject = (project: Project) => {
  selected.value = project
  activeTab.value = 'overview'
  drawerVisible.value = true
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
    if (name === 'members') members.value = await getProjectMembers(selected.value.id)
    if (name === 'config') {
      const data = await getProjectConfig(selected.value.id)
      Object.assign(configForm, {
        storageEndpoint: String(data.storageConfig.endpoint || ''),
        bucket: String(data.storageConfig.bucket || ''),
        trainingEndpoint: String(data.providerConfig.trainingEndpoint || ''),
        credentialRef: data.secretRefs.providerCredential || ''
      })
    }
  } finally {
    tabLoading.value = false
  }
}

const saveMember = async () => {
  if (!selected.value || !memberForm.roles.length) {
    message.warning('请选择至少一个项目角色')
    return
  }
  saving.value = true
  try {
    await upsertProjectMember(selected.value.id, memberForm)
    memberVisible.value = false
    await loadTab('members')
    message.success('成员角色已生效')
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
  saving.value = true
  try {
    await updateProjectConfig(selected.value.id, {
      storageConfig: { endpoint: configForm.storageEndpoint, bucket: configForm.bucket },
      providerConfig: { trainingEndpoint: configForm.trainingEndpoint },
      secretRefs: { providerCredential: configForm.credentialRef }
    })
    message.success('Provider 配置已保存')
  } finally {
    saving.value = false
  }
}

onMounted(loadProjects)
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
}
</style>

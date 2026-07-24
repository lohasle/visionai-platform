<template>
  <main class="asset-center">
    <header class="page-header">
      <div>
        <span class="eyebrow">DATA FOUNDATION</span>
        <h1>数据资产中心</h1>
        <p>可恢复上传、内容校验、哈希去重和可审计回收站。</p>
      </div>
      <div class="actions">
        <input
          ref="fileInput"
          class="file-input"
          type="file"
          accept="image/*"
          @change="selectFile"
        />
        <el-button :disabled="!projectId" @click="fileInput?.click()">
          <Icon icon="lucide:upload" :size="16" />上传图像
        </el-button>
        <el-button type="primary" :disabled="!projectId" @click="importVisible = true">
          <Icon icon="lucide:folder-input" :size="16" />批量导入
        </el-button>
      </div>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadAll">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="project.name"
          :value="project.id"
        />
      </el-select>
      <el-input
        v-model="query.keyword"
        clearable
        placeholder="文件名或 SHA-256"
        @keyup.enter="loadAssets"
      >
        <template #prefix><Icon icon="lucide:search" :size="16" /></template>
      </el-input>
      <el-select v-model="query.status" clearable placeholder="全部可见状态">
        <el-option label="可用" value="READY" />
        <el-option label="无效" value="INVALID" />
        <el-option label="缺失" value="MISSING" />
        <el-option label="回收站" value="DELETED" />
      </el-select>
      <el-button @click="loadAssets">查询</el-button>
    </section>

    <el-progress
      v-if="uploading"
      class="upload-progress"
      :percentage="uploadProgress"
      :status="uploadProgress === 100 ? 'success' : undefined"
    />

    <section class="quality-strip">
      <article>
        <small>项目资产</small><strong>{{ total }}</strong>
      </article>
      <article>
        <small>可用图像</small><strong>{{ qualityCount('READY') }}</strong>
      </article>
      <article>
        <small>需修复</small
        ><strong>{{ qualityCount('INVALID') + qualityCount('MISSING') }}</strong>
      </article>
      <article>
        <small>重复哈希组</small><strong>{{ quality.duplicateGroups }}</strong>
      </article>
    </section>

    <section v-loading="loading" class="asset-grid">
      <article v-for="asset in assets" :key="asset.id" class="asset-card">
        <div class="preview" :class="{ invalid: asset.status !== 'READY' }">
          <img v-if="asset.thumbnailUrl" :src="asset.thumbnailUrl" :alt="asset.filename" />
          <Icon v-else icon="lucide:image-off" :size="30" />
          <el-tag class="status" :type="asset.status === 'READY' ? 'success' : 'danger'">
            {{ asset.status }}
          </el-tag>
        </div>
        <div class="asset-info">
          <strong :title="asset.filename">{{ asset.filename }}</strong>
          <span>{{ asset.width }}×{{ asset.height }} · {{ formatSize(asset.size) }}</span>
          <code>{{ asset.sha256 ? asset.sha256.slice(0, 16) : asset.errorCode }}</code>
          <p v-if="asset.errorMessage">{{ asset.errorMessage }}</p>
        </div>
        <footer>
          <span v-if="asset.duplicateOfId">引用 #{{ asset.duplicateOfId }}</span>
          <span v-else>原始对象</span>
          <el-button
            v-if="asset.status !== 'DELETED'"
            link
            type="danger"
            @click="removeAsset(asset)"
          >
            移入回收站
          </el-button>
        </footer>
      </article>
      <div v-if="!loading && !assets.length" class="empty-state">
        <Icon icon="lucide:images" :size="30" />
        <strong>当前筛选下暂无资产</strong>
        <span>上传图像或从受控目录、S3 前缀、ZIP、JSONL 导入。</span>
      </div>
    </section>

    <el-pagination
      v-if="total > query.pageSize"
      v-model:current-page="query.pageNo"
      :page-size="query.pageSize"
      :total="total"
      layout="prev, pager, next"
      @current-change="loadAssets"
    />

    <el-dialog v-model="importVisible" title="创建批量导入任务" width="540">
      <el-form label-position="top">
        <el-form-item label="导入源类型">
          <el-select v-model="importForm.sourceType" class="full-width">
            <el-option label="受控目录" value="DIRECTORY" />
            <el-option label="S3 前缀" value="S3_PREFIX" />
            <el-option label="ZIP 对象" value="ZIP" />
            <el-option label="JSONL 清单" value="JSONL" />
          </el-select>
        </el-form-item>
        <el-form-item label="源路径或对象 Key">
          <el-input v-model="importForm.source" placeholder="例如 incoming/shift-a" />
        </el-form-item>
        <el-form-item label="重复文件策略">
          <el-radio-group v-model="importForm.duplicatePolicy">
            <el-radio value="REFERENCE">逻辑引用</el-radio>
            <el-radio value="SKIP">跳过</el-radio>
            <el-radio value="KEEP">保留副本</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitImport">创建任务</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<script lang="ts" setup>
import {
  createAssetImport,
  getAssetImport,
  getAssetPage,
  getAssetQuality,
  recycleAsset,
  uploadAsset,
  type Asset,
  type AssetQuality
} from '@/api/ai-platform/assets'
import { getProjectPage, type Project } from '@/api/ai-platform/projects'

defineOptions({ name: 'VisionAIAssets' })

const message = useMessage()
const fileInput = ref<HTMLInputElement>()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const assets = ref<Asset[]>([])
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const uploading = ref(false)
const uploadProgress = ref(0)
const importVisible = ref(false)
const quality = reactive<AssetQuality>({ byStatus: [], duplicateGroups: 0 })
const query = reactive({ pageNo: 1, pageSize: 12, keyword: '', status: '' })
const importForm = reactive({ sourceType: 'DIRECTORY', source: '', duplicatePolicy: 'REFERENCE' })

const qualityCount = (status: string) =>
  quality.byStatus.find((item) => item.status === status)?.count || 0
const formatSize = (size: number) =>
  size < 1024 * 1024 ? `${(size / 1024).toFixed(1)} KiB` : `${(size / 1024 / 1024).toFixed(1)} MiB`

const loadAssets = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    const data = await getAssetPage(projectId.value, query)
    assets.value = data.list
    total.value = data.total
  } finally {
    loading.value = false
  }
}

const loadAll = async () => {
  if (!projectId.value) return
  await Promise.all([
    loadAssets(),
    getAssetQuality(projectId.value).then((data) => Object.assign(quality, data))
  ])
}

const selectFile = async (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file || !projectId.value) return
  uploading.value = true
  uploadProgress.value = 0
  try {
    await uploadAsset(projectId.value, file, (value) => (uploadProgress.value = value))
    message.success('资产上传、校验和缩略图生成完成')
    await loadAll()
  } finally {
    uploading.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

const submitImport = async () => {
  if (!projectId.value || !importForm.source.trim()) {
    message.warning('请填写导入源')
    return
  }
  saving.value = true
  try {
    const data = await createAssetImport(projectId.value, { ...importForm, options: {} })
    importVisible.value = false
    message.success(`导入任务 #${data.importRun.jobId} 已进入队列`)
    const timer = window.setInterval(async () => {
      const current = await getAssetImport(projectId.value!, data.importRun.id)
      if (!['QUEUED', 'RUNNING'].includes(current.importRun.status)) {
        window.clearInterval(timer)
        await loadAll()
        message.success(`导入完成：${current.importRun.succeeded}/${current.importRun.total}`)
      }
    }, 1500)
  } finally {
    saving.value = false
  }
}

const removeAsset = async (asset: Asset) => {
  if (!projectId.value) return
  await message.confirm(`将 ${asset.filename} 移入回收站？`)
  await recycleAsset(projectId.value, asset.id)
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
.asset-center {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.page-header,
.actions,
.toolbar,
.quality-strip,
.asset-card footer {
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

.actions,
.toolbar {
  gap: var(--space-3);
}

.file-input {
  display: none;
}

.toolbar {
  margin: var(--space-6) 0;

  .el-select {
    width: 210px;
  }

  .el-input {
    width: 340px;
  }
}

.upload-progress {
  margin-bottom: var(--space-5);
}

.quality-strip {
  gap: var(--space-3);

  article {
    display: flex;
    min-width: 150px;
    padding: var(--space-4) var(--space-5);
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    flex-direction: column;
  }

  small {
    color: var(--text-tertiary);
  }

  strong {
    margin-top: 2px;
    font-size: 22px;
  }
}

.asset-grid {
  display: grid;
  min-height: 300px;
  margin: var(--space-5) 0;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-4);
}

.asset-card {
  overflow: hidden;
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
}

.preview {
  position: relative;
  display: grid;
  height: 160px;
  overflow: hidden;
  color: var(--text-tertiary);
  background: var(--bg-subtle);
  place-items: center;

  &.invalid {
    color: var(--danger);
    background: var(--danger-soft);
  }

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .status {
    position: absolute;
    top: var(--space-3);
    right: var(--space-3);
  }
}

.asset-info {
  display: flex;
  padding: var(--space-4);
  flex-direction: column;
  gap: 4px;

  strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span,
  code,
  p {
    font-size: 11px;
    color: var(--text-tertiary);
  }

  p {
    margin: 0;
    color: var(--danger);
  }
}

.asset-card footer {
  min-height: 42px;
  padding: 0 var(--space-4);
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-tertiary);
  border-top: 1px solid var(--divider);
}

.empty-state {
  display: grid;
  min-height: 260px;
  color: var(--text-tertiary);
  border: 1px dashed var(--border-default);
  border-radius: var(--radius-lg);
  grid-column: 1 / -1;
  place-items: center;
  align-content: center;
  gap: var(--space-2);

  strong {
    color: var(--text-primary);
  }
}

.full-width {
  width: 100%;
}

@media (width <= 1200px) {
  .asset-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (width <= 760px) {
  .page-header,
  .toolbar,
  .quality-strip {
    align-items: stretch;
    flex-direction: column;
  }

  .toolbar .el-select,
  .toolbar .el-input {
    width: 100%;
  }

  .asset-grid {
    grid-template-columns: 1fr;
  }
}
</style>

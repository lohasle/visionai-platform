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
          accept="image/*,video/*,text/*,.json,.jsonl,.csv,.tsv,.xml,.yaml,.yml"
          multiple
          @change="selectFile"
        />
        <el-button :disabled="!projectId" @click="fileInput?.click()">
          <Icon icon="lucide:upload" :size="16" />上传文件
        </el-button>
        <el-button
          :loading="similarityLoading"
          :disabled="!projectId || !total"
          @click="analyzeSimilarity"
        >
          <Icon icon="lucide:scan-search" :size="16" />FiftyOne 近似去重
        </el-button>
        <el-button type="primary" :disabled="!projectId" @click="importVisible = true">
          <Icon icon="lucide:folder-input" :size="16" />批量导入
        </el-button>
        <el-button :disabled="!projectId || !total" @click="collectionDialogVisible = true">
          <Icon icon="lucide:layers" :size="16" />保存筛选为集合
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
      <el-select v-model="query.mediaKind" clearable placeholder="全部文件类型">
        <el-option label="图像" value="IMAGE" />
        <el-option label="视频" value="VIDEO" />
        <el-option label="文本/清单" value="TEXT" />
      </el-select>
      <el-select v-model="query.errorCode" clearable placeholder="全部质量结果">
        <el-option label="媒体损坏/无法解码" value="MEDIA_DECODE_FAILED" />
        <el-option label="文本无法解码" value="TEXT_DECODE_FAILED" />
        <el-option label="空文本" value="EMPTY_TEXT" />
        <el-option label="格式缺失" value="FORMAT_MISSING" />
        <el-option label="敏感字段" value="SENSITIVE_FIELD" />
      </el-select>
      <el-select
        v-model="query.tagDefinitionIds"
        multiple
        collapse-tags
        clearable
        placeholder="按标签筛选（同时满足）"
      >
        <el-option
          v-for="definition in tagDefinitions"
          :key="definition.id"
          :label="`${categoryText(definition.category)} · ${definition.name}`"
          :value="definition.id"
        />
      </el-select>
      <el-button @click="loadAssets">查询</el-button>
      <el-button :disabled="!selectedAssetIds.length" @click="tagDialogVisible = true">
        <Icon icon="lucide:tags" :size="15" />批量打标
      </el-button>
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
        <small>可用媒体</small><strong>{{ qualityCount('READY') }}</strong>
      </article>
      <article>
        <small>需修复</small
        ><strong>{{ qualityCount('INVALID') + qualityCount('MISSING') }}</strong>
      </article>
      <article>
        <small>重复哈希组</small><strong>{{ quality.duplicateGroups }}</strong>
      </article>
      <article>
        <small>近似重复组</small><strong>{{ quality.nearDuplicateGroups }}</strong>
      </article>
    </section>

    <section v-loading="loading" class="asset-grid">
      <article v-for="asset in assets" :key="asset.id" class="asset-card">
        <div class="preview" :class="{ invalid: asset.status !== 'READY' }">
          <el-checkbox
            class="asset-selector"
            :model-value="selectedAssetIds.includes(asset.id)"
            :aria-label="`选择资产 ${asset.filename}`"
            @change="toggleAsset(asset.id)"
          />
          <img v-if="asset.thumbnailUrl" :src="asset.thumbnailUrl" :alt="asset.filename" />
          <Icon
            v-else
            :icon="
              asset.mediaKind === 'VIDEO'
                ? 'lucide:video'
                : asset.mediaKind === 'TEXT'
                  ? 'lucide:file-text'
                  : 'lucide:image-off'
            "
            :size="30"
          />
          <el-tag class="status" :type="asset.status === 'READY' ? 'success' : 'danger'">
            {{ asset.status }}
          </el-tag>
        </div>
        <div class="asset-info">
          <strong :title="asset.filename">{{ asset.filename }}</strong>
          <span>
            <template v-if="asset.mediaKind !== 'TEXT'"
              >{{ asset.width }}×{{ asset.height }} ·
            </template>
            {{ formatSize(asset.size) }}
            <template v-if="asset.mediaKind === 'VIDEO'">
              · {{ asset.durationSeconds.toFixed(1) }}s · {{ asset.codec }}
            </template>
          </span>
          <span v-if="asset.businessScene || asset.sourceDevice">
            {{ asset.businessScene || '未分类场景' }} · {{ asset.sourceDevice || '未知设备' }}
          </span>
          <code>{{ asset.sha256 ? asset.sha256.slice(0, 16) : asset.errorCode }}</code>
          <p v-if="asset.errorMessage">{{ asset.errorMessage }}</p>
          <div class="asset-tags">
            <el-tag
              v-for="tag in asset.tags"
              :key="tag.definitionId"
              size="small"
              effect="plain"
              :color="`${tag.color}18`"
            >
              {{ tag.name }}
            </el-tag>
          </div>
        </div>
        <footer>
          <span v-if="asset.nearDuplicateOfId">近似组 #{{ asset.nearDuplicateOfId }}</span>
          <span v-else-if="asset.duplicateOfId">引用 #{{ asset.duplicateOfId }}</span>
          <span v-else-if="asset.status === 'DELETED' && asset.purgeEligibleAt">
            保留至 {{ formatDateTime(asset.purgeEligibleAt) }}
          </span>
          <span v-else>原始对象</span>
          <div>
            <el-button link @click="openAssetDetail(asset)">详情与引用</el-button>
            <el-button
              v-if="asset.status !== 'DELETED'"
              link
              type="danger"
              @click="removeAsset(asset)"
            >
              移入回收站
            </el-button>
            <template v-else>
              <el-button link type="primary" @click="doRestoreAsset(asset)">恢复</el-button>
              <el-button
                link
                type="danger"
                :disabled="!isPurgeEligible(asset)"
                @click="doPurgeAsset(asset)"
              >
                永久清理
              </el-button>
            </template>
          </div>
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
        <el-form-item label="业务场景">
          <el-input v-model="importForm.businessScene" placeholder="例如 夜间道路" />
        </el-form-item>
        <el-form-item label="来源设备">
          <el-input v-model="importForm.sourceDevice" placeholder="例如 Camera-A / iPhone" />
        </el-form-item>
        <el-form-item label="语言">
          <el-input v-model="importForm.language" placeholder="例如 zh-CN" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitImport">创建任务</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="metadataVisible" title="媒体元数据" width="560">
      <el-alert
        :closable="false"
        type="info"
        show-icon
        :title="
          pendingFiles.length
            ? pendingFiles.length === 1
              ? `${pendingFiles[0].name} · ${formatSize(pendingFiles[0].size)}`
              : `${pendingFiles.length} 个媒体文件 · 共 ${formatSize(pendingFiles.reduce((sum, file) => sum + file.size, 0))}`
            : '选择媒体文件'
        "
      />
      <el-form class="metadata-form" label-position="top">
        <el-form-item label="业务场景">
          <el-input v-model="uploadMetadata.businessScene" placeholder="例如 夜间道路、产线质检" />
        </el-form-item>
        <el-form-item label="来源设备">
          <el-input
            v-model="uploadMetadata.sourceDevice"
            placeholder="可留空，由视频容器元数据自动补全"
          />
        </el-form-item>
        <el-form-item label="语言">
          <el-input v-model="uploadMetadata.language" placeholder="例如 zh-CN" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cancelUpload">取消</el-button>
        <el-button type="primary" :loading="uploading" @click="submitUpload">
          分片上传并提取元数据
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="tagDialogVisible" title="批量设置资产标签" width="560">
      <el-alert
        :closable="false"
        type="info"
        show-icon
        :title="`已选择 ${selectedAssetIds.length} 个资产。替换模式会移除这些资产原有标签。`"
      />
      <el-form label-position="top" class="tag-form">
        <el-form-item label="操作模式">
          <el-radio-group v-model="tagForm.mode">
            <el-radio-button value="ADD">追加</el-radio-button>
            <el-radio-button value="REPLACE">替换</el-radio-button>
            <el-radio-button value="REMOVE">移除</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="标签定义">
          <el-select v-model="tagForm.definitionIds" multiple filterable class="full-width">
            <el-option-group
              v-for="group in tagGroups"
              :key="group.category"
              :label="categoryText(group.category)"
            >
              <el-option
                v-for="definition in group.items"
                :key="definition.id"
                :label="definition.name"
                :value="definition.id"
              />
            </el-option-group>
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tagDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitTags">应用标签</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="collectionDialogVisible" title="保存筛选结果为集合" width="520">
      <el-alert
        :closable="false"
        type="info"
        show-icon
        :title="`当前筛选命中 ${total} 个资产，将按相同条件生成可审计集合。`"
      />
      <el-form label-position="top" class="tag-form">
        <el-form-item label="集合名称" required>
          <el-input v-model.trim="collectionForm.name" placeholder="例如 夜间道路样本" />
        </el-form-item>
        <el-form-item label="用途说明">
          <el-input v-model.trim="collectionForm.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="collectionDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveFilterCollection">
          创建集合
        </el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="资产详情与引用" size="640">
      <div v-if="assetDetail" class="asset-detail" v-loading="detailLoading">
        <img
          v-if="assetDetail.previewUrl && assetDetail.asset.mediaKind === 'IMAGE'"
          class="detail-preview"
          :src="assetDetail.previewUrl"
          :alt="assetDetail.asset.filename"
        />
        <div class="detail-heading">
          <div>
            <small>Asset #{{ assetDetail.asset.id }}</small>
            <h2>{{ assetDetail.asset.filename }}</h2>
          </div>
          <el-tag :type="assetDetail.asset.status === 'READY' ? 'success' : 'warning'">
            {{ assetDetail.asset.status }}
          </el-tag>
        </div>
        <dl class="detail-grid">
          <div
            ><dt>媒体</dt
            ><dd>{{ assetDetail.asset.mediaKind }} · {{ assetDetail.asset.contentType }}</dd></div
          >
          <div
            ><dt>尺寸</dt
            ><dd>{{ assetDetail.asset.width }} × {{ assetDetail.asset.height }}</dd></div
          >
          <div
            ><dt>SHA-256</dt
            ><dd
              ><code>{{ assetDetail.asset.sha256 }}</code></dd
            ></div
          >
          <div
            ><dt>场景 / 设备</dt
            ><dd
              >{{ assetDetail.asset.businessScene || '未分类' }} ·
              {{ assetDetail.asset.sourceDevice || '未知' }}</dd
            ></div
          >
        </dl>
        <el-alert
          v-if="assetDetail.asset.status === 'DELETED'"
          type="warning"
          :closable="false"
          show-icon
          :title="`回收站保留 ${assetDetail.recycleRetentionDays} 天；${assetDetail.purgeEligibleAt ? formatDateTime(assetDetail.purgeEligibleAt) : '删除时间异常'}后才允许物理清理。`"
        />
        <section class="reference-section">
          <header>
            <div><h3>引用关系</h3><p>标注、冻结数据集和生产反馈对该资产的治理引用。</p></div>
            <el-tag>{{ assetDetail.references.length }} 项</el-tag>
          </header>
          <article
            v-for="reference in assetDetail.references"
            :key="`${reference.resourceType}-${reference.resourceId}`"
          >
            <span>{{ reference.resourceType }}</span>
            <strong>{{ reference.name || `#${reference.resourceId}` }}</strong>
            <el-tag size="small" :type="reference.frozen ? 'danger' : 'info'">
              {{ reference.status || (reference.frozen ? 'FROZEN' : 'REFERENCED') }}
            </el-tag>
          </article>
          <p v-if="!assetDetail.references.length" class="empty-reference"
            >当前资产尚未被业务对象引用。</p
          >
        </section>
      </div>
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import {
  createAssetImport,
  analyzeAssetSimilarity,
  getAssetDetail,
  getAssetImport,
  getAssetPage,
  getAssetQuality,
  purgeAsset,
  recycleAsset,
  restoreAsset,
  uploadAsset,
  type Asset,
  type AssetDetail,
  type AssetQuality
} from '@/api/ai-platform/assets'
import { getProjectPage, type Project } from '@/api/ai-platform/projects'
import {
  getTagDefinitions,
  updateAssetTags,
  type AssetTagDefinition,
  type TagCategory
} from '@/api/ai-platform/ontology'
import { createCollection } from '@/api/ai-platform/collections'

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
const similarityLoading = ref(false)
const uploadProgress = ref(0)
const importVisible = ref(false)
const metadataVisible = ref(false)
const tagDialogVisible = ref(false)
const collectionDialogVisible = ref(false)
const detailVisible = ref(false)
const detailLoading = ref(false)
const assetDetail = ref<AssetDetail>()
const tagDefinitions = ref<AssetTagDefinition[]>([])
const selectedAssetIds = ref<number[]>([])
const quality = reactive<AssetQuality>({
  byStatus: [],
  duplicateGroups: 0,
  nearDuplicateGroups: 0
})
const query = reactive({
  pageNo: 1,
  pageSize: 12,
  keyword: '',
  status: '',
  mediaKind: '',
  errorCode: '',
  tagDefinitionIds: [] as number[]
})
const importForm = reactive({
  sourceType: 'DIRECTORY',
  source: '',
  duplicatePolicy: 'REFERENCE',
  businessScene: '',
  sourceDevice: '',
  language: ''
})
const pendingFiles = ref<File[]>([])
const uploadMetadata = reactive({ businessScene: '', sourceDevice: '', language: '' })
const tagForm = reactive({
  mode: 'ADD' as 'ADD' | 'REPLACE' | 'REMOVE',
  definitionIds: [] as number[]
})
const collectionForm = reactive({ name: '', description: '' })
const categoryText = (category: TagCategory) =>
  ({ BUSINESS: '业务标签', SCENE: '场景标签', SOURCE: '来源标签' })[category]
const tagGroups = computed(() =>
  (['BUSINESS', 'SCENE', 'SOURCE'] as TagCategory[]).map((category) => ({
    category,
    items: tagDefinitions.value.filter((item) => item.category === category && item.enabled)
  }))
)

const qualityCount = (status: string) =>
  (quality.byStatus || []).find((item) => item.status === status)?.count || 0
const formatSize = (size: number) =>
  size < 1024 * 1024 ? `${(size / 1024).toFixed(1)} KiB` : `${(size / 1024 / 1024).toFixed(1)} MiB`

const loadAssets = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    const data = await getAssetPage(projectId.value, query)
    assets.value = data.list
    total.value = data.total
    selectedAssetIds.value = selectedAssetIds.value.filter((id) =>
      data.list.some((asset) => asset.id === id)
    )
  } finally {
    loading.value = false
  }
}

const loadAll = async () => {
  if (!projectId.value) return
  await Promise.all([
    loadAssets(),
    getAssetQuality(projectId.value).then((data) =>
      Object.assign(quality, {
        byStatus: data.byStatus || [],
        duplicateGroups: data.duplicateGroups || 0,
        nearDuplicateGroups: data.nearDuplicateGroups || 0
      })
    ),
    getTagDefinitions(projectId.value).then((data) => (tagDefinitions.value = data))
  ])
}

const toggleAsset = (assetId: number) => {
  if (selectedAssetIds.value.includes(assetId)) {
    selectedAssetIds.value = selectedAssetIds.value.filter((id) => id !== assetId)
  } else {
    selectedAssetIds.value.push(assetId)
  }
}

const submitTags = async () => {
  if (!projectId.value || !selectedAssetIds.value.length) return
  if (tagForm.mode !== 'REPLACE' && !tagForm.definitionIds.length) {
    message.warning('追加或移除模式至少选择一个标签')
    return
  }
  saving.value = true
  try {
    await updateAssetTags(projectId.value, {
      assetIds: selectedAssetIds.value,
      definitionIds: tagForm.definitionIds,
      mode: tagForm.mode
    })
    tagDialogVisible.value = false
    selectedAssetIds.value = []
    tagForm.definitionIds = []
    await loadAll()
    message.success('资产标签已更新')
  } finally {
    saving.value = false
  }
}

const saveFilterCollection = async () => {
  if (!projectId.value || !collectionForm.name) {
    message.warning('请填写集合名称')
    return
  }
  saving.value = true
  try {
    await createCollection(projectId.value, {
      name: collectionForm.name,
      description: collectionForm.description,
      filter: {
        keyword: query.keyword,
        status: query.status || 'READY',
        tagDefinitionIds: query.tagDefinitionIds
      }
    })
    collectionDialogVisible.value = false
    Object.assign(collectionForm, { name: '', description: '' })
    message.success(`已将当前 ${total.value} 个筛选结果保存为资产集合`)
  } finally {
    saving.value = false
  }
}

const selectFile = (event: Event) => {
  const files = Array.from((event.target as HTMLInputElement).files || [])
  if (!files.length || !projectId.value) return
  pendingFiles.value = files
  metadataVisible.value = true
}

const submitUpload = async () => {
  if (!pendingFiles.value.length || !projectId.value) return
  uploading.value = true
  uploadProgress.value = 0
  try {
    const files = [...pendingFiles.value]
    for (let index = 0; index < files.length; index += 1) {
      await uploadAsset(
        projectId.value,
        files[index],
        (value) =>
          (uploadProgress.value = Math.round(((index + value / 100) / files.length) * 100)),
        { ...uploadMetadata }
      )
    }
    metadataVisible.value = false
    message.success(`${files.length} 个媒体的分片上传、内容校验与元数据提取完成`)
    await loadAll()
  } finally {
    uploading.value = false
    pendingFiles.value = []
    if (fileInput.value) fileInput.value.value = ''
  }
}

const cancelUpload = () => {
  metadataVisible.value = false
  pendingFiles.value = []
  if (fileInput.value) fileInput.value.value = ''
}

const analyzeSimilarity = async () => {
  if (!projectId.value) return
  similarityLoading.value = true
  try {
    const result = await analyzeAssetSimilarity(projectId.value)
    message.success(
      `FiftyOne 已分析 ${result.analyzedCount} 张图像，发现 ${result.groups.length} 个近似重复组`
    )
    await loadAll()
  } finally {
    similarityLoading.value = false
  }
}

const submitImport = async () => {
  if (!projectId.value || !importForm.source.trim()) {
    message.warning('请填写导入源')
    return
  }
  saving.value = true
  try {
    const data = await createAssetImport(projectId.value, {
      sourceType: importForm.sourceType,
      source: importForm.source,
      duplicatePolicy: importForm.duplicatePolicy,
      options: {
        businessScene: importForm.businessScene,
        sourceDevice: importForm.sourceDevice,
        language: importForm.language
      }
    })
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

const openAssetDetail = async (asset: Asset) => {
  if (!projectId.value) return
  detailVisible.value = true
  detailLoading.value = true
  try {
    assetDetail.value = await getAssetDetail(projectId.value, asset.id)
  } finally {
    detailLoading.value = false
  }
}

const doRestoreAsset = async (asset: Asset) => {
  if (!projectId.value) return
  await restoreAsset(projectId.value, asset.id)
  message.success('资产已从回收站恢复')
  await loadAll()
}

const doPurgeAsset = async (asset: Asset) => {
  if (!projectId.value) return
  await message.confirm(
    '仅无冻结或业务引用的资产可永久清理；对象删除后不可恢复。确认继续？',
    '永久清理资产'
  )
  await purgeAsset(projectId.value, asset.id)
  message.success('资产对象已按保留策略永久清理')
  await loadAll()
}

const formatDateTime = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })
const isPurgeEligible = (asset: Asset) =>
  Boolean(asset.purgeEligibleAt && Date.now() >= new Date(asset.purgeEligibleAt).getTime())

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

  .asset-selector {
    position: absolute;
    top: var(--space-3);
    left: var(--space-3);
    z-index: 1;
    padding: 4px;
    background: rgb(255 255 255 / 90%);
    border-radius: var(--radius-sm);
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

.asset-tags {
  display: flex;
  min-height: 24px;
  margin-top: var(--space-2);
  flex-wrap: wrap;
  gap: 4px;
}

.tag-form {
  margin-top: var(--space-4);
}

.asset-card footer {
  min-height: 42px;
  padding: 0 var(--space-4);
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-tertiary);
  border-top: 1px solid var(--divider);
}

.asset-detail {
  display: grid;
  gap: var(--space-5);
}

.detail-preview {
  width: 100%;
  max-height: 320px;
  object-fit: contain;
  background: var(--bg-canvas);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
}

.detail-heading,
.reference-section > header,
.reference-section article {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.detail-heading {
  h2 {
    margin: 2px 0 0;
    font-size: 19px;
  }

  small {
    color: var(--text-tertiary);
  }
}

.detail-grid {
  display: grid;
  margin: 0;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);

  div {
    min-width: 0;
    padding: var(--space-3);
    background: var(--bg-canvas);
    border-radius: var(--radius-md);
  }

  dt {
    margin-bottom: 4px;
    font-size: 11px;
    color: var(--text-tertiary);
  }

  dd {
    overflow: hidden;
    margin: 0;
    text-overflow: ellipsis;
  }
}

.reference-section {
  padding: var(--space-4);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);

  h3 {
    margin: 0;
    font-size: 16px;
  }

  header p,
  .empty-reference {
    margin: 2px 0 0;
    font-size: 12px;
    color: var(--text-tertiary);
  }

  article {
    display: grid;
    padding: 12px 0;
    border-top: 1px solid var(--divider);
    grid-template-columns: 140px 1fr auto;

    span {
      font-size: 11px;
      color: var(--text-tertiary);
    }
  }
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

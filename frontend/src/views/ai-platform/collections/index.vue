<template>
  <main class="collection-center">
    <header class="page-header">
      <div>
        <span class="eyebrow">DATA FOUNDATION</span>
        <h1>资产集合</h1>
        <p>圈定训练样本范围，冻结后作为标注任务与数据集版本的不可变来源。</p>
      </div>
      <div class="actions">
        <el-button type="primary" :disabled="!projectId" @click="openCreate">
          <Icon icon="lucide:layers" :size="16" />新建集合
        </el-button>
      </div>
    </header>

    <section class="toolbar">
      <el-select v-model="projectId" placeholder="选择项目" @change="loadCollections">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="project.name"
          :value="project.id"
        />
      </el-select>
      <el-select v-model="frozenFilter" clearable placeholder="全部状态">
        <el-option label="已冻结" value="frozen" />
        <el-option label="可编辑" value="mutable" />
      </el-select>
    </section>

    <section v-loading="loading" class="collection-grid">
      <article
        v-for="collection in filteredCollections"
        :key="collection.id"
        class="collection-card"
      >
        <header>
          <strong :title="collection.name">{{ collection.name }}</strong>
          <el-tag :type="collection.frozen ? 'success' : 'warning'" size="small">
            {{ collection.frozen ? '已冻结' : '可编辑' }}
          </el-tag>
        </header>
        <p class="description">{{ collection.description || '暂无描述' }}</p>
        <dl>
          <div
            ><dt>资产数</dt><dd>{{ collection.assetCount }}</dd></div
          >
          <div
            ><dt>版本</dt><dd>v{{ collection.version }}</dd></div
          >
          <div
            ><dt>编号</dt><dd>#{{ collection.id }}</dd></div
          >
        </dl>
        <footer>
          <el-button link type="primary" @click="openAssets(collection)">查看资产</el-button>
          <template v-if="!collection.frozen">
            <el-button link @click="openAddAssets(collection)">添加资产</el-button>
            <el-button link type="warning" @click="freeze(collection)">冻结</el-button>
          </template>
        </footer>
      </article>
      <div v-if="!loading && !filteredCollections.length" class="empty-state">
        <Icon icon="lucide:layers" :size="30" />
        <strong>暂无资产集合</strong>
        <span>创建集合并加入 READY 资产，冻结后即可用于标注与数据集构建。</span>
      </div>
    </section>

    <el-dialog v-model="createVisible" title="新建资产集合" width="480">
      <el-form label-position="top">
        <el-form-item label="集合名称" required>
          <el-input v-model="createForm.name" maxlength="128" placeholder="例如 一季度巡检抽样" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="createForm.description"
            type="textarea"
            :rows="3"
            maxlength="1024"
            placeholder="集合用途、筛选条件说明"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="addVisible" :title="`向「${activeCollection?.name}」添加资产`" width="720">
      <section class="picker-toolbar">
        <el-input
          v-model="pickerQuery.keyword"
          clearable
          placeholder="文件名或 SHA-256"
          @keyup.enter="loadPickerAssets"
        >
          <template #prefix><Icon icon="lucide:search" :size="16" /></template>
        </el-input>
        <el-button @click="loadPickerAssets">查询</el-button>
        <span class="picker-count">已选 {{ selectedIds.length }} 项</span>
      </section>
      <section v-loading="pickerLoading" class="picker-grid">
        <label
          v-for="asset in pickerAssets"
          :key="asset.id"
          class="picker-item"
          :class="{ checked: selectedSet.has(asset.id) }"
        >
          <input
            type="checkbox"
            :checked="selectedSet.has(asset.id)"
            @change="toggleSelect(asset.id)"
          />
          <img v-if="asset.thumbnailUrl" :src="asset.thumbnailUrl" :alt="asset.filename" />
          <Icon v-else icon="lucide:image-off" :size="22" />
          <span :title="asset.filename">{{ asset.filename }}</span>
        </label>
        <div v-if="!pickerLoading && !pickerAssets.length" class="picker-empty">
          当前筛选下没有 READY 资产
        </div>
      </section>
      <el-pagination
        v-if="pickerTotal > pickerQuery.pageSize"
        v-model:current-page="pickerQuery.pageNo"
        :page-size="pickerQuery.pageSize"
        :total="pickerTotal"
        layout="prev, pager, next"
        @current-change="loadPickerAssets"
      />
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button
          type="primary"
          :disabled="!selectedIds.length"
          :loading="saving"
          @click="submitAddAssets"
        >
          添加 {{ selectedIds.length }} 项
        </el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="assetsVisible" :title="`集合资产 · ${activeCollection?.name}`" size="55%">
      <section class="picker-toolbar">
        <el-input
          v-model="viewerQuery.keyword"
          clearable
          placeholder="文件名或 SHA-256"
          @keyup.enter="loadViewerAssets"
        >
          <template #prefix><Icon icon="lucide:search" :size="16" /></template>
        </el-input>
        <el-button @click="loadViewerAssets">查询</el-button>
      </section>
      <section v-loading="viewerLoading" class="viewer-grid">
        <article v-for="asset in viewerAssets" :key="asset.id" class="viewer-card">
          <div class="thumb">
            <img v-if="asset.thumbnailUrl" :src="asset.thumbnailUrl" :alt="asset.filename" />
            <Icon v-else icon="lucide:image-off" :size="24" />
          </div>
          <strong :title="asset.filename">{{ asset.filename }}</strong>
          <span>{{ asset.width }}×{{ asset.height }}</span>
        </article>
        <div v-if="!viewerLoading && !viewerAssets.length" class="picker-empty">集合内暂无资产</div>
      </section>
      <el-pagination
        v-if="viewerTotal > viewerQuery.pageSize"
        v-model:current-page="viewerQuery.pageNo"
        :page-size="viewerQuery.pageSize"
        :total="viewerTotal"
        layout="prev, pager, next"
        @current-change="loadViewerAssets"
      />
    </el-drawer>
  </main>
</template>

<script lang="ts" setup>
import {
  addCollectionAssets,
  createCollection,
  freezeCollection,
  getCollectionAssets,
  getCollections,
  type AssetCollection
} from '@/api/ai-platform/collections'
import { getAssetPage, type Asset } from '@/api/ai-platform/assets'
import { getProjectPage, type Project } from '@/api/ai-platform/projects'

defineOptions({ name: 'VisionAICollections' })

type AssetRow = Asset & { thumbnailUrl?: string }

const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const collections = ref<AssetCollection[]>([])
const frozenFilter = ref('')
const loading = ref(false)
const saving = ref(false)

const createVisible = ref(false)
const createForm = reactive({ name: '', description: '' })

const activeCollection = ref<AssetCollection>()
const addVisible = ref(false)
const pickerAssets = ref<AssetRow[]>([])
const pickerTotal = ref(0)
const pickerLoading = ref(false)
const pickerQuery = reactive({ pageNo: 1, pageSize: 24, keyword: '', status: 'READY' })
const selectedIds = ref<number[]>([])
const selectedSet = computed(() => new Set(selectedIds.value))

const assetsVisible = ref(false)
const viewerAssets = ref<AssetRow[]>([])
const viewerTotal = ref(0)
const viewerLoading = ref(false)
const viewerQuery = reactive({ pageNo: 1, pageSize: 24, keyword: '' })

const filteredCollections = computed(() =>
  collections.value.filter((item) => {
    if (frozenFilter.value === 'frozen') return item.frozen
    if (frozenFilter.value === 'mutable') return !item.frozen
    return true
  })
)

const loadCollections = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    collections.value = await getCollections(projectId.value)
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  createForm.name = ''
  createForm.description = ''
  createVisible.value = true
}

const submitCreate = async () => {
  if (!projectId.value || !createForm.name.trim()) {
    message.warning('请填写集合名称')
    return
  }
  saving.value = true
  try {
    await createCollection(projectId.value, { ...createForm, filter: {} })
    createVisible.value = false
    message.success('资产集合已创建')
    await loadCollections()
  } finally {
    saving.value = false
  }
}

const toggleSelect = (assetId: number) => {
  const index = selectedIds.value.indexOf(assetId)
  if (index >= 0) {
    selectedIds.value.splice(index, 1)
  } else {
    selectedIds.value.push(assetId)
  }
}

const loadPickerAssets = async () => {
  if (!projectId.value) return
  pickerLoading.value = true
  try {
    const data = await getAssetPage(projectId.value, pickerQuery)
    pickerAssets.value = data.list
    pickerTotal.value = data.total
  } finally {
    pickerLoading.value = false
  }
}

const openAddAssets = async (collection: AssetCollection) => {
  activeCollection.value = collection
  selectedIds.value = []
  pickerQuery.pageNo = 1
  pickerQuery.keyword = ''
  addVisible.value = true
  await loadPickerAssets()
}

const submitAddAssets = async () => {
  if (!projectId.value || !activeCollection.value) return
  saving.value = true
  try {
    await addCollectionAssets(projectId.value, activeCollection.value.id, selectedIds.value)
    addVisible.value = false
    message.success(`已加入 ${selectedIds.value.length} 项资产`)
    await loadCollections()
  } finally {
    saving.value = false
  }
}

const loadViewerAssets = async () => {
  if (!projectId.value || !activeCollection.value) return
  viewerLoading.value = true
  try {
    const data = await getCollectionAssets(projectId.value, activeCollection.value.id, viewerQuery)
    viewerAssets.value = data.list
    viewerTotal.value = data.total
  } finally {
    viewerLoading.value = false
  }
}

const openAssets = async (collection: AssetCollection) => {
  activeCollection.value = collection
  viewerQuery.pageNo = 1
  viewerQuery.keyword = ''
  assetsVisible.value = true
  await loadViewerAssets()
}

const freeze = async (collection: AssetCollection) => {
  await message.confirm(
    `冻结后「${collection.name}」不可再增删资产，且集合内 ${collection.assetCount} 项资产将受引用保护。是否继续？`
  )
  if (!projectId.value) return
  await freezeCollection(projectId.value, collection.id)
  message.success('集合已冻结')
  await loadCollections()
}

onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadCollections()
})
</script>

<style scoped lang="scss">
.collection-center {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.page-header,
.actions,
.toolbar {
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

.toolbar {
  margin: var(--space-6) 0;

  .el-select {
    width: 210px;
  }
}

.collection-grid {
  display: grid;
  min-height: 300px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-4);
}

.collection-card {
  display: flex;
  padding: var(--space-5);
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  flex-direction: column;
  gap: var(--space-3);

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);

    strong {
      overflow: hidden;
      font-size: 15px;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .description {
    display: -webkit-box;
    margin: 0;
    overflow: hidden;
    font-size: 12px;
    color: var(--text-tertiary);
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  dl {
    display: flex;
    margin: 0;
    gap: var(--space-5);

    div {
      display: flex;
      flex-direction: column;
    }

    dt {
      font-size: 11px;
      color: var(--text-tertiary);
    }

    dd {
      margin: 0;
      font-size: 16px;
      font-weight: 600;
    }
  }

  footer {
    display: flex;
    padding-top: var(--space-3);
    border-top: 1px solid var(--divider);
    gap: var(--space-2);
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

.picker-toolbar {
  display: flex;
  align-items: center;
  margin-bottom: var(--space-4);
  gap: var(--space-3);

  .el-input {
    width: 300px;
  }
}

.picker-count {
  font-size: 12px;
  color: var(--text-tertiary);
}

.picker-grid,
.viewer-grid {
  display: grid;
  min-height: 200px;
  margin-bottom: var(--space-4);
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}

.picker-item {
  position: relative;
  display: flex;
  padding: var(--space-2);
  cursor: pointer;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  flex-direction: column;
  align-items: center;
  gap: var(--space-2);

  &.checked {
    border-color: var(--primary);
    box-shadow: 0 0 0 1px var(--primary);
  }

  input {
    position: absolute;
    top: var(--space-2);
    left: var(--space-2);
  }

  img {
    width: 100%;
    height: 90px;
    border-radius: var(--radius-sm);
    object-fit: cover;
  }

  span {
    width: 100%;
    overflow: hidden;
    font-size: 11px;
    text-align: center;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.picker-empty {
  display: grid;
  color: var(--text-tertiary);
  grid-column: 1 / -1;
  place-items: center;
}

.viewer-card {
  display: flex;
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  flex-direction: column;

  .thumb {
    display: grid;
    height: 100px;
    color: var(--text-tertiary);
    background: var(--bg-subtle);
    place-items: center;

    img {
      width: 100%;
      height: 100%;
      object-fit: cover;
    }
  }

  strong {
    padding: var(--space-2) var(--space-2) 0;
    overflow: hidden;
    font-size: 11px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span {
    padding: 0 var(--space-2) var(--space-2);
    font-size: 11px;
    color: var(--text-tertiary);
  }
}

@media (width <= 1200px) {
  .collection-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 760px) {
  .page-header,
  .toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .collection-grid,
  .picker-grid,
  .viewer-grid {
    grid-template-columns: 1fr;
  }
}
</style>

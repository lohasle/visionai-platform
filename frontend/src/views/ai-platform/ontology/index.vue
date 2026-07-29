<template>
  <main class="taxonomy-page">
    <header class="page-header">
      <div>
        <span class="eyebrow">GOVERNED VOCABULARY</span>
        <h1>标签与类别</h1>
        <p>统一维护资产标签字典和不可变类别版本，确保标注、数据集与模型使用同一语义。</p>
      </div>
      <el-select v-model="projectId" aria-label="当前项目" placeholder="选择项目" @change="loadAll">
        <el-option
          v-for="project in projects"
          :key="project.id"
          :label="`${project.name} · ${project.code}`"
          :value="project.id"
        />
      </el-select>
    </header>

    <section class="status-strip" aria-label="标签与类别概览">
      <article>
        <small>类别体系</small>
        <strong>{{ ontologies.length }}</strong>
      </article>
      <article>
        <small>已发布版本</small>
        <strong>{{ publishedVersionCount }}</strong>
      </article>
      <article>
        <small>资产标签定义</small>
        <strong>{{ tagDefinitions.length }}</strong>
      </article>
      <article>
        <small>已使用标签</small>
        <strong>{{ tagDefinitions.filter((item) => item.usageCount > 0).length }}</strong>
      </article>
    </section>

    <el-tabs v-model="activeTab" class="governance-tabs">
      <el-tab-pane label="类别体系" name="ontology">
        <section class="ontology-layout" v-loading="loading">
          <aside class="ontology-sidebar">
            <div class="section-heading">
              <div>
                <h2>类别体系</h2>
                <span>项目内复用，发布版本不可修改</span>
              </div>
              <el-button type="primary" @click="openOntologyCreate">
                <Icon icon="lucide:plus" :size="16" />新建
              </el-button>
            </div>
            <button
              v-for="ontology in ontologies"
              :key="ontology.id"
              type="button"
              class="ontology-item"
              :class="{ active: selectedOntology?.id === ontology.id }"
              @click="selectOntology(ontology)"
            >
              <span class="ontology-code">{{ ontology.code }}</span>
              <strong>{{ ontology.name }}</strong>
              <small>
                {{ ontology.taskType }} · {{ ontology.versionCount }} 个版本 ·
                {{ ontology.publishedCount }} 已发布
              </small>
            </button>
            <div v-if="!ontologies.length" class="empty-panel">
              <Icon icon="lucide:tags" :size="28" />
              <strong>尚无类别体系</strong>
              <span>先定义类别，再发布给标注任务使用。</span>
            </div>
          </aside>

          <section class="ontology-workspace">
            <template v-if="selectedOntology && ontologyDetail">
              <header class="workspace-header">
                <div>
                  <span class="ontology-code">{{ selectedOntology.code }}</span>
                  <h2>{{ selectedOntology.name }}</h2>
                  <p>{{ selectedOntology.description || '暂无说明' }}</p>
                </div>
                <el-button @click="cloneVersion">
                  <Icon icon="lucide:copy-plus" :size="16" />从当前版本创建草稿
                </el-button>
              </header>

              <nav class="version-nav" aria-label="类别体系版本">
                <button
                  v-for="version in ontologyDetail.versions"
                  :key="version.id"
                  type="button"
                  :class="{ active: selectedVersion?.id === version.id }"
                  @click="selectVersion(version)"
                >
                  <span>{{ version.semanticVersion }}</span>
                  <el-tag :type="versionTagType(version.status)" size="small">
                    {{ versionStatusText(version.status) }}
                  </el-tag>
                </button>
              </nav>

              <section v-if="selectedVersion" class="version-panel">
                <div class="version-toolbar">
                  <div class="version-identity">
                    <strong>{{ selectedVersion.semanticVersion }}</strong>
                    <span v-if="selectedVersion.checksum">
                      SHA-256 {{ selectedVersion.checksum.slice(0, 16) }}…
                    </span>
                    <span v-else>草稿尚未生成校验和</span>
                  </div>
                  <div class="version-actions">
                    <el-button
                      v-if="isDraft"
                      @click="loadCoco80"
                      aria-label="导入 COCO 80 个标准类别"
                    >
                      导入 COCO 80
                    </el-button>
                    <el-button v-if="isDraft" @click="openLabelEditor()">
                      <Icon icon="lucide:plus" :size="16" />添加类别
                    </el-button>
                    <el-button v-if="isDraft" :loading="saving" @click="saveLabels">
                      保存草稿
                    </el-button>
                    <el-button
                      v-if="isDraft"
                      type="primary"
                      :disabled="!labels.length"
                      :loading="publishing"
                      @click="publishVersion"
                    >
                      发布并锁定
                    </el-button>
                    <el-button
                      v-if="selectedVersion.status === 'PUBLISHED'"
                      type="danger"
                      plain
                      @click="deprecateVersion"
                    >
                      停止新引用
                    </el-button>
                  </div>
                </div>

                <el-table :data="labels" row-key="code" class="label-table">
                  <el-table-column label="类别" min-width="220">
                    <template #default="{ row }">
                      <div class="label-cell">
                        <span class="color-dot" :style="{ backgroundColor: row.color }"></span>
                        <div>
                          <strong>{{ row.name }}</strong>
                          <code>{{ row.code }}</code>
                        </div>
                      </div>
                    </template>
                  </el-table-column>
                  <el-table-column prop="shapeType" label="标注形状" width="130" />
                  <el-table-column label="属性" min-width="260">
                    <template #default="{ row }">
                      <div class="attribute-list">
                        <el-tag
                          v-for="attribute in row.attributes"
                          :key="attribute.name"
                          size="small"
                          type="info"
                        >
                          {{ attribute.name }}
                        </el-tag>
                        <span v-if="!row.attributes.length">无属性</span>
                      </div>
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" width="150" align="right">
                    <template #default="{ row, $index }">
                      <template v-if="isDraft">
                        <el-button link @click="openLabelEditor(row, $index)">编辑</el-button>
                        <el-button link type="danger" @click="removeLabel($index)">移除</el-button>
                      </template>
                      <span v-else class="locked-text">
                        <Icon icon="lucide:lock-keyhole" :size="14" />已锁定
                      </span>
                    </template>
                  </el-table-column>
                </el-table>
                <div v-if="!labels.length" class="empty-panel large">
                  <Icon icon="lucide:list-tree" :size="30" />
                  <strong>草稿中还没有类别</strong>
                  <span>逐个添加，或一键导入 COCO 80 标准类别。</span>
                </div>
              </section>
            </template>
            <div v-else class="empty-panel workspace-empty">
              <Icon icon="lucide:book-open-check" :size="34" />
              <strong>选择一个类别体系</strong>
              <span>查看版本、类别、属性以及发布校验和。</span>
            </div>
          </section>
        </section>
      </el-tab-pane>

      <el-tab-pane label="资产标签字典" name="tags">
        <section class="tag-panel" v-loading="loading">
          <header class="section-heading">
            <div>
              <h2>资产标签字典</h2>
              <span>标签按业务、场景和来源分类，在数据资产页批量应用。</span>
            </div>
            <el-button type="primary" @click="openTagEditor()">
              <Icon icon="lucide:plus" :size="16" />新建标签
            </el-button>
          </header>
          <el-table :data="tagDefinitions" row-key="id">
            <el-table-column label="标签" min-width="220">
              <template #default="{ row }">
                <div class="label-cell">
                  <span class="color-dot" :style="{ backgroundColor: row.color }"></span>
                  <div>
                    <strong>{{ row.name }}</strong>
                    <code>{{ row.code }}</code>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="分类" width="130">
              <template #default="{ row }">
                <el-tag :type="categoryTagType(row.category)">
                  {{ categoryText(row.category) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="description" label="用途说明" min-width="260" />
            <el-table-column label="使用资产" width="110" align="right">
              <template #default="{ row }">{{ row.usageCount }}</template>
            </el-table-column>
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.enabled ? 'success' : 'info'">
                  {{ row.enabled ? '启用' : '停用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90" align="right">
              <template #default="{ row }">
                <el-button link @click="openTagEditor(row)">编辑</el-button>
              </template>
            </el-table-column>
          </el-table>
        </section>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="ontologyDialogVisible" title="新建类别体系" width="540">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="体系编码" required>
            <el-input v-model.trim="ontologyForm.code" placeholder="例如 coco_detection" />
          </el-form-item>
          <el-form-item label="体系名称" required>
            <el-input v-model.trim="ontologyForm.name" placeholder="例如 COCO 目标检测" />
          </el-form-item>
        </div>
        <el-form-item label="任务类型">
          <el-select v-model="ontologyForm.taskType" class="full-width">
            <el-option label="目标检测" value="CV_DETECTION" />
            <el-option label="实例分割" value="CV_SEGMENTATION" />
            <el-option label="关键点" value="CV_KEYPOINT" />
          </el-select>
        </el-form-item>
        <el-form-item label="用途说明">
          <el-input v-model.trim="ontologyForm.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ontologyDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitOntology">创建草稿</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="labelDialogVisible" title="编辑类别" width="720">
      <el-form label-position="top">
        <div class="form-grid three">
          <el-form-item label="类别编码" required>
            <el-input v-model.trim="labelForm.code" placeholder="person" />
          </el-form-item>
          <el-form-item label="显示名称" required>
            <el-input v-model.trim="labelForm.name" placeholder="行人" />
          </el-form-item>
          <el-form-item label="标注颜色">
            <el-color-picker v-model="labelForm.color" />
          </el-form-item>
        </div>
        <el-form-item label="标注形状">
          <el-select v-model="labelForm.shapeType" class="full-width">
            <el-option label="矩形框" value="rectangle" />
            <el-option label="多边形" value="polygon" />
            <el-option label="掩膜" value="mask" />
            <el-option label="关键点" value="points" />
          </el-select>
        </el-form-item>
        <section class="attribute-editor">
          <div class="section-heading compact">
            <div>
              <h3>类别属性</h3>
              <span>例如遮挡、姿态、严重程度；将同步到 CVAT。</span>
            </div>
            <el-button @click="addAttribute">
              <Icon icon="lucide:plus" :size="15" />添加属性
            </el-button>
          </div>
          <div
            v-for="(attribute, index) in labelForm.attributes"
            :key="index"
            class="attribute-row"
          >
            <el-input v-model.trim="attribute.name" aria-label="属性名称" placeholder="属性名称" />
            <el-select v-model="attribute.inputType" aria-label="属性输入类型">
              <el-option label="单选" value="select" />
              <el-option label="复选框" value="checkbox" />
              <el-option label="文本" value="text" />
              <el-option label="数字" value="number" />
            </el-select>
            <el-input
              v-model="attribute.valuesText"
              aria-label="属性可选值"
              placeholder="可选值，逗号分隔"
            />
            <el-button
              circle
              plain
              type="danger"
              aria-label="移除属性"
              @click="labelForm.attributes.splice(index, 1)"
            >
              <Icon icon="lucide:trash-2" :size="15" />
            </el-button>
          </div>
        </section>
      </el-form>
      <template #footer>
        <el-button @click="labelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="commitLabel">保存类别</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="tagDialogVisible" title="资产标签定义" width="560">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="标签编码" required>
            <el-input v-model.trim="tagForm.code" placeholder="night" />
          </el-form-item>
          <el-form-item label="显示名称" required>
            <el-input v-model.trim="tagForm.name" placeholder="夜间" />
          </el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="标签分类">
            <el-select v-model="tagForm.category" class="full-width">
              <el-option label="业务标签" value="BUSINESS" />
              <el-option label="场景标签" value="SCENE" />
              <el-option label="来源标签" value="SOURCE" />
            </el-select>
          </el-form-item>
          <el-form-item label="显示颜色">
            <el-color-picker v-model="tagForm.color" />
          </el-form-item>
        </div>
        <el-form-item label="用途说明">
          <el-input v-model.trim="tagForm.description" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="tagForm.enabled" active-text="启用" inactive-text="停用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tagDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitTag">保存标签</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<script lang="ts" setup>
import {
  createOntology,
  createOntologyVersion,
  createTagDefinition,
  deprecateOntologyVersion,
  getOntologies,
  getOntology,
  getOntologyVersion,
  getTagDefinitions,
  publishOntologyVersion,
  replaceOntologyLabels,
  updateTagDefinition,
  type AssetTagDefinition,
  type Ontology,
  type OntologyAttribute,
  type OntologyDetail,
  type OntologyLabel,
  type OntologyVersion,
  type TagCategory
} from '@/api/ai-platform/ontology'
import { getProjectPage, type Project } from '@/api/ai-platform/projects'

defineOptions({ name: 'VisionAIOntology' })

type EditableAttribute = OntologyAttribute & { valuesText: string }

const message = useMessage()
const projects = ref<Project[]>([])
const projectId = ref<number>()
const activeTab = ref('ontology')
const loading = ref(false)
const saving = ref(false)
const publishing = ref(false)
const ontologies = ref<Ontology[]>([])
const selectedOntology = ref<Ontology>()
const ontologyDetail = ref<OntologyDetail>()
const selectedVersion = ref<OntologyVersion>()
const labels = ref<OntologyLabel[]>([])
const tagDefinitions = ref<AssetTagDefinition[]>([])
const ontologyDialogVisible = ref(false)
const labelDialogVisible = ref(false)
const tagDialogVisible = ref(false)
const editingLabelIndex = ref(-1)
const editingTagId = ref<number>()

const ontologyForm = reactive({
  code: '',
  name: '',
  taskType: 'CV_DETECTION',
  description: ''
})
const labelForm = reactive<{
  code: string
  name: string
  color: string
  shapeType: string
  attributes: EditableAttribute[]
}>({
  code: '',
  name: '',
  color: '#1677FF',
  shapeType: 'rectangle',
  attributes: []
})
const tagForm = reactive({
  code: '',
  name: '',
  category: 'SCENE' as TagCategory,
  color: '#1677FF',
  description: '',
  enabled: true
})

const publishedVersionCount = computed(() =>
  ontologies.value.reduce((sum, item) => sum + Number(item.publishedCount || 0), 0)
)
const isDraft = computed(() => selectedVersion.value?.status === 'DRAFT')
const versionTagType = (status: string) =>
  status === 'PUBLISHED' ? 'success' : status === 'DEPRECATED' ? 'info' : 'warning'
const versionStatusText = (status: string) =>
  status === 'PUBLISHED' ? '已发布' : status === 'DEPRECATED' ? '已废弃' : '草稿'
const categoryText = (category: TagCategory) =>
  ({ BUSINESS: '业务', SCENE: '场景', SOURCE: '来源' })[category]
const categoryTagType = (category: TagCategory) =>
  category === 'BUSINESS' ? 'primary' : category === 'SCENE' ? 'success' : 'warning'

const normalizeAttribute = (attribute: OntologyAttribute): OntologyAttribute => ({
  ...attribute,
  values:
    typeof attribute.values === 'string'
      ? (() => {
          try {
            return JSON.parse(attribute.values)
          } catch {
            return []
          }
        })()
      : attribute.values
})

const loadAll = async () => {
  if (!projectId.value) return
  loading.value = true
  try {
    const [ontologyRows, tagRows] = await Promise.all([
      getOntologies(projectId.value),
      getTagDefinitions(projectId.value)
    ])
    ontologies.value = ontologyRows || []
    tagDefinitions.value = tagRows || []
    const current =
      ontologies.value.find((item) => item.id === selectedOntology.value?.id) || ontologies.value[0]
    if (current) await selectOntology(current)
    else {
      selectedOntology.value = undefined
      ontologyDetail.value = undefined
      selectedVersion.value = undefined
      labels.value = []
    }
  } finally {
    loading.value = false
  }
}

const selectOntology = async (ontology: Ontology) => {
  if (!projectId.value) return
  selectedOntology.value = ontology
  ontologyDetail.value = await getOntology(projectId.value, ontology.id)
  const preferred =
    ontologyDetail.value.versions.find((item) => item.id === selectedVersion.value?.id) ||
    ontologyDetail.value.versions[0]
  if (preferred) await selectVersion(preferred)
}

const selectVersion = async (version: OntologyVersion) => {
  if (!projectId.value) return
  selectedVersion.value = version
  const detail = await getOntologyVersion(projectId.value, version.id)
  selectedVersion.value = detail.version
  labels.value = detail.labels.map((label) => ({
    ...label,
    attributes: (label.attributes || []).map(normalizeAttribute)
  }))
}

const openOntologyCreate = () => {
  Object.assign(ontologyForm, {
    code: '',
    name: '',
    taskType: 'CV_DETECTION',
    description: ''
  })
  ontologyDialogVisible.value = true
}

const submitOntology = async () => {
  if (!projectId.value || !ontologyForm.code || !ontologyForm.name) {
    message.warning('请填写体系编码和名称')
    return
  }
  saving.value = true
  try {
    const created = await createOntology(projectId.value, ontologyForm)
    ontologyDialogVisible.value = false
    await loadAll()
    const current = ontologies.value.find((item) => item.id === created.ontology.id)
    if (current) await selectOntology(current)
    message.success('类别体系和首个草稿已创建')
  } finally {
    saving.value = false
  }
}

const cloneVersion = async () => {
  if (!projectId.value || !selectedOntology.value) return
  const source =
    selectedVersion.value?.status === 'PUBLISHED' || selectedVersion.value?.status === 'DEPRECATED'
      ? selectedVersion.value.id
      : ontologyDetail.value?.versions.find((item) => item.status === 'PUBLISHED')?.id
  const created = await createOntologyVersion(projectId.value, selectedOntology.value.id, source)
  await selectOntology(selectedOntology.value)
  await selectVersion(created)
  message.success(source ? '已从不可变版本复制新草稿' : '空白草稿已创建')
}

const openLabelEditor = (label?: OntologyLabel, index = -1) => {
  editingLabelIndex.value = index
  Object.assign(labelForm, {
    code: label?.code || '',
    name: label?.name || '',
    color: label?.color || '#1677FF',
    shapeType: label?.shapeType || 'rectangle',
    attributes: (label?.attributes || []).map((attribute) => {
      const values = normalizeAttribute(attribute).values as string[]
      return { ...attribute, values, valuesText: values.join(', ') }
    })
  })
  labelDialogVisible.value = true
}

const addAttribute = () => {
  labelForm.attributes.push({
    name: '',
    inputType: 'select',
    values: [],
    valuesText: '',
    defaultValue: '',
    mutable: false,
    sort: labelForm.attributes.length
  })
}

const commitLabel = () => {
  if (!labelForm.code || !labelForm.name) {
    message.warning('类别编码和显示名称必填')
    return
  }
  const duplicate = labels.value.some(
    (item, index) =>
      index !== editingLabelIndex.value &&
      (item.code.toLowerCase() === labelForm.code.toLowerCase() ||
        item.name.toLowerCase() === labelForm.name.toLowerCase())
  )
  if (duplicate) {
    message.warning('类别编码和名称不能重复')
    return
  }
  const row: OntologyLabel = {
    code: labelForm.code,
    name: labelForm.name,
    color: labelForm.color,
    shapeType: labelForm.shapeType,
    sort: editingLabelIndex.value >= 0 ? editingLabelIndex.value : labels.value.length,
    attributes: labelForm.attributes
      .filter((item) => item.name.trim())
      .map((item, index) => ({
        name: item.name,
        inputType: item.inputType,
        values: item.valuesText
          .split(',')
          .map((value) => value.trim())
          .filter(Boolean),
        defaultValue: item.defaultValue,
        mutable: item.mutable,
        sort: index
      }))
  }
  if (editingLabelIndex.value >= 0) labels.value.splice(editingLabelIndex.value, 1, row)
  else labels.value.push(row)
  labelDialogVisible.value = false
}

const removeLabel = (index: number) => labels.value.splice(index, 1)

const saveLabels = async () => {
  if (!projectId.value || !selectedVersion.value) return
  saving.value = true
  try {
    labels.value = await replaceOntologyLabels(
      projectId.value,
      selectedVersion.value.id,
      labels.value
    )
    message.success('类别草稿已保存')
  } finally {
    saving.value = false
  }
}

const cocoClassNames = [
  'person',
  'bicycle',
  'car',
  'motorcycle',
  'airplane',
  'bus',
  'train',
  'truck',
  'boat',
  'traffic light',
  'fire hydrant',
  'stop sign',
  'parking meter',
  'bench',
  'bird',
  'cat',
  'dog',
  'horse',
  'sheep',
  'cow',
  'elephant',
  'bear',
  'zebra',
  'giraffe',
  'backpack',
  'umbrella',
  'handbag',
  'tie',
  'suitcase',
  'frisbee',
  'skis',
  'snowboard',
  'sports ball',
  'kite',
  'baseball bat',
  'baseball glove',
  'skateboard',
  'surfboard',
  'tennis racket',
  'bottle',
  'wine glass',
  'cup',
  'fork',
  'knife',
  'spoon',
  'bowl',
  'banana',
  'apple',
  'sandwich',
  'orange',
  'broccoli',
  'carrot',
  'hot dog',
  'pizza',
  'donut',
  'cake',
  'chair',
  'couch',
  'potted plant',
  'bed',
  'dining table',
  'toilet',
  'tv',
  'laptop',
  'mouse',
  'remote',
  'keyboard',
  'cell phone',
  'microwave',
  'oven',
  'toaster',
  'sink',
  'refrigerator',
  'book',
  'clock',
  'vase',
  'scissors',
  'teddy bear',
  'hair drier',
  'toothbrush'
]

const loadCoco80 = async () => {
  if (labels.value.length) await message.confirm('这会替换当前草稿中的类别，是否继续？')
  const palette = ['#155EEF', '#147A5C', '#9A4D0F', '#7A3E9D', '#B42318', '#087E8B']
  labels.value = cocoClassNames.map((name, index) => ({
    code: `coco_${String(index).padStart(2, '0')}`,
    name,
    color: palette[index % palette.length],
    shapeType: 'rectangle',
    sort: index,
    attributes: []
  }))
  message.success('已载入 COCO 80 类别，请保存并发布')
}

const publishVersion = async () => {
  if (!projectId.value || !selectedVersion.value) return
  await saveLabels()
  await message.confirm('发布后类别和属性不可修改，只能创建新版本。确认发布？')
  publishing.value = true
  try {
    selectedVersion.value = await publishOntologyVersion(projectId.value, selectedVersion.value.id)
    await selectOntology(selectedOntology.value!)
    message.success('类别版本已发布并生成不可变校验和')
  } finally {
    publishing.value = false
  }
}

const deprecateVersion = async () => {
  if (!projectId.value || !selectedVersion.value) return
  await message.confirm('废弃后历史引用仍可复现，但新任务不能再选择该版本。')
  await deprecateOntologyVersion(projectId.value, selectedVersion.value.id)
  await selectOntology(selectedOntology.value!)
}

const openTagEditor = (row?: AssetTagDefinition) => {
  editingTagId.value = row?.id
  Object.assign(tagForm, {
    code: row?.code || '',
    name: row?.name || '',
    category: row?.category || 'SCENE',
    color: row?.color || '#1677FF',
    description: row?.description || '',
    enabled: row?.enabled ?? true
  })
  tagDialogVisible.value = true
}

const submitTag = async () => {
  if (!projectId.value || !tagForm.code || !tagForm.name) {
    message.warning('标签编码和名称必填')
    return
  }
  saving.value = true
  try {
    if (editingTagId.value) {
      await updateTagDefinition(projectId.value, editingTagId.value, tagForm)
    } else {
      await createTagDefinition(projectId.value, tagForm)
    }
    tagDialogVisible.value = false
    tagDefinitions.value = await getTagDefinitions(projectId.value)
    message.success('标签定义已保存')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  const data = await getProjectPage({ pageNo: 1, pageSize: 100 })
  projects.value = data.list
  projectId.value = projects.value[0]?.id
  await loadAll()
})
</script>

<style scoped lang="scss">
.taxonomy-page {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.page-header,
.section-heading,
.workspace-header,
.version-toolbar,
.label-cell,
.locked-text {
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
    max-width: 720px;
    margin: 0;
    color: var(--text-tertiary);
  }

  .el-select {
    width: 260px;
  }
}

.eyebrow,
.ontology-code {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
  color: var(--primary);
}

.status-strip {
  display: grid;
  margin: var(--space-6) 0;
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  grid-template-columns: repeat(4, minmax(0, 1fr));

  article {
    padding: var(--space-4) var(--space-5);
    border-right: 1px solid var(--divider);
  }

  article:last-child {
    border-right: 0;
  }

  small,
  strong {
    display: block;
  }

  small {
    color: var(--text-tertiary);
  }

  strong {
    margin-top: 2px;
    font-size: 24px;
    font-variant-numeric: tabular-nums;
  }
}

.governance-tabs {
  padding: 0 var(--space-5) var(--space-5);
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
}

.ontology-layout {
  display: grid;
  min-height: 580px;
  grid-template-columns: 300px minmax(0, 1fr);
  border-top: 1px solid var(--divider);
}

.ontology-sidebar {
  padding: var(--space-5) var(--space-4) var(--space-5) 0;
  border-right: 1px solid var(--divider);
}

.section-heading {
  justify-content: space-between;
  gap: var(--space-4);
  margin-bottom: var(--space-4);

  h2,
  h3 {
    margin: 0 0 3px;
  }

  h2 {
    font-size: 18px;
  }

  h3 {
    font-size: 15px;
  }

  span {
    font-size: 12px;
    color: var(--text-tertiary);
  }

  &.compact {
    margin-bottom: var(--space-3);
  }
}

.ontology-item {
  display: flex;
  width: 100%;
  min-height: 88px;
  padding: var(--space-4);
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 0;
  border-top: 1px solid var(--divider);
  flex-direction: column;
  gap: 4px;

  &:last-of-type {
    border-bottom: 1px solid var(--divider);
  }

  &:hover,
  &:focus-visible {
    background: var(--bg-subtle);
  }

  &.active {
    background: var(--primary-soft);
    box-shadow: inset 3px 0 0 var(--primary);
  }

  small {
    font-size: 11px;
    color: var(--text-tertiary);
  }
}

.ontology-workspace {
  min-width: 0;
  padding: var(--space-5) 0 var(--space-5) var(--space-5);
}

.workspace-header {
  justify-content: space-between;
  gap: var(--space-5);
  padding-bottom: var(--space-4);
  border-bottom: 1px solid var(--divider);

  h2 {
    margin: 3px 0;
    font-size: 22px;
  }

  p {
    margin: 0;
    color: var(--text-tertiary);
  }
}

.version-nav {
  display: flex;
  padding: var(--space-4) 0;
  overflow-x: auto;
  gap: var(--space-2);

  button {
    display: inline-flex;
    min-height: 44px;
    padding: 0 var(--space-3);
    cursor: pointer;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    align-items: center;
    gap: var(--space-2);
  }

  button.active {
    border-color: var(--primary);
    box-shadow: inset 0 -2px 0 var(--primary);
  }
}

.version-panel {
  border-top: 1px solid var(--divider);
}

.version-toolbar {
  min-height: 68px;
  justify-content: space-between;
  gap: var(--space-4);
}

.version-identity {
  display: flex;
  min-width: 180px;
  flex-direction: column;

  strong {
    font:
      600 18px ui-monospace,
      Consolas,
      monospace;
  }

  span {
    font:
      11px ui-monospace,
      Consolas,
      monospace;
    color: var(--text-tertiary);
  }
}

.version-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);

  .el-button {
    margin-left: 0;
  }
}

.label-table {
  border-top: 1px solid var(--divider);
}

.label-cell {
  gap: var(--space-3);

  div {
    display: flex;
    flex-direction: column;
  }

  code {
    font-size: 11px;
    color: var(--text-tertiary);
  }
}

.color-dot {
  width: 12px;
  height: 12px;
  border: 1px solid rgb(0 0 0 / 15%);
  border-radius: 50%;
  flex: none;
}

.attribute-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;

  > span {
    font-size: 12px;
    color: var(--text-tertiary);
  }
}

.locked-text {
  justify-content: flex-end;
  gap: 5px;
  font-size: 12px;
  color: var(--text-tertiary);
}

.empty-panel {
  display: grid;
  min-height: 180px;
  padding: var(--space-5);
  color: var(--text-tertiary);
  text-align: center;
  border: 1px dashed var(--border-default);
  place-items: center;
  align-content: center;
  gap: var(--space-2);

  strong {
    color: var(--text-primary);
  }

  &.large {
    min-height: 260px;
  }

  &.workspace-empty {
    min-height: 520px;
  }
}

.tag-panel {
  padding-top: var(--space-5);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);

  &.three {
    grid-template-columns: 1fr 1fr 120px;
  }
}

.full-width {
  width: 100%;
}

.attribute-editor {
  padding: var(--space-4);
  background: var(--bg-subtle);
  border: 1px solid var(--border-default);
}

.attribute-row {
  display: grid;
  margin-top: var(--space-2);
  grid-template-columns: 1fr 130px 1.5fr 44px;
  gap: var(--space-2);
}

@media (width <= 1024px) {
  .status-strip {
    grid-template-columns: repeat(2, 1fr);

    article:nth-child(2) {
      border-right: 0;
    }

    article:nth-child(-n + 2) {
      border-bottom: 1px solid var(--divider);
    }
  }

  .ontology-layout {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  .version-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (width <= 760px) {
  .page-header {
    align-items: stretch;
    flex-direction: column;

    .el-select {
      width: 100%;
    }
  }

  .status-strip,
  .ontology-layout,
  .form-grid,
  .form-grid.three {
    grid-template-columns: 1fr;
  }

  .status-strip article {
    border-right: 0;
    border-bottom: 1px solid var(--divider);
  }

  .ontology-sidebar {
    padding-right: 0;
    border-right: 0;
    border-bottom: 1px solid var(--divider);
  }

  .ontology-workspace {
    padding-left: 0;
  }

  .workspace-header {
    align-items: stretch;
    flex-direction: column;
  }

  .attribute-row {
    grid-template-columns: 1fr;
  }
}
</style>

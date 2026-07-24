<template>
  <el-dialog
    :model-value="modelValue"
    append-to-body
    class="embedded-workbench-dialog"
    destroy-on-close
    fullscreen
    :show-close="false"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <template #header>
      <header class="workbench-header">
        <div class="workbench-identity">
          <span class="provider-mark" :class="provider.toLowerCase()">
            <Icon :icon="provider === 'CVAT' ? 'lucide:scan-line' : 'lucide:gallery-vertical-end'" />
          </span>
          <div>
            <div class="title-line">
              <el-tag effect="dark" size="small">{{ provider }}</el-tag>
              <h2>{{ title }}</h2>
            </div>
            <p>{{ context }}</p>
          </div>
        </div>
        <div class="workbench-actions">
          <el-button :disabled="!url" @click="reload">
            <Icon icon="lucide:refresh-cw" :size="15" />刷新
          </el-button>
          <el-button :disabled="!url" @click="openExternal">
            <Icon icon="lucide:external-link" :size="15" />外部打开
          </el-button>
          <el-button type="primary" @click="close">
            <Icon icon="lucide:panel-top-close" :size="15" />返回 VisionAI
          </el-button>
        </div>
      </header>
    </template>

    <section class="workbench-shell">
      <div v-if="loading" class="workbench-state">
        <Icon icon="lucide:loader-circle" :size="28" class="spinner" />
        <strong>正在连接 {{ provider }} 工作台</strong>
        <span>首次加载外部工作台可能需要几秒钟。</span>
      </div>
      <div v-if="loadFailed" class="workbench-state failed">
        <Icon icon="lucide:shield-alert" :size="30" />
        <strong>内嵌工作台加载时间过长</strong>
        <span>可以重试，或使用“外部打开”继续当前任务。</span>
        <div>
          <el-button @click="reload">重新加载</el-button>
          <el-button type="primary" @click="openExternal">外部打开</el-button>
        </div>
      </div>
      <iframe
        v-if="modelValue && url"
        :key="frameKey"
        :src="url"
        :title="`${provider} ${title}`"
        allow="clipboard-read; clipboard-write; fullscreen"
        referrerpolicy="same-origin"
        @load="handleLoad"
      />
    </section>
  </el-dialog>
</template>

<script lang="ts" setup>
defineOptions({ name: 'VisionAIEmbeddedWorkbench' })

const props = defineProps<{
  modelValue: boolean
  provider: 'CVAT' | 'FiftyOne'
  title: string
  context: string
  url: string
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
}>()

const loading = ref(true)
const loadFailed = ref(false)
const reloadIndex = ref(0)
let loadTimer: ReturnType<typeof setTimeout> | undefined
const frameKey = computed(() => `${props.url}-${reloadIndex.value}`)

const clearLoadTimer = () => {
  if (loadTimer) clearTimeout(loadTimer)
  loadTimer = undefined
}

const beginLoad = () => {
  clearLoadTimer()
  loading.value = true
  loadFailed.value = false
  loadTimer = setTimeout(() => {
    if (loading.value) loadFailed.value = true
  }, 15_000)
}

const handleLoad = () => {
  clearLoadTimer()
  loading.value = false
  loadFailed.value = false
}

const reload = () => {
  reloadIndex.value += 1
  beginLoad()
}

const openExternal = () => {
  if (props.url) window.open(props.url, '_blank', 'noopener,noreferrer')
}

const close = () => emit('update:modelValue', false)

watch(
  () => [props.modelValue, props.url],
  ([visible, url]) => {
    if (visible && url) beginLoad()
    else clearLoadTimer()
  },
  { immediate: true }
)

onBeforeUnmount(clearLoadTimer)
</script>

<style lang="scss">
.embedded-workbench-dialog {
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
  background: #0b1120;

  .el-dialog__header {
    padding: 0;
    margin: 0;
  }

  .el-dialog__body {
    min-height: 0;
    padding: 0;
    flex: 1;
  }
}

.workbench-header {
  min-height: 72px;
  padding: 12px 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  color: #e5edf8;
  background: linear-gradient(110deg, #111c31, #17243b);
  border-bottom: 1px solid rgb(148 163 184 / 22%);
}

.workbench-identity,
.workbench-actions,
.title-line {
  display: flex;
  align-items: center;
}

.workbench-identity {
  gap: 13px;
  min-width: 0;
}

.workbench-identity > div,
.title-line {
  min-width: 0;
}

.provider-mark {
  width: 42px;
  height: 42px;
  display: grid;
  place-items: center;
  flex: none;
  color: white;
  border-radius: 12px;
  background: #1677ff;
  box-shadow: 0 9px 25px rgb(22 119 255 / 28%);

  &.fiftyone {
    background: linear-gradient(145deg, #ff4f81, #8b5cf6);
    box-shadow: 0 9px 25px rgb(139 92 246 / 28%);
  }
}

.title-line {
  gap: 9px;
}

.title-line h2 {
  max-width: 720px;
  margin: 0;
  overflow: hidden;
  font-size: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workbench-identity p {
  margin: 4px 0 0;
  overflow: hidden;
  font-size: 12px;
  color: #93a4bc;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workbench-actions {
  gap: 9px;
  flex: none;
}

.workbench-shell {
  position: relative;
  height: 100%;
  min-height: 0;
  background: #f5f7fa;
}

.workbench-shell iframe {
  width: 100%;
  height: 100%;
  display: block;
  background: white;
  border: 0;
}

.workbench-state {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 9px;
  color: #334155;
  background: #f8fafc;
}

.workbench-state span {
  font-size: 13px;
  color: #64748b;
}

.workbench-state.failed {
  z-index: 3;
}

.workbench-state.failed > div {
  display: flex;
  gap: 10px;
  margin-top: 8px;
}

.spinner {
  color: #1677ff;
  animation: workbench-spin 1s linear infinite;
}

@keyframes workbench-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 900px) {
  .workbench-header {
    min-height: 0;
    padding: 10px 12px;
    align-items: stretch;
    flex-direction: column;
    gap: 8px;
  }

  .workbench-identity,
  .workbench-actions {
    width: 100%;
  }

  .workbench-actions {
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .workbench-identity p {
    max-width: none;
  }

  .title-line h2 {
    font-size: 15px;
  }
}
</style>

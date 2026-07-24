<template>
  <main class="vision-dashboard">
    <header class="hero">
      <div>
        <span class="eyebrow">VISIONAI CONTROL PLANE</span>
        <h1>计算机视觉生命周期工作台</h1>
        <p>统一治理数据、标注、训练、评估、模型、部署和生产反馈。</p>
      </div>
      <div class="hero__actions">
        <el-button :loading="loading" @click="loadSummary">
          <Icon icon="lucide:refresh-cw" :size="16" />
          刷新
        </el-button>
        <el-button type="primary" disabled>
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
      description="工作台历史数据仍可访问，请检查平台 API 或稍后重试。"
      show-icon
      :closable="false"
    />

    <section class="metrics" aria-label="平台任务概览">
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

    <section class="dashboard-grid">
      <article class="panel lifecycle-panel">
        <header class="panel__header">
          <div>
            <h2>生命周期流水线</h2>
            <p>选择阶段后进入带筛选条件的业务列表。</p>
          </div>
          <span class="status-badge neutral">目标检测 MVP</span>
        </header>
        <div v-if="loading" class="skeleton-list">
          <el-skeleton v-for="item in 5" :key="item" animated :rows="1" />
        </div>
        <ol v-else class="pipeline">
          <li v-for="(stage, index) in summary.lifecycle" :key="stage.key">
            <span class="pipeline__index">{{ index + 1 }}</span>
            <div>
              <strong>{{ stage.label }}</strong>
              <small>{{ stage.count }} 个待处理对象</small>
            </div>
            <Icon icon="lucide:chevron-right" :size="18" />
          </li>
        </ol>
      </article>

      <article class="panel">
        <header class="panel__header">
          <div>
            <h2>集成服务健康</h2>
            <p>服务可用性与业务任务状态分别展示。</p>
          </div>
        </header>
        <div v-if="loading" class="skeleton-list">
          <el-skeleton v-for="item in 3" :key="item" animated :rows="1" />
        </div>
        <ul v-else class="health-list">
          <li v-for="service in summary.services" :key="service.name">
            <span class="service-icon"><Icon icon="lucide:server" :size="18" /></span>
            <div>
              <strong>{{ service.name }}</strong>
              <small>最近一次平台健康检查</small>
            </div>
            <span class="status-badge" :class="service.status === 'UP' ? 'success' : 'danger'">
              {{ service.status }}
            </span>
          </li>
        </ul>
      </article>
    </section>

    <section class="panel empty-state">
      <span class="empty-state__icon"><Icon icon="lucide:workflow" :size="26" /></span>
      <div>
        <h2>从第一条真实闭环开始</h2>
        <p>创建项目后，上传图片并发起 CVAT 标注任务。当前阶段不展示假数据或未接通的业务统计。</p>
      </div>
    </section>
  </main>
</template>

<script lang="ts" setup>
import { getDashboardSummary, type DashboardSummary } from '@/api/ai-platform/dashboard'

defineOptions({ name: 'VisionAIDashboard' })

const loading = ref(false)
const errorMessage = ref('')
const summary = reactive<DashboardSummary>({
  jobs: { running: 0, failed: 0, pending: 0 },
  lifecycle: [],
  services: []
})

const metrics = computed(() => [
  {
    label: '运行中任务',
    value: summary.jobs.running,
    note: '包含排队和执行中',
    icon: 'lucide:activity',
    tone: 'primary'
  },
  {
    label: '待提交任务',
    value: summary.jobs.pending,
    note: '等待进入执行队列',
    icon: 'lucide:clock-3',
    tone: 'warning'
  },
  {
    label: '失败任务',
    value: summary.jobs.failed,
    note: '可查看原因与修复建议',
    icon: 'lucide:triangle-alert',
    tone: 'danger'
  },
  {
    label: '服务健康',
    value: `${summary.services.filter((item) => item.status === 'UP').length}/${summary.services.length}`,
    note: '专业引擎与平台服务',
    icon: 'lucide:heart-pulse',
    tone: 'success'
  }
])

const loadSummary = async () => {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await getDashboardSummary()
    Object.assign(summary, data)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '无法加载工作台数据'
  } finally {
    loading.value = false
  }
}

onMounted(loadSummary)
</script>

<style scoped lang="scss">
.vision-dashboard {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.hero,
.panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-5);
}

.hero {
  min-height: 76px;
  align-items: center;

  h1 {
    margin: var(--space-2) 0 var(--space-1);
    font-size: 24px;
    line-height: 32px;
  }

  p,
  &__actions {
    color: var(--text-tertiary);
  }

  &__actions {
    display: flex;
    gap: var(--space-2);
  }
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
  grid-template-columns: repeat(4, minmax(0, 1fr));
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
  padding: var(--space-5);
  gap: var(--space-3);

  &__icon {
    display: grid;
    width: 40px;
    height: 40px;
    flex: none;
    color: var(--primary);
    background: var(--primary-soft);
    border-radius: var(--radius-md);
    place-items: center;

    &.warning {
      color: var(--warning);
      background: var(--warning-soft);
    }

    &.danger {
      color: var(--danger);
      background: var(--danger-soft);
    }

    &.success {
      color: var(--success);
      background: var(--success-soft);
    }
  }

  div {
    display: flex;
    min-width: 0;
    flex-direction: column;
  }

  small,
  p {
    font-size: 12px;
    color: var(--text-tertiary);
  }

  strong {
    margin: 2px 0;
    font-size: 22px;
    font-variant-numeric: tabular-nums;
  }
}

.dashboard-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.5fr) minmax(320px, 1fr);
  gap: var(--space-6);
}

.panel {
  padding: var(--space-6);

  &__header {
    margin-bottom: var(--space-5);

    h2 {
      margin: 0 0 var(--space-1);
      font-size: 17px;
    }

    p {
      font-size: 12px;
      color: var(--text-tertiary);
    }
  }
}

.pipeline,
.health-list {
  padding: 0;
  margin: 0;
  list-style: none;
}

.pipeline {
  display: grid;
  grid-template-columns: repeat(5, minmax(110px, 1fr));

  li {
    position: relative;
    display: flex;
    min-width: 0;
    padding: var(--space-4);
    align-items: center;
    gap: var(--space-3);
    border: 1px solid var(--divider);
    border-right: 0;

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

.health-list li {
  display: grid;
  min-height: 58px;
  border-bottom: 1px solid var(--divider);
  grid-template-columns: 36px minmax(0, 1fr) auto;
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
    font-size: 11px;
    color: var(--text-tertiary);
  }
}

.service-icon {
  display: grid;
  width: 34px;
  height: 34px;
  color: var(--primary);
  background: var(--primary-soft);
  border-radius: var(--radius-md);
  place-items: center;
}

.status-badge {
  padding: 3px 8px;
  font-size: 11px;
  border-radius: var(--radius-sm);

  &.success {
    color: var(--success);
    background: var(--success-soft);
  }

  &.danger {
    color: var(--danger);
    background: var(--danger-soft);
  }

  &.neutral {
    color: var(--text-secondary);
    background: var(--bg-subtle);
  }
}

.skeleton-list {
  display: grid;
  gap: var(--space-3);
}

.empty-state {
  display: flex;
  margin-top: var(--space-6);
  align-items: center;
  gap: var(--space-4);

  &__icon {
    display: grid;
    width: 48px;
    height: 48px;
    flex: none;
    color: var(--primary);
    background: var(--primary-soft);
    border-radius: var(--radius-lg);
    place-items: center;
  }

  h2 {
    margin: 0 0 var(--space-1);
    font-size: 15px;
  }

  p {
    font-size: 12px;
    color: var(--text-tertiary);
  }
}

@media (width <= 1180px) {
  .metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .dashboard-grid {
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
  .hero {
    align-items: flex-start;
    flex-direction: column;
  }

  .metrics {
    grid-template-columns: 1fr;
  }
}
</style>

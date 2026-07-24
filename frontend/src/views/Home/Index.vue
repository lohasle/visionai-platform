<template>
  <main class="nimbus-home">
    <header class="page-heading">
      <div>
        <span class="eyebrow">NIMBUS WORKSPACE</span>
        <h1>{{ greeting }}，{{ username }}</h1>
        <p>从这里进入 VisionAI 项目、数据、训练、评测与部署等平台能力。</p>
      </div>
      <div class="page-heading__actions">
        <el-button tag="a" :href="apiDocsUrl" target="_blank" rel="noreferrer">
          接口文档
        </el-button>
      </div>
    </header>

    <section class="metric-grid" aria-label="技术基线">
      <article v-for="item in metrics" :key="item.label" class="metric-card">
        <span class="metric-card__icon" aria-hidden="true">
          <Icon :icon="item.icon" :size="20" />
        </span>
        <div class="metric-card__body">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.note }}</small>
        </div>
      </article>
    </section>

    <section class="workspace">
      <header class="section-heading">
        <div>
          <h2>平台能力</h2>
          <p>复用框架已有中心；模板模块仅保留清晰的扩展边界。</p>
        </div>
        <span class="section-badge">2 个平台底座 · 3 个扩展边界</span>
      </header>

      <div class="domain-grid">
        <article v-for="domain in domains" :key="domain.name" class="domain-card">
          <span class="domain-card__icon" aria-hidden="true">
            <Icon :icon="domain.icon" :size="20" />
          </span>
          <span class="domain-card__content">
            <strong>{{ domain.name }}</strong>
            <small>{{ domain.description }}</small>
          </span>
          <span class="domain-card__state" :class="{ template: domain.template }">
            {{ domain.template ? '扩展模板' : 'Health 已接入' }}
          </span>
        </article>
      </div>
    </section>

    <section class="foundation-grid">
      <article class="workspace foundation-card">
        <header class="section-heading compact">
          <div>
            <h2>默认基础设施</h2>
            <p>开发与验收使用同一套默认数据库基线。</p>
          </div>
        </header>
        <dl class="foundation-list">
          <div v-for="item in foundations" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
          </div>
        </dl>
      </article>

      <article class="workspace foundation-card">
        <header class="section-heading compact">
          <div>
            <h2>工程约束</h2>
            <p>保持框架干净，业务能力按需扩展。</p>
          </div>
        </header>
        <ul class="principle-list">
          <li v-for="item in principles" :key="item">
            <Icon icon="lucide:check" :size="16" aria-hidden="true" />
            <span>{{ item }}</span>
          </li>
        </ul>
      </article>
    </section>
  </main>
</template>

<script lang="ts" setup>
import { useUserStore } from '@/store/modules/user'

defineOptions({ name: 'Index' })

const userStore = useUserStore()
const username = computed(() => userStore.getUser.nickname || '平台管理员')
const hour = new Date().getHours()
const greeting = computed(() => (hour < 12 ? '早上好' : hour < 18 ? '下午好' : '晚上好'))

const apiDocsUrl = `${import.meta.env.VITE_BASE_URL}/swagger/index.html`

const metrics = [
  {
    label: '数据底座',
    value: 'MySQL 8.4',
    note: '默认开发与部署基线',
    icon: 'lucide:database'
  },
  {
    label: '后端基线',
    value: 'Go 1.26',
    note: 'Gin 1.12',
    icon: 'lucide:braces'
  },
  {
    label: '前端基线',
    value: 'Vue 3',
    note: 'TypeScript · Vite',
    icon: 'lucide:panels-top-left'
  },
  {
    label: '接口规范',
    value: 'OpenAPI',
    note: '统一接口说明',
    icon: 'lucide:file-code-2'
  }
]

const domains = [
  {
    name: '系统中心',
    description: '租户、运营账号、认证与权限入口',
    icon: 'lucide:shield-check'
  },
  {
    name: '基础设施',
    description: '配置、日志、任务、文件与监控边界',
    icon: 'lucide:server-cog'
  },
  {
    name: '应用中心',
    description: '应用版本与渠道的扩展边界',
    icon: 'lucide:app-window',
    template: true
  },
  {
    name: '即时通信',
    description: '会话、消息与推送的扩展边界',
    icon: 'lucide:messages-square',
    template: true
  },
  {
    name: 'App 聚合层',
    description: '面向客户端的用例编排边界',
    icon: 'lucide:smartphone',
    template: true
  }
]

const foundations = [
  { label: '数据库', value: 'MySQL 8.4' },
  { label: '缓存', value: '按需接入 Redis' },
  { label: '接口文档', value: 'OpenAPI 3' },
  { label: '可观测性', value: 'Metrics · Trace ID' }
]

const principles = [
  '复用框架已有业务中心',
  '扩展模块默认只提供健康检查',
  '不预建未经确认的业务实体'
]
</script>

<style scoped lang="scss">
.nimbus-home {
  min-height: 100%;
  padding: var(--app-content-padding);
  color: var(--text-primary);
  background: var(--bg-canvas);
}

.page-heading,
.section-heading {
  display: flex;
  gap: var(--space-6);
  align-items: flex-start;
  justify-content: space-between;

  h1,
  h2 {
    text-wrap: balance;
  }

  h1 {
    margin: var(--space-2) 0 var(--space-1);
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
  }

  h2 {
    margin: 0 0 var(--space-1);
    font-size: 18px;
    font-weight: 600;
    line-height: 26px;
  }

  p {
    font-size: 13px;
    line-height: 20px;
    color: var(--text-tertiary);
  }
}

.page-heading {
  min-height: 72px;
  align-items: center;

  &__actions {
    display: flex;
    gap: var(--space-2);
  }
}

.eyebrow {
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  letter-spacing: 0.12em;
  color: var(--primary);
}

.metric-grid {
  display: grid;
  margin: var(--space-6) 0;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}

.metric-card {
  display: flex;
  min-width: 0;
  padding: var(--space-5);
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
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
  }

  &__body {
    display: flex;
    min-width: 0;
    flex-direction: column;
  }

  span,
  small {
    overflow: hidden;
    font-size: 12px;
    color: var(--text-tertiary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  strong {
    margin: 2px 0;
    overflow: hidden;
    font-size: 20px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.workspace {
  padding: var(--space-6);
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
}

.section-heading {
  margin-bottom: var(--space-5);

  &.compact {
    margin-bottom: var(--space-4);
  }
}

.section-badge {
  padding: var(--space-1) var(--space-2);
  font-size: 12px;
  line-height: 18px;
  color: var(--text-secondary);
  white-space: nowrap;
  background: var(--bg-subtle);
  border-radius: var(--radius-sm);
}

.domain-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-top: 1px solid var(--divider);
  border-left: 1px solid var(--divider);
}

.domain-card {
  display: grid;
  min-width: 0;
  min-height: 76px;
  padding: var(--space-4);
  color: var(--text-primary);
  text-align: left;
  cursor: default;
  background: var(--bg-surface);
  border: 0;
  border-right: 1px solid var(--divider);
  border-bottom: 1px solid var(--divider);
  grid-template-columns: 36px minmax(0, 1fr) auto;
  gap: var(--space-3);
  align-items: center;

  &__icon {
    display: grid;
    width: 36px;
    height: 36px;
    color: var(--primary);
    background: var(--primary-soft);
    border-radius: var(--radius-md);
    place-items: center;
  }

  &__content {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 2px;

    strong {
      overflow: hidden;
      font-size: 14px;
      font-weight: 500;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    small {
      overflow: hidden;
      font-size: 12px;
      color: var(--text-tertiary);
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  &__state {
    padding: 2px var(--space-2);
    font-size: 12px;
    color: var(--success);
    white-space: nowrap;
    background: var(--success-soft);
    border-radius: var(--radius-sm);

    &.template {
      color: var(--text-tertiary);
      background: var(--bg-subtle);
    }
  }
}

.foundation-grid {
  display: grid;
  margin-top: var(--space-6);
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-6);
}

.foundation-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-top: 1px solid var(--divider);
  border-left: 1px solid var(--divider);

  div {
    padding: var(--space-3) var(--space-4);
    border-right: 1px solid var(--divider);
    border-bottom: 1px solid var(--divider);
  }

  dt {
    margin-bottom: var(--space-1);
    font-size: 12px;
    color: var(--text-tertiary);
  }

  dd {
    font-size: 14px;
    font-weight: 500;
  }
}

.principle-list {
  display: flex;
  padding: 0;
  margin: 0;
  list-style: none;
  flex-direction: column;

  li {
    display: flex;
    min-height: 44px;
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--divider);
    gap: var(--space-2);
    align-items: center;

    &:last-child {
      border-bottom: 0;
    }
  }

  svg {
    color: var(--success);
  }

  span {
    font-size: 14px;
  }
}

@media (width <= 1100px) {
  .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 760px) {
  .page-heading,
  .section-heading {
    align-items: flex-start;
    flex-direction: column;
  }

  .page-heading__actions {
    width: 100%;
  }

  .metric-grid,
  .domain-grid,
  .foundation-grid {
    grid-template-columns: 1fr;
  }

  .domain-card {
    grid-template-columns: 36px minmax(0, 1fr) auto;

    > svg {
      display: none;
    }
  }
}
</style>

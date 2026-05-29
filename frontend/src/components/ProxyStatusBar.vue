<script setup lang="ts">
import { computed, ref, h, nextTick } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import { useAppStore, CLI_TYPES } from '../stores/app'
import type { CLITypeMeta, CLIType } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { maskKey, copyToClipboard, getErrorMessage } from '../utils'

const store = useAppStore()
const message = useAppMessage()

const cliLabels: Record<string, string> = Object.fromEntries(
  CLI_TYPES.map(t => [t.key, t.label])
)

const isRestarting = ref(false)

const statusText = computed(() => {
  if (isRestarting.value) return '重启中'
  return store.proxyStatus.running ? '运行中' : '已停止'
})

const statusType = computed(() => {
  if (isRestarting.value) return 'warning'
  return store.proxyStatus.running ? 'success' : 'error'
})

const showConfigModal = ref(false)
const showFullKey = ref(false)
const spinningMap = ref<Record<string, boolean>>({})

const proxyEndpoints = computed(() => {
  const port = store.proxyStatus.port || 18900
  const usedTypes = new Set<string>()
  for (const p of store.providers) {
    if (p.cli_types?.length) {
      p.cli_types.forEach(t => usedTypes.add(t))
    } else {
      CLI_TYPES.forEach(t => usedTypes.add(t.key))
    }
  }
  return CLI_TYPES
    .filter(t => usedTypes.has(t.key))
    .map(ep => ({
      ...ep,
      url: `http://127.0.0.1:${port}${ep.path}`,
    }))
})

const tableData = computed(() => proxyEndpoints.value.map(ep => ({
  key: ep.key,
  cli: ep.label,
  url: ep.url,
})))

const tableColumns = [
  {
    title: 'CLI',
    key: 'cli',
    width: 100,
    render: (row: { key: string; cli: string }) => {
      return h('div', { style: 'display: flex; align-items: center; gap: 6px' }, [
        h(CLIIcon, { type: row.key as CLIType, size: 16 }),
        h('span', { style: 'font-weight: 500' }, row.cli),
      ])
    },
  },
  {
    title: 'Base URL',
    key: 'url',
    render: (row: { key: string; url: string }) => {
      return h('div', { style: 'display: flex; align-items: center; gap: 6px' }, [
        h('code', { style: 'font-size: 12px; word-break: break-all; flex: 1' }, row.url),
        h('button', {
          style: 'background: none; border: none; cursor: pointer; color: #666; padding: 2px; display: flex; align-items: center;',
          onClick: () => handleCopyToClipboard(row.url),
          title: '复制',
        }, h('svg', { xmlns: 'http://www.w3.org/2000/svg', viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': '2', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', width: '14', height: '14', innerHTML: '<path d="M20 8H10c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h10c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2Z"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>' })),
      ])
    },
  },
]

const apiKey = computed(() => store.proxyStatus.proxy_auth_token || '')

const maskedKey = computed(() => maskKey(apiKey.value))

async function handleToggle() {
  if (store.proxyStatus.running) {
    await store.stopProxy()
  } else {
    await store.startProxy()
  }
}

async function handleRestart() {
  isRestarting.value = true
  try {
    await store.restartProxy()
  } finally {
    isRestarting.value = false
  }
}

async function handleCopyToClipboard(text: string) {
  const success = await copyToClipboard(text)
  if (success) {
    message.success('已复制')
  } else {
    message.error('复制失败')
  }
}

async function handleWriteCLI(cliType: string) {
  // Reset then re-trigger spin animation
  spinningMap.value = { ...spinningMap.value, [cliType]: false }
  await nextTick()
  spinningMap.value = { ...spinningMap.value, [cliType]: true }

  try {
    await store.writeCLIConfig(cliType)
    message.success(`${cliLabels[cliType] || cliType} 配置已更新`)
  } catch (e: any) {
    message.error(getErrorMessage(e, `${cliLabels[cliType] || cliType} 更新失败`))
  }
}

function onSpinEnd(cliType: string) {
  spinningMap.value = { ...spinningMap.value, [cliType]: false }
}

function showConfig() {
  showFullKey.value = false
  showConfigModal.value = true
  // 移除按钮焦点，避免关闭弹窗后按钮仍有高亮
  nextTick(() => {
    document.activeElement instanceof HTMLElement && document.activeElement.blur()
  })
}

async function copyConfig() {
  if (proxyEndpoints.value.length === 0) return
  const lines = proxyEndpoints.value.map(ep => `${ep.label} Base URL: ${ep.url}`)
  lines.push(`API Key: ${apiKey.value}`)
  await handleCopyToClipboard(lines.join('\n'))
}
</script>

<template>
  <n-card size="small" :bordered="false" style="margin-bottom: 8px">
    <div class="status-row">
      <div style="display: flex; align-items: center; gap: 10px">
        <span class="status-dot" :class="{ active: store.proxyStatus.running, restarting: isRestarting }"></span>
        <span>代理服务</span>
        <n-tag :type="statusType" size="small" style="min-width: 48px; text-align: center">{{ statusText }}</n-tag>
      </div>
      <div style="display: flex; align-items: center; gap: 8px">
        <n-button type="default" size="tiny" @mousedown.prevent @click="showConfig()">
          <template #icon><n-icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg></n-icon></template>
          查看配置
        </n-button>
        <n-button
          secondary
          type="warning"
          size="tiny"
          :disabled="!store.proxyStatus.running || isRestarting"
          :loading="isRestarting"
          class="restart-btn"
          @click="handleRestart"
        >
          <template #icon>
            <n-icon>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14">
                <path d="M1 4v6h6M23 20v-6h-6"/>
                <path d="M20.49 9A9 9 0 005.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 013.51 15"/>
              </svg>
            </n-icon>
          </template>
          重启服务
        </n-button>
        <n-switch
          :value="store.proxyStatus.running"
          @update:value="handleToggle"
          size="small"
        />
      </div>
    </div>

    <div class="cli-endpoints-row">
      <div
        v-for="ep in proxyEndpoints"
        :key="ep.key"
        class="cli-endpoint-card"
      >
        <div class="cli-endpoint-content">
          <div class="cli-endpoint-info">
            <CLIIcon :type="ep.key as CLIType" :size="18" />
            <span class="cli-endpoint-label">{{ ep.label }}</span>
          </div>
          <button
            class="cli-write-btn"
            @mousedown.prevent @click="handleWriteCLI(ep.key)"
          >
            <svg
              class="cli-write-icon"
              :class="{ spinning: spinningMap[ep.key] }"
              @animationend="onSpinEnd(ep.key)"
              viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="13" height="13"
            ><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>
            <span>更新</span>
          </button>
        </div>
      </div>
    </div>

    <n-modal
      :show="showConfigModal"
      @update:show="(v: boolean) => showConfigModal = v"
      title="查看配置"
      preset="card"
      style="width: 500px"
    >
      <div style="display: flex; flex-direction: column; gap: 16px">
        <n-data-table
          :columns="tableColumns"
          :data="tableData"
          :bordered="false"
          size="small"
        />
        <div>
          <n-text depth="3" style="font-size: 12px; font-weight: 500">API Key</n-text>
          <n-card size="small" style="margin-top: 4px">
            <div style="display: flex; align-items: center; gap: 8px">
              <n-text code style="flex: 1; word-break: break-all">{{ showFullKey ? apiKey : maskedKey }}</n-text>
              <n-button v-if="apiKey" text size="tiny" @click="showFullKey = !showFullKey" :style="{ color: showFullKey ? '#d03050' : '#999' }">
                <template #icon>
                  <n-icon>
                    <svg v-if="showFullKey" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                      <circle cx="12" cy="12" r="3"/>
                    </svg>
                    <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
                      <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/>
                      <line x1="1" y1="1" x2="23" y2="23"/>
                    </svg>
                  </n-icon>
                </template>
              </n-button>
              <n-button v-if="apiKey" type="success" text size="tiny" @click="handleCopyToClipboard(apiKey)">复制</n-button>
            </div>
          </n-card>
        </div>
        <div style="display: flex; justify-content: flex-end">
          <n-button type="primary" @click="copyConfig">复制全部</n-button>
        </div>
      </div>
    </n-modal>
  </n-card>
</template>

<style scoped>
.status-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  height: 28px;
}
.restart-btn {
  min-width: 64px;
  height: 22px !important;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--n-error-color, #d03050);
  flex-shrink: 0;
  transition: background 0.3s;
}
.status-dot.active {
  background: var(--n-success-color, #18a058);
  box-shadow: 0 0 0 0 var(--n-success-color, #18a058);
  animation: pulse-green 2s ease-in-out infinite;
}
.status-dot.restarting {
  background: var(--n-warning-color, #f0a020);
  box-shadow: 0 0 0 0 var(--n-warning-color, #f0a020);
  animation: pulse-yellow 1s ease-in-out infinite;
}
@keyframes pulse-green {
  0%, 100% { box-shadow: 0 0 0 0 rgba(24, 160, 88, 0.4); }
  50% { box-shadow: 0 0 0 6px rgba(24, 160, 88, 0); }
}
@keyframes pulse-yellow {
  0%, 100% { box-shadow: 0 0 0 0 rgba(240, 160, 32, 0.4); }
  50% { box-shadow: 0 0 0 6px rgba(240, 160, 32, 0); }
}

/* CLI endpoint cards row */
.cli-endpoints-row {
  display: flex;
  gap: 8px;
}

.cli-endpoint-card {
  width: 164px;
  border-radius: 8px;
  border: 1px solid var(--app-border-2, #efeff5);
  background: var(--app-bg-2, #fafafa);
  padding: 10px 12px;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  cursor: default;
}
.cli-endpoint-card:hover {
  border-color: var(--n-primary-color-hover, #36ad6a);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  transform: translateY(-1px);
}

.cli-endpoint-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.cli-endpoint-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cli-endpoint-label {
  font-weight: 600;
  font-size: 13px;
}

/* Update button */
.cli-write-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--app-border, #e0e0e6);
  border-radius: 6px;
  background: var(--app-bg-1, #ffffff);
  color: var(--app-text-3, #8a8f8d);
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  line-height: 1;
  white-space: nowrap;
  flex-shrink: 0;
}
.cli-write-btn:hover {
  color: var(--n-primary-color, #18a058);
  border-color: var(--n-primary-color, #18a058);
  background: rgba(24, 160, 88, 0.06);
  box-shadow: 0 1px 4px rgba(24, 160, 88, 0.15);
}
.cli-write-btn:active {
  transform: scale(0.95);
  box-shadow: none;
}
.cli-write-btn .cli-write-icon {
  flex-shrink: 0;
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}
.cli-write-btn:hover .cli-write-icon:not(.spinning) {
  transform: rotate(-45deg);
}
.cli-write-btn .cli-write-icon.spinning {
  animation: spin-once 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}
@keyframes spin-once {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

</style>

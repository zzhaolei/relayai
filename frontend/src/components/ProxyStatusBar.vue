<script setup lang="ts">
import { computed, ref, h, nextTick } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import { useAppStore, CLI_TYPES } from '../stores/app'
import type { CLITypeMeta, CLIType } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { maskKey, copyToClipboard, getErrorMessage } from '../utils'

const store = useAppStore()
const message = useAppMessage()

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
  try {
    await store.writeCLIConfig(cliType)
    message.success(`${cliType} 配置已写入`)
  } catch (e: any) {
    message.error(getErrorMessage(e, '写入失败'))
  }
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

    <div style="display: flex; gap: 8px">
      <n-card
        v-for="ep in proxyEndpoints"
        :key="ep.key"
        size="small"
        style="width: 164px; cursor: default"
        hoverable
      >
        <div style="display: flex; align-items: center; justify-content: space-between">
          <div style="display: flex; align-items: center; gap: 6px">
            <CLIIcon :type="ep.key as CLIType" :size="14" />
            <span style="font-weight: 500">{{ ep.label }}</span>
          </div>
          <n-button
            type="primary"
            size="tiny"
            class="cli-write-btn"
            @mousedown.prevent @click="handleWriteCLI(ep.key)"
          >
            <template #icon><n-icon><svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14"><path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z"/></svg></n-icon></template>
            写入
          </n-button>
        </div>
      </n-card>
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
.cli-write-btn {
  border-radius: 4px;
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
}





@keyframes pulse-yellow {
  0%, 100% { box-shadow: 0 0 0 0 rgba(240, 160, 32, 0.4); }
  50% { box-shadow: 0 0 0 6px rgba(240, 160, 32, 0); }
}
</style>

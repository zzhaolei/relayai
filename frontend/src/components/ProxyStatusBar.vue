<script setup lang="ts">
import { computed, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useAppStore, CLI_TYPES } from '../stores/app'
import type { CLITypeMeta } from '../stores/app'
import CLIIcon from './CLIIcon.vue'

const store = useAppStore()

const isRestarting = ref(false)

const statusText = computed(() => {
  if (isRestarting.value) return '重启中'
  return store.proxyStatus.running ? '运行中' : '已停止'
})

const statusColor = computed(() => {
  if (isRestarting.value) return 'orange'
  return store.proxyStatus.running ? 'green' : 'red'
})

const showConfigModal = ref(false)
const selectedEndpoint = ref<CLITypeMeta & { url: string } | null>(null)
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

const apiKey = computed(() => {
  const providers = store.providers
  if (providers.length === 0) return ''
  return providers[0].api_key || ''
})

const maskedKey = computed(() => {
  const key = apiKey.value
  if (!key) return '未配置'
  if (key.length <= 8) return '****'
  return key.slice(0, 4) + '****' + key.slice(-4)
})

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

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    Message.success({ content: '已复制', duration: 1500 })
  } catch {
    Message.error('复制失败')
  }
}

async function handleWriteCLI(cliType: string) {
  try {
    await store.writeCLIConfig(cliType)
    Message.success({ content: `${cliType} 配置已写入`, duration: 2000 })
  } catch (e: any) {
    Message.error(e?.message || '写入失败')
  }
}

function showConfig(ep: CLITypeMeta & { url: string }) {
  selectedEndpoint.value = ep
  showFullKey.value = false
  showConfigModal.value = true
}

function closeConfigModal() {
  showConfigModal.value = false
  selectedEndpoint.value = null
}

async function copyConfig() {
  if (!selectedEndpoint.value) return
  const text = `Base URL: ${selectedEndpoint.value.url}\nAPI Key: ${apiKey.value}`
  await copyToClipboard(text)
}
</script>

<template>
  <div class="proxy-status-bar">
    <div class="status-row">
      <div class="status-left">
        <div class="status-dot" :class="{ active: store.proxyStatus.running, restarting: isRestarting }" />
        <span class="status-label">代理服务</span>
        <a-tag :color="statusColor" size="small">
          {{ statusText }}
        </a-tag>
      </div>
      <div class="status-right">
        <a-button
          type="text"
          size="mini"
          @click="handleRestart"
          :disabled="!store.proxyStatus.running || isRestarting"
          :loading="isRestarting"
        >
          重启服务
        </a-button>
        <a-switch
          :model-value="store.proxyStatus.running"
          @change="handleToggle"
          size="small"
        />
      </div>
    </div>
    <div class="endpoints-row">
      <div v-for="ep in proxyEndpoints" :key="ep.key" class="endpoint-card">
        <div class="endpoint-top">
          <CLIIcon :type="ep.key as 'claude' | 'codex'" :size="14" />
          <span class="endpoint-label">{{ ep.label }}</span>
        </div>
        <div class="endpoint-actions">
          <a-button type="text" size="mini" @click="showConfig(ep)">配置</a-button>
          <a-button type="primary" size="mini" @click="handleWriteCLI(ep.key)">写入</a-button>
        </div>
      </div>
    </div>

    <a-modal
      v-model:visible="showConfigModal"
      :title="selectedEndpoint?.label + ' 配置'"
      :width="400"
      :footer="false"
    >
      <div v-if="selectedEndpoint" class="config-content">
        <div class="config-item">
          <div class="config-label">Base URL</div>
          <div class="config-value-row">
            <code class="config-value">{{ selectedEndpoint.url }}</code>
            <a-button type="text" size="mini" @click="copyToClipboard(selectedEndpoint!.url)">复制</a-button>
          </div>
        </div>
        <div class="config-item">
          <div class="config-label">API Key</div>
          <div class="config-value-row">
            <code class="config-value">{{ showFullKey ? apiKey : maskedKey }}</code>
            <a-button v-if="apiKey" type="text" size="mini" @click="showFullKey = !showFullKey">
              {{ showFullKey ? '隐藏' : '查看' }}
            </a-button>
            <a-button type="text" size="mini" @click="copyToClipboard(apiKey)" :disabled="!apiKey">复制</a-button>
          </div>
        </div>
        <div class="config-actions">
          <a-button type="primary" @click="copyConfig">复制全部</a-button>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.proxy-status-bar {
  padding: 12px 16px;
  background: var(--color-bg-2);
  border-bottom: 1px solid var(--color-border);
}
.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.status-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-danger-light-4);
  transition: background 0.3s;
}
.status-dot.active {
  background: var(--color-success-light-4);
  box-shadow: 0 0 6px var(--color-success-light-3);
}
.status-dot.restarting {
  background: var(--color-warning-light-4);
  box-shadow: 0 0 6px var(--color-warning-light-3);
  animation: pulse 1s infinite;
}
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
.status-label {
  font-size: 13px;
  font-weight: 500;
}
.status-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
.endpoints-row {
  display: flex;
  gap: 8px;
}
.endpoint-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  background: var(--color-fill-1);
  border-radius: 8px;
  padding: 8px 10px;
  border: 1px solid var(--color-border-2);
  min-height: 56px;
  width: 140px;
}
.endpoint-top {
  display: flex;
  align-items: center;
  gap: 6px;
}
.endpoint-label {
  font-size: 13px;
  font-weight: 500;
}
.endpoint-actions {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  margin-top: 6px;
}
.config-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.config-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.config-label {
  font-size: 12px;
  color: var(--color-text-3);
  font-weight: 500;
}
.config-value-row {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--color-fill-1);
  padding: 8px;
  border-radius: 4px;
}
.config-value {
  flex: 1;
  font-size: 13px;
  font-family: monospace;
  color: var(--color-text-2);
  word-break: break-all;
}
.config-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 8px;
  border-top: 1px solid var(--color-border-2);
}
</style>

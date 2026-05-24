<script setup lang="ts">
import { computed, ref } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import { useAppStore, CLI_TYPES } from '../stores/app'
import type { CLITypeMeta } from '../stores/app'
import CLIIcon from './CLIIcon.vue'

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
    message.success('已复制')
  } catch {
    message.error('复制失败')
  }
}

async function handleWriteCLI(cliType: string) {
  try {
    await store.writeCLIConfig(cliType)
    message.success(`${cliType} 配置已写入`)
  } catch (e: any) {
    message.error(e?.message || '写入失败')
  }
}

function showConfig(ep: CLITypeMeta & { url: string }) {
  selectedEndpoint.value = ep
  showFullKey.value = false
  showConfigModal.value = true
}

async function copyConfig() {
  if (!selectedEndpoint.value) return
  const text = `Base URL: ${selectedEndpoint.value.url}\nAPI Key: ${apiKey.value}`
  await copyToClipboard(text)
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
        <n-button
          text
          size="tiny"
          :disabled="!store.proxyStatus.running || isRestarting"
          :loading="isRestarting"
          class="restart-btn"
          @click="handleRestart"
        >
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
        style="width: 140px"
        hoverable
      >
        <div style="display: flex; flex-direction: column; gap: 6px">
          <div style="display: flex; align-items: center; gap: 6px">
            <CLIIcon :type="ep.key as 'claude' | 'codex'" :size="14" />
            <span style="font-weight: 500">{{ ep.label }}</span>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 4px">
            <n-button text size="tiny" @click="showConfig(ep)">配置</n-button>
            <n-button type="primary" size="tiny" @click="handleWriteCLI(ep.key)">写入</n-button>
          </div>
        </div>
      </n-card>
    </div>

    <n-modal
      :show="showConfigModal"
      @update:show="(v: boolean) => showConfigModal = v"
      :title="selectedEndpoint?.label + ' 配置'"
      preset="card"
      style="width: 400px"
    >
      <div v-if="selectedEndpoint" style="display: flex; flex-direction: column; gap: 16px">
        <div>
          <n-text depth="3" style="font-size: 12px; font-weight: 500">Base URL</n-text>
          <n-card size="small" style="margin-top: 4px">
            <div style="display: flex; align-items: center; gap: 8px">
              <n-text code style="flex: 1; word-break: break-all">{{ selectedEndpoint.url }}</n-text>
              <n-button text size="tiny" @click="copyToClipboard(selectedEndpoint!.url)">复制</n-button>
            </div>
          </n-card>
        </div>
        <div>
          <n-text depth="3" style="font-size: 12px; font-weight: 500">API Key</n-text>
          <n-card size="small" style="margin-top: 4px">
            <div style="display: flex; align-items: center; gap: 8px">
              <n-text code style="flex: 1; word-break: break-all">{{ showFullKey ? apiKey : maskedKey }}</n-text>
              <n-button v-if="apiKey" text size="tiny" @click="showFullKey = !showFullKey">
                {{ showFullKey ? '隐藏' : '查看' }}
              </n-button>
              <n-button text size="tiny" :disabled="!apiKey" @click="copyToClipboard(apiKey)">复制</n-button>
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
</style>

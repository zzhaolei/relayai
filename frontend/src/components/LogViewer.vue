<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import { useAppStore } from '../stores/app'
import type { CLIType } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { copyToClipboard, formatDuration } from '../utils'

const store = useAppStore()
const message = useAppMessage()
const autoRefresh = ref(true)
let timer: ReturnType<typeof setInterval> | null = null

const statusType = (code: number): 'success' | 'warning' | 'error' => {
  if (code >= 200 && code < 300) return 'success'
  if (code >= 400 && code < 500) return 'warning'
  return 'error'
}

const formatTime = (ms: number) => {
  const d = new Date(ms)
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}

async function copyText(text: string) {
  const success = await copyToClipboard(text)
  if (success) {
    message.success('已复制')
  } else {
    message.error('复制失败')
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  timer = setInterval(() => store.fetchLogs(), 3000)
}

function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

function toggleAutoRefresh() {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
}

onMounted(() => {
  store.fetchLogs()
  if (autoRefresh.value) startAutoRefresh()
})

onUnmounted(() => stopAutoRefresh())
</script>

<template>
  <div style="height: 100%; display: flex; flex-direction: column">
    <div style="display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--app-border)">
      <div style="display: flex; align-items: center; gap: 8px">
        <n-text strong style="font-size: 16px">请求日志</n-text>
        <n-text depth="3" style="font-size: 12px">({{ store.logsSizeKB }} KB)</n-text>
        <n-text v-if="store.totalTokens > 0" depth="3" style="font-size: 12px">· {{ store.totalTokens }} tokens</n-text>
      </div>
      <div style="display: flex; gap: 8px">
        <n-button size="tiny" :type="autoRefresh ? 'primary' : 'default'" @click="toggleAutoRefresh">
          {{ autoRefresh ? '自动刷新中' : '自动刷新' }}
        </n-button>
        <n-button size="tiny" @click="store.fetchLogs">刷新</n-button>
        <n-popconfirm @positive-click="store.clearLogs">
          <template #trigger>
            <n-button size="tiny" type="error">清空</n-button>
          </template>
          确定清空所有日志（{{ store.logsSizeKB }} KB）？
        </n-popconfirm>
      </div>
    </div>

    <div style="flex: 1; overflow-y: auto">
      <n-empty v-if="store.logs.length === 0" description="暂无日志" style="padding: 40px 0" />

      <div
        v-for="log in store.logs"
        :key="log.id"
        :style="{
          padding: '8px 20px',
          borderBottom: '1px solid var(--app-border-2)',
          background: log.error ? 'rgba(208, 48, 80, 0.06)' : undefined,
        }"
      >
        <div style="display: flex; flex-direction: column; gap: 4px">
          <div style="display: flex; align-items: center; gap: 8px; flex-wrap: nowrap">
            <n-text depth="3" style="font-size: 12px; font-family: monospace; flex-shrink: 0">
              {{ formatTime(log.time) }}
            </n-text>
            <n-tag :type="statusType(log.status_code)" size="small">{{ log.status_code }}</n-tag>
            <n-tag size="small" type="info">{{ log.method }}</n-tag>
            <n-text code style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; font-size: 12px">
              {{ log.path }}
            </n-text>
            <n-tag v-if="log.model" size="small" style="flex-shrink: 0">{{ log.model }}</n-tag>
            <n-text v-if="log.total_tokens > 0" depth="3" style="font-size: 11px; font-family: monospace; flex-shrink: 0">
              {{ log.prompt_tokens }}+{{ log.completion_tokens }}={{ log.total_tokens }}
            </n-text>
            <n-text depth="3" style="font-size: 12px; font-family: monospace; flex-shrink: 0">
              {{ formatDuration(log.duration_ms) }}
            </n-text>
            <CLIIcon v-if="log.cli_type" :type="log.cli_type as CLIType" :size="12" />
            <n-text v-if="log.provider" depth="3" style="font-size: 12px; flex-shrink: 0">
              {{ log.provider }}
            </n-text>
          </div>

          <n-card v-if="log.error" size="small" :bordered="false" style="background: var(--app-fill-1)">
            <n-text type="error" code style="font-size: 12px; word-break: break-all">{{ log.error }}</n-text>
          </n-card>

          <n-card v-if="log.response_body" size="small" :bordered="false" style="background: var(--app-fill-1)">
            <div style="display: flex; align-items: flex-start; gap: 8px">
              <pre style="flex: 1; margin: 0; font-size: 12px; font-family: monospace; white-space: pre-wrap; word-break: break-all; max-height: 120px; overflow-y: auto">{{ log.response_body }}</pre>
              <n-button text size="tiny" @click="copyText(log.response_body!)">复制响应</n-button>
            </div>
          </n-card>
        </div>
      </div>
    </div>
  </div>
</template>

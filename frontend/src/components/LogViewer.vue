<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useAppStore } from '../stores/app'
import CLIIcon from './CLIIcon.vue'

const store = useAppStore()
const autoRefresh = ref(true)
let timer: ReturnType<typeof setInterval> | null = null

const statusColor = (code: number) => {
  if (code >= 200 && code < 300) return 'green'
  if (code >= 400 && code < 500) return 'orange'
  return 'red'
}

const formatTime = (ms: number) => {
  const d = new Date(ms)
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}

const formatDuration = (ms: number) => {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

async function copyError(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    Message.success({ content: '已复制', duration: 1500 })
  } catch {
    Message.error('复制失败')
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
  <div class="log-viewer">
    <div class="log-header">
      <h3>请求日志</h3>
      <div class="log-actions">
        <a-button size="mini" :type="autoRefresh ? 'primary' : 'outline'" @click="toggleAutoRefresh">
          {{ autoRefresh ? '自动刷新中' : '自动刷新' }}
        </a-button>
        <a-button size="mini" @click="store.fetchLogs">刷新</a-button>
        <a-popconfirm content="确定清空所有日志？" @ok="store.clearLogs">
          <a-button size="mini" status="danger">清空</a-button>
        </a-popconfirm>
      </div>
    </div>

    <div class="log-list">
      <div v-if="store.logs.length === 0" class="log-empty">
        <a-empty description="暂无日志" :style="{ padding: '40px 0' }" />
      </div>

      <div v-for="log in store.logs" :key="log.id" class="log-item" :class="{ 'log-item--error': log.error }">
        <div class="log-row">
          <span class="log-time">{{ formatTime(log.time) }}</span>
          <a-tag :color="statusColor(log.status_code)" size="small">{{ log.status_code }}</a-tag>
          <a-tag size="small" color="arcoblue">{{ log.method }}</a-tag>
          <span class="log-path">{{ log.path }}</span>
          <span v-if="log.model" class="log-model">{{ log.model }}</span>
          <span class="log-duration">{{ formatDuration(log.duration_ms) }}</span>
          <span v-if="log.cli_type" class="log-cli">
            <CLIIcon :type="log.cli_type as 'claude' | 'codex'" :size="12" />
          </span>
          <span v-if="log.provider" class="log-provider">{{ log.provider }}</span>
        </div>
        <div v-if="log.error" class="log-error">
          <code>{{ log.error }}</code>
          <a-button type="text" size="mini" @click="copyError(log.error!)">复制错误</a-button>
        </div>
        <div v-if="log.response_body" class="log-body">
          <pre class="log-body-content">{{ log.response_body }}</pre>
          <a-button type="text" size="mini" @click="copyError(log.response_body!)">复制响应</a-button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.log-viewer {
  height: 100%;
  display: flex;
  flex-direction: column;
}
.log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-border);
}
.log-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.log-actions {
  display: flex;
  gap: 8px;
}
.log-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}
.log-empty {
  padding: 40px 0;
}
.log-item {
  padding: 8px 20px;
  border-bottom: 1px solid var(--color-border-2);
  transition: background 0.15s;
}
.log-item:hover {
  background: var(--color-fill-1);
}
.log-item--error {
  background: var(--color-danger-light-1);
}
.log-item--error:hover {
  background: var(--color-danger-light-2);
}
.log-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  line-height: 1.6;
}
.log-time {
  color: var(--color-text-3);
  font-family: monospace;
  font-size: 12px;
  flex-shrink: 0;
}
.log-path {
  font-family: monospace;
  color: var(--color-text-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.log-model {
  font-size: 12px;
  color: var(--color-text-3);
  background: var(--color-fill-2);
  padding: 0 6px;
  border-radius: 3px;
  flex-shrink: 0;
}
.log-duration {
  font-size: 12px;
  color: var(--color-text-3);
  font-family: monospace;
  flex-shrink: 0;
}
.log-cli {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}
.log-provider {
  font-size: 12px;
  color: var(--color-text-3);
  flex-shrink: 0;
}
.log-error {
  margin-top: 4px;
  padding: 6px 8px;
  background: var(--color-fill-1);
  border-radius: 4px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.log-error code {
  flex: 1;
  font-size: 12px;
  color: var(--color-danger-6);
  word-break: break-all;
}
.log-body {
  margin-top: 4px;
  padding: 6px 8px;
  background: var(--color-fill-1);
  border-radius: 4px;
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.log-body-content {
  flex: 1;
  margin: 0;
  font-size: 12px;
  font-family: monospace;
  color: var(--color-text-2);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 120px;
  overflow-y: auto;
}
</style>

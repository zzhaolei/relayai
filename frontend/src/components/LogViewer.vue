<script setup lang="ts">
defineOptions({ name: 'LogViewer' })
import { ref, onActivated, onDeactivated } from 'vue'

import { useMessage } from 'naive-ui'
import { useAppStore } from '../stores/app'
import type { CLIType } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { copyToClipboard, formatDuration, formatTokens } from '../utils'

const store = useAppStore()
const message = useMessage()
const autoRefresh = ref(true)
const dateRange = ref<[number, number] | null>(null)
const isRangeMode = ref(false)
// Only render the heavy date-picker when user explicitly opens it
const showDatePicker = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

const statusType = (code: number): 'success' | 'warning' | 'error' => {
  if (code >= 200 && code < 300) return 'success'
  if (code >= 400 && code < 500) return 'warning'
  return 'error'
}

const formatTime = (ms: number) => {
  const d = new Date(ms)
  return d.toLocaleString('zh-CN', { hour12: false, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

async function copyText(text: string) {
  const success = await copyToClipboard(text)
  if (success) {
    message.success('已复制')
  } else {
    message.error('复制失败')
  }
}

function refreshLogs() {
  store.fetchLogs()
}

function startAutoRefresh() {
  stopAutoRefresh()
  timer = setInterval(refreshLogs, 5000)
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

function handleDateRangeUpdate(value: [number, number] | null) {
  dateRange.value = value
  if (value && value[0] && value[1]) {
    isRangeMode.value = true
    showDatePicker.value = false
    stopAutoRefresh()
    store.fetchLogsByTimeRange(value[0], value[1])
  }
}

function clearDateRange() {
  dateRange.value = null
  isRangeMode.value = false
  showDatePicker.value = false
  refreshLogs()
  if (autoRefresh.value) startAutoRefresh()
}

function openDatePicker() {
  showDatePicker.value = true
}

onActivated(() => {
  if (autoRefresh.value) startAutoRefresh()
  // Defer data fetch to after the component is fully painted,
  // so the tab switch feels instant
  if (!isRangeMode.value) {
    requestAnimationFrame(() => refreshLogs())
  }
})

onDeactivated(() => {
  stopAutoRefresh()
})
</script>

<template>
  <div style="height: 100%; display: flex; flex-direction: column">
    <div style="display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--app-border)">
      <div style="display: flex; align-items: center; gap: 8px">
        <n-text strong style="font-size: 16px">请求日志</n-text>
        <n-text depth="3" style="font-size: 12px">（{{ store.logs.length }} 条）</n-text>
        <n-text depth="3" style="font-size: 12px">({{ store.logsSizeKB }} KB)</n-text>
        <n-text v-if="store.totalTokens > 0" depth="3" style="font-size: 12px">· {{ formatTokens(store.totalTokens) }} tokens</n-text>
        <n-text v-if="isRangeMode" depth="3" style="font-size: 11px; color: var(--app-primary)">· 已按时间筛选</n-text>
      </div>
      <div style="display: flex; gap: 8px; align-items: center">
        <n-date-picker
          v-if="showDatePicker && !isRangeMode"
          type="datetimerange"
          :value="dateRange"
          @update:value="handleDateRangeUpdate"
          size="tiny"
          style="width: 380px"
          clearable
          format="yyyy-MM-dd HH:mm:ss"
          :default-value="Date.now()"
          :shortcuts="{ '最近1小时': [Date.now() - 3600000, Date.now()], '最近24小时': [Date.now() - 86400000, Date.now()], '最近7天': [Date.now() - 7 * 86400000, Date.now()] }"
        />
        <n-button v-if="!showDatePicker && !isRangeMode" size="tiny" @click="openDatePicker">筛选</n-button>
        <n-button v-if="isRangeMode" size="tiny" @click="clearDateRange">返回实时</n-button>
        <n-button v-if="!isRangeMode" size="tiny" :type="autoRefresh ? 'primary' : 'default'" @click="toggleAutoRefresh">
          {{ autoRefresh ? '自动刷新中' : '自动刷新' }}
        </n-button>
        <n-button v-if="!isRangeMode" size="tiny" @click="refreshLogs">刷新</n-button>
        <n-popconfirm positive-text="确认" negative-text="取消" @positive-click="store.clearLogs">
          <template #trigger>
            <n-button size="tiny" type="error">清空</n-button>
          </template>
          确定清空所有日志（{{ store.logsSizeKB }} KB）？
        </n-popconfirm>
      </div>
    </div>

    <div style="flex: 1; overflow-y: auto">
      <n-empty v-if="store.logs.length === 0" description="暂无日志" style="padding: 40px 0" />
      <n-text v-if="store.logs.length === 0" depth="3" style="display: block; text-align: center; font-size: 11px; margin-top: -28px">
        日志自动保留最近 7 天、最多 10000 条
      </n-text>

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
          <div class="log-row">
            <n-text depth="3" class="log-col log-time">
              {{ formatTime(log.time) }}
            </n-text>
            <n-tag :type="statusType(log.status_code)" size="small" class="log-col log-status">{{ log.status_code }}</n-tag>
            <n-tag size="small" type="info" class="log-col log-method">{{ log.method }}</n-tag>
            <n-tooltip trigger="hover" placement="top">
              <template #trigger>
                <n-text code class="log-col log-path">
                  {{ log.path }}
                </n-text>
              </template>
              <div style="max-width: 500px; word-break: break-all">{{ log.path }}</div>
            </n-tooltip>
            <n-text depth="3" class="log-col log-duration">
              {{ formatDuration(log.duration_ms) }}
            </n-text>
            <div class="log-col log-cli">
              <CLIIcon v-if="log.cli_type" :type="log.cli_type as CLIType" :size="12" />
            </div>
            <n-text v-if="log.provider" depth="3" class="log-col log-provider">
              {{ log.provider }}
            </n-text>
            <div v-else class="log-col log-provider"></div>
            <n-tag v-if="log.model" size="small" class="log-col log-model">{{ log.model }}</n-tag>
            <div v-else class="log-col log-model"></div>
            <n-tooltip v-if="log.total_tokens > 0" trigger="hover" placement="top">
              <template #trigger>
                <n-text depth="3" class="log-col log-tokens">
                  共{{ formatTokens(log.total_tokens) }}
                </n-text>
              </template>
              入{{ formatTokens(log.prompt_tokens) }} 出{{ formatTokens(log.completion_tokens) }} 共{{ formatTokens(log.total_tokens) }}
            </n-tooltip>
            <div v-else class="log-col log-tokens"></div>
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
    <div style="padding: 6px 20px; border-top: 1px solid var(--app-border-2); text-align: right">
      <n-text depth="3" style="font-size: 11px">日志保留策略：后端最近 7 天 · 最多 10000 条 · 前端最多展示近 500 条</n-text>
    </div>
  </div>
</template>

<style scoped>
.log-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
}

.log-col {
  flex-shrink: 0;
}

.log-time {
  width: 160px;
  font-size: 12px;
  font-family: monospace;
  white-space: nowrap;
}

.log-status {
  width: 42px;
  text-align: center;
}

.log-method {
  width: 48px;
  text-align: center;
}

.log-path {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-model {
  width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-tokens {
  width: 70px;
  font-size: 11px;
  font-family: monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-duration {
  width: 60px;
  font-size: 12px;
  font-family: monospace;
  text-align: right;
}

.log-cli {
  width: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.log-provider {
  width: 80px;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

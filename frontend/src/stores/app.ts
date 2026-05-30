import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as App from '../../bindings/relay-ai/app'

export interface ModelMapping {
  from: string
  to: string
}

export type CLIType = 'claude' | 'codex'

export interface Provider {
  id: string
  name: string
  base_url: string
  api_key: string
  default_model: string
  model_mappings: ModelMapping[]
  cli_types: CLIType[]
  chat_compat_mode: boolean
  enabled: boolean
  created_at: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  usage_updated_at: number
}

type ProviderPayload = Omit<Provider, 'id' | 'created_at' | 'enabled' | 'prompt_tokens' | 'completion_tokens' | 'total_tokens' | 'usage_updated_at' >

export interface ProxyStatus {
  running: boolean
  port: number
  addr: string
  proxy_auth_token: string
}

export interface RequestLog {
  id: string
  time: number
  method: string
  path: string
  cli_type: CLIType
  provider: string
  model: string
  status_code: number
  duration_ms: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  error?: string
  response_body?: string
}

export interface ProviderUsageStats {
  provider_id: string
  provider: string
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  updated_at: number
}

export interface ProviderUsagePoint {
  time: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}

export interface CLITypeMeta {
  key: CLIType
  label: string
  path: string
}

export const CLI_TYPES: CLITypeMeta[] = [
  { key: 'claude', label: 'Claude', path: '/anthropic' },
  { key: 'codex', label: 'Codex', path: '/openai' },
]

export const useAppStore = defineStore('app', () => {
  const providers = ref<Provider[]>([])
  const proxyStatus = ref<ProxyStatus>({ running: false, port: 18900, addr: '', proxy_auth_token: '' })
  const logs = ref<RequestLog[]>([])
  const providerUsageStats = ref<ProviderUsageStats[]>([])
  const logsSizeKB = ref(0)
  const totalTokens = ref(0)
  const loading = ref(false)

  const MAX_DISPLAY_LOGS = 500

  // 记录上次获取的最大 ID，用于增量拉取
  let lastLogId = ''

  let statusTimer: ReturnType<typeof setInterval> | null = null

  function startStatusPolling() {
    if (statusTimer) return
    statusTimer = setInterval(fetchProxyStatus, 3000)
  }

  async function fetchProxyStatus() {
    try {
      proxyStatus.value = await App.ProxyStatus()
    } catch {
      // 静默处理
    }
  }

  async function fetchProviders() {
    try {
      providers.value = await App.ProviderList() as any
    } catch {
      // 静默处理
    }
  }

  async function fetchProviderUsageStats() {
    try {
      await fetchProviders()
      providerUsageStats.value = providers.value.map(provider => ({
        provider_id: provider.id,
        provider: provider.name,
        prompt_tokens: provider.prompt_tokens || 0,
        completion_tokens: provider.completion_tokens || 0,
        total_tokens: provider.total_tokens || 0,
        updated_at: provider.usage_updated_at || 0,
      }))
    } catch {
      // 静默处理
    }
  }

  async function fetchProviderUsageSeries(providerID: string) {
    return await App.GetProviderUsageSeries(providerID)
  }

  // Task 4: fetchAll 不阻塞 UI，loading 仅用于首次加载指示
  // Task 5: 所有请求并发执行
  async function fetchAll() {
    loading.value = true
    try {
      await Promise.allSettled([
        fetchProxyStatus(),
        fetchProviderUsageStats(),
      ])
    } finally {
      loading.value = false
    }
    startStatusPolling()
  }

  async function createProvider(p: ProviderPayload) {
    await App.ProviderCreate(p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [], p.chat_compat_mode || false)
    await fetchProviders()
  }

  async function updateProvider(id: string, p: ProviderPayload) {
    await App.ProviderUpdate(id, p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [], p.chat_compat_mode || false)
    await fetchProviders()
  }

  async function deleteProvider(id: string) {
    await App.ProviderDelete(id)
    await fetchProviders()
  }

  async function toggleProviderEnabled(id: string, enabled: boolean) {
    await App.ProviderSetEnabled(id, enabled)
    await fetchProviders()
  }

  async function writeCLIConfig(cliType: string) {
    await App.WriteCLIConfig(cliType)
  }

  async function restartProxy() {
    await App.ProxyRestart()
    await fetchProxyStatus()
  }

  // Task 5: 日志增量获取（由于 Wails 绑定限制，使用前端过滤实现增量）
  async function fetchLogs() {
    // 获取所有日志
    const data = await App.GetProxyLogDataWithLimit(500)
    const allLogs = (data.logs || []) as any
    
    if (allLogs.length === 0) {
      return
    }

    // 如果是首次获取（lastLogId 为空），直接使用所有日志
    if (!lastLogId) {
      logs.value = allLogs.slice(0, MAX_DISPLAY_LOGS)
      lastLogId = allLogs[0].id // 更新为最新的 ID
    } else {
      // 过滤出比 lastLogId 更新的日志
      // 注意：后端返回的是倒序（最新的在前），所以我们需要找到比 lastLogId 更新的
      const lastIdNum = parseInt(lastLogId)
      const newLogs = allLogs.filter((log: any) => parseInt(log.id) > lastIdNum)
      
      if (newLogs.length > 0) {
        // 追加新日志到列表头部
        logs.value = [...newLogs, ...logs.value].slice(0, MAX_DISPLAY_LOGS)
        // 更新 lastLogId 为最新的 ID
        lastLogId = newLogs[0].id
      }
    }

    logsSizeKB.value = data.sizeKB || 0
    totalTokens.value = data.totalUsed || 0
  }

  // 按日期范围过滤日志（使用已验证可用的 GetProxyLogData + 前端过滤）
  async function fetchLogsByTimeRange(from: number, to: number) {
    try {
      const data = await App.GetProxyLogData()
      const allLogs = (data.logs || []) as any[]
      const filtered = allLogs.filter((log: any) => log.time >= from && log.time <= to)
      logs.value = filtered.slice(0, MAX_DISPLAY_LOGS)
      lastLogId = ''
      logsSizeKB.value = data.sizeKB || 0
      totalTokens.value = data.totalUsed || 0
    } catch {
      // Silent
    }
  }

  // Clear local log data only (no IPC call), used when switching to logs tab
  function clearLogsLocal() {
    logs.value = []
    logsSizeKB.value = 0
    totalTokens.value = 0
    lastLogId = ''
  }

  async function clearLogs() {
    await App.ClearProxyLogs()
    clearLogsLocal()
  }

  async function startProxy() {
    await App.ProxyStart()
    await fetchProxyStatus()
  }

  async function stopProxy() {
    await App.ProxyStop()
    await fetchProxyStatus()
  }

  return {
    providers,
    proxyStatus,
    logs,
    providerUsageStats,
    logsSizeKB,
    totalTokens,
    loading,
    fetchAll,
    fetchProxyStatus,
    fetchProviders,
    createProvider,
    updateProvider,
    deleteProvider,
    toggleProviderEnabled,
    writeCLIConfig,
    restartProxy,
    startProxy,
    stopProxy,
    fetchLogs,
    fetchProviderUsageStats,
    fetchProviderUsageSeries,
    clearLogs,
    fetchLogsByTimeRange,
    clearLogsLocal,
  }
})

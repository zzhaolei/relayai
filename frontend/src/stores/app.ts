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
  enabled: boolean
  created_at: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  usage_updated_at: number
}

type ProviderPayload = Omit<Provider, 'id' | 'created_at' | 'enabled' | 'prompt_tokens' | 'completion_tokens' | 'total_tokens' | 'usage_updated_at'>

export interface ProxyStatus {
  running: boolean
  port: number
  addr: string
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
  const proxyStatus = ref<ProxyStatus>({ running: false, port: 18900, addr: '' })
  const logs = ref<RequestLog[]>([])
  const providerUsageStats = ref<ProviderUsageStats[]>([])
  const logsSizeKB = ref(0)
  const totalTokens = ref(0)
  const loading = ref(false)

  async function fetchProxyStatus() {
    proxyStatus.value = await App.ProxyStatus()
  }

  async function fetchProviders() {
    providers.value = await App.ProviderList() as any
  }

  async function fetchProviderUsageStats() {
    await fetchProviders()
    providerUsageStats.value = providers.value.map(provider => ({
      provider_id: provider.id,
      provider: provider.name,
      prompt_tokens: provider.prompt_tokens || 0,
      completion_tokens: provider.completion_tokens || 0,
      total_tokens: provider.total_tokens || 0,
      updated_at: provider.usage_updated_at || 0,
    }))
  }

  async function fetchProviderUsageSeries(providerID: string) {
    return await App.GetProviderUsageSeries(providerID)
  }

  async function fetchAll() {
    loading.value = true
    try {
      await Promise.race([
        Promise.all([fetchProxyStatus(), fetchProviderUsageStats()]),
        new Promise<never>((_, reject) => setTimeout(() => reject(new Error('timeout')), 10_000)),
      ])
    } catch {
      // 加载失败时保持现有数据，静默处理
    } finally {
      loading.value = false
    }
  }

  async function createProvider(p: ProviderPayload) {
    await App.ProviderCreate(p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [])
    await fetchProviders()
  }

  async function updateProvider(id: string, p: ProviderPayload) {
    await App.ProviderUpdate(id, p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [])
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

  async function resetProviderUsage(id: string) {
    await App.ProviderResetUsage(id)
    await fetchProviders()
  }

  async function writeCLIConfig(cliType: string) {
    await App.WriteCLIConfig(cliType)
  }

  async function restartProxy() {
    await App.ProxyRestart()
    await fetchProxyStatus()
  }

  async function fetchLogs() {
    logs.value = ((await App.GetProxyLogs()) || []) as any
    logsSizeKB.value = await App.GetProxyLogsSizeKB()
    totalTokens.value = logs.value.reduce((sum, log) => sum + (log.total_tokens || 0), 0)
  }

  async function clearLogs() {
    await App.ClearProxyLogs()
    logs.value = []
    logsSizeKB.value = 0
    totalTokens.value = 0
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
    resetProviderUsage,
    writeCLIConfig,
    restartProxy,
    startProxy,
    stopProxy,
    fetchLogs,
    fetchProviderUsageStats,
    fetchProviderUsageSeries,
    clearLogs,
  }
})

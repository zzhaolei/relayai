import { defineStore } from 'pinia'
import { ref } from 'vue'

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

declare global {
  interface Window {
    go: {
      main: {
        App: {
          ProxyStart(): Promise<void>
          ProxyStop(): Promise<void>
          ProxyRestart(): Promise<void>
          ProxyStatus(): Promise<ProxyStatus>
          ProviderList(): Promise<Provider[]>
          ProviderCreate(name: string, base_url: string, api_key: string, defaultModel: string, modelMappings: ModelMapping[], cliTypes: string[]): Promise<Provider>
          ProviderUpdate(id: string, name: string, base_url: string, api_key: string, defaultModel: string, modelMappings: ModelMapping[], cliTypes: string[]): Promise<void>
          ProviderDelete(id: string): Promise<void>
          ProviderSetEnabled(id: string, enabled: boolean): Promise<void>
          ProviderResetUsage(id: string): Promise<void>
          WriteCLIConfig(cliType: string): Promise<void>
          GetCLIConfigStatus(): Promise<Record<string, boolean>>
          GetProxyLogs(): Promise<RequestLog[]>
          GetProviderUsageStats(): Promise<ProviderUsageStats[]>
          GetProviderUsageSeries(providerID: string): Promise<ProviderUsagePoint[]>
          ClearProxyLogs(): Promise<void>
          GetProxyLogsSizeKB(): Promise<number>
          SettingsGet(): Promise<any>
          SettingsUpdatePort(port: number): Promise<void>
        }
      }
    }
  }
}

const api = () => window.go.main.App

export const useAppStore = defineStore('app', () => {
  const providers = ref<Provider[]>([])
  const proxyStatus = ref<ProxyStatus>({ running: false, port: 18900, addr: '' })
  const logs = ref<RequestLog[]>([])
  const providerUsageStats = ref<ProviderUsageStats[]>([])
  const logsSizeKB = ref(0)
  const totalTokens = ref(0)
  const loading = ref(false)

  async function fetchProxyStatus() {
    proxyStatus.value = await api().ProxyStatus()
  }

  async function fetchProviders() {
    providers.value = await api().ProviderList()
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
    return await api().GetProviderUsageSeries(providerID)
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
    await api().ProviderCreate(p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [])
    await fetchProviders()
  }

  async function updateProvider(id: string, p: ProviderPayload) {
    await api().ProviderUpdate(id, p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [])
    await fetchProviders()
  }

  async function deleteProvider(id: string) {
    await api().ProviderDelete(id)
    await fetchProviders()
  }

  async function toggleProviderEnabled(id: string, enabled: boolean) {
    await api().ProviderSetEnabled(id, enabled)
    await fetchProviders()
  }

  async function resetProviderUsage(id: string) {
    await api().ProviderResetUsage(id)
    await fetchProviders()
  }

  async function writeCLIConfig(cliType: string) {
    await api().WriteCLIConfig(cliType)
  }

  async function restartProxy() {
    await api().ProxyRestart()
    await fetchProxyStatus()
  }

  async function fetchLogs() {
    logs.value = (await api().GetProxyLogs()) || []
    logsSizeKB.value = await api().GetProxyLogsSizeKB()
    totalTokens.value = logs.value.reduce((sum, log) => sum + (log.total_tokens || 0), 0)
  }

  async function clearLogs() {
    await api().ClearProxyLogs()
    logs.value = []
    logsSizeKB.value = 0
    totalTokens.value = 0
  }

  async function startProxy() {
    await api().ProxyStart()
    await fetchProxyStatus()
  }

  async function stopProxy() {
    await api().ProxyStop()
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

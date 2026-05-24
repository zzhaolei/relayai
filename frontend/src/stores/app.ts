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
}

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
  error?: string
  response_body?: string
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
          WriteCLIConfig(cliType: string): Promise<void>
          GetCLIConfigStatus(): Promise<Record<string, boolean>>
          GetProxyLogs(): Promise<RequestLog[]>
          ClearProxyLogs(): Promise<void>
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
  const loading = ref(false)

  async function fetchProxyStatus() {
    proxyStatus.value = await api().ProxyStatus()
  }

  async function fetchProviders() {
    providers.value = await api().ProviderList()
  }

  async function fetchAll() {
    loading.value = true
    try {
      await Promise.all([fetchProxyStatus(), fetchProviders()])
    } finally {
      loading.value = false
    }
  }

  async function createProvider(p: Omit<Provider, 'id' | 'created_at' | 'enabled'>) {
    await api().ProviderCreate(p.name, p.base_url, p.api_key, p.default_model, p.model_mappings || [], p.cli_types || [])
    await fetchProviders()
  }

  async function updateProvider(id: string, p: Omit<Provider, 'id' | 'created_at' | 'enabled'>) {
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

  async function writeCLIConfig(cliType: string) {
    await api().WriteCLIConfig(cliType)
  }

  async function restartProxy() {
    await api().ProxyRestart()
    await fetchProxyStatus()
  }

  async function fetchLogs() {
    logs.value = await api().GetProxyLogs()
  }

  async function clearLogs() {
    await api().ClearProxyLogs()
    logs.value = []
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
    clearLogs,
  }
})

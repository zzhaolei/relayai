<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import { useAppStore } from '../stores/app'
import type { Provider, ModelMapping, CLIType } from '../stores/app'
import ProxyStatusBar from '../components/ProxyStatusBar.vue'
import ProviderCard from '../components/ProviderCard.vue'
import ProviderDetailsModal from '../components/ProviderDetailsModal.vue'
import ProviderForm from '../components/ProviderForm.vue'
import { getErrorMessage } from '../utils'

const store = useAppStore()
const message = useAppMessage()
const formVisible = ref(false)
const editingProvider = ref<Provider | null>(null)
const detailsVisible = ref(false)
const selectedProviderId = ref('')
const usageRefreshing = ref(false)
let usageRefreshTimer: number | undefined

const selectedProvider = computed(() => {
  return store.providers.find(p => p.id === selectedProviderId.value) || null
})

onMounted(() => {
  store.fetchAll()
  usageRefreshTimer = window.setInterval(() => {
    store.fetchProviders()
  }, 60_000)
})

onUnmounted(() => {
  if (usageRefreshTimer) {
    window.clearInterval(usageRefreshTimer)
  }
})

function openAddForm() {
  editingProvider.value = null
  formVisible.value = true
}

function openEditForm(provider: Provider) {
  editingProvider.value = provider
  detailsVisible.value = false
  formVisible.value = true
}

function openDetails(provider: Provider) {
  selectedProviderId.value = provider.id
  detailsVisible.value = true
}

async function handleFormSubmit(data: {
  name: string
  base_url: string
  api_key: string
  default_model: string
  model_mappings: ModelMapping[]
  cli_types: CLIType[]
  chat_compat_mode: boolean
}) {
  try {
    const payload = {
      name: data.name,
      base_url: data.base_url,
      api_key: data.api_key,
      default_model: data.default_model,
      model_mappings: data.model_mappings,
      cli_types: data.cli_types,
      chat_compat_mode: data.chat_compat_mode,
    }
    if (editingProvider.value) {
      await store.updateProvider(editingProvider.value.id, payload)
      message.success('更新成功')
    } else {
      await store.createProvider(payload)
      message.success('添加成功')
    }
  } catch (e: any) {
    message.error(getErrorMessage(e, '操作失败'))
  }
}

async function handleDelete(id: string) {
  try {
    await store.deleteProvider(id)
    message.success('已删除')
  } catch (e: any) {
    message.error(getErrorMessage(e, '删除失败'))
  }
}

async function refreshUsageStats() {
  usageRefreshing.value = true
  try {
    await store.fetchProviders()
    message.success('用量已刷新')
  } catch (e: any) {
    message.error(getErrorMessage(e, '刷新失败'))
  } finally {
    usageRefreshing.value = false
  }
}

</script>

<template>
  <div style="height: 100%; display: flex; flex-direction: column">
    <ProxyStatusBar />

    <n-divider style="margin: 0" />

    <div style="flex: 1; padding: 20px; overflow-y: auto">
      <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 20px">
        <div>
          <n-text strong style="font-size: 18px; display: block; margin-bottom: 4px">模型提供商</n-text>
          <n-text depth="3" style="font-size: 13px">管理 AI 模型提供方，启用后参与反代路由</n-text>
        </div>
        <div style="display: flex; gap: 8px">
          <n-button :loading="usageRefreshing" @click="refreshUsageStats">刷新用量</n-button>
          <n-button type="primary" @click="openAddForm">+ 添加提供商</n-button>
        </div>
      </div>

      <n-spin :show="store.loading" style="width: 100%">
        <n-empty v-if="store.providers.length === 0 && !store.loading" description="暂无提供商" style="padding: 60px 0">
          <template #extra>
            <n-button type="primary" @click="openAddForm">添加第一个提供商</n-button>
          </template>
        </n-empty>

        <ProviderCard
          v-for="p in store.providers"
          :key="p.id"
          :provider="p"
          @view="openDetails"
          @edit="openEditForm"
          @delete="handleDelete"
        />
      </n-spin>
    </div>

    <ProviderForm
      v-model:visible="formVisible"
      :provider="editingProvider"
      @submit="handleFormSubmit"
    />

    <ProviderDetailsModal
      v-model:visible="detailsVisible"
      :provider="selectedProvider"
    />
  </div>
</template>

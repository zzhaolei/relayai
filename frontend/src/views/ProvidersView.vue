<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useAppStore } from '../stores/app'
import type { Provider, ModelMapping } from '../stores/app'
import ProxyStatusBar from '../components/ProxyStatusBar.vue'
import ProviderCard from '../components/ProviderCard.vue'
import ProviderForm from '../components/ProviderForm.vue'

const store = useAppStore()
const formVisible = ref(false)
const editingProvider = ref<Provider | null>(null)

onMounted(() => {
  store.fetchAll()
})

function openAddForm() {
  editingProvider.value = null
  formVisible.value = true
}

function openEditForm(provider: Provider) {
  editingProvider.value = provider
  formVisible.value = true
}

async function handleFormSubmit(data: {
  name: string
  base_url: string
  api_key: string
  models: string[]
  default_model: string
  model_mappings: ModelMapping[]
}) {
  try {
    const payload = {
      name: data.name,
      base_url: data.base_url,
      api_key: data.api_key,
      models: data.models,
      default_model: data.default_model,
      model_mappings: data.model_mappings,
    }
    if (editingProvider.value) {
      await store.updateProvider(editingProvider.value.id, payload)
      Message.success('更新成功')
    } else {
      await store.createProvider(payload)
      Message.success('添加成功')
    }
  } catch (e: any) {
    Message.error(e?.message || '操作失败')
  }
}

async function handleDelete(id: string) {
  try {
    await store.deleteProvider(id)
    Message.success('已删除')
  } catch (e: any) {
    Message.error(e?.message || '删除失败')
  }
}
</script>

<template>
  <div class="providers-view">
    <ProxyStatusBar />

    <div class="content">
      <div class="content-header">
        <div>
          <h2>模型提供商</h2>
          <p class="subtitle">管理 AI 模型提供方，启用后参与反代路由，点击按钮写入 CLI 配置</p>
        </div>
        <a-button type="primary" @click="openAddForm">
          <template #icon>+</template>
          添加提供商
        </a-button>
      </div>

      <a-spin :loading="store.loading" style="width: 100%">
        <div v-if="store.providers.length === 0 && !store.loading" class="empty-state">
          <a-empty description="暂无提供商">
            <a-button type="primary" @click="openAddForm">添加第一个提供商</a-button>
          </a-empty>
        </div>

        <ProviderCard
          v-for="p in store.providers"
          :key="p.id"
          :provider="p"
          @edit="openEditForm"
          @delete="handleDelete"
        />
      </a-spin>
    </div>

    <ProviderForm
      v-model:visible="formVisible"
      :provider="editingProvider"
      @submit="handleFormSubmit"
    />
  </div>
</template>

<style scoped>
.providers-view {
  height: 100%;
  display: flex;
  flex-direction: column;
}
.content {
  flex: 1;
  padding: 20px;
  overflow-y: auto;
}
.content-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
}
.content-header h2 {
  margin: 0 0 4px 0;
  font-size: 18px;
  font-weight: 600;
}
.subtitle {
  margin: 0;
  font-size: 13px;
  color: var(--color-text-3);
}
.empty-state {
  padding: 60px 0;
}
</style>

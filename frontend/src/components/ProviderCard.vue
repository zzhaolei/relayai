<script setup lang="ts">
import { computed, ref } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import type { Provider } from '../stores/app'
import { useAppStore, CLI_TYPES } from '../stores/app'
import CLIIcon from './CLIIcon.vue'

const props = defineProps<{
  provider: Provider
}>()

const emit = defineEmits<{
  (e: 'edit', provider: Provider): void
  (e: 'delete', id: string): void
}>()

const store = useAppStore()
const message = useAppMessage()
const toggling = ref(false)

async function handleToggleEnabled(val: boolean) {
  toggling.value = true
  try {
    await store.toggleProviderEnabled(props.provider.id, val)
    message.success(val ? '已启用' : '已禁用')
  } catch (e: any) {
    message.error(e?.message || '操作失败')
  } finally {
    toggling.value = false
  }
}

const maskedKey = computed(() => {
  const key = props.provider.api_key
  if (!key) return '未设置'
  if (key.length <= 8) return '****'
  return key.slice(0, 4) + '****' + key.slice(-4)
})

const cliTypes = computed(() => {
  const types = props.provider.cli_types
  if (!types || types.length === 0) return ['claude', 'codex']
  return types
})

const cliLabels: Record<string, string> = Object.fromEntries(
  CLI_TYPES.map(t => [t.key, t.label])
)
</script>

<template>
  <n-card
    size="small"
    :bordered="true"
    hoverable
    :style="{ marginBottom: '10px', opacity: provider.enabled ? 1 : 0.5 }"
  >
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
      <div style="display: flex; align-items: center; gap: 10px">
        <n-switch
          :value="provider.enabled"
          :disabled="toggling"
          @update:value="handleToggleEnabled"
          size="small"
        />
        <n-text strong style="font-size: 15px">{{ provider.name }}</n-text>
      </div>
      <div style="display: flex; gap: 4px">
        <n-button text size="small" @click="emit('edit', provider)">编辑</n-button>
        <n-popconfirm @positive-click="emit('delete', provider.id)">
          <template #trigger>
            <n-button text type="error" size="small">删除</n-button>
          </template>
          确定删除该提供商？
        </n-popconfirm>
      </div>
    </div>

    <div style="display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px">
      <div style="display: flex; align-items: center; gap: 8px">
        <n-text depth="3" style="width: 32px; font-size: 12px; flex-shrink: 0">URL</n-text>
        <n-text code style="font-size: 12px">{{ provider.base_url }}</n-text>
      </div>
      <div style="display: flex; align-items: center; gap: 8px">
        <n-text depth="3" style="width: 32px; font-size: 12px; flex-shrink: 0">Key</n-text>
        <n-text code style="font-size: 12px">{{ maskedKey }}</n-text>
      </div>
      <div style="display: flex; align-items: center; gap: 8px">
        <n-text depth="3" style="width: 32px; font-size: 12px; flex-shrink: 0">CLI</n-text>
        <div style="display: flex; gap: 10px">
          <div v-for="t in cliTypes" :key="t" style="display: flex; align-items: center; gap: 4px">
            <CLIIcon :type="t as 'claude' | 'codex'" :size="14" />
            <n-text style="font-size: 12px">{{ cliLabels[t] || t }}</n-text>
          </div>
        </div>
      </div>
    </div>

    <div v-if="provider.default_model || provider.model_mappings?.length" style="display: flex; flex-wrap: wrap; gap: 4px">
      <n-tag v-if="provider.default_model" size="small" type="warning">
        全部 → {{ provider.default_model }}
      </n-tag>
      <n-tag v-for="m in provider.model_mappings" :key="m.from" size="small" type="info">
        {{ m.from }} → {{ m.to }}
      </n-tag>
    </div>
  </n-card>
</template>

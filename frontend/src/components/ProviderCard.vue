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
        <n-button quaternary size="small" @click="emit('edit', provider)">
          <template #icon><n-icon><svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14"><path d="M2.695 14.763l-1.262 3.154a.5.5 0 00.65.65l3.155-1.262a4 4 0 001.343-.885L17.5 5.5a2.121 2.121 0 00-3-3L3.58 13.42a4 4 0 00-.885 1.343z"/></svg></n-icon></template>
          编辑
        </n-button>
        <n-popconfirm @positive-click="emit('delete', provider.id)">
          <template #trigger>
            <n-button quaternary type="error" size="small">
              <template #icon><n-icon><svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14"><path fill-rule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022 1.005 11.969A2.75 2.75 0 007.765 20h4.47a2.75 2.75 0 002.745-2.58l1.005-11.97.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clip-rule="evenodd"/></svg></n-icon></template>
              删除
            </n-button>
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

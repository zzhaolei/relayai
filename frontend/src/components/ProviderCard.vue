<script setup lang="ts">
import { computed, ref } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import type { Provider, CLIType } from '../stores/app'
import { useAppStore, CLI_TYPES } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { getErrorMessage } from '../utils'

const props = defineProps<{
  provider: Provider
}>()

const emit = defineEmits<{
  (e: 'edit', provider: Provider): void
  (e: 'delete', id: string): void
  (e: 'view', provider: Provider): void
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
    message.error(getErrorMessage(e, '操作失败'))
  } finally {
    toggling.value = false
  }
}

const cliTypes = computed(() => {
  const types = props.provider.cli_types
  if (!types || types.length === 0) return []
  return [types[0]]
})

const primaryCliType = computed(() => cliTypes.value[0] || '')

const cliLabels: Record<string, string> = Object.fromEntries(
  CLI_TYPES.map(t => [t.key, t.label])
)

function formatTokens(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value || 0)
}
</script>

<template>
  <n-card
    size="small"
    :bordered="true"
    hoverable
    :style="{ marginBottom: '10px', opacity: provider.enabled ? 1 : 0.5, position: 'relative', paddingTop: '6px' }"
  >
    <n-tag
      size="small"
      type="info"
      style="position: absolute; left: 12px; top: -10px; display: inline-flex; align-items: center; gap: 4px; background: var(--n-color)"
    >
      <CLIIcon v-if="primaryCliType" :type="primaryCliType as CLIType" :size="14" />
      {{ primaryCliType ? (cliLabels[primaryCliType] || primaryCliType) : '未选择 CLI' }}
    </n-tag>
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
        <n-button quaternary size="small" @mousedown.prevent @click="emit('view', provider)">更多</n-button>
        <n-button type="success" size="small" @mousedown.prevent @click="emit('edit', provider)">编辑</n-button>
        <n-popconfirm positive-text="确认" negative-text="取消" @positive-click="emit('delete', provider.id)">
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
        <n-text code style="font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">{{ provider.base_url }}</n-text>
      </div>
    </div>

    <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px">
      <n-tag v-if="provider.default_model" size="small" type="warning">{{ provider.default_model }}</n-tag>
      <n-tag v-if="primaryCliType === 'codex' && provider.chat_compat_mode" size="small" type="success">Chat兼容</n-tag>
      <n-text depth="3" style="font-size: 12px; margin-left: auto">总用量 {{ formatTokens(provider.total_tokens) }} tokens</n-text>
    </div>
  </n-card>
</template>

<style scoped>
</style>

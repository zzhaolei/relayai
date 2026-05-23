<script setup lang="ts">
import { computed, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
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
const toggling = ref(false)

async function handleToggleEnabled(val: string | number | boolean) {
  toggling.value = true
  try {
    await store.toggleProviderEnabled(props.provider.id, !!val)
    Message.success({ content: val ? '已启用' : '已禁用', duration: 2000 })
  } catch (e: any) {
    Message.error(e?.message || '操作失败')
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
  <a-card
    class="provider-card"
    :class="{ 'provider-card--disabled': !provider.enabled }"
    :bordered="true"
    hoverable
  >
    <div class="card-top">
      <div class="top-left">
        <a-switch
          :model-value="provider.enabled"
          :loading="toggling"
          :disabled="toggling"
          @change="handleToggleEnabled"
          size="small"
        />
        <span class="provider-name">{{ provider.name }}</span>
      </div>
      <div class="top-right">
        <a-button type="text" size="small" @click="emit('edit', provider)">编辑</a-button>
        <a-popconfirm content="确定删除该提供商？" @ok="emit('delete', provider.id)">
          <a-button type="text" status="danger" size="small">删除</a-button>
        </a-popconfirm>
      </div>
    </div>

    <div class="card-info">
      <div class="info-row">
        <span class="info-label">URL</span>
        <span class="info-value">{{ provider.base_url }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">Key</span>
        <span class="info-value">{{ maskedKey }}</span>
      </div>
      <div class="info-row">
        <span class="info-label">CLI</span>
        <span class="cli-icons">
          <span v-for="t in cliTypes" :key="t" class="cli-item">
            <CLIIcon :type="t as 'claude' | 'codex'" :size="14" />
            <span>{{ cliLabels[t] || t }}</span>
          </span>
        </span>
      </div>
    </div>

    <div v-if="provider.default_model || provider.model_mappings?.length" class="card-tags">
      <a-tag v-if="provider.default_model" size="small" color="orangered">
        全部 → {{ provider.default_model }}
      </a-tag>
      <a-tag v-for="m in provider.model_mappings" :key="m.from" size="small" color="purple">
        {{ m.from }} → {{ m.to }}
      </a-tag>
    </div>
  </a-card>
</template>

<style scoped>
.provider-card {
  margin-bottom: 10px;
  transition: opacity 0.2s, border-color 0.2s, box-shadow 0.2s, transform 0.15s;
}
.provider-card:hover:not(.provider-card--disabled) {
  border-color: var(--color-primary-light-3);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  transform: translateY(-1px);
}
.provider-card--disabled {
  opacity: 0.5;
}
.provider-card--disabled:hover {
  opacity: 0.7;
}
.card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.top-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.provider-name {
  font-size: 15px;
  font-weight: 600;
}
.top-right {
  display: flex;
  gap: 4px;
}
.card-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 8px;
}
.info-row {
  display: flex;
  align-items: center;
  font-size: 12px;
  line-height: 1.8;
}
.info-label {
  width: 32px;
  color: var(--color-text-3);
  font-weight: 500;
  flex-shrink: 0;
}
.info-value {
  color: var(--color-text-2);
  font-family: monospace;
}
.card-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 4px;
}
.cli-icons {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cli-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--color-text-2);
}
</style>

<script setup lang="ts">
import { computed } from 'vue'
import { Message } from '@arco-design/web-vue'
import type { Provider } from '../stores/app'
import { useAppStore } from '../stores/app'

const props = defineProps<{
  provider: Provider
}>()

const emit = defineEmits<{
  (e: 'edit', provider: Provider): void
  (e: 'delete', id: string): void
}>()

const store = useAppStore()

const cliButtons = [
  { key: 'claude', label: 'Claude', color: '#D97706' },
  { key: 'codex', label: 'Codex', color: '#10B981' },
  { key: 'gemini', label: 'Gemini', color: '#8B5CF6' },
]

async function handleToggleEnabled(val: string | number | boolean) {
  try {
    await store.toggleProviderEnabled(props.provider.id, !!val)
    Message.success({ content: val ? '已启用' : '已禁用', duration: 2000 })
  } catch (e: any) {
    Message.error(e?.message || '操作失败')
  }
}

async function handleWriteCLI(cliType: string) {
  try {
    await store.writeCLIConfig(cliType)
    Message.success({ content: `${cliType} 配置已写入`, duration: 2000 })
  } catch (e: any) {
    Message.error(e?.message || '写入失败')
  }
}

const maskedKey = computed(() => {
  const key = props.provider.api_key
  if (!key) return '未设置'
  if (key.length <= 8) return '****'
  return key.slice(0, 4) + '****' + key.slice(-4)
})
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
    </div>

    <div v-if="provider.models?.length" class="card-tags">
      <a-tag v-for="m in provider.models" :key="m" size="small" color="arcoblue">{{ m }}</a-tag>
    </div>

    <div v-if="provider.default_model || provider.model_mappings?.length" class="card-tags">
      <a-tag v-if="provider.default_model" size="small" color="orangered">
        全部 → {{ provider.default_model }}
      </a-tag>
      <a-tag v-for="m in provider.model_mappings" :key="m.from" size="small" color="purple">
        {{ m.from }} → {{ m.to }}
      </a-tag>
    </div>

    <a-divider :margin="10" />

    <div class="card-bottom">
      <span class="bottom-label">写入配置</span>
      <div class="cli-buttons">
        <a-tooltip v-for="cli in cliButtons" :key="cli.key" :content="`将代理地址写入 ${cli.label} 配置`">
          <a-button
            size="mini"
            @click="handleWriteCLI(cli.key)"
            :disabled="!provider.enabled"
          >
            <template #icon>
              <span class="cli-dot" :style="{ background: cli.color }" />
            </template>
            {{ cli.label }}
          </a-button>
        </a-tooltip>
      </div>
    </div>
  </a-card>
</template>

<style scoped>
.provider-card {
  margin-bottom: 10px;
  transition: opacity 0.2s, border-color 0.2s;
}
.provider-card--disabled {
  opacity: 0.5;
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
.card-bottom {
  display: flex;
  align-items: center;
  gap: 12px;
}
.bottom-label {
  font-size: 12px;
  color: var(--color-text-3);
  flex-shrink: 0;
}
.cli-buttons {
  display: flex;
  gap: 6px;
}
.cli-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
</style>

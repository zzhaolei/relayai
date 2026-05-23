<script setup lang="ts">
import { ref, watch } from 'vue'
import { Message } from '@arco-design/web-vue'
import type { Provider, ModelMapping } from '../stores/app'
import { CLI_TYPES } from '../stores/app'
import CLIIcon from './CLIIcon.vue'

const props = defineProps<{
  visible: boolean
  provider?: Provider | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'submit', data: {
    name: string
    base_url: string
    api_key: string
    default_model: string
    model_mappings: ModelMapping[]
    cli_types: string[]
  }): void
}>()

const cliOptions = CLI_TYPES.map(t => ({ label: t.label, value: t.key }))

const form = ref({
  name: '',
  base_url: '',
  api_key: '',
  default_model: '',
  cli_types: [] as string[],
})

const mappings = ref<ModelMapping[]>([])

watch(() => props.visible, (val) => {
  if (val && props.provider) {
    form.value = {
      name: props.provider.name,
      base_url: props.provider.base_url,
      api_key: props.provider.api_key,
      default_model: props.provider.default_model || '',
      cli_types: props.provider.cli_types || [],
    }
    mappings.value = (props.provider.model_mappings || []).map(m => ({ ...m }))
  } else if (val) {
    form.value = { name: '', base_url: '', api_key: '', default_model: '', cli_types: [] }
    mappings.value = []
  }
})

function addMapping() {
  mappings.value.push({ from: '', to: '' })
}

function removeMapping(index: number) {
  mappings.value.splice(index, 1)
}

function handleSubmit() {
  if (!form.value.name.trim()) {
    Message.warning('请输入名称')
    return
  }
  if (!form.value.base_url.trim()) {
    Message.warning('请输入 API Base URL')
    return
  }
  if (!form.value.api_key.trim()) {
    Message.warning('请输入 API Key')
    return
  }
  if (!form.value.default_model.trim()) {
    Message.warning('请输入默认模型')
    return
  }
  if (form.value.cli_types.length === 0) {
    Message.warning('请至少选择一个 CLI 平台')
    return
  }

  const validMappings = mappings.value
    .filter(m => m.from.trim() && m.to.trim())
    .map(m => ({ from: m.from.trim(), to: m.to.trim() }))

  emit('submit', {
    name: form.value.name,
    base_url: form.value.base_url.replace(/\/+$/, ''),
    api_key: form.value.api_key,
    default_model: form.value.default_model.trim(),
    model_mappings: validMappings,
    cli_types: form.value.cli_types,
  })
  emit('update:visible', false)
}

function handleCancel() {
  emit('update:visible', false)
}
</script>

<template>
  <a-modal
    :visible="visible"
    :title="provider ? '编辑提供商' : '添加提供商'"
    @ok="handleSubmit"
    @cancel="handleCancel"
    :width="520"
  >
    <a-form :model="form" layout="vertical">
      <a-form-item label="名称" required>
        <a-input v-model="form.name" placeholder="例如：DeepSeek" />
      </a-form-item>
      <a-form-item label="API Base URL" required>
        <a-input v-model="form.base_url" placeholder="例如：https://api.deepseek.com" />
      </a-form-item>
      <a-form-item label="API Key" required>
        <a-input-password v-model="form.api_key" placeholder="sk-..." />
      </a-form-item>

      <a-divider :margin="16" />

      <a-form-item label="默认模型" help="设置后所有请求强制使用此模型，忽略客户端传入的模型名" required>
        <a-input v-model="form.default_model" placeholder="例如：gpt-4o" />
      </a-form-item>

      <a-form-item label="模型映射" help="将客户端请求的模型名映射到实际使用的模型名">
        <div class="mappings-list">
          <div v-for="(m, index) in mappings" :key="index" class="mapping-row">
            <a-input
              v-model="m.from"
              placeholder="外部模型名"
              size="small"
              class="mapping-input"
            />
            <span class="mapping-arrow">&rarr;</span>
            <a-input
              v-model="m.to"
              placeholder="内部模型名"
              size="small"
              class="mapping-input"
            />
            <a-button
              type="text"
              status="danger"
              size="mini"
              @click="removeMapping(index)"
            >
              删除
            </a-button>
          </div>
          <a-button type="dashed" size="small" @click="addMapping" long>
            + 添加映射
          </a-button>
        </div>
      </a-form-item>

      <a-divider :margin="16" />

      <a-form-item label="支持的 CLI 平台" help="选择该提供商支持的 CLI 平台，代理会根据请求类型路由到对应提供商" required>
        <a-checkbox-group v-model="form.cli_types">
          <a-checkbox v-for="opt in cliOptions" :key="opt.value" :value="opt.value">
            <span class="cli-option">
              <CLIIcon :type="opt.value as 'claude' | 'codex'" :size="14" />
              {{ opt.label }}
            </span>
          </a-checkbox>
        </a-checkbox-group>
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<style scoped>
.mappings-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.mapping-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.mapping-input {
  flex: 1;
}
.mapping-arrow {
  color: var(--color-text-3);
  flex-shrink: 0;
}
.cli-option {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
</style>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Provider, ModelMapping } from '../stores/app'

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
    models: string[]
    default_model: string
    model_mappings: ModelMapping[]
  }): void
}>()

const form = ref({
  name: '',
  base_url: '',
  api_key: '',
  models: '',
  default_model: '',
})

const mappings = ref<ModelMapping[]>([])

watch(() => props.visible, (val) => {
  if (val && props.provider) {
    form.value = {
      name: props.provider.name,
      base_url: props.provider.base_url,
      api_key: props.provider.api_key,
      models: (props.provider.models || []).join(', '),
      default_model: props.provider.default_model || '',
    }
    mappings.value = (props.provider.model_mappings || []).map(m => ({ ...m }))
  } else if (val) {
    form.value = { name: '', base_url: '', api_key: '', models: '', default_model: '' }
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
  const models = form.value.models
    .split(',')
    .map(m => m.trim())
    .filter(m => m)

  const validMappings = mappings.value
    .filter(m => m.from.trim() && m.to.trim())
    .map(m => ({ from: m.from.trim(), to: m.to.trim() }))

  emit('submit', {
    name: form.value.name,
    base_url: form.value.base_url.replace(/\/+$/, ''),
    api_key: form.value.api_key,
    models,
    default_model: form.value.default_model.trim(),
    model_mappings: validMappings,
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
      <a-form-item label="模型列表" help="用逗号分隔多个模型，可选">
        <a-input v-model="form.models" placeholder="deepseek-chat, deepseek-reasoner" />
      </a-form-item>

      <a-divider :margin="16" />

      <a-form-item label="默认模型" help="设置后所有请求强制使用此模型，忽略客户端传入的模型名">
        <a-input v-model="form.default_model" placeholder="留空则不强制替换" allow-clear />
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
</style>

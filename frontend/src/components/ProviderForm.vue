<script setup lang="ts">
import { ref, watch } from 'vue'
import { useAppMessage } from '../composables/useMessage'
import type { Provider, ModelMapping, CLIType } from '../stores/app'
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
    cli_types: CLIType[]
  }): void
}>()

const message = useAppMessage()
const cliOptions = CLI_TYPES.map(t => ({ label: t.label, value: t.key }))

const form = ref({
  name: '',
  base_url: '',
  api_key: '',
  default_model: '',
  cli_type: null as CLIType | null,
})

const mappings = ref<ModelMapping[]>([])

watch(() => props.visible, (val) => {
  if (val && props.provider) {
    form.value = {
      name: props.provider.name,
      base_url: props.provider.base_url,
      api_key: props.provider.api_key,
      default_model: props.provider.default_model || '',
      cli_type: props.provider.cli_types?.[0] || null,
    }
    mappings.value = (props.provider.model_mappings || []).map(m => ({ ...m }))
  } else if (val) {
    form.value = { name: '', base_url: '', api_key: '', default_model: '', cli_type: null }
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
    message.warning('请输入名称')
    return
  }
  if (!/^[A-Za-z0-9_-]+$/.test(form.value.name.trim())) {
    message.warning('名称只支持英文、数字、下划线和 -')
    return
  }
  if (!form.value.base_url.trim()) {
    message.warning('请输入 API Base URL')
    return
  }
  if (!form.value.api_key.trim()) {
    message.warning('请输入 API Key')
    return
  }
  if (!form.value.default_model.trim()) {
    message.warning('请输入默认模型')
    return
  }
  if (!form.value.cli_type) {
    message.warning('请选择一个 CLI 平台')
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
    cli_types: [form.value.cli_type],
  })
  emit('update:visible', false)
}

function handleCancel() {
  emit('update:visible', false)
}
</script>

<template>
  <n-modal
    :show="visible"
    @update:show="(v: boolean) => { if (!v) handleCancel() }"
    :title="provider ? '编辑提供商' : '添加提供商'"
    :style="{ width: '520px' }"
    preset="card"
    :bordered="false"
  >
    <n-form label-placement="top">
      <n-form-item label="名称" required>
        <n-input v-model:value="form.name" placeholder="例如：DeepSeek_01" />
        <template #feedback>仅支持英文、数字、下划线和 -</template>
      </n-form-item>
      <n-form-item label="API Base URL" required>
        <n-input v-model:value="form.base_url" placeholder="例如：https://api.deepseek.com" />
      </n-form-item>
      <n-form-item label="API Key" required>
        <n-input v-model:value="form.api_key" type="password" show-password-on="click" placeholder="sk-..." />
      </n-form-item>

      <n-divider style="margin: 8px 0" />

      <n-form-item label="默认模型">
        <n-input v-model:value="form.default_model" placeholder="例如：gpt-4o" />
        <template #feedback>未命中模型映射时使用默认模型</template>
      </n-form-item>

      <n-form-item label="模型映射">
        <div style="display: flex; flex-direction: column; gap: 8px; width: 100%">
          <div v-for="(m, index) in mappings" :key="index" style="display: flex; align-items: center; gap: 8px">
            <n-input v-model:value="m.from" placeholder="外部模型名" size="small" style="flex: 1" />
            <n-text depth="3">→</n-text>
            <n-input v-model:value="m.to" placeholder="内部模型名" size="small" style="flex: 1" />
            <n-button text type="error" size="tiny" @click="removeMapping(index)">删除</n-button>
          </div>
          <n-button dashed size="small" @click="addMapping" block>+ 添加映射</n-button>
        </div>
        <template #feedback>命中映射时优先使用映射模型</template>
      </n-form-item>

      <n-divider style="margin: 8px 0" />

      <n-form-item label="支持的 CLI 平台" required>
        <n-radio-group v-model:value="form.cli_type">
          <n-space>
            <n-radio-button v-for="opt in cliOptions" :key="opt.value" :value="opt.value">
              <div style="display: inline-flex; align-items: center; gap: 6px">
                <CLIIcon :type="opt.value as CLIType" :size="14" />
                <span>{{ opt.label }}</span>
              </div>
            </n-radio-button>
          </n-space>
        </n-radio-group>
        <template #feedback>选择该提供商支持的 CLI 平台，代理会根据请求类型路由到对应提供商</template>
      </n-form-item>
    </n-form>

    <template #footer>
      <div style="display: flex; justify-content: flex-end; gap: 8px">
        <n-button @click="handleCancel">取消</n-button>
        <n-button type="primary" @click="handleSubmit">确定</n-button>
      </div>
    </template>
  </n-modal>
</template>

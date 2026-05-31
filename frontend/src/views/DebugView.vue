<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import * as App from '../../bindings/relay-ai/app'
import { getErrorMessage } from '../utils'

const emit = defineEmits<{ close: [] }>()
const message = useMessage()
const debugEnabled = ref(false)

onMounted(async () => {
  try {
    debugEnabled.value = await App.GetDebugMode()
  } catch {
    // ignore
  }
})

async function toggleDebug() {
  const newVal = !debugEnabled.value
  try {
    await App.SetDebugMode(newVal)
    debugEnabled.value = newVal
    message.success(newVal ? '调试模式已开启' : '调试模式已关闭')
  } catch (e: any) {
    message.error(getErrorMessage(e, '操作失败'))
  }
}
</script>

<template>
  <div style="height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 24px">
    <n-text strong style="font-size: 18px">调试面板</n-text>

    <div style="display: flex; align-items: center; gap: 12px">
      <n-switch :value="debugEnabled" @update:value="toggleDebug" />
      <n-text>详细日志（DEBUG 级别）</n-text>
    </div>

    <n-text depth="3" style="font-size: 12px; max-width: 400px; text-align: center">
      开启后，slog 日志级别切换为 Debug，输出所有调试信息。关闭后恢复 Info 级别，仅输出关键日志。
    </n-text>

    <n-button size="small" @click="emit('close')">关闭调试面板</n-button>
  </div>
</template>

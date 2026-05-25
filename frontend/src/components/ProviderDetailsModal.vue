<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Provider, ProviderUsagePoint, CLIType } from '../stores/app'
import { CLI_TYPES, useAppStore } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { maskKey } from '../utils'

const props = defineProps<{
  visible: boolean
  provider: Provider | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'reset-usage', id: string): void
}>()

const store = useAppStore()
const usageSeries = ref<ProviderUsagePoint[]>([])
const loadingSeries = ref(false)

const cliLabels: Record<string, string> = Object.fromEntries(
  CLI_TYPES.map(t => [t.key, t.label])
)

const cliTypes = computed(() => {
  const types = props.provider?.cli_types
  if (!types || types.length === 0) return []
  return [types[0]]
})

const chart = computed(() => buildChart(usageSeries.value))
const fieldLabelStyle = 'height: 30px; display: flex; align-items: center'
const fieldValueStyle = 'min-height: 30px; display: flex; align-items: center; min-width: 0'
const fieldBoxStyle = 'min-height: 30px; display: flex; align-items: center; border: 1px solid var(--app-border); border-radius: 6px; padding: 4px 8px; min-width: 0'

watch(
  () => [props.visible, props.provider?.id, props.provider?.usage_updated_at, props.provider?.total_tokens],
  async () => {
    if (!props.visible || !props.provider) {
      usageSeries.value = []
      return
    }
    loadingSeries.value = true
    try {
      usageSeries.value = await store.fetchProviderUsageSeries(props.provider.id)
    } finally {
      loadingSeries.value = false
    }
  },
  { immediate: true }
)

function close() {
  emit('update:visible', false)
}

function handleVisibleChange(val: boolean) {
  if (!val) close()
}

function formatTokens(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value || 0)
}

function formatDate(value?: number) {
  if (!value) return '暂无'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function buildChart(points: ProviderUsagePoint[]) {
  const width = 640
  const height = 220
  const padding = { top: 18, right: 24, bottom: 30, left: 46 }
  const innerWidth = width - padding.left - padding.right
  const innerHeight = height - padding.top - padding.bottom
  const maxValue = Math.max(1, ...points.map(p => p.total_tokens || 0))

  const axisBottom = padding.top + innerHeight
  const axisRight = padding.left + innerWidth
  const axisMid = padding.top + innerHeight / 2

  function toPoint(value: number, index: number) {
    const x = padding.left + (points.length <= 1 ? innerWidth : (index / (points.length - 1)) * innerWidth)
    const y = padding.top + innerHeight - (value / maxValue) * innerHeight
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }

  function line(key: keyof Pick<ProviderUsagePoint, 'prompt_tokens' | 'completion_tokens' | 'total_tokens'>) {
    return points.map((point, index) => toPoint(point[key] || 0, index)).join(' ')
  }

  return {
    width,
    height,
    maxValue,
    padding,
    axisBottom,
    axisRight,
    axisMid,
    hasData: points.length > 0,
    promptLine: line('prompt_tokens'),
    completionLine: line('completion_tokens'),
    totalLine: line('total_tokens'),
  }
}
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    :title="provider ? provider.name : '提供商详情'"
    style="width: min(860px, 92vw)"
    :bordered="false"
    @update:show="handleVisibleChange"
  >
    <div v-if="provider" style="display: flex; flex-direction: column; gap: 16px">
      <div style="display: flex; justify-content: space-between; gap: 12px; align-items: center">
        <div style="display: flex; align-items: center; gap: 8px">
          <n-tag :type="provider.enabled ? 'success' : 'default'" size="small">
            {{ provider.enabled ? '已启用' : '已禁用' }}
          </n-tag>
          <n-text depth="3" style="font-size: 12px">最后用量更新：{{ formatDate(provider.usage_updated_at) }}</n-text>
        </div>
        <div style="display: flex; gap: 8px">
          <n-popconfirm
            positive-text="确认"
            negative-text="取消"
            @positive-click="emit('reset-usage', provider.id)"
          >
            <template #trigger>
              <n-button size="small" type="warning">重置用量</n-button>
            </template>
            确定重置该提供商的用量统计？
          </n-popconfirm>
        </div>
      </div>

      <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px">
        <div style="padding: 12px; border: 1px solid var(--app-border); border-radius: 6px">
          <n-text depth="3" style="font-size: 12px; display: block">输入用量</n-text>
          <n-text strong style="font-size: 20px">{{ formatTokens(provider.prompt_tokens) }}</n-text>
        </div>
        <div style="padding: 12px; border: 1px solid var(--app-border); border-radius: 6px">
          <n-text depth="3" style="font-size: 12px; display: block">输出用量</n-text>
          <n-text strong style="font-size: 20px">{{ formatTokens(provider.completion_tokens) }}</n-text>
        </div>
        <div style="padding: 12px; border: 1px solid var(--app-border); border-radius: 6px">
          <n-text depth="3" style="font-size: 12px; display: block">总用量</n-text>
          <n-text strong style="font-size: 20px">{{ formatTokens(provider.total_tokens) }}</n-text>
        </div>
      </div>

      <div style="border: 1px solid var(--app-border); border-radius: 6px; padding: 12px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
          <n-text strong>用量曲线</n-text>
          <div style="display: flex; gap: 12px; align-items: center">
            <n-text depth="3" style="font-size: 12px">输入</n-text>
            <span style="width: 16px; height: 2px; background: #18a058"></span>
            <n-text depth="3" style="font-size: 12px">输出</n-text>
            <span style="width: 16px; height: 2px; background: #2080f0"></span>
            <n-text depth="3" style="font-size: 12px">总量</n-text>
            <span style="width: 16px; height: 2px; background: #d03050"></span>
          </div>
        </div>
        <n-spin :show="loadingSeries">
          <div style="width: 100%; overflow: hidden">
            <svg :viewBox="`0 0 ${chart.width} ${chart.height}`" style="width: 100%; height: 220px; display: block">
              <line :x1="chart.padding.left" :y1="chart.padding.top" :x2="chart.padding.left" :y2="chart.axisBottom" stroke="var(--app-border)" />
              <line :x1="chart.padding.left" :y1="chart.axisBottom" :x2="chart.axisRight" :y2="chart.axisBottom" stroke="var(--app-border)" />
              <line :x1="chart.padding.left" :y1="chart.axisMid" :x2="chart.axisRight" :y2="chart.axisMid" stroke="var(--app-border-2)" stroke-dasharray="4 4" />
              <text :x="chart.padding.left - 42" :y="chart.padding.top + 4" fill="currentColor" font-size="11">{{ formatTokens(chart.maxValue) }}</text>
              <text :x="chart.padding.left - 28" :y="chart.axisBottom + 4" fill="currentColor" font-size="11">0</text>
              <polyline v-if="chart.hasData" :points="chart.promptLine" fill="none" stroke="#18a058" stroke-width="2" />
              <polyline v-if="chart.hasData" :points="chart.completionLine" fill="none" stroke="#2080f0" stroke-width="2" />
              <polyline v-if="chart.hasData" :points="chart.totalLine" fill="none" stroke="#d03050" stroke-width="2.5" />
              <text v-if="!chart.hasData" x="320" y="112" text-anchor="middle" fill="currentColor" font-size="13">暂无曲线数据</text>
            </svg>
          </div>
        </n-spin>
      </div>

      <div style="display: grid; grid-template-columns: 76px minmax(0, 1fr); gap: 6px 10px; align-items: start">
        <n-text depth="3" :style="fieldLabelStyle">ID</n-text>
        <div :style="fieldBoxStyle">
          <n-text style="font-family: monospace; font-size: 12px; word-break: break-all">{{ provider.id }}</n-text>
        </div>
        <n-text depth="3" :style="fieldLabelStyle">创建时间</n-text>
        <n-text :style="fieldValueStyle">{{ formatDate(provider.created_at ? provider.created_at * 1000 : 0) }}</n-text>
        <n-text depth="3" :style="fieldLabelStyle">Base URL</n-text>
        <div :style="fieldBoxStyle">
          <n-text style="font-family: monospace; font-size: 12px; word-break: break-all">{{ provider.base_url }}</n-text>
        </div>
        <n-text depth="3" :style="fieldLabelStyle">API Key</n-text>
        <div :style="fieldBoxStyle">
          <n-text style="font-family: monospace; font-size: 12px; letter-spacing: 0.02em">{{ maskKey(provider.api_key) }}</n-text>
        </div>
        <n-text depth="3" :style="fieldLabelStyle">CLI</n-text>
        <div :style="fieldValueStyle">
          <n-tag v-if="cliTypes.length === 0" size="small">未选择</n-tag>
          <div v-for="t in cliTypes" :key="t" style="display: flex; align-items: center; gap: 4px">
            <CLIIcon :type="t as CLIType" :size="14" />
            <n-text>{{ cliLabels[t] || t }}</n-text>
          </div>
        </div>
      </div>

      <div style="display: grid; grid-template-columns: 76px minmax(0, 1fr); gap: 6px 10px; align-items: start">
        <n-text depth="3" :style="fieldLabelStyle">默认模型</n-text>
        <n-text :style="fieldValueStyle">{{ provider.default_model || '未设置' }}</n-text>
        <n-text depth="3" :style="fieldLabelStyle">模型映射</n-text>
        <div :style="fieldValueStyle" style="gap: 6px; flex-wrap: wrap">
          <n-tag v-if="!provider.model_mappings?.length" size="small">无</n-tag>
          <n-tag v-for="m in provider.model_mappings" :key="`${m.from}-${m.to}`" size="small" type="info">
            {{ m.from }} → {{ m.to }}
          </n-tag>
        </div>
      </div>
    </div>
  </n-modal>
</template>

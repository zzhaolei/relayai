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
}>()

const store = useAppStore()
const usageSeries = ref<ProviderUsagePoint[]>([])
const loadingSeries = ref(false)
const hoverIndex = ref(-1)
const hoverX = ref(0)
const hoverY = ref(0)

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

function formatDateShort(value?: number) {
  if (!value) return ''
  const d = new Date(value)
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
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

  function xPos(index: number) {
    return padding.left + (points.length <= 1 ? innerWidth : (index / (points.length - 1)) * innerWidth)
  }

  function yPos(value: number) {
    return padding.top + innerHeight - (value / maxValue) * innerHeight
  }

  interface Point { x: number; y: number; index: number }
  function makePoints(key: keyof Pick<ProviderUsagePoint, 'prompt_tokens' | 'completion_tokens' | 'total_tokens'>): Point[] {
    return points.map((p, i) => ({ x: xPos(i), y: yPos(p[key] || 0), index: i }))
  }

  function smoothPath(pts: Point[]): string {
    if (pts.length === 0) return ''
    if (pts.length === 1) return `M${pts[0].x.toFixed(1)},${pts[0].y.toFixed(1)}`
    if (pts.length === 2) return `M${pts[0].x.toFixed(1)},${pts[0].y.toFixed(1)} L${pts[1].x.toFixed(1)},${pts[1].y.toFixed(1)}`

    const n = pts.length
    const dxs: number[] = []
    const dys: number[] = []
    const ms: number[] = []
    for (let i = 0; i < n - 1; i++) {
      dxs[i] = pts[i + 1].x - pts[i].x
      dys[i] = pts[i + 1].y - pts[i].y
      ms[i] = dxs[i] !== 0 ? dys[i] / dxs[i] : 0
    }

    let d = `M${pts[0].x.toFixed(1)},${pts[0].y.toFixed(1)}`
    for (let i = 0; i < n - 1; i++) {
      const m = ms[i]
      let m0 = i > 0 ? ms[i - 1] : m
      let m1 = i < n - 2 ? ms[i + 1] : m
      if (m0 * m <= 0) m0 = 0
      if (m1 * m <= 0) m1 = 0
      const dx = dxs[i] / 3
      const cx1 = pts[i].x + dx
      const cy1 = pts[i].y + m0 * dx
      const cx2 = pts[i + 1].x - dx
      const cy2 = pts[i + 1].y - m1 * dx
      d += ` C${cx1.toFixed(1)},${cy1.toFixed(1)} ${cx2.toFixed(1)},${cy2.toFixed(1)} ${pts[i + 1].x.toFixed(1)},${pts[i + 1].y.toFixed(1)}`
    }
    return d
  }

  const promptPts = makePoints('prompt_tokens')
  const completionPts = makePoints('completion_tokens')
  const totalPts = makePoints('total_tokens')

  // Build array of hit targets for hover detection
  const hitTargets = points.map((p, i) => ({
    x: xPos(i),
    y: yPos(p.total_tokens || 0),
    index: i,
    time: p.time,
    prompt_tokens: p.prompt_tokens || 0,
    completion_tokens: p.completion_tokens || 0,
    total_tokens: p.total_tokens || 0,
  }))

  return {
    width,
    height,
    maxValue,
    padding,
    axisBottom,
    axisRight,
    axisMid,
    hasData: points.length > 0,
    promptPath: smoothPath(promptPts),
    completionPath: smoothPath(completionPts),
    totalPath: smoothPath(totalPts),
    hitTargets,
  }
}

function handleChartMouseMove(e: MouseEvent) {
  const svg = (e.currentTarget as SVGElement).closest('.chart-container')?.querySelector('svg')
  if (!svg) return
  const rect = svg.getBoundingClientRect()
  const svgW = chart.value.width
  const svgH = chart.value.height
  const scaleX = svgW / rect.width
  const scaleY = svgH / rect.height
  const mx = (e.clientX - rect.left) * scaleX
  const my = (e.clientY - rect.top) * scaleY

  const targets = chart.value.hitTargets
  if (targets.length === 0) {
    hoverIndex.value = -1
    return
  }

  // Find closest target by x distance within a threshold
  const threshold = 30
  let closest = -1
  let minDist = Infinity
  for (let i = 0; i < targets.length; i++) {
    const dx = Math.abs(targets[i].x - mx)
    const dy = Math.abs(targets[i].y - my)
    const dist = Math.sqrt(dx * dx + dy * dy)
    if (dist < minDist && dist < threshold) {
      minDist = dist
      closest = i
    }
  }
  hoverIndex.value = closest
  if (closest >= 0) {
    hoverX.value = targets[closest].x
    hoverY.value = targets[closest].y
  }
}

function handleChartMouseLeave() {
  hoverIndex.value = -1
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
          <div
            class="chart-container"
            style="width: 100%; overflow: hidden; position: relative"
            @mousemove="handleChartMouseMove"
            @mouseleave="handleChartMouseLeave"
          >
            <svg :viewBox="`0 0 ${chart.width} ${chart.height}`" style="width: 100%; height: 220px; display: block">
              <!-- Axes -->
              <line :x1="chart.padding.left" :y1="chart.padding.top" :x2="chart.padding.left" :y2="chart.axisBottom" stroke="var(--app-border)" />
              <line :x1="chart.padding.left" :y1="chart.axisBottom" :x2="chart.axisRight" :y2="chart.axisBottom" stroke="var(--app-border)" />
              <line :x1="chart.padding.left" :y1="chart.axisMid" :x2="chart.axisRight" :y2="chart.axisMid" stroke="var(--app-border-2)" stroke-dasharray="4 4" />
              <text :x="chart.padding.left - 42" :y="chart.padding.top + 4" fill="currentColor" font-size="11">{{ formatTokens(chart.maxValue) }}</text>
              <text :x="chart.padding.left - 28" :y="chart.axisBottom + 4" fill="currentColor" font-size="11">0</text>

              <!-- Smooth curves -->
              <path v-if="chart.hasData" :d="chart.promptPath" fill="none" stroke="#18a058" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
              <path v-if="chart.hasData" :d="chart.completionPath" fill="none" stroke="#2080f0" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
              <path v-if="chart.hasData" :d="chart.totalPath" fill="none" stroke="#d03050" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />

              <!-- Invisible hit targets for mouse hover -->
              <circle
                v-for="(t, i) in chart.hitTargets"
                :key="i"
                :cx="t.x"
                :cy="t.y"
                r="8"
                fill="transparent"
                stroke="none"
              />

              <!-- Hover indicator dot -->
              <circle
                v-if="hoverIndex >= 0 && chart.hasData"
                :cx="hoverX"
                :cy="hoverY"
                r="4"
                fill="#d03050"
                stroke="#fff"
                stroke-width="2"
              />

              <text v-if="!chart.hasData" x="320" y="112" text-anchor="middle" fill="currentColor" font-size="13">暂无曲线数据</text>
            </svg>

            <!-- Tooltip -->
            <div
              v-if="hoverIndex >= 0 && chart.hasData"
              class="chart-tooltip"
              :style="{
                left: ((hoverX / chart.width) * 100) + '%',
                top: ((hoverY / chart.height) * 100) + '%',
              }"
            >
              <div class="chart-tooltip-time">{{ formatDateShort(chart.hitTargets[hoverIndex].time) }}</div>
              <div class="chart-tooltip-row">
                <span class="chart-tooltip-dot" style="background: #18a058"></span>
                <span>输入</span>
                <span class="chart-tooltip-val">{{ formatTokens(chart.hitTargets[hoverIndex].prompt_tokens) }}</span>
              </div>
              <div class="chart-tooltip-row">
                <span class="chart-tooltip-dot" style="background: #2080f0"></span>
                <span>输出</span>
                <span class="chart-tooltip-val">{{ formatTokens(chart.hitTargets[hoverIndex].completion_tokens) }}</span>
              </div>
              <div class="chart-tooltip-row">
                <span class="chart-tooltip-dot" style="background: #d03050"></span>
                <span>总量</span>
                <span class="chart-tooltip-val">{{ formatTokens(chart.hitTargets[hoverIndex].total_tokens) }}</span>
              </div>
            </div>
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

<style scoped>
.chart-tooltip {
  position: absolute;
  transform: translate(-50%, calc(-100% - 12px));
  background: transparent;
  border: none;
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
  line-height: 1.6;
  pointer-events: none;
  white-space: nowrap;
  z-index: 10;
}

.chart-tooltip-time {
  font-weight: 600;
  color: #333;
  margin-bottom: 2px;
  font-size: 11px;
}

.chart-tooltip-row {
  display: flex;
  align-items: center;
  gap: 5px;
  color: #666;
  font-size: 11px;
}

.chart-tooltip-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
}

.chart-tooltip-val {
  font-family: monospace;
  font-variant-numeric: tabular-nums;
  color: #333;
  margin-left: auto;
  padding-left: 6px;
}
</style>

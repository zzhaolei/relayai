<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Provider, ProviderUsagePoint, CLIType } from '../stores/app'
import { CLI_TYPES, useAppStore } from '../stores/app'
import CLIIcon from './CLIIcon.vue'
import { maskKey, formatTokens } from '../utils'

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
const mouseX = ref(0)
const mouseY = ref(0)
const showPrompt = ref(true)
const showCompletion = ref(true)
const showTotal = ref(true)

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
    showPrompt.value = true
    showCompletion.value = true
    showTotal.value = true
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


function formatDate(value?: number) {
  if (!value) return '暂无'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function formatDateShort(value?: number) {
  if (!value) return ''
  const d = new Date(value)
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
}

function formatChartValue(value: number): string {
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'm'
  if (value >= 1_000) return (value / 1_000).toFixed(1).replace(/\.0$/, '') + 'k'
  return value.toString()
}

function buildChart(points: ProviderUsagePoint[]) {
  const width = 640
  const height = 240
  const padding = { top: 18, right: 24, bottom: 38, left: 50 }
  const innerWidth = width - padding.left - padding.right
  const innerHeight = height - padding.top - padding.bottom
  const maxValue = Math.max(1, ...points.map(p => p.total_tokens || 0))

  const axisBottom = padding.top + innerHeight
  const axisRight = padding.left + innerWidth

  function xPos(index: number) {
    return padding.left + (points.length <= 1 ? innerWidth / 2 : (index / (points.length - 1)) * innerWidth)
  }

  function yPos(value: number) {
    return padding.top + innerHeight - (value / maxValue) * innerHeight
  }

  // 总量曲线点
  const totalPts = points.map((p, i) => ({ x: xPos(i), y: yPos(p.total_tokens || 0) }))

  function smoothPath(pts: { x: number; y: number }[]): string {
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
      const cy1 = Math.min(pts[i].y + m0 * dx, axisBottom)
      const cx2 = pts[i + 1].x - dx
      const cy2 = Math.min(pts[i + 1].y - m1 * dx, axisBottom)
      d += ` C${cx1.toFixed(1)},${cy1.toFixed(1)} ${cx2.toFixed(1)},${cy2.toFixed(1)} ${pts[i + 1].x.toFixed(1)},${pts[i + 1].y.toFixed(1)}`
    }
    return d
  }

  // 柱状图：输入/输出叠加，高度以总量为准
  const barWidth = points.length <= 1 ? 40 : Math.max(6, Math.min(40, innerWidth / points.length * 0.6))

  const bars = points.map((p, i) => {
    const cx = xPos(i)
    const totalVal = p.total_tokens || 0
    const promptVal = p.prompt_tokens || 0
    const completionVal = p.completion_tokens || 0
    const totalH = totalVal / maxValue * innerHeight
    const promptH = totalVal > 0 ? (promptVal / totalVal) * totalH : 0
    const completionH = totalVal > 0 ? (completionVal / totalVal) * totalH : 0
    return {
      cx,
      x: cx - barWidth / 2,
      barWidth,
      // 输出在下面
      completionY: axisBottom - completionH,
      completionH,
      // 输入叠在输出上面
      promptY: axisBottom - completionH - promptH,
      promptH,
    }
  })

  // X轴时间标签（取首、中、尾）
  const xLabels: { x: number; label: string }[] = []
  if (points.length > 0) {
    xLabels.push({ x: xPos(0), label: formatXLabel(points[0].time) })
    if (points.length > 2) {
      const mid = Math.floor(points.length / 2)
      xLabels.push({ x: xPos(mid), label: formatXLabel(points[mid].time) })
    }
    if (points.length > 1) {
      xLabels.push({ x: xPos(points.length - 1), label: formatXLabel(points[points.length - 1].time) })
    }
  }

  // Y轴刻度（4条）
  const yTicks = [0, 0.25, 0.5, 0.75, 1].map(r => ({
    y: axisBottom - r * innerHeight,
    value: Math.round(maxValue * r),
    label: formatChartValue(Math.round(maxValue * r)),
  }))

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
    hasData: points.length > 0,
    totalPath: smoothPath(totalPts),
    bars,
    xLabels,
    yTicks,
    hitTargets,
    barWidth,
  }
}

function formatXLabel(time: number): string {
  const d = new Date(time)
  return `${(d.getMonth() + 1).toString().padStart(2, '0')}/${d.getDate().toString().padStart(2, '0')} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
}

function handleChartMouseMove(e: MouseEvent) {
  const container = e.currentTarget as HTMLElement
  const svg = container.querySelector('svg') as SVGSVGElement | null
  if (!svg) return

  // Tooltip position: relative to the container
  const containerRect = container.getBoundingClientRect()
  mouseX.value = e.clientX - containerRect.left
  mouseY.value = e.clientY - containerRect.top

  // Use SVG getScreenCTM for accurate viewBox coordinate mapping
  // This correctly handles preserveAspectRatio centering/scaling
  const ctm = svg.getScreenCTM()
  if (!ctm) return
  const invCTM = ctm.inverse()
  const svgX = invCTM.a * e.clientX + invCTM.c * e.clientY + invCTM.e

  const targets = chart.value.hitTargets
  if (targets.length === 0) {
    hoverIndex.value = -1
    return
  }

  // Find nearest data point by x distance in viewBox coordinates
  let closest = 0
  let minDx = Math.abs(targets[0].x - svgX)
  for (let i = 1; i < targets.length; i++) {
    const dx = Math.abs(targets[i].x - svgX)
    if (dx < minDx) {
      minDx = dx
      closest = i
    }
  }
  hoverIndex.value = closest
  hoverX.value = targets[closest].x
  hoverY.value = targets[closest].y
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
    :style="{ width: 'min(860px, 92vw)', zIndex: 2000, transform: 'translateY(42px)' }"
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
          <div style="display: flex; gap: 14px; align-items: center">
            <div style="display: flex; align-items: center; gap: 4px; cursor: pointer; user-select: none; opacity: 1" @click="showPrompt = !showPrompt" :style="{ opacity: showPrompt ? 1 : 0.35 }">
              <span style="width: 10px; height: 10px; background: #18a058; border-radius: 2px"></span>
              <n-text depth="3" style="font-size: 12px">输入</n-text>
            </div>
            <div style="display: flex; align-items: center; gap: 4px; cursor: pointer; user-select: none; opacity: 1" @click="showCompletion = !showCompletion" :style="{ opacity: showCompletion ? 1 : 0.35 }">
              <span style="width: 10px; height: 10px; background: #f0a020; border-radius: 2px"></span>
              <n-text depth="3" style="font-size: 12px">输出</n-text>
            </div>
            <div style="display: flex; align-items: center; gap: 4px; cursor: pointer; user-select: none; opacity: 1" @click="showTotal = !showTotal" :style="{ opacity: showTotal ? 1 : 0.35 }">
              <span style="width: 16px; height: 2px; background: #d03050; border-radius: 1px"></span>
              <n-text depth="3" style="font-size: 12px">总量</n-text>
            </div>
          </div>
        </div>
        <n-spin :show="loadingSeries">
          <div
            class="chart-container"
            style="width: 100%; overflow: hidden; position: relative"
            @mousemove="handleChartMouseMove"
            @mouseleave="handleChartMouseLeave"
          >
            <svg :viewBox="`0 0 ${chart.width} ${chart.height}`" style="width: 100%; height: 240px; display: block">
              <!-- Y轴网格线 -->
              <line
                v-for="(tick, i) in chart.yTicks"
                :key="'yg'+i"
                :x1="chart.padding.left"
                :y1="tick.y"
                :x2="chart.axisRight"
                :y2="tick.y"
                stroke="var(--app-border-2)"
                stroke-dasharray="4 4"
                :stroke-opacity="i === chart.yTicks.length - 1 ? 0 : 0.6"
              />

              <!-- 柱状图：输入/输出叠加 -->
              <template v-if="chart.hasData">
                <!-- 输出（下半部分） -->
                <rect
                  v-if="showCompletion"
                  v-for="(bar, i) in chart.bars"
                  :key="'bc'+i"
                  :x="bar.x"
                  :y="bar.completionY"
                  :width="bar.barWidth"
                  :height="bar.completionH"
                  fill="#f0a020"
                  rx="1"
                />
                <!-- 输入（上半部分，叠在输出上面） -->
                <rect
                  v-if="showPrompt"
                  v-for="(bar, i) in chart.bars"
                  :key="'bp'+i"
                  :x="bar.x"
                  :y="bar.promptY"
                  :width="bar.barWidth"
                  :height="bar.promptH"
                  fill="#18a058"
                  rx="1"
                />
              </template>

              <!-- 总量曲线 -->
              <path v-if="chart.hasData && showTotal" :d="chart.totalPath" fill="none" stroke="#d03050" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />

              <!-- 坐标轴 -->
              <line :x1="chart.padding.left" :y1="chart.padding.top" :x2="chart.padding.left" :y2="chart.axisBottom" stroke="var(--app-border)" />
              <line :x1="chart.padding.left" :y1="chart.axisBottom" :x2="chart.axisRight" :y2="chart.axisBottom" stroke="var(--app-border)" />

              <!-- Y轴标签 -->
              <text
                v-for="(tick, i) in chart.yTicks"
                :key="'yl'+i"
                :x="chart.padding.left - 8"
                :y="tick.y + 4"
                fill="currentColor"
                font-size="10"
                text-anchor="end"
              >{{ tick.label }}</text>

              <!-- X轴时间标签 -->
              <text
                v-for="(label, i) in chart.xLabels"
                :key="'xl'+i"
                :x="label.x"
                :y="chart.axisBottom + 16"
                fill="currentColor"
                font-size="10"
                text-anchor="middle"
              >{{ label.label }}</text>

              <!-- Full-width transparent overlay for reliable mouse hover across entire chart -->
              <rect
                :x="chart.padding.left"
                :y="chart.padding.top"
                :width="chart.axisRight - chart.padding.left"
                :height="chart.axisBottom - chart.padding.top"
                fill="transparent"
                stroke="none"
                style="cursor: crosshair"
              />

              <!-- Hover indicator line -->
              <line
                v-if="hoverIndex >= 0 && chart.hasData"
                :x1="hoverX"
                :y1="chart.padding.top"
                :x2="hoverX"
                :y2="chart.axisBottom"
                stroke="var(--app-text-3)"
                stroke-width="1"
                stroke-dasharray="3 3"
              />

              <!-- Hover indicator dot on curve -->
              <circle
                v-if="hoverIndex >= 0 && chart.hasData"
                :cx="hoverX"
                :cy="hoverY"
                r="4"
                fill="#d03050"
                stroke="#fff"
                stroke-width="2"
              />

              <text v-if="!chart.hasData" x="320" y="120" text-anchor="middle" fill="currentColor" font-size="13">暂无曲线数据</text>
            </svg>

            <!-- Tooltip -->
            <div
              v-if="hoverIndex >= 0 && chart.hasData"
              class="chart-tooltip"
              :style="{
                left: mouseX + 'px',
                top: (mouseY - 12) + 'px',
                transform: 'translate(-50%, -100%)',
              }"
            >
              <div class="chart-tooltip-time">{{ formatDateShort(chart.hitTargets[hoverIndex].time) }}</div>
              <div class="chart-tooltip-row">
                <span class="chart-tooltip-dot" style="background: #18a058"></span>
                <span>输入</span>
                <span class="chart-tooltip-val">{{ formatTokens(chart.hitTargets[hoverIndex].prompt_tokens) }}</span>
              </div>
              <div class="chart-tooltip-row">
                <span class="chart-tooltip-dot" style="background: #f0a020"></span>
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
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid var(--app-border);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1.6;
  pointer-events: none;
  white-space: nowrap;
  z-index: 10;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
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

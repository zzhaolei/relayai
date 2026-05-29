<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAppStore } from '../stores/app'
import CLIIcon from '../components/CLIIcon.vue'
import type { CLIType } from '../stores/app'
import appIcon from '../assets/appicon.png'

const store = useAppStore()

const appName = 'RelayAI'
const appVersion = 'v1.0.0'
const appDescription = 'AI 模型反代管理工具'
const goVersion = 'Go 1.26'
const wailsVersion = 'Wails v3'
const vueVersion = 'Vue 3'
const uiFramework = 'Naive UI'

const proxyPort = ref(18900)
const dbPath = '~/.relayai/relayai.db'

onMounted(async () => {
  await store.fetchProxyStatus()
  proxyPort.value = store.proxyStatus.port
})

const links = [
  { label: 'GitHub', url: 'https://github.com/zzhaolei/relayai' },
  { label: '问题反馈', url: 'https://github.com/zzhaolei/relayai/issues' },
  { label: 'Wails', url: 'https://wails.io' },
  { label: 'Naive UI', url: 'https://www.naiveui.com' },
]

function openExternal(url: string) {
  window.open(url, '_blank')
}
</script>

<template>
  <div style="height: 100%; overflow-y: auto; padding: 24px 32px">
    <div style="max-width: 560px; margin: 0 auto">
      <!-- App Info -->
      <div style="display: flex; align-items: center; gap: 16px; margin-bottom: 32px">
        <img
          :src="appIcon"
          alt="RelayAI"
          style="width: 64px; height: 64px; border-radius: 14px; flex-shrink: 0"
        />
        <div>
          <n-text strong style="font-size: 22px; display: block">{{ appName }}</n-text>
          <n-text depth="3" style="font-size: 13px">{{ appDescription }}</n-text>
        </div>
        <n-tag size="small" style="margin-left: auto; align-self: flex-start; margin-top: 4px">{{ appVersion }}</n-tag>
      </div>

      <n-divider style="margin: 0 0 24px 0" />

      <!-- System Info -->
      <n-text strong style="font-size: 14px; display: block; margin-bottom: 12px">运行环境</n-text>
      <div style="display: grid; grid-template-columns: 100px minmax(0, 1fr); gap: 8px 16px; margin-bottom: 24px">
        <n-text depth="3" style="font-size: 13px; display: flex; align-items: center">代理端口</n-text>
        <n-text code style="font-size: 13px">127.0.0.1:{{ proxyPort }}</n-text>
        <n-text depth="3" style="font-size: 13px; display: flex; align-items: center">数据库</n-text>
        <n-text code style="font-size: 13px">{{ dbPath }}</n-text>
        <n-text depth="3" style="font-size: 13px; display: flex; align-items: center">Go 版本</n-text>
        <n-text style="font-size: 13px">{{ goVersion }}</n-text>
        <n-text depth="3" style="font-size: 13px; display: flex; align-items: center">桌面框架</n-text>
        <n-text style="font-size: 13px">{{ wailsVersion }}</n-text>
        <n-text depth="3" style="font-size: 13px; display: flex; align-items: center">前端框架</n-text>
        <n-text style="font-size: 13px">{{ vueVersion }}</n-text>
        <n-text depth="3" style="font-size: 13px; display: flex; align-items: center">UI 组件库</n-text>
        <n-text style="font-size: 13px">{{ uiFramework }}</n-text>
      </div>

      <!-- Supported CLIs -->
      <n-text strong style="font-size: 14px; display: block; margin-bottom: 12px">支持的 CLI</n-text>
      <div style="display: flex; gap: 12px; margin-bottom: 24px">
        <n-card size="small" style="flex: 1" hoverable>
          <div style="display: flex; align-items: center; gap: 8px">
            <CLIIcon type="claude" :size="20" />
            <div>
              <n-text strong style="font-size: 13px; display: block">Claude Code</n-text>
              <n-text depth="3" style="font-size: 11px">Anthropic /anthropic</n-text>
            </div>
          </div>
        </n-card>
        <n-card size="small" style="flex: 1" hoverable>
          <div style="display: flex; align-items: center; gap: 8px">
            <CLIIcon type="codex" :size="20" />
            <div>
              <n-text strong style="font-size: 13px; display: block">Codex</n-text>
              <n-text depth="3" style="font-size: 11px">OpenAI /openai</n-text>
            </div>
          </div>
        </n-card>
      </div>

      <!-- Links -->
      <n-text strong style="font-size: 14px; display: block; margin-bottom: 12px">相关链接</n-text>
      <div style="display: flex; flex-direction: column; gap: 6px; margin-bottom: 24px">
        <div
          v-for="link in links"
          :key="link.label"
          style="display: flex; align-items: center; justify-content: space-between; padding: 8px 12px; border: 1px solid var(--app-border); border-radius: 6px; cursor: pointer; transition: border-color 0.2s"
          @click="openExternal(link.url)"
          @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--app-success)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'var(--app-border)'"
        >
          <n-text style="font-size: 13px">{{ link.label }}</n-text>
          <n-text depth="3" style="font-size: 12px; font-family: monospace">{{ link.url }}</n-text>
        </div>
      </div>

      <!-- Footer -->
      <n-divider style="margin: 0 0 16px 0" />
      <div style="text-align: center; padding-bottom: 16px">
        <n-text depth="3" style="font-size: 12px">Built with Wails v3 + Vue 3 + Go</n-text>
      </div>
    </div>
  </div>
</template>

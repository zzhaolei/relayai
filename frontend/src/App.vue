<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { dateZhCN, zhCN } from 'naive-ui'
import { useTheme } from './composables/useTheme'
import type { ThemeMode } from './composables/useTheme'
import * as App from '../bindings/relay-ai/app'
import ProvidersView from './views/ProvidersView.vue'
import LogViewer from './components/LogViewer.vue'
import AboutView from './views/AboutView.vue'
import DebugView from './views/DebugView.vue'

const activeTab = ref('providers')
const debugPageVisible = ref(false)

const tabs = computed(() => {
  const list = [
    { key: 'providers', label: '提供商' },
    { key: 'logs', label: '日志' },
  ]
  if (debugPageVisible.value) {
    list.push({ key: 'debug', label: '调试' })
  }
  list.push({ key: 'about', label: '关于' })
  return list
})

function onDebugUnlock() {
  debugPageVisible.value = true
  activeTab.value = 'debug'
}

function onDebugClose() {
  debugPageVisible.value = false
  activeTab.value = 'about'
}

const { themeMode, theme, setTheme } = useTheme()

const isMac = computed(() => navigator.platform.toLowerCase().includes('mac'))

const themeOptions: { label: string; value: ThemeMode }[] = [
  { label: '跟随系统', value: 'system' },
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
]

watch(themeMode, (val) => {
  // Defer the Wails bridge call so CSS variable changes apply first.
  // The visual theme (CSS vars) updates instantly; this call only
  // syncs the native window chrome (title bar) with the OS.
  requestAnimationFrame(() => {
    setTimeout(() => {
      App.SetAppearanceMode(val).catch(() => {})
    }, 0)
  })
}, { immediate: true })
</script>

<template>
  <n-config-provider :theme="theme" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
    <div class="app-root" :class="{ 'is-mac': isMac, 'is-win': !isMac }">
      <div class="titlebar-area">
        <div v-if="isMac" class="titlebar-spacer titlebar-spacer-left"></div>

        <div class="titlebar-content">
          <div class="titlebar-tabs">
            <button
              v-for="tab in tabs"
              :key="tab.key"
              class="titlebar-tab"
              :class="{ active: activeTab === tab.key }"
              @click="activeTab = tab.key"
            >{{ tab.label }}</button>
          </div>
          <div class="theme-switcher">
            <button
              v-for="opt in themeOptions"
              :key="opt.value"
              class="theme-btn"
              :class="{ active: themeMode === opt.value }"
              @click="setTheme(opt.value)"
            >{{ opt.label }}</button>
          </div>
        </div>

        <div v-if="!isMac" class="titlebar-spacer titlebar-spacer-right"></div>
      </div>

      <div class="app-content">
        <ProvidersView v-if="activeTab === 'providers'" />
        <keep-alive>
        <LogViewer v-if="activeTab === 'logs'" :active="true" />
      </keep-alive>
        <DebugView v-if="activeTab === 'debug'" @close="onDebugClose" />
        <AboutView v-if="activeTab === 'about'" @debug-unlock="onDebugUnlock" />
      </div>
    </div>
    </n-message-provider>
  </n-config-provider>
</template>

<style>
.app-root {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--app-bg-1);
  overflow: hidden;
  overscroll-behavior: none;
}

/* ---- Title bar ---- */
.titlebar-area {
  display: flex;
  align-items: center;
  height: 38px;
  flex-shrink: 0;
  -webkit-app-region: drag;
  user-select: none;
  background: var(--app-bg-1);
  border-bottom: 1px solid var(--app-border-2);
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 9999;
}

.titlebar-spacer-left {
  width: 70px;
}

.titlebar-spacer-right {
  width: 140px;
}

.titlebar-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex: 1;
  min-width: 0;
  padding: 0 12px;
  -webkit-app-region: no-drag;
}

/* ---- Tabs ---- */
.titlebar-tabs {
  display: flex;
  align-items: center;
  gap: 2px;
}

.titlebar-tab {
  -webkit-app-region: no-drag;
  background: none;
  border: none;
  padding: 5px 14px;
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-3);
  cursor: pointer;
  border-radius: 6px;
  line-height: 1.4;
  white-space: nowrap;
}

.titlebar-tab:hover {
  color: var(--app-text-2);
  background: var(--app-fill-1);
}

.titlebar-tab.active {
  color: var(--app-text-2);
  background: var(--app-fill-2);
  font-weight: 600;
}

/* ---- Theme switcher ---- */
.theme-switcher {
  display: flex;
  align-items: center;
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  overflow: hidden;
  flex-shrink: 0;
}

.theme-btn {
  -webkit-app-region: no-drag;
  background: none;
  border: none;
  border-right: 1px solid var(--app-border-2);
  padding: 3px 10px;
  font-size: 12px;
  font-weight: 500;
  color: var(--app-text-3);
  cursor: pointer;
  white-space: nowrap;
  line-height: 1.4;
}

.theme-btn:last-child {
  border-right: none;
}

.theme-btn:hover {
  background: var(--app-fill-1);
}

.theme-btn.active {
  color: var(--app-success);
  background: rgba(24, 160, 88, 0.08);
  font-weight: 600;
}

/* ---- Content ---- */
.app-content {
  flex: 1;
  overflow: hidden;
  overscroll-behavior: contain;
  padding-top: 38px;
}

/* ---- Modal constraints ---- */
/* Ensure all Naive UI modals stay below the titlebar area on all platforms.
   macOS traffic lights sit at ~0-38px from top; Windows/Linux title bar is similar.
   Target multiple possible class names for robustness. */
.n-modal-mask,
.modal-mask {
  top: 42px !important;
  height: calc(100vh - 42px) !important;
}
.n-modal-container,
.modal-container {
  padding-top: 42px !important;
  max-height: calc(100vh - 42px) !important;
}
/* Also target the scroll content wrapper */
.modal-scroll-content {
  align-items: flex-start !important;
  padding-top: 0 !important;
}

/* Push all notification and message containers below the titlebar (42px) */
.n-message-container,
.n-notification-container {
  top: 42px !important;
}
</style>

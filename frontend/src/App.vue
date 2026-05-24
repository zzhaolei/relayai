<script setup lang="ts">
import { ref } from 'vue'
import { useTheme } from './composables/useTheme'
import type { ThemeMode } from './composables/useTheme'
import ProvidersView from './views/ProvidersView.vue'
import LogViewer from './components/LogViewer.vue'

const activeTab = ref('providers')
const { themeMode, isDark, theme, setTheme } = useTheme()

const themeOptions = [
  { label: '跟随系统', value: 'system' as ThemeMode },
  { label: '浅色', value: 'light' as ThemeMode },
  { label: '深色', value: 'dark' as ThemeMode },
]
</script>

<template>
  <n-config-provider :theme="theme">
    <n-message-provider>
    <n-layout style="height: 100vh; display: flex; flex-direction: column">
      <n-layout-header bordered style="padding: 0 16px; flex-shrink: 0">
        <div style="display: flex; justify-content: space-between; align-items: center">
          <n-tabs v-model:value="activeTab" type="line" animated>
            <n-tab name="providers">提供商</n-tab>
            <n-tab name="logs">日志</n-tab>
          </n-tabs>
          <n-radio-group
            :value="themeMode"
            @update:value="(val: ThemeMode) => setTheme(val)"
            size="small"
          >
            <n-radio-button v-for="opt in themeOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </n-radio-button>
          </n-radio-group>
        </div>
      </n-layout-header>
      <n-layout-content style="flex: 1; overflow: hidden">
        <ProvidersView v-if="activeTab === 'providers'" />
        <LogViewer v-else />
      </n-layout-content>
    </n-layout>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import ProvidersView from './views/ProvidersView.vue'
import LogViewer from './components/LogViewer.vue'

const activeTab = ref('providers')
</script>

<template>
  <div class="app-container">
    <div class="app-tabs">
      <div
        class="tab-item"
        :class="{ active: activeTab === 'providers' }"
        @click="activeTab = 'providers'"
      >
        提供商
      </div>
      <div
        class="tab-item"
        :class="{ active: activeTab === 'logs' }"
        @click="activeTab = 'logs'"
      >
        日志
      </div>
    </div>
    <div class="app-content">
      <ProvidersView v-if="activeTab === 'providers'" />
      <LogViewer v-else />
    </div>
  </div>
</template>

<style scoped>
.app-container {
  width: 100%;
  height: 100vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.app-tabs {
  display: flex;
  gap: 0;
  background: var(--color-bg-2);
  border-bottom: 1px solid var(--color-border);
  padding: 0 16px;
  flex-shrink: 0;
}
.tab-item {
  padding: 10px 20px;
  font-size: 14px;
  font-weight: 500;
  color: var(--color-text-3);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.2s, border-color 0.2s;
  user-select: none;
}
.tab-item:hover {
  color: var(--color-text-2);
}
.tab-item.active {
  color: var(--color-primary-6);
  border-bottom-color: var(--color-primary-6);
}
.app-content {
  flex: 1;
  overflow: hidden;
}
</style>

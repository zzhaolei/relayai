<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '../stores/app'

const store = useAppStore()

const statusText = computed(() => store.proxyStatus.running ? '运行中' : '已停止')

async function handleToggle() {
  if (store.proxyStatus.running) {
    await store.stopProxy()
  } else {
    await store.startProxy()
  }
}
</script>

<template>
  <div class="proxy-status-bar">
    <div class="status-left">
      <div class="status-dot" :class="{ active: store.proxyStatus.running }" />
      <span class="status-label">代理服务</span>
      <a-tag :color="store.proxyStatus.running ? 'green' : 'red'" size="small">
        {{ statusText }}
      </a-tag>
      <span class="addr">{{ store.proxyStatus.addr }}</span>
    </div>
    <div class="status-right">
      <a-button
        type="text"
        size="mini"
        @click="store.restartProxy"
        :disabled="!store.proxyStatus.running"
      >
        重启服务
      </a-button>
      <a-switch
        :model-value="store.proxyStatus.running"
        @change="handleToggle"
        size="small"
      />
    </div>
  </div>
</template>

<style scoped>
.proxy-status-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: var(--color-bg-2);
  border-bottom: 1px solid var(--color-border);
}
.status-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-danger-light-4);
  transition: background 0.3s;
}
.status-dot.active {
  background: var(--color-success-light-4);
  box-shadow: 0 0 6px var(--color-success-light-3);
}
.status-label {
  font-size: 13px;
  font-weight: 500;
}
.addr {
  font-size: 12px;
  color: var(--color-text-3);
  font-family: monospace;
}
.status-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>

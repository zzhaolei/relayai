import { ref, computed, watch } from 'vue'
import { darkTheme } from 'naive-ui'

export type ThemeMode = 'light' | 'dark' | 'system'

const themeMode = ref<ThemeMode>(
  (localStorage.getItem('theme-mode') as ThemeMode) || 'system'
)
const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(mediaQuery.matches)

const isDark = computed(() => {
  if (themeMode.value === 'system') return systemDark.value
  return themeMode.value === 'dark'
})

const theme = computed(() => isDark.value ? darkTheme : null)

mediaQuery.addEventListener('change', (e) => {
  systemDark.value = e.matches
})

watch(isDark, (val) => {
  document.documentElement.classList.toggle('dark', val)
}, { immediate: true })

export function useTheme() {
  function setTheme(mode: ThemeMode) {
    themeMode.value = mode
    localStorage.setItem('theme-mode', mode)
  }

  return {
    themeMode,
    isDark,
    theme,
    setTheme,
  }
}

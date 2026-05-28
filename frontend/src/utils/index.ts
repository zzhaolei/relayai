/**
 * 遮蔽 API Key，只显示前4位和后4位
 * 如果以 sk-local- 开头，则保留该前缀，从后面开始遮蔽
 */
export function maskKey(key: string): string {
  if (!key) return '未设置'
  
  // 如果以 sk-local- 开头，保留前缀，遮蔽中间部分
  const prefix = 'sk-local-'
  if (key.startsWith(prefix)) {
    const rest = key.slice(prefix.length)
    if (rest.length <= 8) return prefix + '****'
    return prefix + rest.slice(0, 4) + '****' + rest.slice(-4)
  }
  
  // 默认：显示前4位和后4位
  if (key.length <= 8) return '****'
  return key.slice(0, 4) + '****' + key.slice(-4)
}

/**
 * 格式化持续时间
 */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60000)
  const seconds = Math.floor((ms % 60000) / 1000)
  return `${minutes}m ${seconds}s`
}

/**
 * 复制文本到剪贴板
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

/**
 * 格式化 token 数量，超过1000使用k，超过1000000使用m
 */
export function formatTokens(value?: number): string {
  const num = value || 0
  if (num >= 1_000_000) {
    return (num / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'm'
  }
  if (num >= 1_000) {
    return (num / 1_000).toFixed(1).replace(/\.0$/, '') + 'k'
  }
  return num.toString()
}

/**
 * 统一错误消息提取
 */
export function getErrorMessage(error: any, fallbackMessage: string = '操作失败'): string {
  if (error?.message) return error.message
  if (typeof error === 'string') return error
  return fallbackMessage
}

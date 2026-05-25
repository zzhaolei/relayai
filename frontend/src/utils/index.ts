/**
 * 遮蔽 API Key，只显示前4位和后4位
 */
export function maskKey(key: string): string {
  if (!key) return '未设置'
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
 * 统一错误消息提取
 */
export function getErrorMessage(error: any, fallbackMessage: string = '操作失败'): string {
  if (error?.message) return error.message
  if (typeof error === 'string') return error
  return fallbackMessage
}

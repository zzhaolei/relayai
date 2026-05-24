/**
 * 遮蔽 API Key，只显示前4位和后4位
 * @param key API Key
 * @returns 遮蔽后的 Key
 */
export function maskKey(key: string): string {
  if (!key) return '未设置'
  if (key.length <= 8) return '****'
  return key.slice(0, 4) + '****' + key.slice(-4)
}

/**
 * 格式化日期时间
 * @param timestamp Unix 时间戳（秒）
 * @returns 格式化后的日期时间字符串
 */
export function formatDateTime(timestamp: number): string {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

/**
 * 格式化持续时间
 * @param ms 毫秒
 * @returns 格式化后的持续时间字符串
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
 * @param text 要复制的文本
 * @returns 是否复制成功
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
 * 生成唯一 ID
 * @returns 唯一 ID
 */
export function generateId(): string {
  return Date.now().toString(36) + Math.random().toString(36).substr(2)
}

/**
 * 延迟执行
 * @param ms 延迟毫秒数
 */
export function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

/**
 * 防抖函数
 * @param fn 要防抖的函数
 * @param delay 延迟毫秒数
 * @returns 防抖后的函数
 */
export function debounce<T extends (...args: any[]) => any>(fn: T, delay: number): T {
  let timeoutId: ReturnType<typeof setTimeout>
  return ((...args: any[]) => {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => fn(...args), delay)
  }) as T
}

/**
 * 节流函数
 * @param fn 要节流的函数
 * @param delay 延迟毫秒数
 * @returns 节流后的函数
 */
export function throttle<T extends (...args: any[]) => any>(fn: T, delay: number): T {
  let lastCall = 0
  return ((...args: any[]) => {
    const now = Date.now()
    if (now - lastCall >= delay) {
      lastCall = now
      fn(...args)
    }
  }) as T
}

/**
 * 深拷贝对象
 * @param obj 要拷贝的对象
 * @returns 拷贝后的对象
 */
export function deepClone<T>(obj: T): T {
  if (obj === null || typeof obj !== 'object') return obj
  if (obj instanceof Date) return new Date(obj.getTime()) as any
  if (obj instanceof Array) return obj.map(item => deepClone(item)) as any
  if (typeof obj === 'object') {
    const copy: any = {}
    for (const key in obj) {
      if (obj.hasOwnProperty(key)) {
        copy[key] = deepClone(obj[key])
      }
    }
    return copy
  }
  return obj
}

/**
 * 检查是否为空值
 * @param value 要检查的值
 * @returns 是否为空
 */
export function isEmpty(value: any): boolean {
  if (value === null || value === undefined) return true
  if (typeof value === 'string') return value.trim() === ''
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value).length === 0
  return false
}

/**
 * 安全解析 JSON
 * @param json JSON 字符串
 * @param defaultValue 默认值
 * @returns 解析后的对象
 */
export function safeJsonParse<T>(json: string, defaultValue: T): T {
  try {
    return JSON.parse(json)
  } catch {
    return defaultValue
  }
}

/**
 * 格式化文件大小
 * @param bytes 字节数
 * @returns 格式化后的文件大小字符串
 */
export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

/**
 * 截断字符串
 * @param str 原字符串
 * @param maxLength 最大长度
 * @param suffix 后缀
 * @returns 截断后的字符串
 */
export function truncateString(str: string, maxLength: number, suffix: string = '...'): string {
  if (!str || str.length <= maxLength) return str
  return str.slice(0, maxLength - suffix.length) + suffix
}

/**
 * 首字母大写
 * @param str 原字符串
 * @returns 首字母大写后的字符串
 */
export function capitalize(str: string): string {
  if (!str) return str
  return str.charAt(0).toUpperCase() + str.slice(1)
}

/**
 * 驼峰转换
 * @param str 原字符串
 * @returns 驼峰格式的字符串
 */
export function toCamelCase(str: string): string {
  return str
    .replace(/(?:^\w|[A-Z]|\b\w)/g, (word, index) => {
      return index === 0 ? word.toLowerCase() : word.toUpperCase()
    })
    .replace(/[\s\-_]+/g, '')
}

/**
 * 短横线转换
 * @param str 原字符串
 * @returns 短横线格式的字符串
 */
export function toKebabCase(str: string): string {
  return str
    .replace(/([a-z])([A-Z])/g, '$1-$2')
    .replace(/[\s_]+/g, '-')
    .toLowerCase()
}

/**
 * 统一错误处理
 * @param error 错误对象
 * @param fallbackMessage 默认错误消息
 * @returns 错误消息字符串
 */
export function getErrorMessage(error: any, fallbackMessage: string = '操作失败'): string {
  if (error?.message) return error.message
  if (typeof error === 'string') return error
  return fallbackMessage
}

/**
 * 安全执行异步函数
 * @param fn 异步函数
 * @param errorHandler 错误处理函数
 * @returns Promise
 */
export async function safeAsync<T>(
  fn: () => Promise<T>,
  errorHandler?: (error: any) => void
): Promise<T | null> {
  try {
    return await fn()
  } catch (error) {
    if (errorHandler) {
      errorHandler(error)
    }
    return null
  }
}

/**
 * 格式化 API 错误
 * @param error 错误对象
 * @returns 格式化后的错误消息
 */
export function formatApiError(error: any): string {
  if (error?.response?.data?.message) {
    return error.response.data.message
  }
  if (error?.response?.statusText) {
    return `${error.response.status}: ${error.response.statusText}`
  }
  if (error?.message) {
    return error.message
  }
  return '未知错误'
}

/**
 * 重试函数
 * @param fn 要重试的函数
 * @param maxRetries 最大重试次数
 * @param delay 重试延迟（毫秒）
 * @returns Promise
 */
export async function retry<T>(
  fn: () => Promise<T>,
  maxRetries: number = 3,
  delay: number = 1000
): Promise<T> {
  let lastError: any
  for (let i = 0; i <= maxRetries; i++) {
    try {
      return await fn()
    } catch (error) {
      lastError = error
      if (i < maxRetries) {
        await sleep(delay * Math.pow(2, i)) // 指数退避
      }
    }
  }
  throw lastError
}
const nf = new Intl.NumberFormat('zh-CN')

export const num = (n: number) => nf.format(n)

const pad = (n: number) => String(n).padStart(2, '0')

export function clock(ms: number) {
  const d = new Date(ms)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 今天 14:32 / 昨天 09:15 / 10月8日 09:15 / 2025年12月1日 */
export function when(ms: number) {
  const d = new Date(ms)
  const now = new Date()
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const diff = Math.round((day(now) - day(d)) / 86400000)
  if (diff === 0) return `今天 ${clock(ms)}`
  if (diff === 1) return `昨天 ${clock(ms)}`
  if (d.getFullYear() === now.getFullYear()) return `${d.getMonth() + 1}月${d.getDate()}日 ${clock(ms)}`
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

/** 2026年10月9日 */
export function day(v: number | string) {
  const d = new Date(v)
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

export function stamp(ms = Date.now()) {
  const d = new Date(ms)
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`
}

/** 1.2 GB */
export function bytes(n: number) {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

export function partLabel(p: { name: string; count: number; repeat: number }) {
  return p.repeat > 1 ? `${p.name} ${num(p.count)}×${p.repeat}` : `${p.name} ${num(p.count)}`
}

/** 路径太长时保留开头和最后两层：D:\…\视频素材\第一批 */
export function shortPath(p: string, max = 46) {
  if (p.length <= max) return p
  const sep = p.includes('\\') ? '\\' : '/'
  const parts = p.split(sep).filter(Boolean)
  if (parts.length <= 3) return p
  const head = p.startsWith(sep) ? sep + parts[0] : parts[0]
  return [head, '…', ...parts.slice(-2)].join(sep)
}

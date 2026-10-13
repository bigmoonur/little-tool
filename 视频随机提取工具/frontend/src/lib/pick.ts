import type { Category, Draft, DraftEntry, Pick, RepeatMode } from '../types'
import { num } from './format'

/** 与后端 picker.MaxRepeat 一致。 */
export const MAX_REPEAT = 99

export interface RowState {
  count: number
  repeat: number
  /** 这一行会复制出多少个文件。 */
  files: number
  error: string
}

/**
 * 两种乘数规则。视频是复制进文件夹的，没有先后顺序，所以只分“重复同一批”和“每轮重新抽”。
 * 示范都按“数量 2、乘数 3”：字母代表抽到的不同视频，每一组是一个视频的几份副本（或一轮抽到的视频）。
 */
export const REPEAT_MODES: { mode: RepeatMode; label: string; short: string; glyph: string; demo: string[][]; help: string }[] = [
  {
    mode: 'repeat',
    label: '重复同一批',
    short: '重复',
    glyph: 'AABB',
    demo: [['A', 'A', 'A'], ['B', 'B', 'B']],
    help: '抽 2 个不同的视频，每个复制 3 份。',
  },
  {
    mode: 'random',
    label: '分轮随机',
    short: '分轮',
    glyph: 'ABCA',
    demo: [['A', 'B'], ['C', 'A'], ['B', 'D']],
    help: '一共抽 3 轮，每轮重新抽 2 个。同一轮里不重复，不同轮可能抽到同一个视频，也可能抽到新的。',
  },
]

export const modeInfo = (mode?: string) => REPEAT_MODES.find(m => m.mode === mode) ?? REPEAT_MODES[0]

/** 用这一行自己的数量和乘数说明会怎样复制，只在乘数大于 1 时需要。 */
export function ruleLine(mode: RepeatMode, count: number, repeat: number) {
  if (mode === 'random') return `抽 ${num(repeat)} 轮，每轮重新抽 ${num(count)} 个`
  return `抽 ${num(count)} 个，每个复制 ${num(repeat)} 份`
}

const COMBINED = /^(\d+)\s*[*xX×]\s*(\d+)$/

/** 在数量栏输入 5*3 时，拆成数量 5、乘数 3。 */
export function splitCombined(entry: DraftEntry): DraftEntry {
  const m = entry.count.trim().match(COMBINED)
  if (!m) return entry
  return { count: String(Number(m[1])), repeat: String(Number(m[2])) }
}

/** 检查一行填写的数量和乘数。 */
export function evaluate(c: Category, entry: DraftEntry | undefined): RowState {
  const empty = { count: 0, repeat: 1, files: 0, error: '' }
  if (!entry) return empty
  const e = splitCombined(entry)
  const countRaw = e.count.trim()
  const repeatRaw = e.repeat.trim()
  if (countRaw === '' && repeatRaw === '') return empty
  if (countRaw !== '' && !/^\d+$/.test(countRaw)) return { ...empty, error: '数量请填写整数，例如 5 或 5*3' }
  if (repeatRaw !== '' && !/^\d+$/.test(repeatRaw)) return { ...empty, error: '乘数请填写整数' }
  const count = countRaw === '' ? 0 : Number(countRaw)
  const repeat = repeatRaw === '' ? 1 : Number(repeatRaw)
  if (repeat < 1) return { ...empty, count, error: '乘数至少为 1' }
  if (repeat > MAX_REPEAT) return { ...empty, count, error: `乘数最多 ${MAX_REPEAT}` }
  if (count > c.count) return { count, repeat, files: 0, error: `最多 ${num(c.count)} 个` }
  return { count, repeat, files: count * repeat, error: '' }
}

export function draftToPicks(draft: Draft, categories: Category[]): Record<string, Pick> {
  const out: Record<string, Pick> = {}
  for (const c of categories) {
    const r = evaluate(c, draft[c.name])
    if (!r.error && r.count > 0) out[c.name] = { count: r.count, repeat: r.repeat }
  }
  return out
}

export function picksToDraft(picks: Record<string, Pick>): Draft {
  const out: Draft = {}
  for (const [name, p] of Object.entries(picks)) {
    out[name] = { count: p.count ? String(p.count) : '', repeat: p.repeat > 1 ? String(p.repeat) : '' }
  }
  return out
}

/** 乘数规则只在用到乘数（或分轮随机）时才影响结果。 */
export const usesRule = (picks: Record<string, Pick>, mode: RepeatMode) =>
  Object.values(picks).some(p => p.count > 0 && (p.repeat > 1 || mode === 'random'))

/** “开场 2×3、结尾 1”，用到乘数规则时后面加上规则名称。 */
export function picksLabel(picks: Record<string, Pick>, mode: RepeatMode) {
  const parts = Object.entries(picks)
    .filter(([, p]) => p.count > 0)
    .map(([name, p]) => (p.repeat > 1 ? `${name} ${num(p.count)}×${p.repeat}` : `${name} ${num(p.count)}`))
  return [parts.join('、'), ...(usesRule(picks, mode) ? [modeInfo(mode).label] : [])].filter(Boolean).join(' · ')
}

export function samePicks(a: Record<string, Pick>, b: Record<string, Pick>) {
  const norm = (p: Record<string, Pick>) =>
    Object.entries(p)
      .filter(([, v]) => v.count > 0)
      .map(([k, v]) => `${k}:${v.count}:${Math.max(1, v.repeat)}`)
      .sort()
      .join('|')
  return norm(a) === norm(b)
}

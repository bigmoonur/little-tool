import type { Draft, DraftEntry, ListInfo, Pick, RepeatMode } from '../types'
import { num } from './format'

export interface RowState {
  count: number
  repeat: number
  lines: number
  error: string
}

/**
 * 三种乘数规则。示范都按“数量 2、乘数 3”：字母代表抽到的不同内容，
 * 每一组是连在一起输出的一段（连续重复按内容分组，另外两种按遍、按轮分组）。
 */
export const REPEAT_MODES: { mode: RepeatMode; label: string; short: string; glyph: string; demo: string[][]; help: string }[] = [
  {
    mode: 'consecutive',
    label: '连续重复',
    short: '连续',
    glyph: 'AABB',
    demo: [['A', 'A', 'A'], ['B', 'B', 'B']],
    help: '抽 2 条不同内容，每条连着出现 3 次，再轮到下一条。',
  },
  {
    mode: 'cycle',
    label: '循环重复',
    short: '循环',
    glyph: 'ABAB',
    demo: [['A', 'B'], ['A', 'B'], ['A', 'B']],
    help: '抽 2 条不同内容排成一批，整批按顺序出现 3 遍。',
  },
  {
    mode: 'random',
    label: '分轮随机',
    short: '随机',
    glyph: 'ABCA',
    demo: [['A', 'B'], ['C', 'A'], ['B', 'D']],
    help: '一共抽 3 轮，每轮重新抽 2 条。同一轮里不重复，不同轮之间可能重复，也可能抽到新内容。',
  },
]

/** 用这一行自己的数量和乘数说明会怎样输出，只在乘数大于 1 时需要。 */
export function ruleLine(mode: RepeatMode, count: number, repeat: number) {
  if (mode === 'cycle') return `抽 ${num(count)} 条排成一批，整批出现 ${num(repeat)} 遍`
  if (mode === 'random') return `抽 ${num(repeat)} 轮，每轮重新抽 ${num(count)} 条`
  return `抽 ${num(count)} 条，每条连着出现 ${num(repeat)} 次`
}

/** 旧方案、旧备份没有保存规则，按连续重复处理。 */
export const modeInfo = (mode?: string) => REPEAT_MODES.find(m => m.mode === mode) ?? REPEAT_MODES[0]

type Ruled = { repeat: number; mode?: RepeatMode }

/** 方案、记住的数量和抽取记录里，一次抽取共用同一种规则，取第一项即可。 */
export const picksMode = (picks: Ruled[]): RepeatMode => modeInfo(picks[0]?.mode).mode

/** 乘数规则只在用到乘数（或分轮随机）时才影响结果。 */
export const usesRule = (picks: Ruled[]) => picksMode(picks) === 'random' || picks.some(p => p.repeat > 1)

/** 用到乘数规则时返回规则名称，例如“循环重复”；否则返回空字符串。 */
export const ruleNote = (picks: Ruled[]) => (usesRule(picks) ? modeInfo(picksMode(picks)).label : '')

const COMBINED = /^(\d+)\s*[*xX×]\s*(\d+)$/

/** 在数量栏输入 25*4 时，拆成数量 25、乘数 4。 */
export function splitCombined(entry: DraftEntry): DraftEntry {
  const m = entry.count.trim().match(COMBINED)
  if (!m) return entry
  return { count: String(Number(m[1])), repeat: String(Number(m[2])) }
}

/** 检查一行填写的数量。 */
export function evaluate(list: ListInfo, entry: DraftEntry | undefined, mode: RepeatMode = 'consecutive'): RowState {
  const empty = { count: 0, repeat: 1, lines: 0, error: '' }
  if (!entry) return empty
  const e = splitCombined(entry)
  const countRaw = e.count.trim()
  const repeatRaw = e.repeat.trim()
  if (countRaw === '' && repeatRaw === '') return empty
  if (countRaw !== '' && !/^\d+$/.test(countRaw)) return { ...empty, error: '数量请填写整数，例如 10 或 25*4' }
  if (repeatRaw !== '' && !/^\d+$/.test(repeatRaw)) return { ...empty, error: '乘数请填写整数' }
  const count = countRaw === '' ? 0 : Number(countRaw)
  const repeat = repeatRaw === '' ? 1 : Number(repeatRaw)
  if (repeat < 1) return { ...empty, count, error: '乘数至少为 1' }
  // 乘数大于 1 或分轮随机时，同一批（同一轮）里的内容各不相同，按不同内容计数。
  const distinct = repeat > 1 || mode === 'random'
  const limit = distinct ? list.unique : list.count
  if (count > limit) {
    const unit = distinct ? '条不同内容' : '条'
    return { count, repeat, lines: 0, error: `${mode === 'random' ? '每轮最多' : '最多'} ${num(limit)} ${unit}` }
  }
  return { count, repeat, lines: count * repeat, error: '' }
}

export function draftToPicks(draft: Draft, lists: ListInfo[], mode: RepeatMode = 'consecutive'): Record<string, Pick> {
  const out: Record<string, Pick> = {}
  for (const l of lists) {
    const r = evaluate(l, draft[l.id], mode)
    if (!r.error && (r.count > 0 || r.repeat > 1)) out[l.id] = { count: r.count, repeat: r.repeat, mode }
  }
  return out
}

export function picksToDraft(picks: Record<string, Pick>): Draft {
  const out: Draft = {}
  for (const [id, p] of Object.entries(picks)) {
    out[id] = { count: p.count ? String(p.count) : '', repeat: p.repeat > 1 ? String(p.repeat) : '' }
  }
  return out
}

export function samePicks(a: Record<string, Pick>, b: Record<string, Pick>) {
  const norm = (p: Record<string, Pick>) =>
    Object.entries(p)
      .filter(([, v]) => v.count > 0)
      .map(([k, v]) => `${k}:${v.count}:${Math.max(1, v.repeat)}:${v.mode || 'consecutive'}`)
      .sort()
      .join('|')
  return norm(a) === norm(b)
}

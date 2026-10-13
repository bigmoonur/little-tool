import type { Draft, DraftEntry, ListInfo, Pick, RepeatMode, Tag } from '../types'
import { num } from './format'

export interface RowState {
  count: number
  repeat: number
  lines: number
  error: string
}

/** 三种乘数规则。示例都按“数量 2、乘数 4”，数字代表抽到的第几条内容。 */
export const REPEAT_MODES: { mode: RepeatMode; label: string; short: string; example: string; help: string }[] = [
  { mode: 'consecutive', label: '连续重复', short: '连续', example: '1111 2222', help: '抽 2 条不同内容，每条连着出现 4 次。' },
  { mode: 'cycle', label: '循环重复', short: '循环', example: '12 12 12 12', help: '抽 2 条不同内容，整批按顺序出现 4 遍。' },
  {
    mode: 'random',
    label: '分轮随机',
    short: '随机',
    example: '12 31 24 13',
    help: '独立抽 4 轮，每轮 2 条。同一分类每轮内容不重复，不同轮之间可能重复，也可能抽到新内容。',
  },
]

/** 旧方案、旧备份没有保存规则，按连续重复处理。 */
export const modeInfo = (mode?: string) => REPEAT_MODES.find(m => m.mode === mode) ?? REPEAT_MODES[0]

type Ruled = { repeat: number; mode?: RepeatMode; tagIds?: string[] }

/** 方案、记住的数量和抽取记录里，一次抽取共用同一种规则和同一组标签，取第一项即可。 */
export function picksRule(picks: Ruled[]): { mode: RepeatMode; tagIds: string[] } {
  return { mode: modeInfo(picks[0]?.mode).mode, tagIds: [...(picks[0]?.tagIds ?? [])] }
}

/** “循环重复 · 标签：新品、常用”。乘数都是 1 且没有筛选时返回空字符串。 */
export function ruleNote(picks: Ruled[], tags: Tag[]) {
  const { mode, tagIds } = picksRule(picks)
  const bits: string[] = []
  if (mode === 'random' || picks.some(p => p.repeat > 1)) bits.push(modeInfo(mode).label)
  if (tagIds.length) bits.push(`标签：${tagIds.map(id => tags.find(t => t.id === id)?.name ?? '已删除的标签').join('、')}`)
  return bits.join(' · ')
}

const COMBINED = /^(\d+)\s*[*xX×]\s*(\d+)$/

/** 在数量栏输入 25*4 时，拆成数量 25、乘数 4。 */
export function splitCombined(entry: DraftEntry): DraftEntry {
  const m = entry.count.trim().match(COMBINED)
  if (!m) return entry
  return { count: String(Number(m[1])), repeat: String(Number(m[2])) }
}

/**
 * 检查一行填写的数量。list 的 count / unique 是当前可抽的条数：
 * 按标签筛选时（filtered）传入筛选后的统计，提示也会说明是标签限制。
 */
export function evaluate(list: ListInfo, entry: DraftEntry | undefined, mode: RepeatMode = 'consecutive', filtered = false): RowState {
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
    const error = filtered
      ? limit === 0 ? '没有符合所选标签的文本' : `符合所选标签的只有 ${num(limit)} ${unit}`
      : `${mode === 'random' ? '每轮最多' : '最多'} ${num(limit)} ${unit}`
    return { count, repeat, lines: 0, error }
  }
  return { count, repeat, lines: count * repeat, error: '' }
}

export function draftToPicks(draft: Draft, lists: ListInfo[], mode: RepeatMode = 'consecutive', tagIds: string[] = []): Record<string, Pick> {
  const out: Record<string, Pick> = {}
  for (const l of lists) {
    const r = evaluate(l, draft[l.id], mode)
    if (!r.error && (r.count > 0 || r.repeat > 1)) out[l.id] = { count: r.count, repeat: r.repeat, mode, tagIds }
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
      .map(([k, v]) => `${k}:${v.count}:${Math.max(1, v.repeat)}:${v.mode || 'consecutive'}:${[...(v.tagIds ?? [])].sort().join(',')}`)
      .sort()
      .join('|')
  return norm(a) === norm(b)
}

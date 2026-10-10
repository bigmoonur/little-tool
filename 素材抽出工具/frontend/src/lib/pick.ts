import type { Draft, DraftEntry, ListInfo, Pick } from '../types'
import { num } from './format'

export interface RowState {
  count: number
  repeat: number
  lines: number
  error: string
}

const COMBINED = /^(\d+)\s*[*xX×]\s*(\d+)$/

/** 在数量栏输入 25*4 时，拆成数量 25、重复 4。 */
export function splitCombined(entry: DraftEntry): DraftEntry {
  const m = entry.count.trim().match(COMBINED)
  if (!m) return entry
  return { count: String(Number(m[1])), repeat: String(Number(m[2])) }
}

export function evaluate(list: ListInfo, entry: DraftEntry | undefined): RowState {
  const empty = { count: 0, repeat: 1, lines: 0, error: '' }
  if (!entry) return empty
  const e = splitCombined(entry)
  const countRaw = e.count.trim()
  const repeatRaw = e.repeat.trim()
  if (countRaw === '' && repeatRaw === '') return empty
  if (countRaw !== '' && !/^\d+$/.test(countRaw)) return { ...empty, error: '数量请填写整数，例如 10 或 25*4' }
  if (repeatRaw !== '' && !/^\d+$/.test(repeatRaw)) return { ...empty, error: '重复次数请填写整数' }
  const count = countRaw === '' ? 0 : Number(countRaw)
  const repeat = repeatRaw === '' ? 1 : Number(repeatRaw)
  if (repeat < 1) return { ...empty, count, error: '重复次数至少为 1' }
  if (count > 0 && repeat > 1 && count > list.unique) {
    return { count, repeat, lines: 0, error: `最多 ${num(list.unique)} 条不同内容` }
  }
  if (count > list.count) return { count, repeat, lines: 0, error: `最多 ${num(list.count)} 条` }
  return { count, repeat, lines: count * repeat, error: '' }
}

export function draftToPicks(draft: Draft, lists: ListInfo[]): Record<string, Pick> {
  const out: Record<string, Pick> = {}
  for (const l of lists) {
    const r = evaluate(l, draft[l.id])
    if (!r.error && (r.count > 0 || r.repeat > 1)) out[l.id] = { count: r.count, repeat: r.repeat }
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
      .map(([k, v]) => `${k}:${v.count}:${Math.max(1, v.repeat)}`)
      .sort()
      .join('|')
  return norm(a) === norm(b)
}

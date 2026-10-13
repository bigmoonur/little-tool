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
 * 三种乘数规则。视频是复制进文件夹的，文件夹里的先后顺序就是文件名的顺序，所以靠文件名来排：
 * 连续重复的副本在原名后面加随机字符，循环重复和分轮随机在前面加轮次序号。
 * 示范都按“数量 2、乘数 3”：字母代表抽到的不同视频，每一组是连在一起的一段；names 是文件夹里开头几个文件的名字。
 */
export const REPEAT_MODES: {
  mode: RepeatMode; label: string; short: string; glyph: string; demo: string[][]; names: string[]; help: string
}[] = [
  {
    mode: 'consecutive',
    label: '连续重复',
    short: '连续',
    glyph: 'AABB',
    demo: [['A', 'A', 'A'], ['B', 'B', 'B']],
    names: ['A.mp4', 'A_k.mp4', 'A_7.mp4', 'B.mp4'],
    help: '抽 2 个不同的视频，每个连着放 3 份。副本在文件名后面加随机字符，紧跟在原视频后面。',
  },
  {
    mode: 'cycle',
    label: '循环重复',
    short: '循环',
    glyph: 'ABAB',
    demo: [['A', 'B'], ['A', 'B'], ['A', 'B']],
    names: ['01_A.mp4', '01_B.mp4', '02_A.mp4', '02_B.mp4'],
    help: '抽 2 个不同的视频排成一批，整批放 3 遍。文件名前面加轮次序号，一遍排完再排下一遍。',
  },
  {
    mode: 'random',
    label: '分轮随机',
    short: '随机',
    glyph: 'ABCA',
    demo: [['A', 'B'], ['C', 'A'], ['B', 'D']],
    names: ['01_A.mp4', '01_B.mp4', '02_A.mp4', '02_C.mp4'],
    help: '一共抽 3 轮，每轮重新抽 2 个。同一轮里不重复，不同轮可能抽到同一个视频。文件名前面同样加轮次序号。',
  },
]

/** 旧方案、旧记录里没有规则或是改名前的写法，按连续重复处理。 */
export const modeInfo = (mode?: string) => REPEAT_MODES.find(m => m.mode === mode) ?? REPEAT_MODES[0]

/** 用这一行自己的数量和乘数说明会怎样复制，只在乘数大于 1 时需要。 */
export function ruleLine(mode: RepeatMode, count: number, repeat: number) {
  if (mode === 'cycle') return `抽 ${num(count)} 个排成一批，整批放 ${num(repeat)} 遍`
  if (mode === 'random') return `抽 ${num(repeat)} 轮，每轮重新抽 ${num(count)} 个`
  return `抽 ${num(count)} 个，每个连着放 ${num(repeat)} 份`
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

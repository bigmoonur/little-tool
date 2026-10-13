export type Theme = 'system' | 'light' | 'dark'

export interface Settings {
  materialRoot: string
  outputRoot: string
  folder: string
  alwaysOnTop: boolean
  openAfter: boolean
  theme: Theme
  rememberDraft: boolean
  /** 记住的数量、乘数和乘数规则（设置里开启时）。 */
  draft: Record<string, number>
  draftRepeat: Record<string, number>
  mode: RepeatMode
  /** 缓存视频缩略图（默认开启）。关闭后素材库只有列表。 */
  thumbnails: boolean
  /** 素材库的显示方式。 */
  libraryView: LibraryMode
}

export type LibraryMode = 'list' | 'grid'

/** 乘数规则：连续重复、循环重复，或者分轮重新抽。 */
export type RepeatMode = 'consecutive' | 'cycle' | 'random'

/** 一个分类要抽几个、复制几份（或抽几轮）。 */
export interface Pick {
  count: number
  repeat: number
}

/** 起了名字保存下来的一组参数。 */
export interface Preset {
  id: string
  name: string
  mode: RepeatMode
  picks: Record<string, Pick>
}

/** 抽取页每个分类输入框里的原始文本。 */
export interface DraftEntry {
  count: string
  repeat: string
}

export type Draft = Record<string, DraftEntry>

/** 锁屏：开启后打开软件要先输入 4 位密码。 */
export interface LockStatus {
  enabled: boolean
  /** 连续输错的次数。 */
  failures: number
  /** 输错太多时，到这个时间（毫秒）之前不能再试；不用等时是 0。 */
  lockedUntil: number
}

export interface InitInfo {
  settings: Settings
  version: string
  platform: string
  lock: LockStatus
}

export type TagColor = 'indigo' | 'teal' | 'amber' | 'rose' | 'violet' | 'green' | 'orange' | 'slate'

export const TAG_COLORS: TagColor[] = ['indigo', 'teal', 'amber', 'rose', 'violet', 'green', 'orange', 'slate']

export interface Tag {
  id: string
  name: string
  color: TagColor
}

export interface Category {
  name: string
  count: number
  bytes: number
  /** 贴在这个分类上的标签。 */
  tagIds: string[]
}

/** 素材库里看到的一个视频。 */
export interface VideoFile {
  name: string
  size: number
  /** 修改时间（毫秒）。 */
  modified: number
  /** 缩略图的缓存名；没开缩略图时为空。 */
  key: string
  /** ready 已有缩略图，none 这个视频生成不了，空字符串是还没生成。 */
  thumb: '' | 'ready' | 'none'
}

/** 往分类里添加视频的结果。 */
export interface ImportResult {
  category: string
  added: string[]
  skipped: { name: string; reason: string }[]
  bytes: number
}

export interface ImportDone {
  ok: boolean
  cancelled: boolean
  error: string
  result: ImportResult
}

export interface Destination {
  name: string
  videos: number
}

export interface State {
  materialRoot: string
  outputRoot: string
  folder: string
  categories: Category[]
  tags: Tag[]
  presets: Preset[]
  destinations: Destination[]
  materialError: string
  outputError: string
  leftovers: string[]
  busy: boolean
}

export interface Item {
  category: string
  /** 放进目标文件夹时用的文件名。 */
  name: string
  size: number
  /** 副本加了后缀，或者按轮加了序号时，这里是原来的文件名。 */
  origin?: string
}

export interface PlanSummary {
  id: string
  count: number
  bytes: number
  old: number
  destination: string
  folder: string
  items: Item[]
}

export interface Progress {
  done: number
  total: number
  name: string
  bytesDone: number
  bytesTotal: number
}

export interface Result {
  destination: string
  folder: string
  items: Item[]
  removed: number
  avoided: number
  warning: string
}

export interface HistoryEntry {
  id: string
  time: number
  destination: string
  folder: string
  items: Item[]
  removed: number
  bytes: number
  /** 这一次用的数量、乘数和乘数规则。 */
  picks: Record<string, Pick>
  mode: RepeatMode
}

export interface Done {
  ok: boolean
  cancelled: boolean
  error: string
  result: Result
  entry: HistoryEntry
}

export type View = 'extract' | 'library' | 'history'

export interface LicenseStatus {
  state: 'checking' | 'active' | 'inactive'
  key: string
  masked: string
  expiry: string
  offline: boolean
  graceUntil: number
  code: string
  message: string
  network: boolean
  machine: string
}

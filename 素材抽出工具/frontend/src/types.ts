export type GroupColor = 'indigo' | 'teal' | 'amber' | 'rose' | 'violet' | 'green' | 'orange' | 'slate'

export const GROUP_COLORS: GroupColor[] = ['indigo', 'teal', 'amber', 'rose', 'violet', 'green', 'orange', 'slate']

export interface Group {
  id: string
  name: string
  color: GroupColor
  collapsed: boolean
}

export interface ListInfo {
  id: string
  name: string
  groupId: string
  count: number
  unique: number
  updatedAt: number
}

export interface Pick {
  count: number
  repeat: number
}

export interface Preset {
  id: string
  name: string
  picks: Record<string, Pick>
}

export type AfterCopy = 'stay' | 'minimize' | 'quit'
export type Theme = 'system' | 'light' | 'dark'

export interface Settings {
  alwaysOnTop: boolean
  afterCopy: AfterCopy
  rememberDraft: boolean
  theme: Theme
  importDedupe: boolean
}

export interface Snapshot {
  groups: Group[]
  lists: ListInfo[]
  presets: Preset[]
  draft: Record<string, Pick>
  settings: Settings
  undo: string
}

export interface InitInfo {
  snapshot: Snapshot
  notice: string
  version: string
  platform: string
  maxLines: number
}

export interface PickRequest {
  listId: string
  count: number
  repeat: number
}

export interface PickPart {
  listId: string
  name: string
  count: number
  repeat: number
}

export interface ExtractResult {
  text: string
  total: number
  parts: PickPart[]
}

export interface Extracted {
  result: ExtractResult
  copied: boolean
  historyId: string
  time: number
}

export type ImportMode = 'new' | 'append' | 'replace'

export interface ImportItem {
  source: string
  text: string
  mode: ImportMode
  targetId: string
  newName: string
  groupId: string
}

export interface ImportPlan {
  items: ImportItem[]
  dedupe: boolean
}

export interface ImportStat {
  source: string
  target: string
  mode: ImportMode
  lines: number
  added: number
  duplicates: number
  removed: number
  error: string
}

export interface ImportReport {
  items: ImportStat[]
  added: number
  duplicates: number
  removed: number
  created: number
  touched: number
  ok: boolean
  error: string
}

export interface HistoryEntry {
  id: string
  time: number
  total: number
  parts: PickPart[]
  text: string
  truncated: boolean
}

/** 抽取页每个分类输入框里的原始文本。 */
export interface DraftEntry {
  count: string
  repeat: string
}

export type Draft = Record<string, DraftEntry>

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

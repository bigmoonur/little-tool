export type Theme = 'system' | 'light' | 'dark'

export interface Settings {
  materialRoot: string
  outputRoot: string
  folder: string
  alwaysOnTop: boolean
  openAfter: boolean
  theme: Theme
  rememberDraft: boolean
  draft: Record<string, number>
}

export interface InitInfo {
  settings: Settings
  version: string
  platform: string
}

export interface Category {
  name: string
  count: number
  bytes: number
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
  destinations: Destination[]
  materialError: string
  outputError: string
  leftovers: string[]
  busy: boolean
}

export interface Item {
  category: string
  name: string
  size: number
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
}

export interface Done {
  ok: boolean
  cancelled: boolean
  error: string
  result: Result
  entry: HistoryEntry
}

export type View = 'extract' | 'history'

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

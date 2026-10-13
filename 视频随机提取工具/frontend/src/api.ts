// 对 Wails 生成的绑定做一层类型整理。
import * as Go from '../wailsjs/go/main/App'
import * as Runtime from '../wailsjs/runtime/runtime'
import type { Done, HistoryEntry, InitInfo, LicenseStatus, Pick, PlanSummary, Progress, RepeatMode, Settings, State, Theme } from './types'

const go = Go as unknown as Record<string, (...args: unknown[]) => Promise<unknown>>
const call = <T,>(name: string, ...args: unknown[]) => go[name](...args) as Promise<T>

export const api = {
  init: () => call<InitInfo>('Init'),
  state: () => call<State>('State'),
  chooseMaterialRoot: () => call<State>('ChooseMaterialRoot'),
  chooseOutputRoot: () => call<State>('ChooseOutputRoot'),
  setFolder: (name: string) => call<State>('SetFolder', name),
  createFolder: (name: string) => call<State>('CreateFolder', name),

  createTag: (name: string) => call<{ state: State; id: string }>('CreateTag', name),
  renameTag: (id: string, name: string) => call<State>('RenameTag', id, name),
  setTagColor: (id: string, color: string) => call<State>('SetTagColor', id, color),
  deleteTag: (id: string) => call<State>('DeleteTag', id),
  setCategoryTags: (category: string, tagIds: string[]) => call<State>('SetCategoryTags', category, tagIds),

  prepare: (picks: Record<string, Pick>, mode: RepeatMode, clean: boolean) => call<PlanSummary>('Prepare', picks, mode, clean),
  start: (id: string) => call<void>('Start', id),
  cancel: () => call<void>('Cancel'),
  cancelAndQuit: () => call<void>('CancelAndQuit'),

  history: () => call<HistoryEntry[]>('History'),
  clearHistory: () => call<void>('ClearHistory'),

  updateSettings: (s: { alwaysOnTop: boolean; openAfter: boolean; theme: Theme; rememberDraft: boolean }) =>
    call<Settings>('UpdateSettings', s),
  saveDraft: (counts: Record<string, number>, repeats: Record<string, number>, mode: RepeatMode) => call<void>('SaveDraft', counts, repeats, mode),

  savePreset: (name: string, picks: Record<string, Pick>, mode: RepeatMode) => call<{ state: State; id: string }>('SavePreset', name, picks, mode),
  renamePreset: (id: string, name: string) => call<State>('RenamePreset', id, name),
  deletePreset: (id: string) => call<State>('DeletePreset', id),
  openFolder: (path: string) => call<void>('OpenFolder', path),
  openDataFolder: () => call<void>('OpenDataFolder'),

  licenseStatus: () => call<LicenseStatus>('LicenseStatus'),
  activateLicense: (key: string) => call<LicenseStatus>('ActivateLicense', key),
  refreshLicense: () => call<LicenseStatus>('RefreshLicense'),
  deactivateLicense: () => call<LicenseStatus>('DeactivateLicense'),
}

export const events = {
  onLicenseChanged: (fn: (s: LicenseStatus) => void) => Runtime.EventsOn('license-changed', fn),
  onProgress: (fn: (p: Progress) => void) => Runtime.EventsOn('copy-progress', fn),
  onDone: (fn: (d: Done) => void) => Runtime.EventsOn('copy-done', fn),
  onCloseWhileBusy: (fn: () => void) => Runtime.EventsOn('close-while-busy', fn),
}

export function errorText(err: unknown): string {
  if (typeof err === 'string') return err
  if (err instanceof Error) return err.message
  return '操作没有完成，请重试'
}

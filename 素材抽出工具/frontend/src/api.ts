// 对 Wails 生成的绑定做一层类型整理。生成的 models 是 class，这里统一用普通对象。
import * as Go from '../wailsjs/go/main/App'
import * as Runtime from '../wailsjs/runtime/runtime'
import type {
  Extracted, HistoryEntry, ImportPlan, ImportReport, InitInfo, LicenseStatus, Pick, PickRequest, Settings, Snapshot,
} from './types'

type Created = { snapshot: Snapshot; id: string }
const go = Go as unknown as Record<string, (...args: unknown[]) => Promise<unknown>>
const call = <T,>(name: string, ...args: unknown[]) => go[name](...args) as Promise<T>

export const api = {
  init: () => call<InitInfo>('Init'),
  snapshot: () => call<Snapshot>('Snapshot'),
  listText: (id: string) => call<string>('ListText', id),
  undo: () => call<Snapshot>('Undo'),

  createGroup: (name: string) => call<Created>('CreateGroup', name),
  renameGroup: (id: string, name: string) => call<Snapshot>('RenameGroup', id, name),
  setGroupColor: (id: string, color: string) => call<Snapshot>('SetGroupColor', id, color),
  setGroupCollapsed: (id: string, collapsed: boolean) => call<Snapshot>('SetGroupCollapsed', id, collapsed),
  moveGroup: (id: string, index: number) => call<Snapshot>('MoveGroup', id, index),
  deleteGroup: (id: string, deleteLists: boolean) => call<Snapshot>('DeleteGroup', id, deleteLists),

  createList: (name: string, groupId: string) => call<Created>('CreateList', name, groupId),
  renameList: (id: string, name: string) => call<Snapshot>('RenameList', id, name),
  moveList: (id: string, groupId: string, beforeId: string) => call<Snapshot>('MoveList', id, groupId, beforeId),
  deleteList: (id: string) => call<Snapshot>('DeleteList', id),
  setListText: (id: string, text: string) => call<Snapshot>('SetListText', id, text),
  dedupeList: (id: string) => call<{ snapshot: Snapshot; removed: number }>('DedupeList', id),

  createTag: (name: string) => call<Created>('CreateTag', name),
  renameTag: (id: string, name: string) => call<Snapshot>('RenameTag', id, name),
  setTagColor: (id: string, color: string) => call<Snapshot>('SetTagColor', id, color),
  deleteTag: (id: string) => call<Snapshot>('DeleteTag', id),
  setListTags: (id: string, tagIds: string[]) => call<Snapshot>('SetListTags', id, tagIds),

  previewImport: (plan: ImportPlan) => call<ImportReport>('PreviewImport', plan),
  applyImport: (plan: ImportPlan) => call<{ snapshot: Snapshot; report: ImportReport }>('ApplyImport', plan),

  extract: (reqs: PickRequest[]) => call<Extracted>('Extract', reqs),
  copyText: (text: string) => call<void>('CopyText', text).then(() => true),
  saveDraft: (draft: Record<string, Pick>) => call<void>('SaveDraft', draft),

  savePreset: (name: string, picks: Record<string, Pick>) => call<Created>('SavePreset', name, picks),
  renamePreset: (id: string, name: string) => call<Snapshot>('RenamePreset', id, name),
  deletePreset: (id: string) => call<Snapshot>('DeletePreset', id),

  history: () => call<HistoryEntry[]>('History'),
  historyText: (id: string) => call<string>('HistoryText', id),
  clearHistory: () => call<void>('ClearHistory'),

  updateSettings: (s: Settings) => call<Snapshot>('UpdateSettings', s),

  saveTextFile: (name: string, text: string) => call<string>('SaveTextFile', name, text),
  exportList: (id: string) => call<string>('ExportList', id),
  exportBackup: () => call<string>('ExportBackup'),
  restoreBackup: () => call<{ snapshot: Snapshot; restored: boolean }>('RestoreBackup'),
  openDataFolder: () => call<void>('OpenDataFolder'),

  licenseStatus: () => call<LicenseStatus>('LicenseStatus'),
  activateLicense: (key: string) => call<LicenseStatus>('ActivateLicense', key),
  refreshLicense: () => call<LicenseStatus>('RefreshLicense'),
  deactivateLicense: () => call<LicenseStatus>('DeactivateLicense'),

  setUnsaved: (v: boolean) => call<void>('SetUnsaved', v),
  holdClose: () => call<void>('HoldClose'),
  confirmClose: () => call<void>('ConfirmClose'),
}

export const runtime = {
  onCloseRequested: (fn: () => void) => Runtime.EventsOn('close-requested', fn),
  minimise: () => Runtime.WindowMinimise(),
  onLicenseChanged: (fn: (s: LicenseStatus) => void) => Runtime.EventsOn('license-changed', fn),
}

/** 把后端返回的错误变成可显示的文字。 */
export function errorText(err: unknown): string {
  if (typeof err === 'string') return err
  if (err instanceof Error) return err.message
  return '操作没有完成，请重试'
}

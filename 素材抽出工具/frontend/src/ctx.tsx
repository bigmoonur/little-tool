import { createContext, useContext } from 'react'
import type { Draft, Extracted, LicenseStatus, PickRequest, RepeatMode, Snapshot, View } from './types'
import type { ConfirmOptions, PromptOptions } from './ui/Modal'
import type { ToastOptions } from './ui/Toasts'
import type { LoadedFile } from './lib/files'

export interface ImportOpen {
  tab?: 'paste' | 'files'
  files?: LoadedFile[]
  /** 预先选中的目标分类（追加到已有分类）。 */
  targetId?: string
  /** 新建分类时默认放入的分组。 */
  groupId?: string
}

export interface AppInfo {
  version: string
  platform: string
  maxLines: number
}

export interface Ctx {
  snap: Snapshot
  setSnap: (s: Snapshot) => void
  info: AppInfo
  view: View
  go: (v: View) => Promise<boolean>
  draft: Draft
  setDraft: React.Dispatch<React.SetStateAction<Draft>>
  repeatMode: RepeatMode
  setRepeatMode: (v: RepeatMode) => void
  /** 抽取页只看带这些标签的分类；放在这里，切换页面再回来还在。 */
  tagFilter: string[]
  setTagFilter: React.Dispatch<React.SetStateAction<string[]>>
  lastExtraction: { response: Extracted; requests: PickRequest[] } | null
  setLastExtraction: React.Dispatch<React.SetStateAction<{ response: Extracted; requests: PickRequest[] } | null>>
  selectedId: string
  select: (id: string) => Promise<boolean>
  toast: (text: string, opts?: ToastOptions) => number
  confirm: (o: ConfirmOptions) => Promise<string | null>
  prompt: (o: PromptOptions) => Promise<string | null>
  /** 执行后端调用，失败时显示错误提示并返回 undefined。 */
  run: <T>(p: Promise<T>) => Promise<T | undefined>
  /** 更新素材库状态；带撤销信息时提示可以撤销。 */
  commit: (s: Snapshot, message?: string) => void
  openImport: (o?: ImportOpen) => void
  openSettings: () => void
  /** 编辑器有未保存修改时注册的离开确认。 */
  setGuard: (g: (() => Promise<boolean>) | null) => void
  historyVersion: number
  bumpHistory: () => void
  license: LicenseStatus
  setLicense: (s: LicenseStatus) => void
  /** 离开前检查编辑器里未保存的修改，用户取消时返回 false。 */
  leave: () => Promise<boolean>
}

export const AppContext = createContext<Ctx>(null as unknown as Ctx)
export const useApp = () => useContext(AppContext)

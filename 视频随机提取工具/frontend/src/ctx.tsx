import { createContext, useContext } from 'react'
import type { Done, Draft, LicenseStatus, LockStatus, Pick, PlanSummary, Progress, RepeatMode, Settings, State, View } from './types'
import type { ConfirmOptions, PromptOptions } from './ui/Modal'
import type { ToastOptions } from './ui/Toasts'

export interface CopyState {
  phase: 'idle' | 'copying' | 'done'
  plan?: PlanSummary
  progress?: Progress
  done?: Done
}

export interface Ctx {
  state: State
  setState: (s: State) => void
  /** 重新扫描两个根目录。 */
  refresh: () => Promise<void>
  settings: Settings
  setSettings: (s: Settings) => void
  version: string
  view: View
  go: (v: View) => void
  /** 素材库里正在看的分类。 */
  selected: string
  select: (category: string) => void
  toast: (text: string, opts?: ToastOptions) => number
  confirm: (o: ConfirmOptions) => Promise<string | null>
  prompt: (o: PromptOptions) => Promise<string | null>
  /** 执行后端调用，失败时显示错误提示并返回 undefined。 */
  run: <T>(p: Promise<T>) => Promise<T | undefined>
  historyVersion: number
  copy: CopyState
  /** 抽取页填写的数量和乘数；放在这里，切到记录页再回来还在，记录页也能把用过的参数填回去。 */
  draft: Draft
  setDraft: React.Dispatch<React.SetStateAction<Draft>>
  repeatMode: RepeatMode
  setRepeatMode: (m: RepeatMode) => void
  /** 抽取一批并开始复制；需要清空旧视频时先确认。 */
  startCopy: (picks: Record<string, Pick>, mode: RepeatMode, clean: boolean) => Promise<void>
  cancelCopy: () => void
  clearResult: () => void
  /** 正在往哪个分类里添加视频；没有在添加时是 null。 */
  adding: { category: string; progress?: Progress } | null
  /** 选择视频文件并复制进这个分类。 */
  addVideos: (category: string) => Promise<void>
  license: LicenseStatus
  setLicense: (s: LicenseStatus) => void
  /** 锁屏密码的状态；设置里开关或修改后更新。 */
  lock: LockStatus
  setLock: (s: LockStatus) => void
}

export const AppContext = createContext<Ctx>(null as unknown as Ctx)
export const useApp = () => useContext(AppContext)

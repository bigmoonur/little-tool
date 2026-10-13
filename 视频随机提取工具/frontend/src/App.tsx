import { useCallback, useEffect, useRef, useState } from 'react'
import { History, Library, Pin, PinOff, Settings as SettingsIcon, Shuffle, Lock } from 'lucide-react'
import { api, errorText, events } from './api'
import { AppContext, type CopyState, type Ctx } from './ctx'
import { SettingsDialog } from './dialogs/SettingsDialog'
import { day, num } from './lib/format'
import { draftToPicks, modeInfo, picksToDraft } from './lib/pick'
import type { Draft, InitInfo, LicenseStatus, LockStatus, Pick, RepeatMode, Settings, State, View } from './types'
import { LockScreen } from './views/LockScreen'
import { ErrorBoundary } from './ui/ErrorBoundary'
import { Logo } from './ui/Logo'
import { useDialogs } from './ui/Modal'
import { useToasts } from './ui/Toasts'
import { ExtractView } from './views/ExtractView'
import { HistoryView } from './views/HistoryView'
import { LibraryView } from './views/LibraryView'
import { LicenseGate } from './views/LicenseGate'

const NO_LICENSE: LicenseStatus = {
  state: 'inactive', key: '', masked: '', expiry: '', offline: false, graceUntil: 0, code: '', message: '', network: false, machine: '',
}

export default function App() {
  const [boot, setBoot] = useState<{ init: InitInfo; state: State } | { error: string } | null>(null)
  const [license, setLicense] = useState<LicenseStatus | null>(null)
  const opened = useRef(false)

  const load = () => Promise.all([api.init(), api.state()]).then(
    ([init, state]) => setBoot({ init, state }),
    e => setBoot({ error: errorText(e) }),
  )

  useEffect(() => {
    load()
    api.licenseStatus().then(setLicense, e => setLicense({ ...NO_LICENSE, network: true, message: errorText(e) }))
    return events.onLicenseChanged(setLicense)
  }, [])

  if (!boot || !license) return <div className="boot"><div className="boot-msg"><Logo />正在验证授权…</div></div>
  if ('error' in boot) {
    return (
      <div className="fatal">
        <h1>视频分发工具无法启动</h1>
        <p>{boot.error}</p>
        <p className="dim">请确认当前 Windows 账户可以写入“应用数据”文件夹，然后重新打开程序。</p>
      </div>
    )
  }
  if (license.state !== 'active' && !opened.current) {
    return <LicenseGate status={license} version={boot.init.version} onActivated={st => {
      setBoot(null)
      load().then(() => setLicense(st))
    }} />
  }
  if (license.state === 'active') opened.current = true
  // 授权失效时保持 Shell 挂载，保留数量、正在复制的任务和进度事件监听。
  return <Shell init={boot.init} initialState={boot.state} license={license} setLicense={setLicense} />
}

const NAV: { view: View; label: string; icon: React.ReactNode }[] = [
  { view: 'extract', label: '抽取', icon: <Shuffle size={16} /> },
  { view: 'library', label: '素材库', icon: <Library size={16} /> },
  { view: 'history', label: '记录', icon: <History size={16} /> },
]

function Shell({ init, initialState, license, setLicense }: {
  init: InitInfo; initialState: State; license: LicenseStatus; setLicense: (s: LicenseStatus) => void
}) {
  const [state, setState] = useState(initialState)
  const [settings, setSettings] = useState<Settings>(init.settings)
  const [view, setView] = useState<View>('extract')
  const [selected, select] = useState('')
  const [copy, setCopy] = useState<CopyState>({ phase: 'idle' })
  const [draft, setDraft] = useState<Draft>(() => {
    const picks: Record<string, Pick> = {}
    for (const [name, count] of Object.entries(init.settings.draft ?? {})) picks[name] = { count, repeat: init.settings.draftRepeat?.[name] ?? 1 }
    return picksToDraft(picks)
  })
  const [repeatMode, setRepeatMode] = useState<RepeatMode>(modeInfo(init.settings.mode).mode)
  const [historyVersion, setHistoryVersion] = useState(0)
  const [lock, setLock] = useState<LockStatus>(init.lock ?? { enabled: false, failures: 0, lockedUntil: 0 })
  // 开了锁屏密码时，一打开就是锁着的；initial 直接盖住，manual（手动锁定）有落下来的动画。
  const [locked, setLocked] = useState<'initial' | 'manual' | null>(init.lock?.enabled ? 'initial' : null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const { toast, node: toastNode } = useToasts()
  const dialogs = useDialogs()
  const copyRef = useRef(copy)
  copyRef.current = copy

  const run = useCallback(
    async <T,>(p: Promise<T>): Promise<T | undefined> => {
      try {
        return await p
      } catch (e) {
        toast(errorText(e), { kind: 'error' })
        return undefined
      }
    },
    [toast],
  )

  const refresh = useCallback(async () => {
    try {
      setState(await api.state())
    } catch {
      /* 刷新失败时保留原状态 */
    }
  }, [])

  // 记住数量（设置中开启时）
  useEffect(() => {
    if (!settings.rememberDraft) return
    const t = window.setTimeout(() => {
      const counts: Record<string, number> = {}
      const repeats: Record<string, number> = {}
      for (const [name, p] of Object.entries(draftToPicks(draft, state.categories))) {
        counts[name] = p.count
        if (p.repeat > 1) repeats[name] = p.repeat
      }
      api.saveDraft(counts, repeats, repeatMode)
    }, 600)
    return () => window.clearTimeout(t)
  }, [draft, repeatMode, state.categories, settings.rememberDraft])

  const startCopy = async (picks: Record<string, Pick>, mode: RepeatMode, clean: boolean) => {
    if (license.state !== 'active') return
    if (copyRef.current.phase === 'copying') return
    const plan = await run(api.prepare(picks, mode, clean))
    if (!plan) return refresh()
    if (plan.old > 0) {
      const ok = await dialogs.confirm({
        title: `清空「${plan.folder}」里的 ${num(plan.old)} 个旧视频？`,
        message: (
          <>
            会先删除这个文件夹里现有的 {num(plan.old)} 个视频，再放入本次抽到的 {num(plan.count)} 个。其他文件和子文件夹会保留。
            <br />
            请确认处理程序已经停止使用这个文件夹。
          </>
        ),
        confirmText: '清空并复制',
        danger: true,
      })
      if (!ok) return
    }
    setCopy({ phase: 'copying', plan, progress: { done: 0, total: plan.count, name: '', bytesDone: 0, bytesTotal: plan.bytes } })
    try {
      await api.start(plan.id)
    } catch (e) {
      toast(errorText(e), { kind: 'error' })
      setCopy({ phase: 'idle' })
    }
  }

  // 复制进度与结果
  useEffect(() => {
    const offProgress = events.onProgress(p => {
      setCopy(c => (c.phase === 'copying' ? { ...c, progress: p } : c))
    })
    const offDone = events.onDone(d => {
      setCopy(c => ({ phase: 'done', plan: c.plan, done: d }))
      refresh()
      if (d.ok) {
        setHistoryVersion(v => v + 1)
        toast(`已复制 ${num(d.result.items.length)} 个视频到「${d.result.folder}」`, { kind: 'ok' })
      } else if (d.cancelled) {
        toast(d.error)
      } else {
        toast(d.error, { kind: 'error', duration: 10000 })
      }
    })
    const offClose = events.onCloseWhileBusy(async () => {
      const ok = await dialogs.confirm({
        title: '正在复制，取消并退出？',
        message: '会停止复制，把目标文件夹恢复原样后再退出。',
        confirmText: '取消复制并退出',
        danger: true,
      })
      if (ok) api.cancelAndQuit()
    })
    return () => {
      offProgress()
      offDone()
      offClose()
    }
  }, [])

  // 回到窗口时自动刷新：在资源管理器里增减视频后不用手动刷新。
  useEffect(() => {
    const onFocus = () => {
      if (copyRef.current.phase !== 'copying') refresh()
    }
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [refresh])

  // 主题
  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () => {
      document.documentElement.dataset.theme = settings.theme === 'system' ? (mq.matches ? 'dark' : 'light') : settings.theme
    }
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  }, [settings.theme])

  const offlineNotice = useRef('')
  useEffect(() => {
    const until = license.offline ? String(license.graceUntil) : ''
    if (until && until !== offlineNotice.current) {
      toast(`暂时无法连接授权服务器，可以离线使用到 ${day(license.graceUntil)}，请在此之前联网打开一次`, { duration: 10000 })
    }
    offlineNotice.current = until
  }, [license.offline, license.graceUntil, toast])

  // 快捷键 Ctrl+1 / Ctrl+2 / Ctrl+3 切换页面
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (document.querySelector('.modal-layer, .license-overlay, .lock-overlay')) return
      if ((e.ctrlKey || e.metaKey) && (e.key === '1' || e.key === '2' || e.key === '3')) {
        e.preventDefault()
        setView(NAV[Number(e.key) - 1].view)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const togglePin = async () => {
    const next = { ...settings, alwaysOnTop: !settings.alwaysOnTop }
    setSettings(next)
    const s = await run(api.updateSettings(next))
    if (s) setSettings(s)
  }

  const ctx: Ctx = {
    state,
    setState,
    refresh,
    settings,
    setSettings,
    version: init.version,
    view,
    go: setView,
    selected,
    select,
    toast,
    confirm: dialogs.confirm,
    prompt: dialogs.prompt,
    run,
    historyVersion,
    copy,
    draft,
    setDraft,
    repeatMode,
    setRepeatMode,
    startCopy,
    cancelCopy: () => api.cancel(),
    clearResult: () => setCopy({ phase: 'idle' }),
    license,
    setLicense,
    lock,
    setLock,
  }

  return (
    <AppContext.Provider value={ctx}>
      <div className="licensed-content" inert={license.state !== 'active'}>
      <div className="app" inert={!!locked}>
        <header className="topbar">
          <div className="brand">
            <Logo />
            <span>视频分发工具</span>
          </div>
          <nav className="nav" role="tablist" aria-label="页面">
            {NAV.map((n, i) => (
              <button
                key={n.view}
                role="tab"
                aria-selected={view === n.view}
                className={view === n.view ? 'is-on' : ''}
                onClick={() => setView(n.view)}
                title={`Ctrl+${i + 1}`}
              >
                {n.icon}
                {n.label}
              </button>
            ))}
          </nav>
          <div className="topbar-right">
            {copy.phase === 'copying' && view !== 'extract' && (
              <button className="btn btn-ghost copying-chip" onClick={() => setView('extract')}>
                正在复制 {copy.progress ? `${Math.floor((copy.progress.bytesDone / Math.max(1, copy.progress.bytesTotal)) * 100)}%` : ''}
              </button>
            )}
            {lock.enabled && (
              <button className="icon-btn" onClick={() => setLocked('manual')} aria-label="立即锁定" title="立即锁定：再次使用要输入密码">
                <Lock size={17} />
              </button>
            )}
            <button
              className={`icon-btn pin ${settings.alwaysOnTop ? 'is-on' : ''}`}
              onClick={togglePin}
              aria-pressed={settings.alwaysOnTop}
              aria-label="窗口置顶"
              title={settings.alwaysOnTop ? '窗口置顶：已开启' : '窗口置顶：已关闭'}
            >
              {settings.alwaysOnTop ? <Pin size={17} /> : <PinOff size={17} />}
            </button>
            <button className="icon-btn" onClick={() => setSettingsOpen(true)} aria-label="设置" title="设置">
              <SettingsIcon size={17} />
            </button>
          </div>
        </header>
        <main className="main">
          {/* 按页面分别兜底：一个页面出错时仍可切换到其他页面。 */}
          <ErrorBoundary key={view}>
            {view === 'extract' && <ExtractView />}
            {view === 'library' && <LibraryView />}
            {view === 'history' && <HistoryView />}
          </ErrorBoundary>
        </main>
      </div>
      {settingsOpen && <SettingsDialog onClose={() => setSettingsOpen(false)} />}
      {dialogs.node}
      {toastNode}
      <div id="overlay-root" />
      {locked && (
        <LockScreen
          status={lock}
          dropIn={locked === 'manual'}
          onStatus={setLock}
          onUnlocked={reset => {
            setLocked(null)
            if (reset) toast('锁屏密码已关闭，可以在设置里重新设置', { kind: 'ok' })
          }}
        />
      )}
      </div>
      {license.state !== 'active' && (
        <div className="license-overlay" role="dialog" aria-modal="true" aria-label="重新验证授权">
          {copy.phase === 'copying' && (
            <div className="license-copy-notice" role="status">
              <span>已开始的复制会继续完成；重新验证后可以开始下一批。</span>
              <button className="btn btn-sm" onClick={() => api.cancel()}>取消复制</button>
              <button className="btn btn-sm" onClick={() => api.cancelAndQuit()}>取消复制并退出</button>
            </div>
          )}
          <LicenseGate status={license} version={init.version} onActivated={setLicense} />
        </div>
      )}
    </AppContext.Provider>
  )
}

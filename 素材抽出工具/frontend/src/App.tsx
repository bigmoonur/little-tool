import { useCallback, useEffect, useRef, useState } from 'react'
import { History, Library, Pin, PinOff, Plus, Settings, Shuffle, Upload } from 'lucide-react'
import { api, errorText, runtime } from './api'
import { AppContext, type Ctx, type ImportOpen } from './ctx'
import { ImportDialog } from './dialogs/ImportDialog'
import { SettingsDialog } from './dialogs/SettingsDialog'
import { hasFiles, loadFiles, type LoadedFile } from './lib/files'
import { draftToPicks, picksToDraft } from './lib/pick'
import type { Draft, Extracted, InitInfo, LicenseStatus, PickRequest, RepeatMode, Snapshot, View } from './types'
import { useDialogs } from './ui/Modal'
import { useToasts } from './ui/Toasts'
import { ExtractView } from './views/ExtractView'
import { HistoryView } from './views/HistoryView'
import { LibraryView } from './views/LibraryView'
import { LicenseGate } from './views/LicenseGate'
import { Logo } from './ui/Logo'
import { day } from './lib/format'

const NO_LICENSE: LicenseStatus = {
  state: 'inactive', key: '', masked: '', expiry: '', offline: false, graceUntil: 0, code: '', message: '', network: false, machine: '',
}

export default function App() {
  const [boot, setBoot] = useState<InitInfo | { error: string } | null>(null)
  const [license, setLicense] = useState<LicenseStatus | null>(null)
  const opened = useRef(false)

  const load = () => api.init().then(setBoot, e => setBoot({ error: errorText(e) }))

  useEffect(() => {
    load()
    api.licenseStatus().then(setLicense, e => setLicense({ ...NO_LICENSE, network: true, message: errorText(e) }))
    return runtime.onLicenseChanged(setLicense)
  }, [])

  if (!boot || !license) {
    return (
      <div className="boot">
        {!license && (
          <div className="boot-msg">
            <Logo />
            正在验证授权…
          </div>
        )}
      </div>
    )
  }
  if (!('error' in boot) && license.state !== 'active' && !opened.current) {
    return (
      <LicenseGate
        status={license}
        version={boot.version}
        onActivated={st => {
          // 重新读取素材库后再进入主界面。
          setBoot(null)
          load().then(() => setLicense(st))
        }}
      />
    )
  }
  if ('error' in boot) {
    return (
      <div className="fatal">
        <h1>链接抽取工具无法启动</h1>
        <p>{boot.error}</p>
        <p className="dim">请确认当前 Windows 账户可以写入“应用数据”文件夹，然后重新打开程序。</p>
      </div>
    )
  }
  if (license.state === 'active') opened.current = true
  return (
    <>
      <div className="licensed-content" inert={license.state !== 'active'}>
        <Shell init={boot} license={license} setLicense={setLicense} />
        <div id="overlay-root" />
      </div>
      {license.state !== 'active' && (
        <div className="license-overlay" role="dialog" aria-modal="true" aria-label="重新验证授权">
          <LicenseGate status={license} version={boot.version} onActivated={setLicense} />
        </div>
      )}
    </>
  )
}

const NAV: { view: View; label: string; icon: React.ReactNode }[] = [
  { view: 'extract', label: '抽取', icon: <Shuffle size={16} /> },
  { view: 'library', label: '素材库', icon: <Library size={16} /> },
  { view: 'history', label: '记录', icon: <History size={16} /> },
]

function isEditable(el: Element | null) {
  return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || (el as HTMLElement).isContentEditable)
}

function Shell({ init, license, setLicense }: { init: InitInfo; license: LicenseStatus; setLicense: (s: LicenseStatus) => void }) {
  const [snap, setSnap] = useState<Snapshot>(init.snapshot)
  const [view, setView] = useState<View>(init.snapshot.lists.length ? 'extract' : 'library')
  const [draft, setDraft] = useState<Draft>(() => picksToDraft(init.snapshot.draft ?? {}))
  const remembered = Object.values(init.snapshot.draft ?? {})[0]
  const [repeatMode, setRepeatMode] = useState<RepeatMode>(remembered?.mode ?? 'consecutive')
  const [tagFilter, setTagFilter] = useState<string[]>(remembered?.tagIds ?? [])
  const [lastExtraction, setLastExtraction] = useState<{ response: Extracted; requests: PickRequest[] } | null>(null)
  const [selectedId, setSelectedId] = useState('')
  const [importInit, setImportInit] = useState<ImportOpen | null>(null)
  const [dropped, setDropped] = useState<LoadedFile[] | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [fileDrag, setFileDrag] = useState(false)
  const [historyVersion, setHistoryVersion] = useState(0)
  const guard = useRef<(() => Promise<boolean>) | null>(null)
  const { toast, node: toastNode } = useToasts()
  const dialogs = useDialogs()

  const passGuard = async () => (guard.current ? guard.current() : true)

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

  const undo = async () => {
    const s = await run(api.undo())
    if (s) {
      setSnap(s)
      toast('已撤销', { kind: 'ok' })
    }
  }

  const ctx: Ctx = {
    snap,
    setSnap,
    info: { version: init.version, platform: init.platform, maxLines: init.maxLines },
    view,
    go: async v => {
      if (v === view) return true
      if (!(await passGuard())) return false
      setView(v)
      return true
    },
    draft,
    setDraft,
    repeatMode, setRepeatMode, tagFilter, setTagFilter, lastExtraction, setLastExtraction,
    selectedId,
    select: async id => {
      if (id === selectedId) return true
      if (!(await passGuard())) return false
      setSelectedId(id)
      return true
    },
    toast,
    confirm: dialogs.confirm,
    prompt: dialogs.prompt,
    run,
    commit: (s, message) => {
      setSnap(s)
      if (message) toast(message, { kind: 'ok', action: s.undo ? { label: '撤销', onClick: undo } : undefined })
    },
    openImport: o => setImportInit(o ?? {}),
    openSettings: () => setSettingsOpen(true),
    setGuard: g => {
      // 告诉后端有没有未保存的修改：没有时点关闭按钮立即关闭。
      if (!!g !== !!guard.current) api.setUnsaved(!!g)
      guard.current = g
    },
    historyVersion,
    bumpHistory: () => setHistoryVersion(v => v + 1),
    license,
    setLicense,
    leave: passGuard,
  }
  const ctxRef = useRef(ctx)
  ctxRef.current = ctx

  // 主题
  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () => {
      const t = snap.settings.theme === 'system' ? (mq.matches ? 'dark' : 'light') : snap.settings.theme
      document.documentElement.dataset.theme = t
    }
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  }, [snap.settings.theme])

  // 记住数量（设置中开启时）
  useEffect(() => {
    if (!snap.settings.rememberDraft) return
    const t = window.setTimeout(() => api.saveDraft(draftToPicks(draft, snap.lists, repeatMode, tagFilter)), 600)
    return () => window.clearTimeout(t)
  }, [draft, snap.lists, repeatMode, tagFilter, snap.settings.rememberDraft])

  const noticeShown = useRef(false)
  useEffect(() => {
    if (noticeShown.current) return
    noticeShown.current = true
    if (init.notice) toast(init.notice, { kind: 'error', duration: 15000 })
    if (license.offline) {
      toast(`暂时无法连接授权服务器，可以离线使用到 ${day(license.graceUntil)}，请在此之前联网打开一次`, { duration: 10000 })
    }
  }, [])

  // 关闭窗口前检查未保存的修改
  useEffect(() => {
    return runtime.onCloseRequested(async () => {
      if (!guard.current) return api.confirmClose()
      api.holdClose()
      if (await guard.current()) api.confirmClose()
    })
  }, [])

  // 全局快捷键
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (document.querySelector('.modal-layer, .license-overlay')) return
      const mod = e.ctrlKey || e.metaKey
      if (mod && ['1', '2', '3'].includes(e.key)) {
        e.preventDefault()
        ctxRef.current.go(NAV[Number(e.key) - 1].view)
      } else if (mod && e.key.toLowerCase() === 'z' && !e.shiftKey && !isEditable(document.activeElement) && ctxRef.current.snap.undo) {
        e.preventDefault()
        undo()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // 把 TXT 文件拖进窗口任意位置
  const importOpenRef = useRef(false)
  importOpenRef.current = importInit !== null
  useEffect(() => {
    let timer = 0
    const over = (e: DragEvent) => {
      if (!hasFiles(e)) return
      e.preventDefault()
      if (!importOpenRef.current) setFileDrag(true)
      window.clearTimeout(timer)
      timer = window.setTimeout(() => setFileDrag(false), 160)
    }
    const drop = async (e: DragEvent) => {
      if (!hasFiles(e)) return
      e.preventDefault()
      setFileDrag(false)
      const { loaded, skipped } = await loadFiles(Array.from(e.dataTransfer?.files ?? []))
      if (skipped.length) toast(`只支持 TXT 文本文件，已跳过：${skipped.join('、')}`, { kind: 'error' })
      if (!loaded.length) return
      if (importOpenRef.current) setDropped(loaded)
      else setImportInit({ tab: 'files', files: loaded })
    }
    window.addEventListener('dragover', over)
    window.addEventListener('drop', drop)
    return () => {
      window.removeEventListener('dragover', over)
      window.removeEventListener('drop', drop)
    }
  }, [])

  const togglePin = async () => {
    const next = !snap.settings.alwaysOnTop
    setSnap({ ...snap, settings: { ...snap.settings, alwaysOnTop: next } })
    const s = await run(api.updateSettings({ ...snap.settings, alwaysOnTop: next }))
    if (s) setSnap(s)
  }

  return (
    <AppContext.Provider value={ctx}>
      <div className="app">
        <header className="topbar">
          <div className="brand">
            <Logo />
            <span>链接抽取工具</span>
          </div>
          <nav className="nav" role="tablist" aria-label="页面">
            {NAV.map((n, i) => (
              <button
                key={n.view}
                role="tab"
                aria-selected={view === n.view}
                className={view === n.view ? 'is-on' : ''}
                onClick={() => ctx.go(n.view)}
                title={`Ctrl+${i + 1}`}
              >
                {n.icon}
                {n.label}
              </button>
            ))}
          </nav>
          <div className="topbar-right">
            <button className="btn" onClick={() => ctx.openImport()}>
              <Plus size={16} />
              添加素材
            </button>
            <button
              className={`icon-btn pin ${snap.settings.alwaysOnTop ? 'is-on' : ''}`}
              onClick={togglePin}
              aria-pressed={snap.settings.alwaysOnTop}
              aria-label="窗口置顶"
              title={snap.settings.alwaysOnTop ? '窗口置顶：已开启' : '窗口置顶：已关闭'}
            >
              {snap.settings.alwaysOnTop ? <Pin size={17} /> : <PinOff size={17} />}
            </button>
            <button className="icon-btn" onClick={() => setSettingsOpen(true)} aria-label="设置" title="设置">
              <Settings size={17} />
            </button>
          </div>
        </header>

        <main className="main">
          {view === 'extract' && <ExtractView />}
          {view === 'library' && <LibraryView />}
          {view === 'history' && <HistoryView />}
        </main>

        {fileDrag && (
          <div className="drop-overlay" aria-hidden>
            <div className="drop-card">
              <Upload size={26} />
              <b>松开即可添加这些 TXT 文件</b>
              <span>下一步选择新建分类还是添加到已有分类</span>
            </div>
          </div>
        )}
      </div>
      {importInit && (
        <ImportDialog init={importInit} dropped={dropped} onConsumeDropped={() => setDropped(null)} onClose={() => setImportInit(null)} />
      )}
      {settingsOpen && <SettingsDialog onClose={() => setSettingsOpen(false)} />}
      {dialogs.node}
      {toastNode}
    </AppContext.Provider>
  )
}

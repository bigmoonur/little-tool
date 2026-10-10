import { useEffect, useMemo, useRef, useState } from 'react'
import { ClipboardPaste, FileText, FolderOpen, Upload, X } from 'lucide-react'
import { api } from '../api'
import { useApp, type ImportOpen } from '../ctx'
import { countLines, hasFiles, loadFiles, pickFiles, type LoadedFile } from '../lib/files'
import { num } from '../lib/format'
import type { ImportItem, ImportPlan, ImportReport, ListInfo, Snapshot } from '../types'
import { Modal } from '../ui/Modal'

type Tab = 'paste' | 'files'
type Conflict = 'append' | 'replace' | 'rename'

const lower = (s: string) => s.trim().toLowerCase()

function ListSelect({ snap, value, onChange, id }: { snap: Snapshot; value: string; onChange: (v: string) => void; id?: string }) {
  const loose = snap.lists.filter(l => !l.groupId)
  const opt = (l: ListInfo) => (
    <option key={l.id} value={l.id}>
      {l.name}（{num(l.count)} 条）
    </option>
  )
  return (
    <select id={id} className="select" value={value} onChange={e => onChange(e.target.value)}>
      <option value="" disabled>
        选择分类…
      </option>
      {snap.groups.map(g => {
        const lists = snap.lists.filter(l => l.groupId === g.id)
        return lists.length ? (
          <optgroup key={g.id} label={g.name}>
            {lists.map(opt)}
          </optgroup>
        ) : null
      })}
      {loose.length > 0 && (snap.groups.length ? <optgroup label="未分组">{loose.map(opt)}</optgroup> : loose.map(opt))}
    </select>
  )
}

function GroupSelect({ snap, value, onChange }: { snap: Snapshot; value: string; onChange: (v: string) => void }) {
  if (!snap.groups.length) return null
  return (
    <label className="inline-field">
      <span>放入分组</span>
      <select className="select" value={value} onChange={e => onChange(e.target.value)}>
        <option value="">未分组</option>
        {snap.groups.map(g => (
          <option key={g.id} value={g.id}>
            {g.name}
          </option>
        ))}
      </select>
    </label>
  )
}

export function ImportDialog({ init, dropped, onConsumeDropped, onClose }: {
  init: ImportOpen
  dropped: LoadedFile[] | null
  onConsumeDropped: () => void
  onClose: () => void
}) {
  const { snap, run, commit, toast, select, view } = useApp()
  const hasLists = snap.lists.length > 0

  const [tab, setTab] = useState<Tab>(init.tab ?? (init.files?.length ? 'files' : 'paste'))
  const [paste, setPaste] = useState('')
  const [files, setFiles] = useState<LoadedFile[]>(init.files ?? [])
  const [pasteMode, setPasteMode] = useState<'existing' | 'new'>(init.targetId || (hasLists && init.groupId === undefined) ? 'existing' : 'new')
  const [filesMode, setFilesMode] = useState<'each' | 'existing' | 'merge'>(init.targetId ? 'existing' : 'each')
  const [targetId, setTargetId] = useState(init.targetId ?? '')
  const [replace, setReplace] = useState(false)
  const [newName, setNewName] = useState('')
  const [groupId, setGroupId] = useState(init.groupId ?? snap.lists.find(l => l.id === init.targetId)?.groupId ?? '')
  const [conflicts, setConflicts] = useState<Record<string, Conflict>>({})
  const [dedupe, setDedupe] = useState(snap.settings.importDedupe)
  const [report, setReport] = useState<ImportReport | null>(null)
  const [busy, setBusy] = useState(false)
  const [dragOver, setDragOver] = useState(false)
  const pasteRef = useRef<HTMLTextAreaElement>(null)

  const target = snap.lists.find(l => l.id === targetId)
  const pasteLines = useMemo(() => countLines(paste), [paste])

  const addFiles = (more: LoadedFile[]) => {
    setFiles(f => [...f, ...more])
    setTab('files')
  }

  useEffect(() => {
    if (dropped?.length) {
      addFiles(dropped)
      onConsumeDropped()
    }
  }, [dropped])

  const chooseFiles = async () => {
    const picked = await pickFiles()
    if (!picked.length) return
    const { loaded, skipped } = await loadFiles(picked)
    if (skipped.length) toast(`只支持 TXT 文本文件，已跳过：${skipped.join('、')}`, { kind: 'error' })
    addFiles(loaded)
  }

  const onDrop = async (e: React.DragEvent) => {
    if (!hasFiles(e)) return
    e.preventDefault()
    e.stopPropagation()
    setDragOver(false)
    const { loaded, skipped } = await loadFiles(Array.from(e.dataTransfer.files))
    if (skipped.length) toast(`只支持 TXT 文本文件，已跳过：${skipped.join('、')}`, { kind: 'error' })
    addFiles(loaded)
  }

  // 每个文件对应的去向：同名分类默认追加，也可以替换或另建。
  const eachTargets = useMemo(() => {
    const used = new Set(snap.lists.map(l => lower(l.name)))
    return files.map(f => {
      const existing = snap.lists.find(l => lower(l.name) === lower(f.baseName))
      const how = conflicts[f.key] ?? 'append'
      if (existing && how !== 'rename') return { file: f, existing, mode: how as 'append' | 'replace', name: existing.name }
      let name = f.baseName
      for (let k = 2; used.has(lower(name)); k++) name = `${f.baseName} (${k})`
      used.add(lower(name))
      return { file: f, existing, mode: 'new' as const, name }
    })
  }, [files, conflicts, snap.lists])

  const plan: ImportPlan | null = useMemo(() => {
    const item = (p: Partial<ImportItem>): ImportItem => ({ source: '', text: '', mode: 'new', targetId: '', newName: '', groupId: '', ...p })
    if (tab === 'paste') {
      if (!paste.trim()) return null
      if (pasteMode === 'existing') {
        if (!targetId) return null
        return { dedupe, items: [item({ source: '粘贴的文本', text: paste, mode: replace ? 'replace' : 'append', targetId })] }
      }
      return { dedupe, items: [item({ source: '粘贴的文本', text: paste, mode: 'new', newName, groupId })] }
    }
    if (!files.length) return null
    if (filesMode === 'existing') {
      if (!targetId) return null
      if (replace) {
        return { dedupe, items: [item({ source: files.map(f => f.fileName).join('、'), text: files.map(f => f.text).join('\n'), mode: 'replace', targetId })] }
      }
      return { dedupe, items: files.map(f => item({ source: f.fileName, text: f.text, mode: 'append', targetId })) }
    }
    if (filesMode === 'merge') {
      return {
        dedupe,
        items: [item({ source: files.map(f => f.fileName).join('、'), text: files.map(f => f.text).join('\n'), mode: 'new', newName, groupId })],
      }
    }
    return {
      dedupe,
      items: eachTargets.map(t =>
        item({
          source: t.file.fileName,
          text: t.file.text,
          mode: t.mode,
          targetId: t.mode === 'new' ? '' : t.existing!.id,
          newName: t.mode === 'new' ? t.name : '',
          groupId,
        }),
      ),
    }
  }, [tab, paste, pasteMode, targetId, replace, newName, groupId, files, filesMode, eachTargets, dedupe])

  // 预览统计（新增、重复、替换）由后端按真实数据计算。
  useEffect(() => {
    if (!plan) return setReport(null)
    let alive = true
    const t = window.setTimeout(() => {
      api.previewImport(plan).then(
        r => alive && setReport(r),
        () => alive && setReport({ items: [], added: 0, duplicates: 0, removed: 0, created: 0, touched: 0, ok: false, error: '无法计算，请稍后再试' }),
      )
    }, 180)
    return () => {
      alive = false
      window.clearTimeout(t)
    }
  }, [plan])

  const missing = (() => {
    if (tab === 'paste') {
      if (!paste.trim()) return '粘贴要添加的内容'
      if (pasteMode === 'existing' && !targetId) return '选择要添加到哪个分类'
      if (pasteMode === 'new' && !newName.trim()) return '填写新分类的名称'
    } else {
      if (!files.length) return '选择或拖入 TXT 文件'
      if (filesMode === 'existing' && !targetId) return '选择要添加到哪个分类'
      if (filesMode === 'merge' && !newName.trim()) return '填写新分类的名称'
    }
    return ''
  })()

  const isReplace = !!plan?.items.some(i => i.mode === 'replace')
  const ready = !!plan && !!report?.ok && !missing

  const apply = async () => {
    if (!plan || !ready || busy) return
    setBusy(true)
    const res = await run(api.applyImport(plan))
    setBusy(false)
    if (!res) return
    const r = res.report
    const parts = [`已添加 ${num(r.added)} 条`]
    if (r.created) parts.push(`新建 ${r.created} 个分类`)
    if (r.duplicates) parts.push(`跳过重复 ${num(r.duplicates)} 条`)
    commit(res.snapshot, parts.join('，'))
    if (view === 'library') {
      const name = r.items.find(i => !i.error)?.target
      const l = res.snapshot.lists.find(x => x.name === name)
      if (l) select(l.id)
    }
    onClose()
  }

  const summary = () => {
    if (missing) return <span className="dim">{missing}</span>
    if (!report) return <span className="dim">正在计算…</span>
    if (!report.ok) return <span className="err-text">{report.error}</span>
    const bits = [`将新增 ${num(report.added)} 条`]
    if (report.created) bits.push(`新建 ${report.created} 个分类`)
    if (report.duplicates) bits.push(`跳过重复 ${num(report.duplicates)} 条`)
    return (
      <span>
        {bits.join('，')}
        {report.removed > 0 && <span className="err-text">，替换掉原有的 {num(report.removed)} 条</span>}
      </span>
    )
  }

  const stat = (i: number) => {
    const s = report?.items[i]
    if (!s) return null
    if (s.error) return <span className="err-text">{s.error}</span>
    if (s.lines === 0) return <span className="dim">没有内容</span>
    return (
      <span className="file-stat">
        <b>+{num(s.added)}</b>
        {s.duplicates > 0 && <span className="dim"> 重复 {num(s.duplicates)}</span>}
      </span>
    )
  }

  const footer = (
    <>
      <div className="import-summary">{summary()}</div>
      <button className="btn" onClick={onClose}>
        取消
      </button>
      <button className={`btn ${isReplace ? 'btn-danger' : 'btn-primary'}`} onClick={apply} disabled={!ready || busy}>
        {isReplace ? '替换内容' : report?.ok ? `添加 ${num(report.added)} 条` : '添加'}
      </button>
    </>
  )

  return (
    <Modal title="添加素材" onClose={onClose} width={640} footer={footer} className="import" initialFocus={tab === 'paste' ? 'textarea' : undefined}>
      <div className="tabs" role="tablist">
        <button role="tab" aria-selected={tab === 'paste'} className={tab === 'paste' ? 'is-on' : ''} onClick={() => setTab('paste')}>
          <ClipboardPaste size={16} />
          粘贴文本
        </button>
        <button role="tab" aria-selected={tab === 'files'} className={tab === 'files' ? 'is-on' : ''} onClick={() => setTab('files')}>
          <FileText size={16} />
          TXT 文件{files.length > 0 && <span className="tab-badge">{files.length}</span>}
        </button>
      </div>

      {tab === 'paste' ? (
        <div className="import-source">
          <textarea
            ref={pasteRef}
            className="paste-box"
            value={paste}
            onChange={e => setPaste(e.target.value)}
            placeholder="每行一条素材，可以一次粘贴很多行"
            spellCheck={false}
            wrap="off"
            aria-label="要添加的内容"
          />
          <div className="paste-count dim">{pasteLines ? `识别到 ${num(pasteLines)} 行（空行会被忽略）` : ' '}</div>
        </div>
      ) : (
        <div
          className={`import-source drop-zone ${dragOver ? 'is-over' : ''} ${files.length ? 'has-files' : ''}`}
          onDragOver={e => {
            if (!hasFiles(e)) return
            e.preventDefault()
            e.stopPropagation()
            setDragOver(true)
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
        >
          {files.length === 0 ? (
            <div className="drop-empty">
              <Upload size={22} />
              <p>把 TXT 文件拖到这里，可以一次放多个</p>
              <button className="btn" onClick={chooseFiles}>
                <FolderOpen size={16} />
                选择文件
              </button>
            </div>
          ) : (
            <>
              <ul className="file-list">
                {files.map((f, i) => {
                  const t = eachTargets[i]
                  return (
                    <li key={f.key}>
                      <FileText size={16} className="dim" />
                      <div className="file-main">
                        <div className="file-name" title={f.fileName}>
                          {f.fileName}
                          {f.encoding !== 'UTF-8' && <span className="badge">{f.encoding}</span>}
                        </div>
                        <div className="file-sub dim">
                          {num(f.lines)} 行
                          {filesMode === 'each' && t && (
                            <>
                              {' → '}
                              {t.existing ? (
                                <select
                                  className="select select-sm"
                                  value={conflicts[f.key] ?? 'append'}
                                  onChange={e => setConflicts(c => ({ ...c, [f.key]: e.target.value as Conflict }))}
                                  aria-label={`${f.fileName} 的添加方式`}
                                >
                                  <option value="append">追加到已有的「{t.existing.name}」</option>
                                  <option value="replace">替换「{t.existing.name}」的全部内容</option>
                                  <option value="rename">另建新分类</option>
                                </select>
                              ) : (
                                <span>新建「{t.name}」</span>
                              )}
                            </>
                          )}
                        </div>
                      </div>
                      <div className="file-right">{filesMode === 'merge' || (filesMode === 'existing' && replace) ? null : stat(i)}</div>
                      <button className="icon-btn sm" aria-label={`移除 ${f.fileName}`} onClick={() => setFiles(fs => fs.filter(x => x.key !== f.key))}>
                        <X size={14} />
                      </button>
                    </li>
                  )
                })}
              </ul>
              <button className="link add-more" onClick={chooseFiles}>
                继续添加文件
              </button>
            </>
          )}
        </div>
      )}

      <fieldset className="dest">
        <legend>添加到</legend>
        {tab === 'paste' ? (
          <>
            <label className={`radio-row ${!hasLists ? 'is-disabled' : ''}`}>
              <input type="radio" checked={pasteMode === 'existing'} disabled={!hasLists} onChange={() => setPasteMode('existing')} />
              <span className="radio-label">已有分类</span>
              {pasteMode === 'existing' && <ListSelect snap={snap} value={targetId} onChange={setTargetId} />}
            </label>
            <label className="radio-row">
              <input type="radio" checked={pasteMode === 'new'} onChange={() => setPasteMode('new')} />
              <span className="radio-label">新建分类</span>
              {pasteMode === 'new' && (
                <input className="input" value={newName} maxLength={40} onChange={e => setNewName(e.target.value)} placeholder="分类名称" aria-label="新分类名称" />
              )}
            </label>
            {pasteMode === 'new' && (
              <div className="radio-sub">
                <GroupSelect snap={snap} value={groupId} onChange={setGroupId} />
              </div>
            )}
          </>
        ) : (
          <>
            <label className="radio-row">
              <input type="radio" checked={filesMode === 'each'} onChange={() => setFilesMode('each')} />
              <span className="radio-label">每个文件一个分类</span>
              <span className="dim radio-note">以文件名命名，同名的分类默认追加</span>
            </label>
            {filesMode === 'each' && snap.groups.length > 0 && (
              <div className="radio-sub">
                <GroupSelect snap={snap} value={groupId} onChange={setGroupId} />
              </div>
            )}
            <label className={`radio-row ${!hasLists ? 'is-disabled' : ''}`}>
              <input type="radio" checked={filesMode === 'existing'} disabled={!hasLists} onChange={() => setFilesMode('existing')} />
              <span className="radio-label">全部添加到已有分类</span>
              {filesMode === 'existing' && <ListSelect snap={snap} value={targetId} onChange={setTargetId} />}
            </label>
            <label className="radio-row">
              <input type="radio" checked={filesMode === 'merge'} onChange={() => setFilesMode('merge')} />
              <span className="radio-label">合并成一个新分类</span>
              {filesMode === 'merge' && (
                <input className="input" value={newName} maxLength={40} onChange={e => setNewName(e.target.value)} placeholder="分类名称" aria-label="新分类名称" />
              )}
            </label>
            {filesMode === 'merge' && snap.groups.length > 0 && (
              <div className="radio-sub">
                <GroupSelect snap={snap} value={groupId} onChange={setGroupId} />
              </div>
            )}
          </>
        )}

        {((tab === 'paste' && pasteMode === 'existing') || (tab === 'files' && filesMode === 'existing')) && target && (
          <label className={`check-row ${replace ? 'is-danger' : ''}`}>
            <input type="checkbox" checked={replace} onChange={e => setReplace(e.target.checked)} />
            替换「{target.name}」原有的 {num(target.count)} 条内容，而不是追加
          </label>
        )}
      </fieldset>

      <label className="check-row">
        <input type="checkbox" checked={dedupe} onChange={e => setDedupe(e.target.checked)} />
        跳过重复的行（分类里已有的，或这次添加中重复出现的）
      </label>
    </Modal>
  )
}

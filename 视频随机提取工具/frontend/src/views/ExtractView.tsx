import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Check, Eraser, FolderOpen, FolderPlus, RefreshCw, TriangleAlert, X,
} from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { bytes, clock, num, shortPath } from '../lib/format'
import type { Category, Item } from '../types'
import { EnterKey } from '../ui/EnterKey'
import { Setup } from './Setup'

interface Row {
  count: number
  error: string
}

function evaluate(c: Category, raw: string | undefined): Row {
  const v = (raw ?? '').trim()
  if (v === '') return { count: 0, error: '' }
  if (!/^\d+$/.test(v)) return { count: 0, error: '请填写整数' }
  const n = Number(v)
  if (n > c.count) return { count: n, error: `最多 ${num(c.count)} 个` }
  return { count: n, error: '' }
}

function validateFolderName(name: string) {
  if (!name.trim()) return '请输入文件夹名称'
  if (name === '.' || name === '..' || /[ .]$/.test(name)) return '名称不能是 . 或 ..，也不能以空格或句点结尾'
  if (/[<>:"/\\|?*\x00-\x1f]/.test(name)) return '名称不能包含以下字符：< > : " / \\ | ? *'
  const stem = name.split('.')[0].trimEnd().toUpperCase()
  if (/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(stem)) return '这个名称是 Windows 保留名称，请换一个'
  return ''
}

function groupItems(items: Item[]) {
  const map = new Map<string, Item[]>()
  for (const it of items) map.set(it.category, [...(map.get(it.category) ?? []), it])
  return [...map.entries()]
}

export function ExtractView() {
  const { state, setState, refresh, settings, run, prompt, toast, copy, startCopy, cancelCopy, clearResult } = useApp()
  const [draft, setDraft] = useState<Record<string, string>>(() =>
    Object.fromEntries(Object.entries(settings.draft ?? {}).map(([k, v]) => [k, v ? String(v) : ''])),
  )
  const [clean, setClean] = useState(false)
  const inputs = useRef(new Map<string, HTMLInputElement>())
  const doneBtn = useRef<HTMLButtonElement>(null)
  const copying = copy.phase === 'copying'

  const rows = useMemo(() => {
    const m = new Map<string, Row>()
    for (const c of state.categories) m.set(c.name, evaluate(c, draft[c.name]))
    return m
  }, [state.categories, draft])

  const active = state.categories.filter(c => (rows.get(c.name)?.count ?? 0) > 0)
  const errors = state.categories.filter(c => rows.get(c.name)?.error)
  const total = active.reduce((s, c) => s + (rows.get(c.name)?.count ?? 0), 0)
  const folder = state.destinations.find(d => d.name === state.folder)
  const order = state.categories.filter(c => c.count > 0).map(c => c.name)
  const hasInput = Object.values(draft).some(v => v.trim())
  const ready = !!folder && total > 0 && errors.length === 0 && !copying

  // 记住数量（设置中开启时）
  useEffect(() => {
    if (!settings.rememberDraft) return
    const t = window.setTimeout(() => {
      const d: Record<string, number> = {}
      for (const [k, v] of Object.entries(draft)) if (/^\d+$/.test(v.trim()) && Number(v) > 0) d[k] = Number(v)
      api.saveDraft(d)
    }, 600)
    return () => window.clearTimeout(t)
  }, [draft, settings.rememberDraft])

  // 换了文件夹或文件夹里没有视频时，取消“清空旧视频”。
  useEffect(() => {
    if (!folder || folder.videos === 0) setClean(false)
  }, [folder?.name, folder?.videos])

  useEffect(() => {
    if (copy.phase === 'done' && copy.done?.ok) doneBtn.current?.focus()
  }, [copy.phase])

  const start = () => {
    if (copying) return
    if (errors.length) {
      toast(`「${errors[0].name}」：${rows.get(errors[0].name)?.error}`, { kind: 'error' })
      return focusRow(errors[0].name)
    }
    if (total === 0) {
      toast('请至少为一个分类填写数量', { kind: 'error' })
      return focusRow(order[0])
    }
    if (!folder) {
      toast('请选择或新建一个放入视频的文件夹', { kind: 'error' })
      return
    }
    const counts: Record<string, number> = {}
    for (const c of active) counts[c.name] = rows.get(c.name)!.count
    startCopy(counts, clean).then(() => setClean(false))
  }
  const startRef = useRef(start)
  startRef.current = start
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !document.querySelector('.modal-layer, .license-overlay')) {
        e.preventDefault()
        startRef.current()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const focusRow = (name?: string) => {
    if (!name) return
    const el = inputs.current.get(name)
    el?.focus()
    el?.select()
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>, name: string) => {
    const i = order.indexOf(name)
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
      e.preventDefault()
      if (e.shiftKey) return focusRow(order[i - 1])
      if (i < order.length - 1) return focusRow(order[i + 1])
      start()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      focusRow(order[i + 1])
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      focusRow(order[i - 1])
    }
  }

  const chooseMaterial = async () => {
    const s = await run(api.chooseMaterialRoot())
    if (s) {
      setState(s)
      setDraft({})
    }
  }
  const chooseOutput = async () => {
    const s = await run(api.chooseOutputRoot())
    if (s) setState(s)
  }
  const pickFolder = async (name: string) => setState(await api.setFolder(name))

  const newFolder = async () => {
    const existing = new Set(state.destinations.map(d => d.name.toLowerCase()))
    const name = await prompt({
      title: '新建文件夹',
      label: '文件夹名称',
      placeholder: '例如：第一批',
      confirmText: '新建',
      hint: '会建在目标根目录里，并直接选中它。',
      validate: v => validateFolderName(v) || (existing.has(v.toLowerCase()) ? '已经有同名文件夹了，可以直接选择它' : ''),
    })
    if (!name) return
    const s = await run(api.createFolder(name))
    if (s) {
      setState(s)
      toast(`已新建并选中「${name}」`, { kind: 'ok' })
    }
  }

  const clearAll = () => {
    const prev = draft
    setDraft({})
    toast('已清空所有数量', { action: { label: '撤销', onClick: () => setDraft(prev) } })
  }

  if (!state.materialRoot || !state.outputRoot) return <Setup />

  const progress = copy.progress
  const pct = progress ? Math.min(100, Math.floor((progress.bytesDone / Math.max(1, progress.bytesTotal)) * 100)) : 0
  const result = copy.phase === 'done' && copy.done?.ok ? copy.done.result : null
  const failure = copy.phase === 'done' && copy.done && !copy.done.ok && !copy.done.cancelled ? copy.done.error : ''

  return (
    <div className="extract">
      <section className="extract-main" aria-label="填写抽取数量">
        {state.leftovers.length > 0 && (
          <div className="banner">
            <TriangleAlert size={16} />
            <div>
              上次复制意外中断，目标根目录里留下了临时文件夹。其中的 old 子文件夹可能存有旧视频，确认后可以手动删除。
              <div className="banner-paths">
                {state.leftovers.map(p => (
                  <button key={p} className="link" onClick={() => run(api.openFolder(p))}>
                    {p}
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        <div className="root-bar">
          <FolderOpen size={16} className="dim" />
          <span className="root-label">素材根目录</span>
          <span className="root-path" title={state.materialRoot}>
            {shortPath(state.materialRoot)}
          </span>
          <button className="btn btn-ghost btn-sm" onClick={chooseMaterial} disabled={copying}>
            更换
          </button>
          <button className="icon-btn sm" onClick={refresh} aria-label="刷新" title="刷新（回到窗口时也会自动刷新）" disabled={copying}>
            <RefreshCw size={15} />
          </button>
        </div>

        {state.materialError ? (
          <div className="empty-inline is-error">
            <p>{state.materialError}</p>
            <button className="btn" onClick={chooseMaterial}>
              重新选择素材根目录
            </button>
          </div>
        ) : state.categories.length === 0 ? (
          <div className="empty-inline">
            <h2>素材根目录里还没有分类</h2>
            <p>在素材根目录里新建子文件夹，每个子文件夹是一个分类，把视频放进去。回到这个窗口时会自动刷新。</p>
            <button className="btn" onClick={() => run(api.openFolder(state.materialRoot))}>
              <FolderOpen size={16} />
              打开素材根目录
            </button>
          </div>
        ) : (
          <>
            <div className="toolbar">
              {hasInput && (
                <button className="btn btn-ghost" onClick={clearAll} disabled={copying}>
                  <Eraser size={16} />
                  清空数量
                </button>
              )}
              <span className="toolbar-spacer" />
              <span className="toolbar-hint" title="在数量栏按回车跳到下一项，最后一项按回车直接抽取">
                <EnterKey /> 下一项，最后一项直接抽取
              </span>
            </div>
            <div className="pick-table cols-3" role="table">
              <div className="pick-head" role="row">
                <span role="columnheader">分类</span>
                <span role="columnheader" className="ta-c">数量</span>
                <span role="columnheader" className="ta-r">本次</span>
              </div>
              {state.categories.map(c => {
                const r = rows.get(c.name)!
                const empty = c.count === 0
                return (
                  <div
                    key={c.name}
                    role="row"
                    className={`pick-row c-none ${r.count > 0 ? 'is-active' : ''} ${r.error ? 'is-error' : ''} ${empty ? 'is-empty' : ''}`}
                  >
                    <div className="pick-name" role="cell">
                      <span className="pick-title" title={c.name}>
                        {c.name}
                      </span>
                      <span className="pick-meta">{empty ? '没有视频' : `共 ${num(c.count)} 个视频 · ${bytes(c.bytes)}`}</span>
                      {r.error && <span className="pick-error">{r.error}</span>}
                    </div>
                    <div role="cell" className="ta-c">
                      <input
                        ref={el => {
                          if (el) inputs.current.set(c.name, el)
                          else inputs.current.delete(c.name)
                        }}
                        className="num-input"
                        inputMode="numeric"
                        placeholder="0"
                        disabled={empty || copying}
                        aria-label={`${c.name} 的数量`}
                        value={draft[c.name] ?? ''}
                        onChange={e => setDraft(d => ({ ...d, [c.name]: e.target.value }))}
                        onKeyDown={e => onKeyDown(e, c.name)}
                        onFocus={e => e.target.select()}
                      />
                    </div>
                    <div role="cell" className="pick-out ta-r">
                      {r.count > 0 && !r.error ? num(r.count) : '—'}
                    </div>
                  </div>
                )
              })}
            </div>
          </>
        )}
      </section>

      <aside className="extract-side">
        <div className="slip">
          <div className="slip-perf" aria-hidden />
          <div className="slip-label">本次抽取</div>
          <div className="slip-total">
            <span key={total} className="slip-num">
              {num(total)}
            </span>
            <span className="slip-unit">个视频</span>
          </div>
          {active.length > 0 ? (
            <ul className="slip-parts">
              {active.slice(0, 6).map(c => (
                <li key={c.name} className={rows.get(c.name)?.error ? 'is-error' : ''}>
                  <span className="slip-part-name">{c.name}</span>
                  <span className="slip-part-n">{num(rows.get(c.name)!.count)}</span>
                </li>
              ))}
              {active.length > 6 && <li className="dim">另外 {active.length - 6} 个分类</li>}
            </ul>
          ) : (
            <p className="slip-hint">在左侧填写每个分类要抽几个视频。每类随机抽取、不重复，原素材始终保留。</p>
          )}

          <div className="dest">
            <div className="dest-label">
              <span>放入文件夹</span>
              <button className="link dest-root" onClick={chooseOutput} disabled={copying} title={`目标根目录：${state.outputRoot}（点击更换）`}>
                {state.outputRoot.split(/[\\/]/).filter(Boolean).pop()}
              </button>
            </div>
            {state.outputError ? (
              <p className="err-text dest-error">{state.outputError}</p>
            ) : (
              <div className="dest-row">
                <select
                  className="select"
                  value={state.folder}
                  onChange={e => pickFolder(e.target.value)}
                  disabled={copying || state.destinations.length === 0}
                  aria-label="放入文件夹"
                >
                  <option value="" disabled>
                    {state.destinations.length ? '选择文件夹…' : '还没有文件夹'}
                  </option>
                  {state.destinations.map(d => (
                    <option key={d.name} value={d.name}>
                      {d.videos ? `${d.name} · ${d.videos} 个视频` : `${d.name} · 空`}
                    </option>
                  ))}
                </select>
                <button className="btn" onClick={newFolder} disabled={copying} title="新建文件夹">
                  <FolderPlus size={16} />
                  新建
                </button>
              </div>
            )}
            {folder && folder.videos > 0 && (
              <label className={`check-row dest-clean ${clean ? 'is-danger' : ''}`}>
                <input type="checkbox" checked={clean} disabled={copying} onChange={e => setClean(e.target.checked)} />
                复制前清空这里的 {num(folder.videos)} 个旧视频（其他文件保留）
              </label>
            )}
          </div>

          {copying ? (
            <div className="copying" role="status">
              <div className="copying-head">
                <span>
                  正在复制 {num(Math.min((progress?.done ?? 0) + 1, progress?.total ?? 0))} / {num(progress?.total ?? 0)}
                </span>
                <span className="tnum">{pct}%</span>
              </div>
              <div className="bar">
                <div style={{ width: `${pct}%` }} />
              </div>
              <div className="copying-file" title={progress?.name}>
                {progress?.name || '准备中…'}
              </div>
              <div className="copying-foot">
                <span className="dim tnum">
                  {bytes(progress?.bytesDone ?? 0)} / {bytes(progress?.bytesTotal ?? 0)}
                </span>
                <button className="btn btn-sm" onClick={cancelCopy}>
                  <X size={14} />
                  取消
                </button>
              </div>
            </div>
          ) : (
            <button className="btn-draw" onClick={start} disabled={!ready}>
              <span>抽取并复制</span>
              <span className="kbds" aria-label="快捷键 Ctrl + 回车">
                <kbd>Ctrl</kbd>
                <span aria-hidden>+</span>
                <EnterKey className="on-dark" />
              </span>
            </button>
          )}
        </div>

        {result && (
          <div className="result">
            <div className="result-head is-ok">
              <Check size={16} />
              <span>
                已复制 {num(result.items.length)} 个视频 · {bytes(result.items.reduce((s, i) => s + i.size, 0))}
              </span>
              <span className="toolbar-spacer" />
              <span className="dim">{copy.done?.entry.time ? clock(copy.done.entry.time) : ''}</span>
            </div>
            <div className="result-parts">
              放入「{result.folder}」
              {result.removed > 0 && `，清空了 ${num(result.removed)} 个旧视频`}
              {result.avoided > 0 && `，自动避开了 ${num(result.avoided)} 个同名视频`}
            </div>
            <ul className="file-list">
              {groupItems(result.items).map(([cat, items]) => (
                <li key={cat}>
                  <div className="file-cat">
                    {cat} <span className="dim">{items.length}</span>
                  </div>
                  {items.map(it => (
                    <div key={it.name} className="file-name" title={it.name}>
                      {it.name}
                    </div>
                  ))}
                </li>
              ))}
            </ul>
            {result.warning && <p className="warn-text result-more">{result.warning}</p>}
            <div className="result-actions">
              <button ref={doneBtn} className="btn" onClick={() => run(api.openFolder(result.destination))}>
                <FolderOpen size={15} />
                打开文件夹
              </button>
              <button className="btn" onClick={clearResult}>
                完成
              </button>
            </div>
          </div>
        )}

        {failure && (
          <div className="result is-failed">
            <div className="result-head is-warn">
              <TriangleAlert size={16} />
              <span>复制没有完成</span>
            </div>
            <p className="result-error">{failure}</p>
            <div className="result-actions">
              <button className="btn" onClick={clearResult}>
                知道了
              </button>
            </div>
          </div>
        )}
      </aside>
    </div>
  )
}

import { useEffect, useMemo, useRef, useState } from 'react'
import { BookmarkPlus, Check, ChevronDown, ClipboardCopy, Eraser, FileDown, Layers, RefreshCw, Settings2, TriangleAlert } from 'lucide-react'
import { api, runtime } from '../api'
import { useApp } from '../ctx'
import { clock, num, partLabel, stamp } from '../lib/format'
import { draftToPicks, evaluate, picksToDraft, samePicks, splitCombined, type RowState } from '../lib/pick'
import type { Draft, Extracted, Group, ListInfo, Snapshot } from '../types'
import { anchorOf, useMenu, type MenuItem } from '../ui/Menu'
import { EmptyLibrary } from './EmptyLibrary'
import { EnterKey } from '../ui/EnterKey'
import { PresetManager } from '../dialogs/PresetManager'

export interface Section {
  group: Group | null
  lists: ListInfo[]
}

/** 按分组顺序整理分类；未分组的分类排在最后。 */
export function buildSections(snap: Snapshot): Section[] {
  const out: Section[] = snap.groups.map(g => ({ group: g, lists: snap.lists.filter(l => l.groupId === g.id) }))
  const loose = snap.lists.filter(l => !l.groupId)
  if (loose.length || !snap.groups.length) out.push({ group: null, lists: loose })
  return out
}

const PREVIEW_LINES = 300

export function ExtractView() {
  const { snap, setSnap, draft, setDraft, run, toast, info, openImport, prompt, confirm, bumpHistory, go } = useApp()
  const inputs = useRef(new Map<string, HTMLInputElement>())
  const copyBtn = useRef<HTMLButtonElement>(null)
  const [result, setResult] = useState<Extracted | null>(null)
  const [flash, setFlash] = useState(0)
  const [busy, setBusy] = useState(false)
  const [managing, setManaging] = useState(false)
  const menu = useMenu()

  const sections = useMemo(() => buildSections(snap), [snap])
  const rows = useMemo(() => {
    const m = new Map<string, RowState>()
    for (const l of snap.lists) m.set(l.id, evaluate(l, draft[l.id]))
    return m
  }, [snap.lists, draft])

  const active = snap.lists.filter(l => (rows.get(l.id)?.count ?? 0) > 0)
  const errors = snap.lists.filter(l => rows.get(l.id)?.error)
  const total = active.reduce((s, l) => s + (rows.get(l.id)?.lines ?? 0), 0)
  const overLimit = total > info.maxLines
  const picks = useMemo(() => draftToPicks(draft, snap.lists), [draft, snap.lists])
  const activePreset = snap.presets.find(p => samePicks(p.picks, picks))
  const hasInput = Object.values(draft).some(e => e.count.trim() || e.repeat.trim())

  // 回车依次经过的输入框：展开的分组中、有素材的分类。
  const order = sections.flatMap(s => (s.group?.collapsed ? [] : s.lists.filter(l => l.count > 0).map(l => l.id)))

  const focusRow = (id: string | undefined) => {
    if (!id) return
    const el = inputs.current.get(id)
    el?.focus()
    el?.select()
  }

  const setEntry = (id: string, key: 'count' | 'repeat', value: string) =>
    setDraft(d => ({ ...d, [id]: { count: d[id]?.count ?? '', repeat: d[id]?.repeat ?? '', [key]: value } }))

  const normalize = (id: string) =>
    setDraft(d => {
      const e = d[id]
      if (!e) return d
      const next = splitCombined(e)
      return next === e ? d : { ...d, [id]: next }
    })

  const extract = async () => {
    if (busy) return
    if (errors.length) {
      toast(`「${errors[0].name}」：${rows.get(errors[0].id)?.error}`, { kind: 'error' })
      revealRow(errors[0].id)
      return
    }
    if (total === 0) {
      toast('请至少为一个分类填写数量', { kind: 'error' })
      focusRow(order[0])
      return
    }
    if (overLimit) {
      toast(`一次最多输出 ${num(info.maxLines)} 条，请减小数量或重复次数`, { kind: 'error' })
      return
    }
    setDraft(d => {
      const next: Draft = {}
      for (const [k, v] of Object.entries(d)) next[k] = splitCombined(v)
      return next
    })
    const reqs = active.map(l => {
      const r = rows.get(l.id)!
      return { listId: l.id, count: r.count, repeat: r.repeat }
    })
    setBusy(true)
    const res = await run(api.extract(reqs))
    setBusy(false)
    if (!res) return
    setResult(res)
    setFlash(f => f + 1)
    bumpHistory()
    if (!res.copied) {
      toast('抽取完成，但剪贴板正被其他程序占用，没有复制成功。请点“复制”重试', { kind: 'error' })
      return
    }
    if (snap.settings.afterCopy === 'minimize') window.setTimeout(() => runtime.minimise(), 700)
    if (snap.settings.afterCopy === 'quit') window.setTimeout(() => api.confirmClose(), 900)
  }

  // 抽取完成后焦点移到“复制”按钮：再按回车只会重新复制同一批内容，不会抽新的一批。
  useEffect(() => {
    if (flash) copyBtn.current?.focus()
  }, [flash])

  const extractRef = useRef(extract)
  extractRef.current = extract
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !document.querySelector('.modal-layer')) {
        e.preventDefault()
        extractRef.current()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const revealRow = async (id: string) => {
    const list = snap.lists.find(l => l.id === id)
    const g = list && snap.groups.find(x => x.id === list.groupId)
    if (g?.collapsed) {
      setSnap({ ...snap, groups: snap.groups.map(x => (x.id === g.id ? { ...x, collapsed: false } : x)) })
      await api.setGroupCollapsed(g.id, false)
    }
    requestAnimationFrame(() => focusRow(id))
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>, id: string) => {
    const i = order.indexOf(id)
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
      e.preventDefault()
      normalize(id)
      if (e.shiftKey) return focusRow(order[i - 1])
      if (i < order.length - 1) return focusRow(order[i + 1])
      extract()
    } else if (e.key === 'ArrowDown' && i < order.length - 1) {
      e.preventDefault()
      focusRow(order[i + 1])
    } else if (e.key === 'ArrowUp' && i > 0) {
      e.preventDefault()
      focusRow(order[i - 1])
    }
  }

  const toggleGroup = (g: Group) => {
    setSnap({ ...snap, groups: snap.groups.map(x => (x.id === g.id ? { ...x, collapsed: !g.collapsed } : x)) })
    api.setGroupCollapsed(g.id, !g.collapsed).then(setSnap)
  }

  const applyPreset = (id: string) => {
    const p = snap.presets.find(x => x.id === id)
    if (!p) return
    const prev = draft
    setDraft(picksToDraft(p.picks))
    setResult(null)
    toast(`已填入方案「${p.name}」`, { kind: 'ok', action: { label: '撤销', onClick: () => setDraft(prev) } })
  }

  const savePreset = async () => {
    if (!Object.keys(picks).length) {
      toast('请先填写数量，再保存为方案', { kind: 'error' })
      return
    }
    const name = await prompt({
      title: '保存为方案',
      label: '方案名称',
      initial: activePreset?.name ?? '',
      placeholder: '例如：日常发布',
      confirmText: '保存',
      hint: '以后可以在“方案”里一键填入这组数量。',
    })
    if (!name) return
    const existing = snap.presets.find(p => p.name.toLowerCase() === name.toLowerCase() && p.id !== activePreset?.id)
    if (existing) {
      const ok = await confirm({ title: `替换方案「${existing.name}」？`, message: '已有同名方案，保存后会用当前数量替换它。', confirmText: '替换' })
      if (!ok) return
    }
    const res = await run(api.savePreset(name, picks))
    if (res) {
      setSnap(res.snapshot)
      toast(`已保存方案「${name}」`, { kind: 'ok' })
    }
  }

  const clearAll = () => {
    const prev = draft
    setDraft({})
    toast('已清空所有数量', { action: { label: '撤销', onClick: () => setDraft(prev) } })
  }

  const openPresetMenu = (el: HTMLElement) => {
    const items: MenuItem[] = []
    if (snap.presets.length) {
      items.push({ heading: '填入方案' })
      for (const p of snap.presets) {
        const lines = Object.entries(p.picks).reduce(
          (s, [id, v]) => s + (snap.lists.some(l => l.id === id) ? v.count * Math.max(1, v.repeat) : 0),
          0,
        )
        items.push({ label: p.name, hint: `${num(lines)} 条`, checked: p.id === activePreset?.id, icon: p.id === activePreset?.id ? <Check size={15} /> : null, onSelect: () => applyPreset(p.id) })
      }
      items.push({ divider: true })
    }
    items.push({ label: '把当前数量保存为方案…', icon: <BookmarkPlus size={15} />, onSelect: savePreset, disabled: !Object.keys(picks).length })
    if (snap.presets.length) items.push({ label: '管理方案…', icon: <Settings2 size={15} />, onSelect: () => setManaging(true) })
    menu.open(anchorOf(el), items, 260)
  }

  const copyAgain = async () => {
    if (!result) return
    const ok = await run(api.copyText(result.result.text))
    if (ok !== undefined) {
      setResult({ ...result, copied: true })
      toast(`已复制 ${num(result.result.total)} 条`, { kind: 'ok' })
    }
  }

  const saveResult = async () => {
    if (!result) return
    const path = await run(api.saveTextFile(`抽取结果-${stamp(result.time)}`, result.result.text))
    if (path) toast('已保存为 TXT', { kind: 'ok' })
  }

  if (!snap.lists.length) {
    return <EmptyLibrary title="素材库还是空的" lead="先添加素材，再回到这里抽取。" />
  }

  const previewLines = result ? result.result.text.split('\n', PREVIEW_LINES) : []

  return (
    <div className="extract">
      <section className="extract-main" aria-label="填写抽取数量">
        <div className="toolbar">
          <button className="btn btn-ghost" onClick={e => openPresetMenu(e.currentTarget)} aria-haspopup="menu">
            <Layers size={16} />
            {activePreset ? `方案：${activePreset.name}` : '方案'}
            <ChevronDown size={14} className="dim" />
          </button>
          {hasInput && (
            <button className="btn btn-ghost" onClick={clearAll}>
              <Eraser size={16} />
              清空数量
            </button>
          )}
          <span className="toolbar-spacer" />
          <span className="toolbar-hint" title="在数量栏按回车跳到下一项，最后一项按回车直接抽取">
            <EnterKey /> 下一项，最后一项直接抽取
          </span>
        </div>

        <div className="pick-table" role="table">
          <div className="pick-head" role="row">
            <span role="columnheader">分类</span>
            <span role="columnheader" className="ta-c">数量</span>
            <span role="columnheader" className="ta-c">每条重复</span>
            <span role="columnheader" className="ta-r">输出</span>
          </div>
          {sections.map(sec => {
            const sub = sec.lists.reduce((s, l) => s + (rows.get(l.id)?.lines ?? 0), 0)
            const g = sec.group
            const showHeader = g || snap.groups.length > 0
            return (
              <div key={g?.id ?? 'loose'} className={`pick-section ${g ? `c-${g.color}` : 'c-none'}`}>
                {showHeader && (
                  <button
                    className={`pick-group ${g?.collapsed ? 'is-collapsed' : ''}`}
                    onClick={() => g && toggleGroup(g)}
                    disabled={!g}
                    aria-expanded={g ? !g.collapsed : undefined}
                  >
                    {g && <ChevronDown size={15} className="chev" />}
                    <span className="group-dot" />
                    <span className="pick-group-name">{g?.name ?? '未分组'}</span>
                    <span className="pick-group-meta">{sec.lists.length} 个分类</span>
                    <span className="toolbar-spacer" />
                    {sub > 0 && <span className="pick-group-sub">{num(sub)} 条</span>}
                  </button>
                )}
                {!g?.collapsed &&
                  sec.lists.map(l => {
                    const r = rows.get(l.id)!
                    const e = draft[l.id]
                    const empty = l.count === 0
                    return (
                      <div
                        key={l.id}
                        role="row"
                        className={`pick-row ${r.count > 0 ? 'is-active' : ''} ${r.error ? 'is-error' : ''} ${empty ? 'is-empty' : ''}`}
                      >
                        <div className="pick-name" role="cell">
                          <span className="pick-title" title={l.name}>{l.name}</span>
                          {empty ? (
                            <button className="link" onClick={() => openImport({ tab: 'paste', targetId: l.id })}>
                              还没有素材，去添加
                            </button>
                          ) : (
                            <span className="pick-meta">
                              共 {num(l.count)} 条
                              {r.repeat > 1 && l.unique < l.count && ` · 不同内容 ${num(l.unique)} 条`}
                            </span>
                          )}
                          {r.error && <span className="pick-error">{r.error}</span>}
                        </div>
                        <div role="cell" className="ta-c">
                          <input
                            ref={el => {
                              if (el) inputs.current.set(l.id, el)
                              else inputs.current.delete(l.id)
                            }}
                            className="num-input"
                            inputMode="numeric"
                            placeholder="0"
                            disabled={empty}
                            aria-label={`${l.name} 的数量`}
                            value={e?.count ?? ''}
                            onChange={ev => setEntry(l.id, 'count', ev.target.value)}
                            onBlur={() => normalize(l.id)}
                            onKeyDown={ev => onKeyDown(ev, l.id)}
                            onFocus={ev => ev.target.select()}
                          />
                        </div>
                        <div role="cell" className="ta-c">
                          <label className="rep-input">
                            <span aria-hidden>×</span>
                            <input
                              inputMode="numeric"
                              placeholder="1"
                              disabled={empty}
                              aria-label={`${l.name} 每条重复次数`}
                              value={e?.repeat ?? ''}
                              onChange={ev => setEntry(l.id, 'repeat', ev.target.value)}
                              onKeyDown={ev => onKeyDown(ev, l.id)}
                              onFocus={ev => ev.target.select()}
                            />
                          </label>
                        </div>
                        <div role="cell" className="pick-out ta-r">{r.lines > 0 ? num(r.lines) : '—'}</div>
                      </div>
                    )
                  })}
              </div>
            )
          })}
        </div>
      </section>

      <aside className="extract-side">
        <div className="slip">
          <div className="slip-perf" aria-hidden />
          <div className="slip-label">本次抽取</div>
          <div className={`slip-total ${overLimit ? 'is-over' : ''}`}>
            <span key={total} className="slip-num">{num(total)}</span>
            <span className="slip-unit">条</span>
          </div>
          {active.length ? (
            <ul className="slip-parts">
              {active.slice(0, 6).map(l => {
                const r = rows.get(l.id)!
                return (
                  <li key={l.id} className={r.error ? 'is-error' : ''}>
                    <span className="slip-part-name">{l.name}</span>
                    <span className="slip-part-n">{r.repeat > 1 ? `${num(r.count)} × ${r.repeat}` : num(r.count)}</span>
                  </li>
                )
              })}
              {active.length > 6 && <li className="dim">另外 {active.length - 6} 个分类</li>}
            </ul>
          ) : (
            <p className="slip-hint">
              在左侧填写每个分类要抽取的数量。输入 <b>25*4</b> 表示抽 25 条不同内容、每条连续重复 4 次。
            </p>
          )}
          {errors.length > 0 && (
            <p className="slip-warn">
              <TriangleAlert size={15} /> 有 {errors.length} 处数量需要修改
            </p>
          )}
          {overLimit && (
            <p className="slip-warn">
              <TriangleAlert size={15} /> 一次最多输出 {num(info.maxLines)} 条
            </p>
          )}
          <button className="btn-draw" onClick={extract} disabled={busy || total === 0 || errors.length > 0 || overLimit}>
            <span>抽取并复制</span>
            <span className="kbds" aria-label="快捷键 Ctrl + 回车">
              <kbd>Ctrl</kbd>
              <span aria-hidden>+</span>
              <EnterKey className="on-dark" />
            </span>
          </button>
        </div>

        {result && (
          <div key={flash} className="result">
            <div className={`result-head ${result.copied ? 'is-ok' : 'is-warn'}`}>
              {result.copied ? <Check size={16} /> : <TriangleAlert size={16} />}
              <span>{result.copied ? `已复制 ${num(result.result.total)} 条` : '抽取完成，未能复制'}</span>
              <span className="toolbar-spacer" />
              <span className="dim">{clock(result.time)}</span>
            </div>
            <div className="result-parts">{result.result.parts.map(partLabel).join('、')}</div>
            <ol className="result-lines">
              {previewLines.map((line, i) => (
                <li key={i}>{line}</li>
              ))}
            </ol>
            {result.result.total > PREVIEW_LINES && (
              <p className="result-more">仅显示前 {PREVIEW_LINES} 行，复制和保存的是全部 {num(result.result.total)} 行</p>
            )}
            <div className="result-actions">
              <button ref={copyBtn} className="btn" onClick={copyAgain}>
                <ClipboardCopy size={15} />
                复制
              </button>
              <button className="btn" onClick={extract} disabled={busy || total === 0 || errors.length > 0 || overLimit} title="按当前数量重新抽取一批">
                <RefreshCw size={15} />
                重新抽取
              </button>
              <button className="btn" onClick={saveResult}>
                <FileDown size={15} />
                保存为 TXT
              </button>
            </div>
            <button className="link result-history" onClick={() => go('history')}>
              查看抽取记录
            </button>
          </div>
        )}
      </aside>
      {menu.node}
      {managing && <PresetManager onClose={() => setManaging(false)} />}
    </div>
  )
}

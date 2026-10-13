import { useEffect, useMemo, useRef, useState } from 'react'
import {
  BookmarkPlus, Check, ChevronDown, Eraser, FolderOpen, FolderPlus, Layers, Plus, RefreshCw, Settings2, Tags, TriangleAlert, X,
} from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { useCategoryTagging } from '../dialogs/CategoryTagging'
import { PresetManager } from '../dialogs/PresetManager'
import { bytes, clock, num, shortPath } from '../lib/format'
import { validateFolderName } from '../lib/names'
import { draftToPicks, evaluate, modeInfo, picksToDraft, ruleLine, samePicks, splitCombined, usesRule, type RowState } from '../lib/pick'
import type { Category, Item, Tag } from '../types'
import { EnterKey } from '../ui/EnterKey'
import { anchorOf, useMenu, type MenuItem } from '../ui/Menu'
import { ModeSwitch } from './ModeSwitch'
import { Setup } from './Setup'

function groupItems(items: Item[]) {
  const map = new Map<string, Item[]>()
  for (const it of items) map.set(it.category, [...(map.get(it.category) ?? []), it])
  return [...map.entries()]
}

export function ExtractView() {
  const {
    state, setState, refresh, run, prompt, confirm, toast, copy, startCopy, cancelCopy, clearResult, draft, setDraft, repeatMode, setRepeatMode, adding,
  } = useApp()
  const [clean, setClean] = useState(false)
  const [tagFilter, setTagFilter] = useState<string[]>([])
  const [managing, setManaging] = useState(false)
  const tagging = useCategoryTagging()
  const menu = useMenu()
  const inputs = useRef(new Map<string, HTMLInputElement>())
  const doneBtn = useRef<HTMLButtonElement>(null)
  const copying = copy.phase === 'copying'

  const rows = useMemo(() => {
    const m = new Map<string, RowState>()
    for (const c of state.categories) m.set(c.name, evaluate(c, draft[c.name]))
    return m
  }, [state.categories, draft])

  const active = state.categories.filter(c => (rows.get(c.name)?.count ?? 0) > 0)
  const errors = state.categories.filter(c => rows.get(c.name)?.error)
  const total = active.reduce((s, c) => s + (rows.get(c.name)?.files ?? 0), 0)
  const folder = state.destinations.find(d => d.name === state.folder)
  const filled = (name: string) => !!(draft[name]?.count.trim() || draft[name]?.repeat.trim())
  const hasInput = Object.keys(draft).some(filled)
  const picks = useMemo(() => draftToPicks(draft, state.categories), [draft, state.categories])
  const activePreset = state.presets.find(p => p.mode === repeatMode && samePicks(p.picks, picks))

  // ---------- 标签：贴在分类上，点标签只看带这个标签的分类 ----------

  const tagById = useMemo(() => new Map<string, Tag>(state.tags.map(t => [t.id, t])), [state.tags])
  // 只按还在的标签筛选，标签被删除后自动不再算数。
  const activeTags = tagFilter.filter(id => tagById.has(id))
  const filtering = activeTags.length > 0
  const toggleTag = (id: string) => setTagFilter(activeTags.includes(id) ? activeTags.filter(x => x !== id) : [...activeTags, id])
  const tagTotal = (id: string) => state.categories.filter(c => c.tagIds.includes(id)).length
  // 筛选时只留下带所选标签的分类。已经填了数量的分类不隐藏，免得抽到看不见的内容。
  const shown = (c: Category) => !filtering || c.tagIds.some(id => activeTags.includes(id)) || filled(c.name)
  const visible = state.categories.filter(shown)
  // 回车依次经过的输入框：看得见的、有视频可抽的分类。
  const order = visible.filter(c => c.count > 0).map(c => c.name)
  const ready = !!folder && total > 0 && errors.length === 0 && !copying && !adding

  // 换了文件夹或文件夹里没有视频时，取消“清空旧视频”。
  useEffect(() => {
    if (!folder || folder.videos === 0) setClean(false)
  }, [folder?.name, folder?.videos])

  // 复制完成后焦点移到“打开文件夹”，但不滚动右侧：“本次抽取”留在原来的位置。
  useEffect(() => {
    if (copy.phase === 'done' && copy.done?.ok) doneBtn.current?.focus({ preventScroll: true })
  }, [copy.phase])

  const start = () => {
    if (copying) return
    if (adding) return void toast('正在往素材库里添加视频，完成后再抽取', { kind: 'error' })
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
    setDraft(d => Object.fromEntries(Object.entries(d).map(([k, v]) => [k, splitCombined(v)])))
    startCopy(picks, repeatMode, clean).then(() => setClean(false))
  }
  const startRef = useRef(start)
  startRef.current = start
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !document.querySelector('.modal-layer, .license-overlay, .lock-overlay')) {
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

  const setEntry = (name: string, key: 'count' | 'repeat', value: string) =>
    setDraft(d => ({ ...d, [name]: { count: d[name]?.count ?? '', repeat: d[name]?.repeat ?? '', [key]: value } }))

  // 离开数量栏时把 5*3 拆成数量 5、乘数 3。
  const normalize = (name: string) =>
    setDraft(d => {
      const e = d[name]
      if (!e) return d
      const next = splitCombined(e)
      return next === e ? d : { ...d, [name]: next }
    })

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>, name: string) => {
    const i = order.indexOf(name)
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
      e.preventDefault()
      normalize(name)
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

  // ---------- 方案：起了名字保存下来的一组数量、乘数和规则 ----------

  const applyPreset = (id: string) => {
    const p = state.presets.find(x => x.id === id)
    if (!p) return
    const prev = { draft, repeatMode }
    // 只填还在素材根目录里的分类。
    setDraft(picksToDraft(Object.fromEntries(Object.entries(p.picks).filter(([name]) => state.categories.some(c => c.name === name)))))
    setRepeatMode(modeInfo(p.mode).mode)
    const undo = () => {
      setDraft(prev.draft)
      setRepeatMode(prev.repeatMode)
    }
    toast(`已填入方案「${p.name}」`, { kind: 'ok', action: { label: '撤销', onClick: undo } })
  }

  const savePreset = async () => {
    const name = await prompt({
      title: '保存为方案',
      label: '方案名称',
      initial: activePreset?.name ?? '',
      placeholder: '例如：日常发布',
      confirmText: '保存',
      hint: '以后可以在“方案”里一键填入这组数量和乘数，乘数规则也会一起保存。',
    })
    if (!name) return
    const existing = state.presets.find(p => p.name.toLowerCase() === name.toLowerCase())
    if (existing && existing.id !== activePreset?.id) {
      const ok = await confirm({ title: `替换方案「${existing.name}」？`, message: '已有同名方案，保存后会用当前的数量、乘数和规则替换它。', confirmText: '替换' })
      if (!ok) return
    }
    const res = await run(api.savePreset(name, picks, repeatMode))
    if (res) {
      setState(res.state)
      toast(`已保存方案「${name}」`, { kind: 'ok' })
    }
  }

  const openPresetMenu = (el: HTMLElement) => {
    const items: MenuItem[] = []
    if (state.presets.length) {
      items.push({ heading: '填入方案' })
      for (const p of state.presets) {
        const files = Object.entries(p.picks).reduce((n, [name, v]) => n + (state.categories.some(c => c.name === name) ? v.count * Math.max(1, v.repeat) : 0), 0)
        const hint = [`${num(files)} 个`, ...(usesRule(p.picks, p.mode) ? [modeInfo(p.mode).short] : [])].join(' · ')
        items.push({ label: p.name, hint, checked: p.id === activePreset?.id, icon: p.id === activePreset?.id ? <Check size={15} /> : null, onSelect: () => applyPreset(p.id) })
      }
      items.push({ divider: true })
    }
    items.push({ label: '把当前数量保存为方案…', icon: <BookmarkPlus size={15} />, onSelect: savePreset, disabled: !Object.keys(picks).length })
    if (state.presets.length) items.push({ label: '管理方案…', icon: <Settings2 size={15} />, onSelect: () => setManaging(true) })
    menu.open(anchorOf(el), items, 260)
  }

  if (!state.materialRoot || !state.outputRoot) return <Setup />

  const addTag = (c: Category) => (
    <button
      className={`tag-add ${tagging.openName === c.name ? 'is-open' : ''}`}
      aria-haspopup="dialog"
      aria-label={`给「${c.name}」贴标签`}
      title="给这个分类贴标签"
      onClick={e => tagging.open(c, e.currentTarget)}
    >
      <Plus size={13} />
      {!c.tagIds.length && <span>标签</span>}
    </button>
  )

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
              <button className="btn btn-ghost" onClick={e => openPresetMenu(e.currentTarget)} aria-haspopup="menu" disabled={copying}>
                <Layers size={16} />
                {activePreset ? `方案：${activePreset.name}` : '方案'}
                <ChevronDown size={14} className="dim" />
              </button>
              <span className="toolbar-spacer" />
              <ModeSwitch value={repeatMode} onChange={setRepeatMode} disabled={copying} />
            </div>
            <div className="pick-table" role="table">
              {state.tags.length > 0 && (
                <div className="tag-bar" role="group" aria-label="按标签筛选分类">
                  <span className="tag-bar-label" title="点标签，只显示带这个标签的分类">
                    <Tags size={16} />
                    <span>标签</span>
                  </span>
                  <button className={`chip ${filtering ? '' : 'is-on'}`} aria-pressed={!filtering} onClick={() => setTagFilter([])} title="显示所有分类">
                    全部
                  </button>
                  {state.tags.map(t => {
                    const on = activeTags.includes(t.id)
                    return (
                      <button
                        key={t.id}
                        className={`chip c-${t.color} ${on ? 'is-on' : ''}`}
                        aria-pressed={on}
                        onClick={() => toggleTag(t.id)}
                        title={on ? `不再按「${t.name}」筛选` : `只显示带「${t.name}」标签的分类`}
                      >
                        <i className="tag-dot" aria-hidden />
                        <span className="chip-name">{t.name}</span>
                        <span className="chip-n">{num(tagTotal(t.id))}</span>
                      </button>
                    )
                  })}
                  {filtering && (
                    <span className="tag-bar-note">
                      显示 {num(visible.length)} / {num(state.categories.length)} 个分类
                    </span>
                  )}
                  <span className="toolbar-spacer" />
                  <button className="icon-btn sm" onClick={tagging.manage} aria-label="管理标签" title="管理标签：新建、改名、换颜色、删除">
                    <Settings2 size={15} />
                  </button>
                </div>
              )}
              <div className="pick-head" role="row">
                <span role="columnheader">分类</span>
                <span role="columnheader" className="ta-c">数量</span>
                <span role="columnheader" className="ta-c">乘数</span>
                <span role="columnheader" className="ta-r pick-col-out">本次</span>
              </div>
              {visible.map(c => {
                const r = rows.get(c.name)!
                const empty = c.count === 0
                return (
                  <div
                    key={c.name}
                    role="row"
                    className={`pick-row c-none ${r.count > 0 ? 'is-active' : ''} ${r.error ? 'is-error' : ''} ${empty ? 'is-empty' : ''}`}
                  >
                    <div className="pick-name" role="cell">
                      <span className="pick-title-row">
                        <span className="pick-title" title={c.name}>
                          {c.name}
                        </span>
                        {c.tagIds.map((id, i) => {
                          const t = tagById.get(id)
                          if (!t) return null
                          const on = activeTags.includes(id)
                          const pill = (
                            <button
                              key={id}
                              className={`tag-pill is-button c-${t.color} ${on ? 'is-hit' : ''}`}
                              aria-pressed={on}
                              onClick={() => toggleTag(id)}
                              title={on ? `不再按「${t.name}」筛选` : `点一下，只显示带「${t.name}」标签的分类`}
                            >
                              <i className="tag-dot" aria-hidden />
                              <span className="tag-pill-name">{t.name}</span>
                            </button>
                          )
                          // 最后一个标签和“贴标签”的按钮连在一起换行，按钮不会单独掉到下一行。
                          return i < c.tagIds.length - 1 ? pill : (
                            <span key={id} className="tag-tail">
                              {pill}
                              {addTag(c)}
                            </span>
                          )
                        })}
                        {!c.tagIds.length && addTag(c)}
                      </span>
                      <span className="pick-meta">
                        {empty ? '没有视频' : `共 ${num(c.count)} 个视频 · ${bytes(c.bytes)}`}
                        {r.repeat > 1 && r.files > 0 && ` · ${ruleLine(repeatMode, r.count, r.repeat)}`}
                      </span>
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
                        value={draft[c.name]?.count ?? ''}
                        onChange={e => setEntry(c.name, 'count', e.target.value)}
                        onBlur={() => normalize(c.name)}
                        onKeyDown={e => onKeyDown(e, c.name)}
                        onFocus={e => e.target.select()}
                      />
                    </div>
                    <div role="cell" className="ta-c">
                      <label className="rep-input">
                        <span aria-hidden>×</span>
                        <input
                          inputMode="numeric"
                          placeholder="1"
                          disabled={empty || copying}
                          aria-label={`${c.name} 的乘数`}
                          value={draft[c.name]?.repeat ?? ''}
                          onChange={e => setEntry(c.name, 'repeat', e.target.value)}
                          onKeyDown={e => onKeyDown(e, c.name)}
                          onFocus={e => e.target.select()}
                        />
                      </label>
                    </div>
                    <div role="cell" className="pick-out pick-col-out ta-r">
                      {r.files > 0 ? num(r.files) : '—'}
                    </div>
                  </div>
                )
              })}
              {filtering && visible.length === 0 && (
                <div className="pick-empty">
                  还没有分类带所选的标签。点分类名称旁边的“标签”可以贴上。
                  <button className="link" onClick={() => setTagFilter([])}>
                    显示所有分类
                  </button>
                </div>
              )}
            </div>
          </>
        )}
      </section>

      <aside className="extract-side">
        <div className="slip">
          <div className="slip-perf" aria-hidden />
          <div className="slip-head">
            <span className="slip-label">本次抽取</span>
            {hasInput && (
              <button className="slip-clear" onClick={clearAll} disabled={copying} title="清空所有分类填写的数量">
                <Eraser size={14} />
                清空数量
              </button>
            )}
          </div>
          <div className="slip-total">
            <span key={total} className="slip-num">
              {num(total)}
            </span>
            <span className="slip-unit">个视频</span>
          </div>
          {usesRule(picks, repeatMode) && (
            <p className="slip-rule">
              <span className="slip-mode">{modeInfo(repeatMode).label}</span>
            </p>
          )}
          {active.length > 0 ? (
            <ul className="slip-parts">
              {active.slice(0, 6).map(c => (
                <li key={c.name} className={rows.get(c.name)?.error ? 'is-error' : ''}>
                  <span className="slip-part-name">{c.name}</span>
                  <span className="slip-part-n">
                    {rows.get(c.name)!.repeat > 1 ? `${num(rows.get(c.name)!.count)} × ${rows.get(c.name)!.repeat}` : num(rows.get(c.name)!.count)}
                  </span>
                </li>
              ))}
              {active.length > 6 && <li className="dim">另外 {active.length - 6} 个分类</li>}
            </ul>
          ) : (
            <p className="slip-hint">
              在左侧填写每个分类要抽几个视频，按回车跳到下一项，最后一项按回车直接抽取。输入 <b>5*3</b> 表示数量 5、乘数 3。原素材始终保留。
            </p>
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
                    <div key={it.name} className="file-name" title={it.origin ? `${it.name}（原名 ${it.origin}）` : it.name}>
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
      {tagging.node}
      {menu.node}
      {managing && <PresetManager onClose={() => setManaging(false)} />}
    </div>
  )
}

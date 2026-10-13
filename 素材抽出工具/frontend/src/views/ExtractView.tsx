import { useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowDown, ArrowUp, BookmarkPlus, Check, ChevronDown, ClipboardCopy, Eraser, FileDown, FolderInput, GripVertical, Layers, Plus, RefreshCw, Settings2,
  Tags, TriangleAlert,
} from 'lucide-react'
import { api, errorText, runtime } from '../api'
import { useApp } from '../ctx'
import { clock, num, partLabel, stamp } from '../lib/format'
import { draftToPicks, evaluate, modeInfo, picksMode, picksToDraft, ruleLine, ruleNote, samePicks, splitCombined, usesRule, type RowState } from '../lib/pick'
import { buildSections, useReorder } from '../lib/reorder'
import { useTagMap, useTagTotals } from '../lib/tags'
import type { Draft, Extracted, Group, ListInfo, PickRequest } from '../types'
import { anchorOf, useMenu, type MenuItem } from '../ui/Menu'
import { EmptyLibrary } from './EmptyLibrary'
import { EnterKey } from '../ui/EnterKey'
import { useListTagging } from '../dialogs/ListTagging'
import { PresetManager } from '../dialogs/PresetManager'
import { TagPicker } from '../dialogs/TagManager'
import { ModeSwitch } from './ModeSwitch'

const PREVIEW_LINES = 300
/** 标签栏里直接摆出来的标签个数，更多的收进“更多”。 */
const BAR_TAGS = 8

type Kind = 'list' | 'group'

export function ExtractView() {
  const {
    snap, setSnap, draft, setDraft, repeatMode, setRepeatMode, tagFilter, setTagFilter, lastExtraction, setLastExtraction,
    run, toast, info, openImport, prompt, confirm, bumpHistory, go,
  } = useApp()
  const inputs = useRef(new Map<string, HTMLInputElement>())
  // 调整顺序之后要取回焦点的元素：手柄、输入框、分组标题。
  const anchors = useRef(new Map<string, HTMLElement>())
  const copyBtn = useRef<HTMLButtonElement>(null)
  const refocusCopy = useRef(false)
  const working = useRef(false)
  const [flash, setFlash] = useState(0)
  const [busy, setBusy] = useState(false)
  const [managing, setManaging] = useState(false)
  const [choosingTags, setChoosingTags] = useState(false)
  const menu = useMenu()
  const tagging = useListTagging()
  const reorder = useReorder()

  const sections = useMemo(() => buildSections(snap), [snap])

  // ---------- 标签：贴在分类上，点标签只看带这个标签的分类 ----------

  const tagById = useTagMap()
  const tagTotals = useTagTotals()
  // 只按还在的标签筛选，标签被删除后自动不再算数。
  const activeTags = tagFilter.filter(id => tagById.has(id))
  const filtering = activeTags.length > 0
  // 标签栏：前几个标签直接摆出来，正在筛选的标签即使排在后面也摆出来。
  const barTags = snap.tags.filter((t, i) => i < BAR_TAGS || activeTags.includes(t.id))
  const toggleTag = (id: string) => setTagFilter(activeTags.includes(id) ? activeTags.filter(x => x !== id) : [...activeTags, id])

  const rows = useMemo(() => {
    const m = new Map<string, RowState>()
    for (const l of snap.lists) m.set(l.id, evaluate(l, draft[l.id], repeatMode))
    return m
  }, [snap.lists, draft, repeatMode])

  const active = snap.lists.filter(l => (rows.get(l.id)?.count ?? 0) > 0)
  const errors = snap.lists.filter(l => rows.get(l.id)?.error)
  const total = active.reduce((s, l) => s + (rows.get(l.id)?.lines ?? 0), 0)
  const overLimit = total > info.maxLines
  const picks = useMemo(() => draftToPicks(draft, snap.lists, repeatMode), [draft, snap.lists, repeatMode])
  const activePreset = snap.presets.find(p => samePicks(p.picks, picks))
  const filled = (id: string) => !!(draft[id]?.count.trim() || draft[id]?.repeat.trim())
  const hasInput = Object.values(draft).some(e => e.count.trim() || e.repeat.trim())
  const ready = total > 0 && !errors.length && !overLimit
  // 筛选时只留下带所选标签的分类。已经填了数量的分类不隐藏，免得抽到看不见的内容。
  const shown = (l: ListInfo) => !filtering || l.tagIds.some(id => activeTags.includes(id)) || filled(l.id)
  const shownCount = snap.lists.filter(shown).length

  // 回车依次经过的输入框：看得见的、有文本可抽的分类。筛选时折叠的分组也会展开。
  const order = sections.flatMap(s => (s.group?.collapsed && !filtering ? [] : s.lists.filter(l => l.count > 0 && shown(l)).map(l => l.id)))

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

  // ---------- 抽取与复制 ----------

  /** 复制成功后清空数量和乘数。记住的数量也立即清空，赶在最小化或退出程序之前完成。 */
  const clearInputs = async () => {
    setDraft({})
    await api.saveDraft({}).catch(() => {})
  }

  /** 抽取、重新抽取、手动复制共用一把锁：同一时间只进行一个，做完（包括失败）才放开。 */
  const exclusive = async <T,>(job: () => Promise<T>): Promise<T | undefined> => {
    if (working.current) return undefined
    working.current = true
    setBusy(true)
    try {
      return await job()
    } finally {
      working.current = false
      setBusy(false)
    }
  }

  const perform = async (reqs: PickRequest[], again: boolean) => {
    const extracted = await exclusive(async () => {
      let res: Extracted
      try {
        res = await api.extract(reqs)
      } catch (e) {
        const msg = errorText(e)
        toast(again ? `上一批的数量和规则已经不能使用（${msg}）。请在左边重新填写，再点“抽取并复制”` : msg, { kind: 'error' })
        return false
      }
      setLastExtraction({ response: res, requests: reqs })
      bumpHistory()
      if (!res.copied) {
        toast('抽取完成，但剪贴板正被其他程序占用，没有复制成功。请点“复制”重试', { kind: 'error' })
        return true
      }
      // 数量清空并保存完成之后才放开。
      await clearInputs()
      if (snap.settings.afterCopy === 'minimize') window.setTimeout(() => runtime.minimise(), 700)
      if (snap.settings.afterCopy === 'quit') window.setTimeout(() => api.confirmClose(), 900)
      return true
    })
    // 放开之后“复制”按钮才可用，这时再把焦点移过去。
    if (extracted) setFlash(f => f + 1)
  }

  const extract = async () => {
    if (working.current) return
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
      toast(`一次最多输出 ${num(info.maxLines)} 条，请减小数量或乘数`, { kind: 'error' })
      return
    }
    setDraft(d => {
      const next: Draft = {}
      for (const [k, v] of Object.entries(d)) next[k] = splitCombined(v)
      return next
    })
    const reqs = active.map(l => {
      const r = rows.get(l.id)!
      return { listId: l.id, count: r.count, repeat: r.repeat, mode: repeatMode }
    })
    await perform(reqs, false)
  }

  /** 按上一批实际提交的数量、乘数和规则再抽一批，与现在填写的内容无关；参数是否还可用由后端检查。 */
  const extractAgain = () => lastExtraction && perform(lastExtraction.requests, true)

  // 抽取完成后焦点移到“复制”按钮：再按回车只会重新复制同一批内容，不会抽新的一批。
  // 只移焦点，不滚动：右侧停在原来的位置，“本次抽取”和抽取按钮不会被顶出去。
  useEffect(() => {
    if (flash) copyBtn.current?.focus({ preventScroll: true })
  }, [flash])

  const extractRef = useRef(extract)
  extractRef.current = extract
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !document.querySelector('.modal-layer, .license-overlay, .lock-overlay')) {
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

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>, id: string, part: 'count' | 'repeat') => {
    const i = order.indexOf(id)
    if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      // Alt + 上下方向键：移动这一行，焦点留在原来的输入框里。
      e.preventDefault()
      if (!filtering) move('list', id, e.key === 'ArrowUp' ? -1 : 1, `${part}:${id}`)
    } else if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
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

  const copyAgain = async () => {
    if (!lastExtraction || working.current) return
    const { response } = lastExtraction
    // “复制”按钮在复制期间是禁用的，会丢掉焦点；做完后放回去，再按回车还是复制同一批。
    refocusCopy.current = document.activeElement === copyBtn.current
    await exclusive(async () => {
      // 复制失败时 run 已经提示，填写的数量原样保留。
      if (!(await run(api.copyText(response.result.text)))) return
      setLastExtraction(cur => (cur?.response.historyId === response.historyId ? { ...cur, response: { ...cur.response, copied: true } } : cur))
      await clearInputs()
      toast(`已复制 ${num(response.result.total)} 条`, { kind: 'ok' })
    })
  }
  useEffect(() => {
    if (busy || !refocusCopy.current) return
    refocusCopy.current = false
    copyBtn.current?.focus({ preventScroll: true })
  }, [busy])

  const saveResult = async () => {
    if (!lastExtraction) return
    const { response } = lastExtraction
    const path = await run(api.saveTextFile(`抽取结果-${stamp(response.time)}`, response.result.text))
    if (path) toast('已保存为 TXT', { kind: 'ok' })
  }

  // ---------- 方案 ----------

  const applyPreset = (id: string) => {
    const p = snap.presets.find(x => x.id === id)
    if (!p) return
    const prev = { draft, repeatMode }
    setDraft(picksToDraft(p.picks))
    setRepeatMode(picksMode(Object.values(p.picks)))
    const undo = () => {
      setDraft(prev.draft)
      setRepeatMode(prev.repeatMode)
    }
    toast(`已填入方案「${p.name}」`, { kind: 'ok', action: { label: '撤销', onClick: undo } })
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
      hint: '以后可以在“方案”里一键填入这组数量，乘数规则也会一起保存。',
    })
    if (!name) return
    const existing = snap.presets.find(p => p.name.toLowerCase() === name.toLowerCase() && p.id !== activePreset?.id)
    if (existing) {
      const ok = await confirm({ title: `替换方案「${existing.name}」？`, message: '已有同名方案，保存后会用当前的数量和规则替换它。', confirmText: '替换' })
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
        const all = Object.entries(p.picks)
        const lines = all.reduce((s, [id, v]) => s + (snap.lists.some(l => l.id === id) ? v.count * Math.max(1, v.repeat) : 0), 0)
        const mode = picksMode(all.map(([, v]) => v))
        const hint = [`${num(lines)} 条`, ...(mode !== 'consecutive' ? [modeInfo(mode).short] : [])].join(' · ')
        items.push({ label: p.name, hint, checked: p.id === activePreset?.id, icon: p.id === activePreset?.id ? <Check size={16} /> : null, onSelect: () => applyPreset(p.id) })
      }
      items.push({ divider: true })
    }
    items.push({ label: '把当前数量保存为方案…', icon: <BookmarkPlus size={16} />, onSelect: savePreset, disabled: !Object.keys(picks).length })
    if (snap.presets.length) items.push({ label: '管理方案…', icon: <Settings2 size={16} />, onSelect: () => setManaging(true) })
    menu.open(anchorOf(el), items, 280)
  }

  // ---------- 调整顺序：拖动手柄，或用手柄菜单、Alt + 方向键 ----------

  const anchor = (key: string) => (el: HTMLElement | null) => {
    if (el) anchors.current.set(key, el)
    else anchors.current.delete(key)
  }

  // 跨分组移动时这一行会被重新创建，等界面更新完再把焦点放回去。
  const refocus = (key: string) => requestAnimationFrame(() => requestAnimationFrame(() => anchors.current.get(key)?.focus()))

  /** 移动一格。focus 是移动后取回焦点的位置，默认是它的手柄。 */
  const move = async (kind: Kind, id: string, delta: -1 | 1, focus = `grip:${kind}:${id}`) => {
    await (kind === 'list' ? reorder.moveListBy(id, delta) : reorder.moveGroupBy(id, delta))
    refocus(focus)
  }

  const openMoveMenu = (el: HTMLElement, kind: Kind, id: string) => {
    const items: MenuItem[] = []
    if (kind === 'group') {
      const i = snap.groups.findIndex(g => g.id === id)
      items.push(
        { label: '上移', icon: <ArrowUp size={16} />, hint: 'Alt+↑', disabled: i <= 0, onSelect: () => move(kind, id, -1) },
        { label: '下移', icon: <ArrowDown size={16} />, hint: 'Alt+↓', disabled: i === snap.groups.length - 1, onSelect: () => move(kind, id, 1) },
      )
    } else {
      const flat = sections.flatMap(s => s.lists)
      const l = flat.find(x => x.id === id)
      if (!l) return
      const toGroup = async (groupId: string) => {
        await reorder.moveListTo(id, groupId)
        refocus(`grip:list:${id}`)
      }
      // 分组里的第一个还能上移到前一个分组，所以只有头尾两个分组的边缘才不能再移。
      const tail = sections[sections.length - 1].lists
      const first = sections[0].lists[0]?.id === id
      const last = tail[tail.length - 1]?.id === id
      items.push(
        { label: '上移', icon: <ArrowUp size={16} />, hint: 'Alt+↑', disabled: first, onSelect: () => move(kind, id, -1) },
        { label: '下移', icon: <ArrowDown size={16} />, hint: 'Alt+↓', disabled: last, onSelect: () => move(kind, id, 1) },
      )
      if (snap.groups.length) {
        items.push({ divider: true }, { heading: '移动到分组' })
        for (const g of snap.groups) {
          const here = l.groupId === g.id
          items.push({ label: g.name, icon: here ? <Check size={16} /> : <FolderInput size={16} />, disabled: here, onSelect: () => toGroup(g.id) })
        }
        items.push({ label: '未分组', icon: !l.groupId ? <Check size={16} /> : <FolderInput size={16} />, disabled: !l.groupId, onSelect: () => toGroup('') })
      }
    }
    menu.open(anchorOf(el), items, 200)
  }

  const dragStart = (e: React.DragEvent<HTMLElement>, kind: Kind, id: string) => {
    reorder.start(e, kind, id)
    // 拖动时带着整行走，而不是只有一个小手柄。
    const row = e.currentTarget.closest<HTMLElement>('.pick-row, .pick-group')
    if (row && !e.isDefaultPrevented()) {
      const r = row.getBoundingClientRect()
      e.dataTransfer.setDragImage(row, e.clientX - r.left, e.clientY - r.top)
    }
  }

  // 只有手柄可以拖动，输入框和按钮不会意外开始拖动。手柄不占用 Tab 顺序，免得打断逐行填写数量；
  // 键盘上在这一行的输入框（或分组标题）里按 Alt + 上下方向键移动。
  const grip = (kind: Kind, id: string, name: string) => {
    const what = kind === 'list' ? '分类' : '分组'
    return (
      <span
        ref={anchor(`grip:${kind}:${id}`)}
        className="grip"
        role="button"
        tabIndex={-1}
        draggable={!reorder.pending && !filtering}
        aria-label={`移动${what}「${name}」`}
        aria-disabled={filtering}
        title={filtering ? '正在按标签筛选，点“全部”显示所有分类后才能调整顺序' : `按住拖动可以调整${what}的顺序；点一下可以选择上移、下移`}
        onDragStart={e => dragStart(e, kind, id)}
        onDragEnd={reorder.end}
        onClick={e => !filtering && openMoveMenu(e.currentTarget, kind, id)}
        onKeyDown={e => {
          if (filtering || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return
          e.preventDefault()
          move(kind, id, e.key === 'ArrowUp' ? -1 : 1)
        }}
      >
        <GripVertical size={20} />
      </span>
    )
  }

  const addTag = (l: ListInfo) => (
    <button
      className={`tag-add ${tagging.openId === l.id ? 'is-open' : ''}`}
      aria-haspopup="dialog"
      aria-label={`给「${l.name}」贴标签`}
      title="给这个分类贴标签"
      onClick={ev => tagging.open(l, ev.currentTarget)}
    >
      <Plus size={14} />
      {!l.tagIds.length && <span>标签</span>}
    </button>
  )

  const last = lastExtraction?.response
  const previewLines = useMemo(() => (last ? last.result.text.split('\n', PREVIEW_LINES) : []), [last])

  if (!snap.lists.length) {
    return <EmptyLibrary title="素材库还是空的" lead="先添加素材，再回到这里抽取。" />
  }

  const mode = modeInfo(repeatMode)
  const draggingList = reorder.active?.kind === 'list'
  // 乘数规则只在用到乘数（或分轮随机）时才影响结果，这时才值得在签条上写出来。
  const ruled = usesRule(Object.values(picks))
  const lastNote = last ? ruleNote(last.result.parts) : ''

  return (
    <div className="extract">
      <section className="extract-main" aria-label="填写抽取数量">
        <div className="toolbar">
          <button className="btn btn-ghost" onClick={e => openPresetMenu(e.currentTarget)} aria-haspopup="menu">
            <Layers size={18} />
            {activePreset ? `方案：${activePreset.name}` : '方案'}
            <ChevronDown size={16} className="dim" />
          </button>
          <span className="toolbar-spacer" />
          <ModeSwitch value={repeatMode} onChange={setRepeatMode} />
        </div>

        <div
          className={`pick-table ${reorder.active ? `is-moving-${reorder.active.kind}` : ''}`}
          role="table"
          onDragLeave={reorder.leave}
        >
          {snap.tags.length > 0 && (
            <div className="tag-bar" role="group" aria-label="按标签筛选分类">
              <span className="tag-bar-label" title="点标签，只显示带这个标签的分类">
                <Tags size={18} />
                <span>标签</span>
              </span>
              <button className={`chip ${filtering ? '' : 'is-on'}`} aria-pressed={!filtering} onClick={() => setTagFilter([])} title="显示所有分类">
                全部
              </button>
              {barTags.map(t => {
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
                    <span className="chip-n">{num(tagTotals.get(t.id) ?? 0)}</span>
                  </button>
                )
              })}
              {snap.tags.length > barTags.length && (
                <button className="chip" onClick={() => setChoosingTags(true)} aria-haspopup="dialog">
                  更多 {snap.tags.length - barTags.length} 个…
                </button>
              )}
              {filtering && (
                <span className="tag-bar-note">
                  显示 {num(shownCount)} / {num(snap.lists.length)} 个分类
                </span>
              )}
            </div>
          )}
          <div className="pick-head" role="row">
            <span aria-hidden />
            <span role="columnheader">分类</span>
            <span role="columnheader" className="ta-c">数量</span>
            <span role="columnheader" className="ta-c">乘数</span>
            <span role="columnheader" className="ta-r pick-col-out">输出</span>
          </div>
          {sections.map(sec => {
            const sub = sec.lists.reduce((s, l) => s + (rows.get(l.id)?.lines ?? 0), 0)
            const g = sec.group
            const gid = g?.id ?? ''
            const lists = sec.lists.filter(shown)
            const showHeader = g || snap.groups.length > 0
            // 筛选时不显示没有匹配分类的分组，折叠的分组也展开，免得漏看。
            if (filtering && !lists.length) return null
            const collapsed = !!g?.collapsed && !filtering
            // “未分组”里没有分类时，只在拖动分类期间出现，作为把分类移出分组的落点。
            if (!g && showHeader && !sec.lists.length && !draggingList) return null
            const into = !!reorder.dropClass('group-into', gid)
            const moving = reorder.active?.kind === 'group' && reorder.active.id === gid
            return (
              <div
                key={gid || 'loose'}
                className={`pick-section ${g ? `c-${g.color}` : 'c-none'} ${into ? 'drop-into' : ''} ${g ? reorder.dropClass('group', gid) : ''} ${moving ? 'is-moving' : ''}`}
                onDragEnter={e => reorder.overGroup(e, gid)}
                onDragOver={e => reorder.overGroup(e, gid)}
                onDrop={reorder.onDrop}
              >
                {showHeader && (
                  <div className={`pick-group ${collapsed ? 'is-collapsed' : ''}`}>
                    {g ? grip('group', g.id, g.name) : <span className="grip-space" />}
                    <button
                      ref={anchor(`toggle:${gid}`)}
                      className="pick-group-toggle"
                      onClick={() => g && toggleGroup(g)}
                      onKeyDown={e => {
                        if (!g || filtering || !e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return
                        e.preventDefault()
                        move('group', g.id, e.key === 'ArrowUp' ? -1 : 1, `toggle:${g.id}`)
                      }}
                      disabled={!g || filtering}
                      aria-expanded={g ? !collapsed : undefined}
                    >
                      {g && <ChevronDown size={18} className="chev" />}
                      <span className="group-dot" />
                      <span className="pick-group-name">{g?.name ?? '未分组'}</span>
                      {into ? (
                        <span className="pick-group-drop">松开鼠标，放到{g ? '这个分组' : '“未分组”'}的最后</span>
                      ) : (
                        <span className="pick-group-meta">{lists.length} 个分类</span>
                      )}
                    </button>
                    {sub > 0 && <span className="pick-group-sub">{num(sub)} 条</span>}
                  </div>
                )}
                {!collapsed &&
                  lists.map(l => {
                    const r = rows.get(l.id)!
                    const e = draft[l.id]
                    const empty = l.count === 0
                    const movingRow = draggingList && reorder.active?.id === l.id
                    return (
                      <div
                        key={l.id}
                        role="row"
                        className={`pick-row ${r.count > 0 ? 'is-active' : ''} ${r.error ? 'is-error' : ''} ${empty ? 'is-empty' : ''} ${movingRow ? 'is-moving' : ''} ${reorder.dropClass('list', l.id)}`}
                        onDragEnter={ev => reorder.overList(ev, l)}
                        onDragOver={ev => reorder.overList(ev, l)}
                        onDrop={reorder.onDrop}
                      >
                        <div role="cell">{grip('list', l.id, l.name)}</div>
                        <div className="pick-name" role="cell">
                          <span className="pick-title-row">
                            <span className="pick-title" title={l.name}>{l.name}</span>
                            {l.tagIds.map((id, i) => {
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
                              return i < l.tagIds.length - 1 ? pill : (
                                <span key={id} className="tag-tail">
                                  {pill}
                                  {addTag(l)}
                                </span>
                              )
                            })}
                            {!l.tagIds.length && addTag(l)}
                          </span>
                          {empty ? (
                            <button className="link" onClick={() => openImport({ tab: 'paste', targetId: l.id })}>
                              还没有素材，去添加
                            </button>
                          ) : (
                            <span className="pick-meta">
                              共 {num(l.count)} 条
                              {(r.repeat > 1 || repeatMode === 'random') && l.unique < l.count && ` · 不同内容 ${num(l.unique)} 条`}
                              {r.repeat > 1 && r.lines > 0 && ` · ${ruleLine(repeatMode, r.count, r.repeat)}`}
                            </span>
                          )}
                          {r.error && <span className="pick-error">{r.error}</span>}
                        </div>
                        <div role="cell" className="ta-c">
                          <input
                            ref={el => {
                              anchor(`count:${l.id}`)(el)
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
                            onKeyDown={ev => onKeyDown(ev, l.id, 'count')}
                            onFocus={ev => ev.target.select()}
                          />
                        </div>
                        <div role="cell" className="ta-c">
                          <label className="rep-input">
                            <span aria-hidden>×</span>
                            <input
                              ref={anchor(`repeat:${l.id}`)}
                              inputMode="numeric"
                              placeholder="1"
                              disabled={empty}
                              aria-label={`${l.name} 的乘数`}
                              value={e?.repeat ?? ''}
                              onChange={ev => setEntry(l.id, 'repeat', ev.target.value)}
                              onKeyDown={ev => onKeyDown(ev, l.id, 'repeat')}
                              onFocus={ev => ev.target.select()}
                            />
                          </label>
                        </div>
                        <div role="cell" className="pick-out pick-col-out ta-r">{r.lines > 0 ? num(r.lines) : '—'}</div>
                      </div>
                    )
                  })}
                {!collapsed && showHeader && sec.lists.length === 0 && (
                  <div className="pick-empty">
                    {draggingList ? '松开鼠标，把分类放到这里' : '这个分组里还没有分类，可以把分类拖进来'}
                  </div>
                )}
              </div>
            )
          })}
          {filtering && shownCount === 0 && (
            <div className="pick-empty">
              还没有分类带所选的标签。点分类名称旁边的“标签”可以贴上。
              <button className="link" onClick={() => setTagFilter([])}>
                显示所有分类
              </button>
            </div>
          )}
        </div>
      </section>

      <aside className="extract-side">
        <div className="slip">
          <div className="slip-perf" aria-hidden />
          <div className="slip-head">
            <span className="slip-label">本次抽取</span>
            {hasInput && (
              <button className="slip-clear" onClick={clearAll} title="清空所有分类填写的数量和乘数">
                <Eraser size={16} />
                清空数量
              </button>
            )}
          </div>
          <div className={`slip-total ${overLimit ? 'is-over' : ''}`}>
            <span key={total} className="slip-num">{num(total)}</span>
            <span className="slip-unit">条</span>
          </div>
          {ruled && (
            <p className="slip-rule">
              <span className="slip-mode">{mode.label}</span>
            </p>
          )}
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
              在左侧填写每个分类要抽取的数量，按回车跳到下一项，最后一项按回车直接抽取。输入 <b>25*4</b> 表示数量 25、乘数 4。
            </p>
          )}
          {errors.length > 0 && (
            <p className="slip-warn">
              <TriangleAlert size={18} /> 有 {errors.length} 处数量需要修改
            </p>
          )}
          {overLimit && (
            <p className="slip-warn">
              <TriangleAlert size={18} /> 一次最多输出 {num(info.maxLines)} 条
            </p>
          )}
          <button className="btn-draw" onClick={extract} disabled={busy || !ready}>
            <span>抽取并复制</span>
            <span className="kbds" aria-label="快捷键 Ctrl + 回车">
              <kbd>Ctrl</kbd>
              <span aria-hidden>+</span>
              <EnterKey className="on-dark" />
            </span>
          </button>
        </div>

        {last && (
          <div key={flash} className="result">
            <div className={`result-head ${last.copied ? 'is-ok' : 'is-warn'}`}>
              {last.copied ? <Check size={18} /> : <TriangleAlert size={18} />}
              <span>{last.copied ? `已复制 ${num(last.result.total)} 条` : '抽取完成，未能复制'}</span>
              <span className="toolbar-spacer" />
              <span className="dim">{clock(last.time)}</span>
            </div>
            <div className="result-parts">{last.result.parts.map(partLabel).join('、')}</div>
            {lastNote && (
              <div className="result-rule">
                <span className="slip-mode">{lastNote}</span>
              </div>
            )}
            <ol className="result-lines">
              {previewLines.map((line, i) => (
                <li key={i}>{line}</li>
              ))}
            </ol>
            {last.result.total > PREVIEW_LINES && (
              <p className="result-more">仅显示前 {PREVIEW_LINES} 行，复制和保存的是全部 {num(last.result.total)} 行</p>
            )}
            <div className="result-actions">
              <button ref={copyBtn} className={`btn ${last.copied ? '' : 'btn-primary'}`} onClick={copyAgain} disabled={busy} title="再复制一次这一批内容">
                <ClipboardCopy size={18} />
                复制
              </button>
              <button
                className="btn"
                onClick={extractAgain}
                disabled={busy}
                title="按这一批的数量、乘数和规则再抽新的一批，与左边现在填写的内容无关"
              >
                <RefreshCw size={18} />
                重新抽取
              </button>
              <button className="btn" onClick={saveResult}>
                <FileDown size={18} />
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
      {tagging.node}
      {choosingTags && (
        <TagPicker
          title="按标签找分类"
          help="选一个或多个标签，只显示带这些标签的分类。选了多个时，带有其中任意一个标签的分类都会显示。"
          initial={activeTags}
          noneLabel="所有分类"
          onClose={() => setChoosingTags(false)}
          onApply={ids => {
            setTagFilter(ids)
            setChoosingTags(false)
          }}
        />
      )}
    </div>
  )
}

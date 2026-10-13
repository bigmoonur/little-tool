import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, RefreshCw, Search, Tags, X } from 'lucide-react'
import { api, errorText } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import type { ListInfo, ListItems, TextItem } from '../types'
import { Modal } from '../ui/Modal'
import { TagManager, TagPicker } from './TagManager'

const PAGE_SIZE = 50
const UNTAGGED = ':untagged'

type Action = 'set' | 'add' | 'remove'

/**
 * 给分类里的每一条文本设置标签，可以逐条设置，也可以勾选多条一起添加或移除，确定后立即保存。
 * 标签按行号保存：只有读到的内容与素材库当前版本一致时才允许操作，避免标签贴到别的文本上。
 */
export function TextTagDialog({ list, onClose }: { list: ListInfo; onClose: () => void }) {
  const { snap, setSnap, run, commit, toast } = useApp()
  const [data, setData] = useState<ListItems | null>(null)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState('')
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [editing, setEditing] = useState<{ indexes: number[]; initial: string[]; action: Action } | null>(null)
  const [managing, setManaging] = useState(false)
  const [busy, setBusy] = useState(false)
  const rowsRef = useRef<HTMLUListElement>(null)

  // 首次打开，或分类内容、标签发生变化（保存、撤销等）后重新读取。行号可能已经对不上，同时取消勾选并关掉正在进行的选择。
  useEffect(() => {
    let alive = true
    setError('')
    setSelected(new Set())
    setEditing(null)
    api.listItems(list.id).then(
      d => alive && setData(d),
      e => alive && setError(errorText(e)),
    )
    return () => {
      alive = false
    }
  }, [list.id, list.updatedAt, reload])

  const synced = !!data && data.updatedAt === list.updatedAt
  const locked = !synced || busy

  // 版本对不上时，连同素材库概要一起重新读取。
  const refresh = async () => {
    const s = await run(api.snapshot())
    if (s) setSnap(s)
    setReload(n => n + 1)
  }

  const names = useMemo(() => new Map(snap.tags.map(t => [t.id, t.name])), [snap.tags])
  const shown = filter === UNTAGGED || names.has(filter) ? filter : ''
  const q = query.trim().toLowerCase()
  const rows = useMemo(
    () =>
      (data?.items ?? []).filter(
        item => (!q || item.text.toLowerCase().includes(q)) && (!shown || (shown === UNTAGGED ? !item.tagIds.length : item.tagIds.includes(shown))),
      ),
    [data, q, shown],
  )
  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const pageChecked = visible.length > 0 && visible.every(item => selected.has(item.index))
  const narrowed = !!q || !!shown

  useEffect(() => {
    rowsRef.current?.scrollTo(0, 0)
  }, [current, q, shown])

  // 换了查找或筛选条件后，之前勾选的文本可能已经不在眼前，一并取消勾选，免得批量操作改到看不见的文本。
  const narrow = (change: () => void) => {
    change()
    setPage(0)
    setSelected(new Set())
  }

  const toggle = (index: number) =>
    setSelected(s => {
      const next = new Set(s)
      if (!next.delete(index)) next.add(index)
      return next
    })
  const togglePage = (on: boolean) =>
    setSelected(s => {
      const next = new Set(s)
      for (const item of visible) on ? next.add(item.index) : next.delete(item.index)
      return next
    })

  const edit = (item: TextItem) => setEditing({ indexes: [item.index], initial: item.tagIds, action: 'set' })
  const bulk = (action: Action) => setEditing({ indexes: [...selected].sort((a, b) => a - b), initial: [], action })
  // 批量移除时只列出选中文本上确实有的标签。
  const selectedTags = useMemo(() => {
    const ids = new Set<string>()
    for (const item of data?.items ?? []) if (selected.has(item.index)) for (const id of item.tagIds) ids.add(id)
    return [...ids]
  }, [data, selected])

  const apply = async (ids: string[]) => {
    if (!editing || !data || locked) return
    const { indexes, action } = editing
    setBusy(true)
    try {
      const s = await api.setItemTags(list.id, indexes, ids, data.updatedAt, action)
      const n = num(indexes.length)
      commit(s, action === 'set' ? (indexes.length === 1 ? `已保存第 ${num(indexes[0] + 1)} 条文本的标签` : `已保存 ${n} 条文本的标签`) : action === 'add' ? `已给 ${n} 条文本添加标签` : `已从 ${n} 条文本移除标签`)
    } catch (e) {
      // 多半是文本或标签刚被改动过：提示后重新读取，不再使用旧的行号。
      toast(errorText(e), { kind: 'error' })
      await refresh()
    }
    setBusy(false)
  }

  const pickerProps = () => {
    if (!editing) return null
    const n = editing.indexes.length
    if (editing.action === 'add') {
      return { title: `给 ${num(n)} 条文本添加标签`, help: '选中的标签会加到这些文本上，它们原有的其他标签保持不变。', confirmText: '添加标签', canCreate: true }
    }
    if (editing.action === 'remove') {
      return { title: `从 ${num(n)} 条文本移除标签`, help: '选中的标签会从这些文本上去掉，其他标签保持不变。', confirmText: '移除标签', only: selectedTags }
    }
    const text = data?.items[editing.indexes[0]]?.text ?? ''
    return {
      title: `设置第 ${num(editing.indexes[0] + 1)} 条文本的标签`,
      help: <span className="tag-target" title={text}>{text}</span>,
      confirmText: '保存标签',
      noneLabel: '不设标签',
      canCreate: true,
    }
  }
  const picker = pickerProps()

  const footer = (
    <>
      <span className="tt-summary">
        {!data ? '' : narrowed ? `找到 ${num(rows.length)} 条，共 ${num(data.items.length)} 条文本` : `共 ${num(data.items.length)} 条文本`}
      </span>
      <button className="btn" onClick={() => setPage(current - 1)} disabled={current === 0}>
        <ChevronLeft size={18} />
        上一页
      </button>
      <span className="tt-page tnum">
        {current + 1} / {pages}
      </span>
      <button className="btn" onClick={() => setPage(current + 1)} disabled={current + 1 >= pages}>
        下一页
        <ChevronRight size={18} />
      </button>
      <button className="btn btn-primary btn-lg" onClick={onClose} disabled={busy}>
        完成
      </button>
    </>
  )

  return (
    <>
      <Modal title={`文本标签 · ${list.name}`} width={920} onClose={() => !busy && onClose()} className="text-tags" footer={footer}>
        <p className="tag-help">点每行右边的“设置标签”给这一条文本选标签；也可以先勾选多条，再一起添加或移除。确定后自动保存。</p>
        <div className="tt-tools">
          <label className="tt-search">
            <Search size={18} className="dim" />
            <input
              placeholder="查找文本内容"
              aria-label="查找要设置标签的文本"
              value={query}
              onChange={e => narrow(() => setQuery(e.target.value))}
            />
            {query && (
              <button className="icon-btn sm" onClick={() => narrow(() => setQuery(''))} aria-label="清除查找">
                <X size={16} />
              </button>
            )}
          </label>
          <select
            className="select"
            aria-label="按标签查看文本"
            value={shown}
            onChange={e => narrow(() => setFilter(e.target.value))}
          >
            <option value="">全部文本</option>
            <option value={UNTAGGED}>还没有标签的文本</option>
            {snap.tags.map(t => (
              <option key={t.id} value={t.id}>
                带标签：{t.name}
              </option>
            ))}
          </select>
          <button className="btn btn-tags" onClick={() => setManaging(true)}>
            <Tags size={18} />
            管理标签
          </button>
        </div>
        <div className={`tt-bulk ${selected.size ? 'is-on' : ''}`}>
          <label className="tt-check">
            <input type="checkbox" checked={pageChecked} disabled={!visible.length || locked} onChange={e => togglePage(e.target.checked)} />
            勾选本页
          </label>
          {rows.length > visible.length && (
            <button className="link" disabled={locked} onClick={() => setSelected(new Set(rows.map(item => item.index)))}>
              勾选全部 {num(rows.length)} 条
            </button>
          )}
          <span className="toolbar-spacer" />
          <span className="tt-count">{selected.size ? `已勾选 ${num(selected.size)} 条` : '勾选后可以一起设置'}</span>
          <button className="btn btn-tags" disabled={!selected.size || locked} onClick={() => bulk('add')}>
            添加标签
          </button>
          <button className="btn" disabled={!selected.size || !selectedTags.length || locked} onClick={() => bulk('remove')}>
            移除标签
          </button>
          {selected.size > 0 && (
            <button className="btn btn-ghost" onClick={() => setSelected(new Set())}>
              取消勾选
            </button>
          )}
        </div>
        {error ? (
          <div className="tt-state" role="alert">
            <p className="err-text">读取文本失败：{error}</p>
            <button className="btn" onClick={refresh}>
              <RefreshCw size={16} />
              重新读取
            </button>
          </div>
        ) : !data ? (
          <div className="tt-state">
            <p className="dim">正在读取文本…</p>
          </div>
        ) : (
          <ul className={`tt-rows ${locked ? 'is-locked' : ''}`} ref={rowsRef} aria-busy={locked}>
            {visible.map(item => (
              <li key={item.index} className={`tt-row ${selected.has(item.index) ? 'is-selected' : ''}`}>
                <label className="tt-pick">
                  <input
                    type="checkbox"
                    checked={selected.has(item.index)}
                    disabled={locked}
                    onChange={() => toggle(item.index)}
                    aria-label={`勾选第 ${item.index + 1} 条文本`}
                  />
                  <span className="tt-no tnum">{item.index + 1}</span>
                  <span className="tt-text" title={item.text}>
                    {item.text}
                  </span>
                </label>
                <div className="tt-tags">
                  {item.tagIds.length ? (
                    item.tagIds.map(id => (
                      <span className="tag-badge" key={id}>
                        {names.get(id) ?? '已删除的标签'}
                      </span>
                    ))
                  ) : (
                    <span className="dim">未设标签</span>
                  )}
                </div>
                <button className="btn" onClick={() => edit(item)} disabled={locked} aria-label={`设置第 ${item.index + 1} 条文本的标签`}>
                  <Tags size={16} />
                  设置标签
                </button>
              </li>
            ))}
            {!rows.length && (
              <li className="tt-none">
                {narrowed ? '没有符合条件的文本，请换一个查找内容或筛选条件。' : '这个分类还没有文本。'}
                {narrowed && (
                  <button
                    className="link"
                    onClick={() =>
                      narrow(() => {
                        setQuery('')
                        setFilter('')
                      })
                    }
                  >
                    显示全部文本
                  </button>
                )}
              </li>
            )}
          </ul>
        )}
        {data && !synced && !error && (
          <p className="tt-syncing" role="status">
            文本或标签刚刚有变化，正在重新读取…
            <button className="link" onClick={refresh}>
              重新读取
            </button>
          </p>
        )}
      </Modal>
      {editing && picker && <TagPicker {...picker} initial={editing.initial} busy={busy} onClose={() => !busy && setEditing(null)} onApply={apply} />}
      {managing && <TagManager onClose={() => setManaging(false)} />}
    </>
  )
}

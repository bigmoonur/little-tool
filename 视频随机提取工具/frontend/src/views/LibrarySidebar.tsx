import { useEffect, useState } from 'react'
import { ArrowDown, ArrowUp, Check, ChevronDown, FilePlus2, FolderInput, FolderOpen, FolderPlus, MoreHorizontal, Pencil, Search, Tags, Trash2, X } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { DeleteCategoryDialog } from '../dialogs/DeleteCategoryDialog'
import type { useCategoryTagging } from '../dialogs/CategoryTagging'
import { num, shortPath } from '../lib/format'
import { validateFolderName } from '../lib/names'
import { buildSections, useReorder } from '../lib/reorder'
import { TAG_COLORS, type Category, type Group } from '../types'
import { anchorOf, useMenu, type MenuAnchor, type MenuItem } from '../ui/Menu'

const COLOR_NAMES: Record<string, string> = {
  indigo: '靛蓝', teal: '青绿', amber: '琥珀', rose: '玫红', violet: '紫', green: '绿', orange: '橙', slate: '灰',
}

export function LibrarySidebar({ tagging, newCategory }: {
  tagging: ReturnType<typeof useCategoryTagging>
  newCategory: (groupId?: string) => Promise<void>
}) {
  const { state, setState, selected, select, run, prompt, confirm, toast, setDraft, copy, adding } = useApp()
  const [query, setQuery] = useState('')
  const [deleting, setDeleting] = useState<Category | null>(null)
  const menu = useMenu()
  const reorder = useReorder()
  const busy = copy.phase === 'copying' || !!adding || reorder.disabled
  const q = query.trim().toLowerCase()
  useEffect(() => {
    if (busy) menu.close()
  }, [busy])
  const createCategory = async (groupId = '') => {
    await newCategory(groupId)
    setQuery('')
  }
  const sections = buildSections(state)
    .map(s => ({ ...s, categories: q ? s.categories.filter(c => c.name.toLowerCase().includes(q)) : s.categories }))
    .filter(s => !q || s.categories.length)

  const newGroup = async () => {
    if (busy) return
    const name = await prompt({ title: '新建分组', label: '分组名称', placeholder: '例如：产品展示', confirmText: '新建' })
    if (!name) return
    const res = await run(api.createGroup(name))
    if (res) setState(res.state)
  }
  const renameGroup = async (g: Group) => {
    if (busy) return
    const name = await prompt({ title: '重命名分组', label: '分组名称', initial: g.name, confirmText: '重命名' })
    if (!name || name === g.name) return
    const s = await run(api.renameGroup(g.id, name))
    if (s) setState(s)
  }
  const deleteGroup = async (g: Group) => {
    if (busy) return
    const ok = await confirm({ title: `删除分组「${g.name}」？`, message: '分组里的分类会移到“未分组”，文件夹和视频会保留。', confirmText: '删除分组', danger: true })
    if (!ok) return
    const s = await run(api.deleteGroup(g.id))
    if (s) {
      setState(s)
      toast(`已删除分组「${g.name}」`, { kind: 'ok' })
    }
  }
  const moveGroupBy = async (g: Group, delta: -1 | 1) => {
    if (busy) return
    const i = state.groups.findIndex(x => x.id === g.id)
    const rest = state.groups.filter(x => x.id !== g.id)
    const s = await run(api.moveGroup(g.id, rest[i + delta]?.id ?? ''))
    if (s) setState(s)
  }
  const groupMenu = (g: Group, anchor: MenuAnchor) => {
    const i = state.groups.findIndex(x => x.id === g.id)
    menu.open(anchor, [
      { label: '重命名', icon: <Pencil size={15} />, disabled: busy, onSelect: () => renameGroup(g) },
      { heading: '颜色' },
      { custom: (
        <div className="swatches" role="group" aria-label="分组颜色">
          {TAG_COLORS.map(c => (
            <button
              key={c}
              className={`swatch c-${c} ${g.color === c ? 'is-on' : ''}`}
              title={COLOR_NAMES[c]}
              aria-label={`颜色：${COLOR_NAMES[c]}`}
              disabled={busy}
              onClick={async () => {
                menu.close()
                const s = await run(api.setGroupColor(g.id, c))
                if (s) setState(s)
              }}
            >
              {g.color === c && <Check size={12} />}
            </button>
          ))}
        </div>
      ) },
      { label: '在这个分组里新建分类', icon: <FilePlus2 size={15} />, disabled: busy || !!state.materialError, onSelect: () => createCategory(g.id) },
      { divider: true },
      { label: '上移', icon: <ArrowUp size={15} />, disabled: busy || i === 0, onSelect: () => moveGroupBy(g, -1) },
      { label: '下移', icon: <ArrowDown size={15} />, disabled: busy || i === state.groups.length - 1, onSelect: () => moveGroupBy(g, 1) },
      { divider: true },
      { label: '删除分组', icon: <Trash2 size={15} />, danger: true, disabled: busy, onSelect: () => deleteGroup(g) },
    ], 220)
  }
  const renameCategory = async (c: Category) => {
    if (busy) return
    const existing = new Set(state.categories.filter(x => x.name !== c.name).map(x => x.name.toLowerCase()))
    const name = await prompt({
      title: '重命名分类', label: '分类名称', initial: c.name, confirmText: '重命名',
      validate: v => validateFolderName(v) || (existing.has(v.toLowerCase()) ? '已经有同名分类了，请换一个名称' : ''),
    })
    if (!name || name === c.name) return
    const s = await run(api.renameCategory(c.name, name))
    if (!s) return
    setState(s)
    select(name)
    // 草稿是输入框里的原文，改名时也保留还没输完的数量。
    setDraft(d => {
      if (!d[c.name]) return d
      const next = { ...d, [name]: d[c.name] }
      delete next[c.name]
      return next
    })
    toast(`已重命名为「${name}」`, { kind: 'ok' })
  }
  const categoryMenu = (c: Category, anchor: MenuAnchor, el: Element) => {
    const items: MenuItem[] = [
      { label: '重命名', icon: <Pencil size={15} />, disabled: busy, onSelect: () => renameCategory(c) },
      { divider: true },
      { heading: '移动到分组' },
      ...state.groups.map(g => ({ label: g.name, icon: c.groupId === g.id ? <Check size={15} /> : <FolderInput size={15} />, disabled: busy || c.groupId === g.id, onSelect: () => reorder.moveCategoryTo(c.name, g.id) })),
      { label: '未分组', icon: !c.groupId ? <Check size={15} /> : <FolderInput size={15} />, disabled: busy || !c.groupId, onSelect: () => reorder.moveCategoryTo(c.name, '') },
      { divider: true },
      { label: '标签…', icon: <Tags size={15} />, disabled: busy, onSelect: () => tagging.open(c, el) },
      { label: '打开文件夹', icon: <FolderOpen size={15} />, onSelect: () => run(api.openCategory(c.name)) },
      { divider: true },
      { label: '删除分类…', icon: <Trash2 size={15} />, danger: true, disabled: busy, onSelect: () => setDeleting(c) },
    ]
    menu.open(anchor, items, 220)
  }
  const toggle = async (g: Group) => {
    if (busy) return
    const s = await run(api.setGroupCollapsed(g.id, !g.collapsed))
    if (s) setState(s)
  }

  return (
    <nav className="lib-side" aria-label="分类列表">
      <div className="lib-search">
        <Search size={15} className="dim" />
        <input placeholder="查找分类" value={query} onChange={e => setQuery(e.target.value)} aria-label="查找分类" />
        {query && <button className="icon-btn sm" onClick={() => setQuery('')} aria-label="清除查找"><X size={14} /></button>}
      </div>
      <div className="tree" onDragLeave={reorder.leave}>
        {sections.map(sec => {
          const g = sec.group
          const gid = g?.id ?? ''
          const collapsed = !!g?.collapsed && !q
          return (
            <div key={gid || 'loose'} className={`tree-section ${g ? `c-${g.color}` : 'c-none'}`}>
              {(g || state.groups.length > 0) && (
                <div
                  className={`tree-group ${reorder.dropClass('group-into', gid)} ${g ? reorder.dropClass('group', gid) : ''}`}
                  draggable={!!g && !q && !busy}
                  onDragStart={e => g && reorder.start(e, 'group', g.id)}
                  onDragEnd={reorder.end}
                  onDragEnter={e => reorder.overGroup(e, gid)}
                  onDragOver={e => reorder.overGroup(e, gid)}
                  onDrop={reorder.onDrop}
                  onContextMenu={e => {
                    if (!g) return
                    e.preventDefault()
                    groupMenu(g, { x: e.clientX, y: e.clientY })
                  }}
                >
                  <button className="tree-group-toggle" onClick={() => g && toggle(g)} aria-expanded={g ? !collapsed : undefined} disabled={!g || busy}>
                    {g ? <ChevronDown size={14} className={`chev ${collapsed ? 'is-collapsed' : ''}`} /> : <span className="chev-space" />}
                    <span className="group-dot" />
                    <span className="tree-group-name">{g?.name ?? '未分组'}</span>
                    <span className="tree-count">{sec.categories.length}</span>
                  </button>
                  {g && <button className="icon-btn sm tree-more" aria-label={`分组 ${g.name} 的更多操作`} onClick={e => groupMenu(g, anchorOf(e.currentTarget, 'end'))}><MoreHorizontal size={16} /></button>}
                </div>
              )}
              {!collapsed && (
                <ul className={`tree-lists ${state.groups.length ? '' : 'is-flat'}`}>
                  {sec.categories.map(c => (
                    <li
                      key={c.name}
                      className={`tree-item ${c.name === selected ? 'is-selected' : ''} ${c.count ? '' : 'is-empty'} ${reorder.dropClass('category', c.name)}`}
                      draggable={!q && !busy}
                      onDragStart={e => reorder.start(e, 'category', c.name)}
                      onDragEnd={reorder.end}
                      onDragEnter={e => reorder.overCategory(e, c)}
                      onDragOver={e => reorder.overCategory(e, c)}
                      onDrop={reorder.onDrop}
                      onContextMenu={e => {
                        e.preventDefault()
                        categoryMenu(c, { x: e.clientX, y: e.clientY }, e.currentTarget)
                      }}
                    >
                      <button className="tree-item-btn" onClick={() => select(c.name)} onDoubleClick={() => renameCategory(c)}>
                        <span className="tree-item-name" title={c.name}>{c.name}</span>
                        {c.tagIds.length > 0 && (
                          <span className="tree-tags" title={`标签：${c.tagIds.map(id => state.tags.find(t => t.id === id)?.name).filter(Boolean).join('、')}`}>
                            {c.tagIds.slice(0, 4).map(id => <i key={id} className={`tag-dot c-${state.tags.find(t => t.id === id)?.color}`} />)}
                          </span>
                        )}
                        <span className="tree-count">{num(c.count)}</span>
                      </button>
                      <button className="icon-btn sm tree-more" aria-label={`分类 ${c.name} 的更多操作`} onClick={e => categoryMenu(c, anchorOf(e.currentTarget, 'end'), e.currentTarget)}><MoreHorizontal size={16} /></button>
                    </li>
                  ))}
                  {sec.categories.length === 0 && state.groups.length > 0 && (
                    <li className={`tree-empty ${reorder.dropClass('group-into', gid)}`} onDragEnter={e => reorder.overGroup(e, gid)} onDragOver={e => reorder.overGroup(e, gid)} onDrop={reorder.onDrop}>
                      <span className="dim">{g ? '空分组，可以把分类拖到这里' : '可以把分类拖到这里'}</span>
                      <span className="tree-empty-actions"><button className="link" disabled={busy || !!state.materialError} onClick={() => createCategory(gid)}>新建分类</button></span>
                    </li>
                  )}
                </ul>
              )}
            </div>
          )
        })}
        {q && sections.length === 0 && <p className="tree-none">没有名称包含“{query.trim()}”的分类</p>}
      </div>
      <div className="lib-side-foot">
        <div className="lib-root" title={state.materialRoot}><span className="dim">素材根目录</span><span className="lib-root-path">{shortPath(state.materialRoot, 30)}</span></div>
        <button className="btn btn-ghost" onClick={() => createCategory()} disabled={busy || !!state.materialError}><FilePlus2 size={16} />新建分类</button>
        <button className="btn btn-ghost" onClick={newGroup} disabled={busy}><FolderPlus size={16} />新建分组</button>
        <button className="btn btn-ghost" onClick={() => run(api.openFolder(state.materialRoot))}><FolderOpen size={16} />打开素材根目录</button>
        <button className="btn btn-ghost" onClick={tagging.manage} disabled={busy}><Tags size={16} />管理标签{state.tags.length > 0 && <span className="tree-count">{state.tags.length}</span>}</button>
      </div>
      {deleting && <DeleteCategoryDialog category={state.categories.find(c => c.name === deleting.name) ?? deleting} onClose={() => setDeleting(null)} />}
      {menu.node}
    </nav>
  )
}

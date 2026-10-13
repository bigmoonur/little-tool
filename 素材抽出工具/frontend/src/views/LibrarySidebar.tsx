import { useState } from 'react'
import {
  ArrowDown, ArrowUp, Check, ChevronDown, Download, FilePlus2, FolderInput, FolderPlus, MoreHorizontal, Pencil, Plus, Search, Tags, Trash2, X,
} from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import { GROUP_COLORS, type Group, type ListInfo } from '../types'
import { anchorOf, useMenu, type MenuAnchor, type MenuItem } from '../ui/Menu'
import { buildSections, useReorder } from '../lib/reorder'
import { TagManager } from '../dialogs/TagManager'

const COLOR_NAMES: Record<string, string> = {
  indigo: '靛蓝', teal: '青绿', amber: '琥珀', rose: '玫红', violet: '紫', green: '绿', orange: '橙', slate: '灰',
}

export function LibrarySidebar() {
  const { snap, setSnap, selectedId, select, run, prompt, confirm, commit, toast, openImport } = useApp()
  const [query, setQuery] = useState('')
  const reorder = useReorder()
  const [tagsOpen, setTagsOpen] = useState(false)
  const menu = useMenu()
  const { deleteList } = useDeleteList()

  const q = query.trim().toLowerCase()
  const sections = buildSections(snap)
    .map(s => ({ ...s, lists: q ? s.lists.filter(l => l.name.toLowerCase().includes(q)) : s.lists }))
    .filter(s => !q || s.lists.length)

  // ---------- 分组操作 ----------

  const newGroup = async () => {
    const name = await prompt({ title: '新建分组', label: '分组名称', placeholder: '例如：服装类', confirmText: '新建' })
    if (!name) return
    const res = await run(api.createGroup(name))
    if (res) setSnap(res.snapshot)
  }

  const newList = async (groupId = '') => {
    const g = snap.groups.find(x => x.id === groupId)
    const name = await prompt({
      title: '新建分类',
      label: '分类名称',
      placeholder: '例如：上衣',
      confirmText: '新建',
      hint: g ? `将放在分组「${g.name}」中。新建后可以直接粘贴素材。` : '新建后可以直接粘贴素材，每行一条。',
    })
    if (!name) return
    const res = await run(api.createList(name, groupId))
    if (res) {
      setSnap(res.snapshot)
      select(res.id)
    }
  }

  const renameGroup = async (g: Group) => {
    const name = await prompt({ title: '重命名分组', label: '分组名称', initial: g.name, confirmText: '重命名' })
    if (!name || name === g.name) return
    const s = await run(api.renameGroup(g.id, name))
    if (s) setSnap(s)
  }

  const deleteGroup = async (g: Group) => {
    const lists = snap.lists.filter(l => l.groupId === g.id)
    let deleteLists = false
    if (lists.length) {
      const choice = await confirm({
        title: `删除分组「${g.name}」？`,
        message: (
          <>
            分组中有 {lists.length} 个分类。可以只删除分组、把分类移到“未分组”，也可以连同分类和其中的素材一起删除。
          </>
        ),
        buttons: [
          { key: 'cancel', label: '取消' },
          { key: 'all', label: '连同分类一起删除', kind: 'danger' },
          { key: 'keep', label: '只删除分组', kind: 'primary' },
        ],
      })
      if (!choice) return
      deleteLists = choice === 'all'
    }
    const s = await run(api.deleteGroup(g.id, deleteLists))
    if (s) commit(s, deleteLists ? `已删除分组「${g.name}」及其中的分类` : `已删除分组「${g.name}」`)
  }

  const moveGroupBy = async (g: Group, delta: number) => {
    const i = snap.groups.findIndex(x => x.id === g.id)
    const s = await run(api.moveGroup(g.id, i + delta))
    if (s) setSnap(s)
  }

  const groupMenu = (g: Group, anchor: MenuAnchor) => {
    const i = snap.groups.findIndex(x => x.id === g.id)
    const items: MenuItem[] = [
      { label: '在此分组中新建分类', icon: <FilePlus2 size={15} />, onSelect: () => newList(g.id) },
      { label: '添加素材到此分组', icon: <Plus size={15} />, onSelect: () => openImport({ groupId: g.id }) },
      { divider: true },
      { label: '重命名分组', icon: <Pencil size={15} />, onSelect: () => renameGroup(g) },
      {
        custom: (
          <div className="swatches" role="group" aria-label="分组颜色">
            {GROUP_COLORS.map(c => (
              <button
                key={c}
                className={`swatch c-${c} ${g.color === c ? 'is-on' : ''}`}
                title={COLOR_NAMES[c]}
                aria-label={`颜色：${COLOR_NAMES[c]}`}
                onClick={async () => {
                  menu.close()
                  const s = await run(api.setGroupColor(g.id, c))
                  if (s) setSnap(s)
                }}
              >
                {g.color === c && <Check size={12} />}
              </button>
            ))}
          </div>
        ),
      },
      { label: '上移', icon: <ArrowUp size={15} />, disabled: i === 0, onSelect: () => moveGroupBy(g, -1) },
      { label: '下移', icon: <ArrowDown size={15} />, disabled: i === snap.groups.length - 1, onSelect: () => moveGroupBy(g, 1) },
      { divider: true },
      { label: '删除分组…', icon: <Trash2 size={15} />, danger: true, onSelect: () => deleteGroup(g) },
    ]
    menu.open(anchor, items, 220)
  }

  // ---------- 分类操作 ----------

  const renameList = async (l: ListInfo) => {
    const name = await prompt({ title: '重命名分类', label: '分类名称', initial: l.name, confirmText: '重命名' })
    if (!name || name === l.name) return
    const s = await run(api.renameList(l.id, name))
    if (s) setSnap(s)
  }

  const moveList = async (l: ListInfo, groupId: string, beforeId = '') => {
    const s = await run(api.moveList(l.id, groupId, beforeId))
    if (s) setSnap(s)
  }

  const exportList = async (l: ListInfo) => {
    const path = await run(api.exportList(l.id))
    if (path) toast(`已导出「${l.name}」`, { kind: 'ok' })
  }

  const listMenu = (l: ListInfo, anchor: MenuAnchor) => {
    const items: MenuItem[] = [
      { label: '重命名', icon: <Pencil size={15} />, onSelect: () => renameList(l) },
      { label: '添加素材', icon: <Plus size={15} />, onSelect: () => openImport({ tab: 'paste', targetId: l.id }) },
      { label: '导出为 TXT', icon: <Download size={15} />, disabled: l.count === 0, onSelect: () => exportList(l) },
    ]
    if (snap.groups.length) {
      items.push({ divider: true }, { heading: '移动到分组' })
      for (const g of snap.groups) {
        items.push({
          label: g.name,
          icon: l.groupId === g.id ? <Check size={15} /> : <FolderInput size={15} />,
          disabled: l.groupId === g.id,
          onSelect: () => moveList(l, g.id),
        })
      }
      items.push({
        label: '未分组',
        icon: !l.groupId ? <Check size={15} /> : <FolderInput size={15} />,
        disabled: !l.groupId,
        onSelect: () => moveList(l, ''),
      })
    }
    items.push({ divider: true }, { label: '删除分类…', icon: <Trash2 size={15} />, danger: true, onSelect: () => deleteList(l) })
    menu.open(anchor, items, 220)
  }

  const toggle = (g: Group) => {
    setSnap({ ...snap, groups: snap.groups.map(x => (x.id === g.id ? { ...x, collapsed: !g.collapsed } : x)) })
    api.setGroupCollapsed(g.id, !g.collapsed).then(setSnap)
  }

  return (
    <nav className="lib-side" aria-label="分类列表">
      <div className="lib-search">
        <Search size={15} className="dim" />
        <input placeholder="查找分类" value={query} onChange={e => setQuery(e.target.value)} aria-label="查找分类" />
        {query && (
          <button className="icon-btn sm" onClick={() => setQuery('')} aria-label="清除查找">
            <X size={14} />
          </button>
        )}
      </div>

      <div className="tree" onDragLeave={reorder.leave}>
        {sections.map(sec => {
          const g = sec.group
          const gid = g?.id ?? ''
          const collapsed = !!g?.collapsed && !q
          const showHeader = g || snap.groups.length > 0
          return (
            <div key={gid || 'loose'} className={`tree-section ${g ? `c-${g.color}` : 'c-none'}`}>
              {showHeader && (
                <div
                  className={`tree-group ${reorder.dropClass('group-into', gid)} ${g ? reorder.dropClass('group', gid) : ''}`}
                  draggable={!!g && !reorder.pending}
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
                  <button
                    className="tree-group-toggle"
                    onClick={() => g && toggle(g)}
                    aria-expanded={g ? !collapsed : undefined}
                    disabled={!g}
                  >
                    {g ? <ChevronDown size={14} className={`chev ${collapsed ? 'is-collapsed' : ''}`} /> : <span className="chev-space" />}
                    <span className="group-dot" />
                    <span className="tree-group-name">{g?.name ?? '未分组'}</span>
                    <span className="tree-count">{sec.lists.length}</span>
                  </button>
                  {g && (
                    <button className="icon-btn sm tree-more" aria-label={`分组 ${g.name} 的更多操作`} onClick={e => groupMenu(g, anchorOf(e.currentTarget, 'end'))}>
                      <MoreHorizontal size={16} />
                    </button>
                  )}
                </div>
              )}
              {!collapsed && (
                <ul className="tree-lists">
                  {sec.lists.map(l => (
                    <li
                      key={l.id}
                      className={`tree-item ${l.id === selectedId ? 'is-selected' : ''} ${reorder.dropClass('list', l.id)}`}
                      draggable={!q && !reorder.pending}
                      onDragStart={e => reorder.start(e, 'list', l.id)}
                      onDragEnd={reorder.end}
                      onDragEnter={e => reorder.overList(e, l)}
                      onDragOver={e => reorder.overList(e, l)}
                      onDrop={reorder.onDrop}
                      onContextMenu={e => {
                        e.preventDefault()
                        listMenu(l, { x: e.clientX, y: e.clientY })
                      }}
                    >
                      <button className="tree-item-btn" onClick={() => select(l.id)} onDoubleClick={() => renameList(l)}>
                        <span className="tree-item-name">{l.name}</span>
                        <span className="tree-count">{num(l.count)}</span>
                      </button>
                      <button className="icon-btn sm tree-more" aria-label={`分类 ${l.name} 的更多操作`} onClick={e => listMenu(l, anchorOf(e.currentTarget, 'end'))}>
                        <MoreHorizontal size={16} />
                      </button>
                    </li>
                  ))}
                  {g && sec.lists.length === 0 && (
                    <li
                      className={`tree-empty ${reorder.dropClass('group-into', gid)}`}
                      onDragEnter={e => reorder.overGroup(e, gid)}
                      onDragOver={e => reorder.overGroup(e, gid)}
                      onDrop={reorder.onDrop}
                    >
                      <span className="dim">空分组，可以把分类拖到这里</span>
                      <span className="tree-empty-actions">
                        <button className="link" onClick={() => openImport({ groupId: gid })}>
                          添加素材
                        </button>
                        <button className="link" onClick={() => newList(gid)}>
                          新建分类
                        </button>
                      </span>
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
        <button className="btn btn-ghost" onClick={() => newList('')}>
          <FilePlus2 size={16} />
          新建分类
        </button>
        <button className="btn btn-ghost" onClick={newGroup}>
          <FolderPlus size={16} />
          新建分组
        </button>
      </div>
      <div className="lib-tag-foot">
        <button className="btn btn-tags" onClick={() => setTagsOpen(true)}>
          <Tags size={18} />
          管理标签
          {snap.tags.length > 0 && <span className="tree-count">{snap.tags.length}</span>}
        </button>
      </div>
      {tagsOpen && <TagManager onClose={() => setTagsOpen(false)} />}
      {menu.node}
    </nav>
  )
}

/** 删除分类（带确认与撤销），侧栏和详情页共用。 */
export function useDeleteList() {
  const { run, confirm, commit } = useApp()
  const deleteList = async (l: ListInfo) => {
    const ok = await confirm({
      title: `删除分类「${l.name}」？`,
      message: l.count > 0 ? `其中的 ${num(l.count)} 条素材会一起删除。删除后可以马上撤销。` : '这个分类是空的。',
      confirmText: '删除',
      danger: true,
    })
    if (!ok) return false
    const s = await run(api.deleteList(l.id))
    if (s) commit(s, `已删除分类「${l.name}」`)
    return !!s
  }
  return { deleteList }
}

import { useEffect } from 'react'
import { CopyMinus, Download, FileText, Pencil, Plus, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { loadFiles, pickFiles } from '../lib/files'
import { num, when } from '../lib/format'
import type { ListInfo } from '../types'
import { EmptyLibrary } from './EmptyLibrary'
import { ListEditor } from './ListEditor'
import { LibrarySidebar, useDeleteList } from './LibrarySidebar'
import { buildSections } from '../lib/reorder'
import { useTagMap } from '../lib/tags'
import { useListTagging } from '../dialogs/ListTagging'
import { TagPill } from '../ui/TagPill'

export function LibraryView() {
  const { snap, selectedId, select } = useApp()
  const list = snap.lists.find(l => l.id === selectedId)

  // 没有选中（或选中的分类被删除）时，自动选中第一个分类。
  useEffect(() => {
    const first = buildSections(snap).flatMap(s => s.lists)[0]
    if (!list && first) select(first.id)
  }, [list, snap.lists, snap.groups])

  return (
    <div className="library">
      <LibrarySidebar />
      <section className="lib-detail">
        {list ? (
          <ListDetail key={list.id} list={list} />
        ) : (
          <EmptyLibrary title="添加第一批素材" lead="可以先新建分组，再把素材直接添加到分组里；也可以不分组，直接添加。" />
        )}
      </section>
    </div>
  )
}

function ListDetail({ list }: { list: ListInfo }) {
  const { snap, setSnap, run, prompt, toast, commit, openImport } = useApp()
  const { deleteList } = useDeleteList()
  const dupes = list.count - list.unique
  const tags = useTagMap()
  const tagging = useListTagging()

  const rename = async () => {
    const name = await prompt({ title: '重命名分类', label: '分类名称', initial: list.name, confirmText: '重命名' })
    if (!name || name === list.name) return
    const s = await run(api.renameList(list.id, name))
    if (s) setSnap(s)
  }

  const importFiles = async () => {
    const files = await pickFiles()
    if (!files.length) return
    const { loaded, skipped } = await loadFiles(files)
    if (skipped.length) toast(`只支持 TXT 文本文件，已跳过：${skipped.join('、')}`, { kind: 'error' })
    if (loaded.length) openImport({ tab: 'files', files: loaded, targetId: list.id })
  }

  const exportList = async () => {
    const path = await run(api.exportList(list.id))
    if (path) toast(`已导出「${list.name}」`, { kind: 'ok' })
  }

  const dedupe = async () => {
    const res = await run(api.dedupeList(list.id))
    if (res) commit(res.snapshot, `已删除 ${num(res.removed)} 条重复内容`)
  }

  const moveTo = async (groupId: string) => {
    const s = await run(api.moveList(list.id, groupId, ''))
    if (s) setSnap(s)
  }

  return (
    <div className="detail">
      <header className="detail-head">
        <div className="detail-title">
          <h1 title={list.name}>{list.name}</h1>
          <button className="icon-btn" onClick={rename} aria-label="重命名分类" title="重命名">
            <Pencil size={16} />
          </button>
        </div>
        <div className="detail-meta">
          {snap.groups.length > 0 && (
            <label className="pill-select">
              <span className="dim">分组</span>
              <select value={list.groupId} onChange={e => moveTo(e.target.value)} aria-label="所在分组">
                {snap.groups.map(g => (
                  <option key={g.id} value={g.id}>
                    {g.name}
                  </option>
                ))}
                <option value="">未分组</option>
              </select>
            </label>
          )}
          <span>
            <b className="tnum">{num(list.count)}</b> 条素材
          </span>
          {dupes > 0 && <span className="warn-text">其中 {num(dupes)} 条重复</span>}
          {list.updatedAt > 0 && <span className="dim">{when(list.updatedAt)} 更新</span>}
        </div>
        <div className="detail-tags">
          <span className="dim">标签</span>
          {list.tagIds.map(id => (
            <TagPill key={id} tag={tags.get(id)} />
          ))}
          <button
            className={`tag-add ${tagging.openId === list.id ? 'is-open' : ''}`}
            aria-haspopup="dialog"
            title="标签贴在整个分类上，抽取时点标签就能只看带这个标签的分类"
            onClick={e => tagging.open(list, e.currentTarget)}
          >
            <Plus size={14} />
            <span>{list.tagIds.length ? '修改' : '贴标签'}</span>
          </button>
        </div>
        <div className="detail-actions">
          <button className="btn btn-primary" onClick={() => openImport({ tab: 'paste', targetId: list.id })}>
            <Plus size={16} />
            添加素材
          </button>
          <button className="btn" onClick={importFiles}>
            <FileText size={16} />
            从 TXT 导入
          </button>
          <button className="btn" onClick={exportList} disabled={list.count === 0}>
            <Download size={16} />
            导出 TXT
          </button>
          {dupes > 0 && (
            <button className="btn" onClick={dedupe}>
              <CopyMinus size={16} />
              去除重复
            </button>
          )}
          <span className="toolbar-spacer" />
          <button className="btn btn-ghost is-danger" onClick={() => deleteList(list)}>
            <Trash2 size={16} />
            删除分类
          </button>
        </div>
      </header>
      <ListEditor list={list} />
      {tagging.node}
    </div>
  )
}

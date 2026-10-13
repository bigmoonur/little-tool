import { useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, FolderOpen, Play, Plus, RefreshCw, Search, Tags, X } from 'lucide-react'
import { api, errorText } from '../api'
import { useApp } from '../ctx'
import { useCategoryTagging } from '../dialogs/CategoryTagging'
import { bytes, num, shortPath, when } from '../lib/format'
import type { Category, Tag, VideoFile } from '../types'
import { TagPill } from '../ui/TagPill'
import { Setup } from './Setup'

/** 一次最多列出这么多行；再多就请用查找缩小范围，免得列表卡顿。 */
const MAX_ROWS = 1000

type SortKey = 'name' | 'size' | 'modified'

const COLUMNS: { key: SortKey; label: string; className: string }[] = [
  { key: 'name', label: '文件名', className: '' },
  { key: 'size', label: '大小', className: 'ta-r' },
  { key: 'modified', label: '修改时间', className: 'ta-r' },
]

/** 素材库：左边是素材根目录里的分类，右边是选中分类里的视频。只用来查看和贴标签，视频本身在文件夹里管理。 */
export function LibraryView() {
  const { state, selected, select, run, refresh } = useApp()
  const [query, setQuery] = useState('')
  const tagging = useCategoryTagging()
  const tagById = useMemo(() => new Map<string, Tag>(state.tags.map(t => [t.id, t])), [state.tags])
  const current = state.categories.find(c => c.name === selected)

  // 没有选中（或选中的分类已经不在了）时，自动选中第一个分类。
  useEffect(() => {
    if (!current && state.categories.length) select(state.categories[0].name)
  }, [current, state.categories])

  if (!state.materialRoot || !state.outputRoot) return <Setup />

  const q = query.trim().toLowerCase()
  const shown = q ? state.categories.filter(c => c.name.toLowerCase().includes(q)) : state.categories

  return (
    <div className="library">
      <nav className="lib-side" aria-label="分类列表">
        <div className="lib-search">
          <Search size={15} className="dim" />
          <input value={query} onChange={e => setQuery(e.target.value)} placeholder="查找分类" aria-label="查找分类" />
          {query && (
            <button className="icon-btn sm" onClick={() => setQuery('')} aria-label="清除查找">
              <X size={14} />
            </button>
          )}
        </div>
        <ul className="lib-list">
          {shown.map(c => (
            <li key={c.name}>
              <button className={`lib-item ${c.name === current?.name ? 'is-selected' : ''} ${c.count ? '' : 'is-empty'}`} onClick={() => select(c.name)}>
                <span className="lib-item-name" title={c.name}>
                  {c.name}
                </span>
                {c.tagIds.length > 0 && (
                  <span className="lib-item-tags" title={`标签：${c.tagIds.map(id => tagById.get(id)?.name).filter(Boolean).join('、')}`}>
                    {c.tagIds.map(id => (
                      <i key={id} className={`tag-dot c-${tagById.get(id)?.color}`} />
                    ))}
                  </span>
                )}
                <span className="lib-item-count tnum">{num(c.count)}</span>
              </button>
            </li>
          ))}
        </ul>
        {q && shown.length === 0 && <p className="lib-none">没有名称包含“{query.trim()}”的分类</p>}
        <div className="lib-side-foot">
          <div className="lib-root" title={state.materialRoot}>
            <span className="dim">素材根目录</span>
            <span className="lib-root-path">{shortPath(state.materialRoot, 30)}</span>
          </div>
          <button className="btn btn-ghost" onClick={() => run(api.openFolder(state.materialRoot))}>
            <FolderOpen size={16} />
            打开素材根目录
          </button>
          <button className="btn btn-ghost" onClick={tagging.manage}>
            <Tags size={16} />
            管理标签
            {state.tags.length > 0 && <span className="lib-item-count tnum">{state.tags.length}</span>}
          </button>
        </div>
      </nav>
      <section className="lib-detail">
        {state.materialError ? (
          <div className="empty-inline is-error">
            <p>{state.materialError}</p>
            <button className="btn" onClick={refresh}>
              <RefreshCw size={16} />
              重新读取
            </button>
          </div>
        ) : current ? (
          <CategoryDetail key={current.name} category={current} tags={tagById} tagging={tagging} />
        ) : (
          <div className="empty-inline">
            <h2>素材根目录里还没有分类</h2>
            <p>在素材根目录里新建子文件夹，每个子文件夹是一个分类，把视频放进去。回到这个窗口时会自动刷新。</p>
            <button className="btn" onClick={() => run(api.openFolder(state.materialRoot))}>
              <FolderOpen size={16} />
              打开素材根目录
            </button>
          </div>
        )}
      </section>
      {tagging.node}
    </div>
  )
}

function CategoryDetail({ category, tags, tagging }: {
  category: Category
  tags: Map<string, Tag>
  tagging: ReturnType<typeof useCategoryTagging>
}) {
  const { run, refresh } = useApp()
  const [files, setFiles] = useState<VideoFile[] | null>(null)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: 'name', desc: false })
  const [version, setVersion] = useState(0)

  // 数量或总大小变了（回到窗口时会自动重新扫描），或者点了刷新，就重新读取这个分类。
  useEffect(() => {
    let stale = false
    api.videos(category.name).then(
      list => {
        if (stale) return
        setFiles(list)
        setError('')
      },
      e => {
        if (stale) return
        setFiles([])
        setError(errorText(e))
      },
    )
    return () => {
      stale = true
    }
  }, [category.name, category.count, category.bytes, version])

  const reload = () => {
    setVersion(v => v + 1)
    refresh()
  }

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = (files ?? []).filter(v => !q || v.name.toLowerCase().includes(q))
    // 后端已经按名称排好，只有按大小或时间时才需要重排。
    const key = sort.key
    if (key !== 'name') list.sort((a, b) => a[key] - b[key])
    return sort.desc ? list.reverse() : list
  }, [files, query, sort])

  const sortBy = (key: SortKey) => setSort(s => (s.key === key ? { key, desc: !s.desc } : { key, desc: key !== 'name' }))
  const play = (v: VideoFile) => run(api.openVideo(category.name, v.name))

  return (
    <div className="detail">
      <header className="detail-head">
        <div className="detail-title">
          <h1 title={category.name}>{category.name}</h1>
        </div>
        <div className="detail-meta">
          <span>
            <b className="tnum">{num(category.count)}</b> 个视频
          </span>
          {category.count > 0 && <span>{bytes(category.bytes)}</span>}
        </div>
        <div className="detail-tags">
          <span className="dim">标签</span>
          {category.tagIds.map(id => {
            const t = tags.get(id)
            return t ? <TagPill key={id} tag={t} /> : null
          })}
          <button
            className={`tag-add ${tagging.openName === category.name ? 'is-open' : ''}`}
            aria-haspopup="dialog"
            title="标签贴在整个分类上，抽取时点标签就能只看带这个标签的分类"
            onClick={e => tagging.open(category, e.currentTarget)}
          >
            <Plus size={13} />
            <span>{category.tagIds.length ? '修改' : '贴标签'}</span>
          </button>
        </div>
        <div className="detail-actions">
          <button className="btn btn-primary" onClick={() => run(api.openCategory(category.name))}>
            <FolderOpen size={16} />
            打开文件夹
          </button>
          <button className="btn" onClick={reload} title="重新读取这个分类里的视频（回到窗口时也会自动刷新）">
            <RefreshCw size={16} />
            刷新
          </button>
          <span className="toolbar-spacer" />
          {(files?.length ?? 0) > 0 && (
            <label className="lib-search is-inline">
              <Search size={15} className="dim" />
              <input value={query} onChange={e => setQuery(e.target.value)} placeholder="查找视频" aria-label="在这个分类里查找视频" />
              {query && (
                <button className="icon-btn sm" onClick={() => setQuery('')} aria-label="清除查找">
                  <X size={14} />
                </button>
              )}
            </label>
          )}
        </div>
      </header>

      {error ? (
        <div className="empty-inline is-error">
          <p>{error}</p>
        </div>
      ) : files && files.length === 0 ? (
        <div className="empty-inline">
          <h2>这个分类里还没有视频</h2>
          <p>把视频直接放进这个分类的文件夹（不含子文件夹），回到这个窗口时会自动刷新。</p>
        </div>
      ) : (
        <div className="video-table" role="table" aria-label={`「${category.name}」里的视频`} aria-busy={!files}>
          <div className="video-head" role="row">
            {COLUMNS.map(col => (
              <button
                key={col.key}
                role="columnheader"
                aria-sort={sort.key === col.key ? (sort.desc ? 'descending' : 'ascending') : 'none'}
                className={`video-sort ${col.className} ${sort.key === col.key ? 'is-on' : ''}`}
                onClick={() => sortBy(col.key)}
                title={`按${col.label}排序`}
              >
                {col.label}
                {sort.key === col.key && (sort.desc ? <ArrowDown size={13} /> : <ArrowUp size={13} />)}
              </button>
            ))}
            <span role="columnheader" aria-label="播放" />
          </div>
          <div className="video-rows">
            {rows.slice(0, MAX_ROWS).map(v => (
              <div key={v.name} className="video-row" role="row" onDoubleClick={() => play(v)}>
                <span className="video-name" role="cell" title={v.name}>
                  {v.name}
                </span>
                <span className="ta-r tnum dim" role="cell">
                  {bytes(v.size)}
                </span>
                <span className="ta-r dim" role="cell">
                  {when(v.modified)}
                </span>
                <span role="cell">
                  <button className="icon-btn sm" onClick={() => play(v)} aria-label={`播放 ${v.name}`} title="用默认播放器打开（双击这一行也可以）">
                    <Play size={14} />
                  </button>
                </span>
              </div>
            ))}
            {files && rows.length === 0 && <p className="video-none">没有文件名包含“{query.trim()}”的视频</p>}
            {rows.length > MAX_ROWS && (
              <p className="video-none">
                只列出前 {num(MAX_ROWS)} 个，还有 {num(rows.length - MAX_ROWS)} 个没有显示。可以用上面的查找缩小范围。
              </p>
            )}
          </div>
        </div>
      )}
      <p className="lib-note">
        这里用来核对素材，原视频不会被改动。增加、删除或改名请直接在文件夹里操作。
        {query.trim() && files && rows.length > 0 && ` 当前显示 ${num(rows.length)} / ${num(files.length)} 个。`}
      </p>
    </div>
  )
}

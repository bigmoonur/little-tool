import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { ArrowDown, ArrowUp, Film, FolderOpen, FolderPlus, LayoutGrid, List, Play, Plus, RefreshCw, Search, X } from 'lucide-react'
import { api, errorText } from '../api'
import { useApp } from '../ctx'
import { useCategoryTagging } from '../dialogs/CategoryTagging'
import { bytes, num, when } from '../lib/format'
import { validateFolderName } from '../lib/names'
import { thumbUrl, useThumb, useVisible, type ThumbState } from '../lib/thumbs'
import type { Category, LibraryMode, Tag, VideoFile } from '../types'
import { TagPill } from '../ui/TagPill'
import { LibrarySidebar } from './LibrarySidebar'
import { Setup } from './Setup'

/** 一次最多列出这么多个；再多就请用查找缩小范围，免得列表卡顿。 */
const MAX_ROWS = 1000

type SortKey = 'name' | 'size' | 'modified'

const COLUMNS: { key: SortKey; label: string; className: string }[] = [
  { key: 'name', label: '文件名', className: '' },
  { key: 'size', label: '大小', className: 'ta-r' },
  { key: 'modified', label: '修改时间', className: 'ta-r' },
]

const MODES: { mode: LibraryMode; label: string; icon: React.ReactNode }[] = [
  { mode: 'list', label: '列表', icon: <List size={15} /> },
  { mode: 'grid', label: '图标', icon: <LayoutGrid size={15} /> },
]

/** 素材库：左边是素材根目录里的分类，右边是选中分类里的视频。可以新建分类、往分类里添加视频、贴标签、整理分组和分类。 */
export function LibraryView() {
  const { state, setState, selected, select, run, refresh, prompt, toast, copy, adding } = useApp()
  const tagging = useCategoryTagging()
  const tagById = useMemo(() => new Map<string, Tag>(state.tags.map(t => [t.id, t])), [state.tags])
  const current = state.categories.find(c => c.name === selected)
  const busy = copy.phase === 'copying' || !!adding

  // 没有选中（或选中的分类已经不在了）时，自动选中第一个分类。
  useEffect(() => {
    if (!current && state.categories.length) select(state.categories[0].name)
  }, [current, state.categories])

  if (!state.materialRoot || !state.outputRoot) return <Setup />

  const newCategory = async (groupId = '') => {
    if (busy) return
    const existing = new Set(state.categories.map(c => c.name.toLowerCase()))
    const name = await prompt({
      title: '新建分类',
      label: '分类名称',
      placeholder: '例如：开场',
      confirmText: '新建',
      hint: groupId ? `会放在分组「${state.groups.find(g => g.id === groupId)?.name}」里；同名的隐藏分类会恢复。` : '会在素材根目录里新建同名文件夹；之前移除的同名分类会恢复。',
      validate: v => validateFolderName(v) || (existing.has(v.toLowerCase()) ? '已经有同名分类了，可以直接选择它' : ''),
    })
    if (!name) return
    const s = await run(api.createCategoryIn(name, groupId))
    if (!s) return
    const g = s.groups.find(g => g.id === groupId)
    const expanded = g?.collapsed ? await run(api.setGroupCollapsed(groupId, false)) : undefined
    setState(expanded ?? s)
    select(s.categories.find(c => c.name.toLowerCase() === name.toLowerCase())?.name ?? name)
    toast(`已选中分类「${name}」`, { kind: 'ok' })
  }

  return (
    <div className="library">
      <LibrarySidebar tagging={tagging} newCategory={newCategory} />
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
            <p>每个分类是素材根目录里的一个子文件夹。可以在这里新建，也可以直接在资源管理器里建好文件夹，回到这个窗口时会自动刷新。</p>
            <div className="empty-inline-actions">
              <button className="btn btn-primary" onClick={() => newCategory()} disabled={busy}>
                <FolderPlus size={16} />
                新建分类
              </button>
              <button className="btn" onClick={() => run(api.openFolder(state.materialRoot))}>
                <FolderOpen size={16} />
                打开素材根目录
              </button>
            </div>
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
  const { run, refresh, settings, setSettings, copy, adding, addVideos, cancelCopy } = useApp()
  const [files, setFiles] = useState<VideoFile[] | null>(null)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: 'name', desc: false })
  const [version, setVersion] = useState(0)
  const thumbs = settings.thumbnails
  const mode: LibraryMode = thumbs && settings.libraryView === 'grid' ? 'grid' : 'list'
  const busy = copy.phase === 'copying' || !!adding
  const addingHere = adding?.category === category.name ? adding : null

  // 数量或总大小变了（回到窗口时会自动重新扫描），开关了缩略图，或者点了刷新，就重新读取这个分类。
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
  }, [category.name, category.count, category.bytes, thumbs, version])

  const reload = () => {
    setVersion(v => v + 1)
    refresh()
  }

  const setMode = async (m: LibraryMode) => {
    setSettings({ ...settings, libraryView: m })
    const s = await run(api.setLibraryView(m))
    if (s) setSettings(s)
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

  // ---------- 列表里悬停的预览 ----------

  const [hover, setHover] = useState<VideoFile | null>(null)
  const hoverRef = useRef(hover)
  hoverRef.current = hover
  const hoverTimer = useRef(0)
  /** 鼠标的横向位置和所在行的中线；预览跟着鼠标左右走，上下贴着行。 */
  const point = useRef({ x: 0, y: 0 })
  const pop = useRef<HTMLDivElement>(null)

  const place = () => {
    const el = pop.current
    if (!el) return
    const { x, y } = point.current
    const w = el.offsetWidth
    const h = el.offsetHeight
    // 放在鼠标右边；右边放不下就换到左边。
    const left = x + 22 + w > window.innerWidth - 8 ? x - 22 - w : x + 22
    // 第一次摆放直接到位；之后换行时才顺着滑过去。
    const first = !el.dataset.placed
    if (first) el.style.transition = 'none'
    el.style.left = `${Math.max(8, left)}px`
    el.style.top = `${Math.min(Math.max(8, y - h / 2), window.innerHeight - h - 8)}px`
    if (first) {
      void el.offsetWidth
      el.style.transition = ''
      el.dataset.placed = '1'
    }
  }
  const track = (e: React.MouseEvent<HTMLElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    point.current = { x: e.clientX, y: r.top + r.height / 2 }
    place()
  }
  const enter = (v: VideoFile, e: React.MouseEvent<HTMLElement>) => {
    track(e)
    window.clearTimeout(hoverTimer.current)
    // 已经弹出时直接换到这一行；刚移上来时稍等一下，鼠标只是路过就不弹。
    if (hoverRef.current) setHover(v)
    else hoverTimer.current = window.setTimeout(() => setHover(v), 110)
  }
  const leave = () => {
    window.clearTimeout(hoverTimer.current)
    hoverTimer.current = window.setTimeout(() => setHover(null), 70)
  }
  useEffect(() => () => window.clearTimeout(hoverTimer.current), [])

  const pct = addingHere?.progress ? Math.min(100, Math.floor((addingHere.progress.bytesDone / Math.max(1, addingHere.progress.bytesTotal)) * 100)) : 0
  const addButton = (primary: boolean) => (
    <button
      className={`btn ${primary ? 'btn-primary' : ''}`}
      onClick={() => addVideos(category.name)}
      disabled={busy}
      title={busy ? '正在复制，完成后再添加' : '选择视频文件，复制进这个分类；原文件会保留'}
    >
      <Plus size={16} />
      添加视频
    </button>
  )

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
            disabled={busy}
            title="标签贴在整个分类上，抽取时点标签就能只看带这个标签的分类"
            onClick={e => tagging.open(category, e.currentTarget)}
          >
            <Plus size={13} />
            <span>{category.tagIds.length ? '修改' : '贴标签'}</span>
          </button>
        </div>
        <div className="detail-actions">
          {addButton(true)}
          <button className="btn" onClick={() => run(api.openCategory(category.name))}>
            <FolderOpen size={16} />
            打开文件夹
          </button>
          <button className="icon-btn" onClick={reload} aria-label="刷新" title="重新读取这个分类里的视频（回到窗口时也会自动刷新）">
            <RefreshCw size={16} />
          </button>
          <span className="toolbar-spacer" />
          {thumbs && (
            <div className="seg view-seg" role="radiogroup" aria-label="显示方式">
              {MODES.map(m => (
                <button
                  key={m.mode}
                  role="radio"
                  aria-checked={mode === m.mode}
                  aria-label={m.label}
                  title={m.mode === 'list' ? '列表：鼠标移到一行上会弹出预览' : '图标：直接显示每个视频的画面'}
                  className={`seg-btn ${mode === m.mode ? 'is-on' : ''}`}
                  onClick={() => setMode(m.mode)}
                >
                  {m.icon}
                </button>
              ))}
            </div>
          )}
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

      {addingHere && (
        <div className="copying import-bar" role="status">
          <div className="copying-head">
            <span>
              正在添加 {num(Math.min((addingHere.progress?.done ?? 0) + 1, addingHere.progress?.total ?? 0))} / {num(addingHere.progress?.total ?? 0)}
            </span>
            <span className="tnum">{pct}%</span>
          </div>
          <div className="bar">
            <div style={{ width: `${pct}%` }} />
          </div>
          <div className="copying-foot">
            <span className="copying-file" title={addingHere.progress?.name}>
              {addingHere.progress?.name || '准备中…'}
            </span>
            <button className="btn btn-sm" onClick={cancelCopy} title="停止添加，这次已经放进来的视频也会撤掉">
              <X size={14} />
              取消
            </button>
          </div>
        </div>
      )}

      {error ? (
        <div className="empty-inline is-error">
          <p>{error}</p>
        </div>
      ) : files && files.length === 0 ? (
        <div className="empty-inline">
          <h2>这个分类里还没有视频</h2>
          <p>点「添加视频」选择文件复制进来，原文件会保留。也可以直接把视频放进这个分类的文件夹（不含子文件夹），回到这个窗口时会自动刷新。</p>
          {addButton(false)}
        </div>
      ) : mode === 'grid' ? (
        <div className="video-grid-wrap" aria-busy={!files}>
          <div className="video-grid" role="list" aria-label={`「${category.name}」里的视频`}>
            {rows.slice(0, MAX_ROWS).map(v => (
              <VideoTile key={v.name} category={category.name} v={v} onPlay={play} />
            ))}
          </div>
          {files && rows.length === 0 && <p className="video-none">没有文件名包含“{query.trim()}”的视频</p>}
          {rows.length > MAX_ROWS && <More shown={MAX_ROWS} total={rows.length} />}
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
          <div className="video-rows" onMouseLeave={leave} onScroll={() => setHover(null)}>
            {rows.slice(0, MAX_ROWS).map(v => (
              <VideoRow
                key={v.name}
                category={category.name}
                v={v}
                thumbs={thumbs}
                active={hover?.name === v.name}
                onPlay={play}
                onEnter={enter}
                onMove={track}
              />
            ))}
            {files && rows.length === 0 && <p className="video-none">没有文件名包含“{query.trim()}”的视频</p>}
            {rows.length > MAX_ROWS && <More shown={MAX_ROWS} total={rows.length} />}
          </div>
        </div>
      )}
      <p className="lib-note">
        {mode === 'grid' ? '双击一个视频用默认播放器打开。' : thumbs ? '鼠标移到一行上会弹出预览，双击用默认播放器打开。' : '双击一行用默认播放器打开。'}
        删除或改名请直接在文件夹里操作。
        {query.trim() && files && rows.length > 0 && ` 当前显示 ${num(rows.length)} / ${num(files.length)} 个。`}
      </p>
      {thumbs && mode === 'list' && hover && <HoverThumb popRef={pop} category={category.name} v={hover} place={place} />}
    </div>
  )
}

function More({ shown, total }: { shown: number; total: number }) {
  return (
    <p className="video-none">
      只列出前 {num(shown)} 个，还有 {num(total - shown)} 个没有显示。可以用上面的查找缩小范围。
    </p>
  )
}

function VideoRow({ category, v, thumbs, active, onPlay, onEnter, onMove }: {
  category: string
  v: VideoFile
  thumbs: boolean
  active: boolean
  onPlay: (v: VideoFile) => void
  onEnter: (v: VideoFile, e: React.MouseEvent<HTMLElement>) => void
  onMove: (e: React.MouseEvent<HTMLElement>) => void
}) {
  const [ref, visible] = useVisible<HTMLDivElement>()
  // 看得见的行提前在后台生成，鼠标移上来时就不用等。
  useThumb(category, v, thumbs && visible)
  return (
    <div
      ref={ref}
      className={`video-row ${active ? 'is-hover' : ''}`}
      role="row"
      onDoubleClick={() => onPlay(v)}
      onMouseEnter={thumbs ? e => onEnter(v, e) : undefined}
      onMouseMove={thumbs ? onMove : undefined}
    >
      <span className="video-name" role="cell" title={thumbs ? undefined : v.name}>
        {v.name}
      </span>
      <span className="ta-r tnum dim" role="cell">
        {bytes(v.size)}
      </span>
      <span className="ta-r dim" role="cell">
        {when(v.modified)}
      </span>
      <span role="cell">
        <button className="icon-btn sm" onClick={() => onPlay(v)} aria-label={`播放 ${v.name}`} title="用默认播放器打开（双击这一行也可以）">
          <Play size={14} />
        </button>
      </span>
    </div>
  )
}

function VideoTile({ category, v, onPlay }: { category: string; v: VideoFile; onPlay: (v: VideoFile) => void }) {
  const [ref, visible] = useVisible<HTMLDivElement>()
  const state = useThumb(category, v, visible)
  return (
    <div ref={ref} className="video-tile" role="listitem" onDoubleClick={() => onPlay(v)} title={`${v.name}\n${bytes(v.size)} · ${when(v.modified)}`}>
      <div className="tile-thumb">
        <Thumb state={state} v={v} />
        <button className="tile-play" onClick={() => onPlay(v)} aria-label={`播放 ${v.name}`} title="用默认播放器打开（双击也可以）">
          <Play size={15} />
        </button>
      </div>
      <div className="tile-name">{v.name}</div>
      <div className="tile-meta tnum">{bytes(v.size)}</div>
    </div>
  )
}

/** 缩略图本身：有就显示，正在生成时是一块微微发亮的底，生成不了时是一个胶片图标。 */
function Thumb({ state, v, onLoad }: { state: ThumbState; v: VideoFile; onLoad?: () => void }) {
  const [broken, setBroken] = useState(false)
  if (state === 'ready' && !broken) {
    return <img src={thumbUrl(v.key)} alt="" draggable={false} onLoad={onLoad} onError={() => setBroken(true)} />
  }
  const none = state === 'none' || broken
  return (
    <span className={`thumb-empty ${none ? '' : 'is-loading'}`}>
      <Film size={22} />
      {none && <span>没有预览</span>}
    </span>
  )
}

/** 列表里悬停时弹出的预览。位置由外面的 place 直接写到元素上，跟着鼠标走时不用重画。 */
function HoverThumb({ popRef, category, v, place }: {
  popRef: React.RefObject<HTMLDivElement | null>
  category: string
  v: VideoFile
  place: () => void
}) {
  const state = useThumb(category, v, true, true)
  // 换到另一行时，等新的图读出来再换上，中间不会闪一下空白。
  const [shown, setShown] = useState({ v, state })
  useEffect(() => {
    if (state !== 'ready') return setShown({ v, state })
    let stale = false
    const img = new Image()
    img.onload = img.onerror = () => {
      if (!stale) setShown({ v, state })
    }
    img.src = thumbUrl(v.key)
    return () => {
      stale = true
    }
  }, [v.key, state])
  useLayoutEffect(place, [shown])
  return createPortal(
    <div ref={popRef} className="thumb-pop" aria-hidden>
      <Thumb key={shown.v.key} state={shown.state} v={shown.v} onLoad={place} />
    </div>,
    document.body,
  )
}

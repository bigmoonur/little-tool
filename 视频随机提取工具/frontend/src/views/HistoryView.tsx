import { useEffect, useState } from 'react'
import { BookmarkPlus, FolderOpen, History, ListRestart, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { bytes, num, when } from '../lib/format'
import { modeInfo, picksLabel, picksToDraft } from '../lib/pick'
import type { HistoryEntry, Item } from '../types'

function groupItems(items: Item[]) {
  const map = new Map<string, Item[]>()
  for (const it of items) map.set(it.category, [...(map.get(it.category) ?? []), it])
  return [...map.entries()]
}

export function HistoryView() {
  const { historyVersion, run, toast, confirm, prompt, go, state, setState, setDraft, setRepeatMode, copy } = useApp()
  const [items, setItems] = useState<HistoryEntry[] | null>(null)
  const [selId, setSelId] = useState('')

  const load = () =>
    api.history().then(res => {
      const h = res ?? []
      setItems(h)
      setSelId(id => (h.some(x => x.id === id) ? id : h[0]?.id ?? ''))
    })

  useEffect(() => {
    load()
  }, [historyVersion])

  const sel = items?.find(x => x.id === selId)

  // 只带回还在素材根目录里的分类。
  const here = (h: HistoryEntry) => Object.fromEntries(Object.entries(h.picks).filter(([name]) => state.categories.some(c => c.name === name)))
  const missing = sel ? Object.keys(sel.picks).length - Object.keys(here(sel)).length : 0

  const reuse = () => {
    if (!sel) return
    setDraft(picksToDraft(here(sel)))
    setRepeatMode(modeInfo(sel.mode).mode)
    go('extract')
  }

  const saveAsPreset = async () => {
    if (!sel) return
    const name = await prompt({
      title: '保存为方案',
      label: '方案名称',
      placeholder: '例如：日常发布',
      confirmText: '保存',
      hint: '把这一次用的数量、乘数和规则固定下来，以后在抽取页的“方案”里一键填入。',
    })
    if (!name) return
    const existing = state.presets.find(p => p.name.toLowerCase() === name.toLowerCase())
    if (existing) {
      const ok = await confirm({ title: `替换方案「${existing.name}」？`, message: '已有同名方案，保存后会用这一次的参数替换它。', confirmText: '替换' })
      if (!ok) return
    }
    const res = await run(api.savePreset(name, sel.picks, sel.mode))
    if (res) {
      setState(res.state)
      toast(`已保存方案「${name}」`, { kind: 'ok' })
    }
  }

  const clear = async () => {
    const ok = await confirm({ title: '清空复制记录？', message: '只删除记录，已经复制的视频不受影响。', confirmText: '清空', danger: true })
    if (!ok) return
    if ((await run(api.clearHistory())) !== undefined) {
      load()
      toast('已清空复制记录', { kind: 'ok' })
    }
  }

  if (items && !items.length) {
    return (
      <div className="empty">
        <div className="empty-card">
          <History size={28} className="dim" />
          <h2>还没有复制记录</h2>
          <p className="empty-lead">每次复制完成后，会在这里保留放入的文件夹和视频清单（最近 50 次）。</p>
          <button className="btn btn-primary btn-lg" onClick={() => go('extract')}>
            去抽取
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="history">
      <nav className="hist-side" aria-label="复制记录">
        <div className="hist-side-head">
          <span>最近 {items?.length ?? 0} 次</span>
          <button className="btn btn-ghost is-danger btn-sm" onClick={clear}>
            <Trash2 size={14} />
            清空
          </button>
        </div>
        <ul className="hist-list">
          {items?.map(h => (
            <li key={h.id}>
              <button className={`hist-item ${h.id === selId ? 'is-selected' : ''}`} onClick={() => setSelId(h.id)}>
                <span className="hist-when">{when(h.time)}</span>
                <span className="hist-total tnum">{num(h.items.length)} 个</span>
                <span className="hist-parts">
                  放入「{h.folder}」 · {picksLabel(h.picks, h.mode)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </nav>
      {sel && (
        <section className="hist-detail">
          <header className="detail-head">
            <div className="detail-title">
              <h1>
                {when(sel.time)} · {num(sel.items.length)} 个视频
              </h1>
            </div>
            <div className="detail-meta">
              <span>{picksLabel(sel.picks, sel.mode)}</span>
              <span>放入「{sel.folder}」</span>
              <span>{bytes(sel.bytes)}</span>
              {sel.removed > 0 && <span>清空了 {num(sel.removed)} 个旧视频</span>}
            </div>
            <div className="detail-path dim" title={sel.destination}>
              {sel.destination}
            </div>
            <div className="detail-actions">
              <button className="btn btn-primary" onClick={() => run(api.openFolder(sel.destination))}>
                <FolderOpen size={16} />
                打开文件夹
              </button>
              <button className="btn" onClick={reuse} disabled={copy.phase === 'copying'} title="把这一次的数量、乘数和规则填到抽取页，再抽一批">
                <ListRestart size={16} />
                按这组参数再抽一次
              </button>
              <button className="btn" onClick={saveAsPreset} title="给这组参数起个名字固定下来，以后一键填入">
                <BookmarkPlus size={16} />
                保存为方案
              </button>
              {missing > 0 && <span className="dim">其中 {missing} 个分类已不在素材根目录里</span>}
            </div>
          </header>
          <ul className="file-list hist-files">
            {groupItems(sel.items).map(([cat, list]) => (
              <li key={cat}>
                <div className="file-cat">
                  {cat} <span className="dim">{list.length}</span>
                </div>
                {list.map(it => (
                  <div key={it.name} className="file-name" title={it.origin ? `${it.name}（${it.origin} 的副本）` : it.name}>
                    <span>{it.name}</span>
                    <span className="dim tnum">{bytes(it.size)}</span>
                  </div>
                ))}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}

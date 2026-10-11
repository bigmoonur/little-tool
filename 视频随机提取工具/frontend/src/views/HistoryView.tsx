import { useEffect, useState } from 'react'
import { FolderOpen, History, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { bytes, num, when } from '../lib/format'
import type { HistoryEntry, Item } from '../types'

function groupItems(items: Item[]) {
  const map = new Map<string, Item[]>()
  for (const it of items) map.set(it.category, [...(map.get(it.category) ?? []), it])
  return [...map.entries()]
}

export function HistoryView() {
  const { historyVersion, run, toast, confirm, go } = useApp()
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
                <span className="hist-parts">放入「{h.folder}」</span>
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
            </div>
          </header>
          <ul className="file-list hist-files">
            {groupItems(sel.items).map(([cat, list]) => (
              <li key={cat}>
                <div className="file-cat">
                  {cat} <span className="dim">{list.length}</span>
                </div>
                {list.map(it => (
                  <div key={it.name} className="file-name" title={it.name}>
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

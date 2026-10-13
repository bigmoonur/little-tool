import { useEffect, useState } from 'react'
import { ClipboardCopy, FileDown, History, ListRestart, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { num, partLabel, stamp, when } from '../lib/format'
import { picksMode, picksToDraft, ruleNote } from '../lib/pick'
import type { HistoryEntry, Pick } from '../types'

export function HistoryView() {
  const { historyVersion, run, toast, confirm, setDraft, setRepeatMode, go, snap, bumpHistory } = useApp()
  const [items, setItems] = useState<HistoryEntry[] | null>(null)
  const [selId, setSelId] = useState('')
  const [text, setText] = useState<{ id: string; text: string | null; error?: string } | null>(null)

  useEffect(() => {
    api.history().then(h => {
      setItems(h)
      setSelId(id => (h.some(x => x.id === id) ? id : h[0]?.id ?? ''))
    })
  }, [historyVersion])

  const sel = items?.find(x => x.id === selId)

  useEffect(() => {
    if (!sel) return setText(null)
    if (sel.truncated) return setText({ id: sel.id, text: null, error: '这次抽取的内容过大，没有保存在记录中。' })
    setText({ id: sel.id, text: null })
    api.historyText(sel.id).then(t => setText({ id: sel.id, text: t }))
  }, [selId, items])

  const copy = async () => {
    if (!text?.text) return
    if (!(await run(api.copyText(text.text)))) return
    setDraft({})
    await api.saveDraft({}).catch(() => {})
    toast(`已复制 ${num(sel!.total)} 条`, { kind: 'ok' })
  }

  const save = async () => {
    if (!text?.text || !sel) return
    const path = await run(api.saveTextFile(`抽取结果-${stamp(sel.time)}`, text.text))
    if (path) toast('已保存为 TXT', { kind: 'ok' })
  }

  const reuse = () => {
    if (!sel) return
    const picks: Record<string, Pick> = {}
    for (const p of sel.parts) if (snap.lists.some(l => l.id === p.listId)) picks[p.listId] = { count: p.count, repeat: p.repeat }
    // 乘数规则一起带回去。
    setDraft(picksToDraft(picks))
    setRepeatMode(picksMode(sel.parts))
    go('extract')
  }

  const clear = async () => {
    const ok = await confirm({ title: '清空抽取记录？', message: '所有记录会被删除，素材库不受影响。', confirmText: '清空', danger: true })
    if (!ok) return
    if ((await run(api.clearHistory())) !== undefined) {
      bumpHistory()
      toast('已清空抽取记录', { kind: 'ok' })
    }
  }

  if (items && !items.length) {
    return (
      <div className="empty">
        <div className="empty-card">
          <History size={28} className="dim" />
          <h2>还没有抽取记录</h2>
          <p className="empty-lead">每次抽取的结果会保存在这里（最近 30 次），复制被覆盖了也能找回来。</p>
          <button className="btn btn-primary btn-lg" onClick={() => go('extract')}>
            去抽取
          </button>
        </div>
      </div>
    )
  }

  const lines = text?.text ? text.text.split('\n', 500) : []
  const missing = sel ? sel.parts.filter(p => !snap.lists.some(l => l.id === p.listId)).length : 0
  /** “上衣 25×4、裤子 10 · 循环重复” */
  const summary = (h: HistoryEntry) => [h.parts.map(partLabel).join('、'), ruleNote(h.parts)].filter(Boolean).join(' · ')

  return (
    <div className="history">
      <nav className="hist-side" aria-label="抽取记录">
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
                <span className="hist-total tnum">{num(h.total)} 条</span>
                <span className="hist-parts">{summary(h)}</span>
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
                {when(sel.time)} · {num(sel.total)} 条
              </h1>
            </div>
            <div className="detail-meta">{summary(sel)}</div>
            <div className="detail-actions">
              <button className="btn btn-primary" onClick={copy} disabled={!text?.text}>
                <ClipboardCopy size={16} />
                复制
              </button>
              <button className="btn" onClick={save} disabled={!text?.text}>
                <FileDown size={16} />
                保存为 TXT
              </button>
              <button className="btn" onClick={reuse} title="把这次的数量和乘数规则填到抽取页，重新抽一批">
                <ListRestart size={16} />
                按这组数量再抽一次
              </button>
              {missing > 0 && <span className="dim">其中 {missing} 个分类已删除</span>}
            </div>
          </header>
          <div className="hist-text">
            {text?.error ? (
              <p className="dim">{text.error}</p>
            ) : (
              <ol className="result-lines">
                {lines.map((l, i) => (
                  <li key={i}>{l}</li>
                ))}
              </ol>
            )}
            {sel.total > 500 && text?.text && <p className="result-more">仅显示前 500 行，复制和保存的是全部 {num(sel.total)} 行</p>}
          </div>
        </section>
      )}
    </div>
  )
}

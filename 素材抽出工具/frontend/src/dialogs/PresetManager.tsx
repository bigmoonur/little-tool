import { Pencil, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { num, partLabel } from '../lib/format'
import { ruleNote } from '../lib/pick'
import { Modal } from '../ui/Modal'

export function PresetManager({ onClose }: { onClose: () => void }) {
  const { snap, setSnap, run, prompt, commit } = useApp()

  const rename = async (id: string, current: string) => {
    const name = await prompt({ title: '重命名方案', label: '方案名称', initial: current, confirmText: '重命名' })
    if (!name || name === current) return
    const s = await run(api.renamePreset(id, name))
    if (s) setSnap(s)
  }

  const remove = async (id: string, name: string) => {
    const s = await run(api.deletePreset(id))
    if (s) commit(s, `已删除方案「${name}」`)
  }

  return (
    <Modal
      title="管理方案"
      width={560}
      onClose={onClose}
      footer={
        <button className="btn btn-primary" onClick={onClose}>
          完成
        </button>
      }
    >
      {snap.presets.length === 0 ? (
        <p className="dim">还没有方案。在抽取页填好数量后，可以从“方案”菜单保存，乘数规则会一起保存。</p>
      ) : (
        <ul className="preset-list">
          {snap.presets.map(p => {
            const parts = Object.entries(p.picks).flatMap(([id, v]) => {
              const l = snap.lists.find(x => x.id === id)
              return l ? [{ name: l.name, count: v.count, repeat: v.repeat }] : []
            })
            const total = parts.reduce((s, x) => s + x.count * Math.max(1, x.repeat), 0)
            const note = ruleNote(Object.values(p.picks))
            return (
              <li key={p.id}>
                <div className="preset-info">
                  <b>{p.name}</b>
                  <span className="dim">
                    {num(total)} 条 · {parts.map(partLabel).join('、') || '分类已全部删除'}
                  </span>
                  {note && <span className="preset-rule">{note}</span>}
                </div>
                <button className="icon-btn" onClick={() => rename(p.id, p.name)} aria-label={`重命名方案 ${p.name}`} title="重命名">
                  <Pencil size={16} />
                </button>
                <button className="icon-btn is-danger" onClick={() => remove(p.id, p.name)} aria-label={`删除方案 ${p.name}`} title="删除">
                  <Trash2 size={16} />
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
  )
}

import { Pencil, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import { picksLabel } from '../lib/pick'
import type { Preset } from '../types'
import { Modal } from '../ui/Modal'

export function PresetManager({ onClose }: { onClose: () => void }) {
  const { state, setState, run, prompt, confirm } = useApp()

  const rename = async (p: Preset) => {
    const name = await prompt({ title: '重命名方案', label: '方案名称', initial: p.name, confirmText: '重命名' })
    if (!name || name === p.name) return
    const s = await run(api.renamePreset(p.id, name))
    if (s) setState(s)
  }

  const remove = async (p: Preset) => {
    const ok = await confirm({ title: `删除方案「${p.name}」？`, message: '只删除这组保存的参数，素材和复制记录不受影响。', confirmText: '删除', danger: true })
    if (!ok) return
    const s = await run(api.deletePreset(p.id))
    if (s) setState(s)
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
      {state.presets.length === 0 ? (
        <p className="dim">还没有方案。在抽取页填好数量后，可以从“方案”菜单保存；也可以在记录页把用过的一组参数保存为方案。</p>
      ) : (
        <ul className="preset-list">
          {state.presets.map(p => {
            const here = Object.fromEntries(Object.entries(p.picks).filter(([name]) => state.categories.some(c => c.name === name)))
            const total = Object.values(here).reduce((s, v) => s + v.count * Math.max(1, v.repeat), 0)
            return (
              <li key={p.id}>
                <div className="preset-info">
                  <b>{p.name}</b>
                  <span className="dim">
                    {num(total)} 个 · {picksLabel(here, p.mode) || '分类已全部不在素材根目录里'}
                  </span>
                </div>
                <button className="icon-btn" onClick={() => rename(p)} aria-label={`重命名方案 ${p.name}`} title="重命名">
                  <Pencil size={15} />
                </button>
                <button className="icon-btn is-danger" onClick={() => remove(p)} aria-label={`删除方案 ${p.name}`} title="删除">
                  <Trash2 size={15} />
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
  )
}

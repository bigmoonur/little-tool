import { useState } from 'react'
import { Check, Pencil, Plus, Trash2 } from 'lucide-react'
import { api, errorText } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import { TAG_COLORS, type Tag } from '../types'
import { Modal } from '../ui/Modal'
import { anchorOf, Popover, type Anchor } from '../ui/Popover'

const COLOR_NAMES: Record<string, string> = {
  indigo: '靛蓝', teal: '青绿', amber: '琥珀', rose: '玫红', violet: '紫', green: '绿', orange: '橙', slate: '灰',
}

/** 输入名称直接新建标签；重名等错误显示在输入框下面。compact 用在小面板里。 */
export function NewTag({ onCreated, compact = false }: { onCreated?: (id: string) => void; compact?: boolean }) {
  const { setState, copy, adding } = useApp()
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const disabled = busy || copy.phase === 'copying' || !!adding

  const create = async () => {
    if (disabled) return
    if (!name.trim()) return setError('请先填写标签名称')
    setBusy(true)
    try {
      const res = await api.createTag(name.trim())
      setState(res.state)
      setName('')
      setError('')
      onCreated?.(res.id)
    } catch (e) {
      setError(errorText(e))
    }
    setBusy(false)
  }

  return (
    <div className={`tag-new ${compact ? 'is-compact' : ''}`}>
      <div className="tag-new-row">
        <input
          className={`input ${error ? 'is-invalid' : ''}`}
          value={name}
          disabled={disabled}
          maxLength={40}
          placeholder={compact ? '新标签的名称' : '新标签的名称，例如：开场、常用'}
          aria-label="新标签的名称"
          onChange={e => {
            setName(e.target.value)
            setError('')
          }}
          onKeyDown={e => {
            if (e.key === 'Enter') {
              e.preventDefault()
              create()
            }
          }}
        />
        <button className="btn" onClick={create} disabled={disabled}>
          <Plus size={16} />
          {busy ? '正在新建…' : compact ? '新建' : '新建标签'}
        </button>
      </div>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

/** 全部标签：新建、改名、换颜色、删除。标签贴在分类上，在抽取页的分类名称旁边贴。 */
export function TagManager({ onClose }: { onClose: () => void }) {
  const { state, setState, prompt, confirm, run, copy, adding } = useApp()
  const [busy, setBusy] = useState('')
  const disabled = !!busy || copy.phase === 'copying' || !!adding
  const [coloring, setColoring] = useState<{ tag: Tag; anchor: Anchor } | null>(null)
  const used = (id: string) => state.categories.filter(c => c.tagIds.includes(id)).length

  const act = async (id: string, job: Promise<typeof state>) => {
    setBusy(id)
    const s = await run(job)
    setBusy('')
    if (s) setState(s)
  }

  const rename = async (t: Tag) => {
    const name = await prompt({
      title: '修改标签名称',
      label: '标签名称',
      initial: t.name,
      confirmText: '保存',
      hint: '只改名称，已经贴上这个标签的分类不受影响。',
    })
    if (name && name !== t.name) await act(t.id, api.renameTag(t.id, name))
  }

  const remove = async (t: Tag) => {
    const n = used(t.id)
    const ok = await confirm({
      title: `删除标签「${t.name}」？`,
      message: `${n > 0 ? `有 ${num(n)} 个分类贴着这个标签。` : ''}分类和里面的视频都不受影响，只是不再带这个标签。`,
      confirmText: '删除标签',
      danger: true,
    })
    if (ok) await act(t.id, api.deleteTag(t.id))
  }

  return (
    <>
      <Modal
        title="管理标签"
        width={560}
        onClose={onClose}
        className="tag-manager"
        footer={
          <button className="btn btn-primary" onClick={onClose}>
            完成
          </button>
        }
      >
        <p className="tag-help">标签贴在分类上，用来在抽取页快速找到要用的分类：点一下标签，只显示带这个标签的分类。在分类名称旁边就能贴。</p>
        <NewTag />
        {state.tags.length === 0 ? (
          <p className="tag-empty">还没有标签。在上面填写名称，新建第一个标签。</p>
        ) : (
          <ul className="tag-catalog">
            {state.tags.map(t => {
              const n = used(t.id)
              return (
                <li key={t.id}>
                  <button
                    className={`swatch tag-color c-${t.color}`}
                    onClick={e => setColoring({ tag: t, anchor: anchorOf(e.currentTarget) })}
                    disabled={disabled}
                    aria-label={`标签 ${t.name} 的颜色：${COLOR_NAMES[t.color]}，点一下更换`}
                    title="更换颜色"
                  />
                  <div className="tag-catalog-main">
                    <b title={t.name}>{t.name}</b>
                    <span className="dim">{n > 0 ? `贴在 ${num(n)} 个分类上` : '还没有分类使用'}</span>
                  </div>
                  <button className="btn btn-sm" onClick={() => rename(t)} disabled={disabled} aria-label={`修改标签 ${t.name} 的名称`}>
                    <Pencil size={14} />
                    改名
                  </button>
                  <button className="btn btn-sm is-danger" onClick={() => remove(t)} disabled={disabled} aria-label={`删除标签 ${t.name}`}>
                    <Trash2 size={14} />
                    删除
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </Modal>
      {coloring && (
        <Popover anchor={coloring.anchor} onClose={() => setColoring(null)} width={232} label={`「${coloring.tag.name}」的颜色`}>
          <div className="pop-title">「{coloring.tag.name}」的颜色</div>
          <div className="swatches" role="group" aria-label="标签颜色">
            {TAG_COLORS.map(c => (
              <button
                key={c}
                className={`swatch c-${c} ${coloring.tag.color === c ? 'is-on' : ''}`}
                title={COLOR_NAMES[c]}
                aria-label={`颜色：${COLOR_NAMES[c]}`}
                disabled={disabled}
                onClick={() => {
                  const id = coloring.tag.id
                  setColoring(null)
                  act(id, api.setTagColor(id, c))
                }}
              >
                {coloring.tag.color === c && <Check size={12} />}
              </button>
            ))}
          </div>
        </Popover>
      )}
    </>
  )
}

import { useState, type ReactNode } from 'react'
import { Check, Pencil, Plus, Trash2 } from 'lucide-react'
import { api, errorText } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import { useTagTotals } from '../lib/tags'
import { GROUP_COLORS, type Tag } from '../types'
import { anchorOf, useMenu } from '../ui/Menu'
import { Modal } from '../ui/Modal'

export const COLOR_NAMES: Record<string, string> = {
  indigo: '靛蓝', teal: '青绿', amber: '琥珀', rose: '玫红', violet: '紫', green: '绿', orange: '橙', slate: '灰',
}

/** 输入名称直接新建标签；重名等错误显示在输入框下面。compact 用在小面板里。 */
export function NewTag({ onCreated, compact = false }: { onCreated?: (id: string) => void; compact?: boolean }) {
  const { setSnap } = useApp()
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const create = async () => {
    if (busy) return
    if (!name.trim()) return setError('请先填写标签名称')
    setBusy(true)
    try {
      const res = await api.createTag(name.trim())
      setSnap(res.snapshot)
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
          maxLength={40}
          placeholder={compact ? '新标签的名称' : '新标签的名称，例如：新品、常用'}
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
        <button className="btn" onClick={create} disabled={busy}>
          <Plus size={18} />
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

/** 全部标签：新建、改名、换颜色、删除。标签贴在分类上，在素材库或抽取页里贴。 */
export function TagManager({ onClose }: { onClose: () => void }) {
  const { snap, prompt, confirm, run, setSnap, commit } = useApp()
  const usage = useTagTotals()
  const [busy, setBusy] = useState('')
  const menu = useMenu()

  const pickColor = (el: HTMLElement, t: Tag) =>
    menu.open(
      anchorOf(el),
      [
        { heading: `「${t.name}」的颜色` },
        {
          custom: (
            <div className="swatches" role="group" aria-label="标签颜色">
              {GROUP_COLORS.map(c => (
                <button
                  key={c}
                  className={`swatch c-${c} ${t.color === c ? 'is-on' : ''}`}
                  title={COLOR_NAMES[c]}
                  aria-label={`颜色：${COLOR_NAMES[c]}`}
                  onClick={async () => {
                    menu.close()
                    const s = await run(api.setTagColor(t.id, c))
                    if (s) setSnap(s)
                  }}
                >
                  {t.color === c && <Check size={12} />}
                </button>
              ))}
            </div>
          ),
        },
      ],
      220,
    )

  const rename = async (t: Tag) => {
    const name = await prompt({
      title: '修改标签名称',
      label: '标签名称',
      initial: t.name,
      confirmText: '保存',
      hint: '只改名称，已经贴上这个标签的分类不受影响。',
    })
    if (!name || name === t.name) return
    setBusy(t.id)
    const s = await run(api.renameTag(t.id, name))
    setBusy('')
    if (s) setSnap(s)
  }

  const remove = async (t: Tag) => {
    const used = usage.get(t.id) ?? 0
    const ok = await confirm({
      title: `删除标签「${t.name}」？`,
      message: `${used > 0 ? `有 ${num(used)} 个分类贴着这个标签。` : ''}分类和里面的素材都会保留，只是不再带这个标签。删除后可以马上撤销。`,
      confirmText: '删除标签',
      danger: true,
    })
    if (!ok) return
    setBusy(t.id)
    const s = await run(api.deleteTag(t.id))
    setBusy('')
    if (s) commit(s, `已删除标签「${t.name}」`)
  }

  return (
    <>
    <Modal
      title="管理标签"
      width={600}
      onClose={onClose}
      className="tag-manager"
      footer={
        <button className="btn btn-primary btn-lg" onClick={onClose}>
          完成
        </button>
      }
    >
      <p className="tag-help">标签贴在分类上，用来在抽取页快速找到要用的分类：点一下标签，只显示带这个标签的分类。在分类名称旁边就能贴。</p>
      <NewTag />
      {snap.tags.length === 0 ? (
        <p className="tag-empty">还没有标签。在上面填写名称，新建第一个标签。</p>
      ) : (
        <ul className="tag-catalog">
          {snap.tags.map(t => {
            const n = usage.get(t.id) ?? 0
            return (
              <li key={t.id}>
                <button
                  className={`swatch tag-color c-${t.color}`}
                  onClick={e => pickColor(e.currentTarget, t)}
                  disabled={!!busy}
                  aria-label={`标签 ${t.name} 的颜色：${COLOR_NAMES[t.color]}，点一下更换`}
                  title="更换颜色"
                />
                <div className="tag-catalog-main">
                  <b title={t.name}>{t.name}</b>
                  <span className="dim">{n > 0 ? `贴在 ${num(n)} 个分类上` : '还没有分类使用'}</span>
                </div>
                <button className="btn" onClick={() => rename(t)} disabled={!!busy} aria-label={`修改标签 ${t.name} 的名称`}>
                  <Pencil size={16} />
                  改名
                </button>
                <button className="btn is-danger" onClick={() => remove(t)} disabled={!!busy} aria-label={`删除标签 ${t.name}`}>
                  <Trash2 size={16} />
                  删除
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
    {menu.node}
    </>
  )
}

/** 在弹窗里选择只看哪些标签的分类：抽取页标签太多、一行放不下时用它。 */
export function TagPicker({ title, help, initial, noneLabel, onApply, onClose }: {
  title: string
  help?: ReactNode
  initial: string[]
  /** 一个都不选时的含义，例如“全部文本”。 */
  noneLabel: string
  onApply: (ids: string[]) => void
  onClose: () => void
}) {
  const { snap } = useApp()
  const usage = useTagTotals()
  const [selected, setSelected] = useState<string[]>(initial)
  const [touched, setTouched] = useState(false)
  const [managing, setManaging] = useState(false)
  // 已删除的标签不算在选择里。
  const valid = selected.filter(id => snap.tags.some(t => t.id === id))
  // 原来选的标签全都被删除了：不替用户默认成“一个都不选”（那会悄悄扩大范围），必须亲手重新选一次。
  const lost = !touched && initial.length > 0 && valid.length === 0
  const choose = (next: (s: string[]) => string[]) => {
    setTouched(true)
    setSelected(next)
  }
  const toggle = (id: string) => choose(s => (s.includes(id) ? s.filter(x => x !== id) : [...s, id]))
  const noneOn = !valid.length && !lost

  return (
    <>
      <Modal
        title={title}
        width={640}
        onClose={onClose}
        className="tag-picker"
        initialFocus=".tag-choice"
        footer={
          <>
            <span className={`tag-picked ${lost ? 'err-text' : ''}`}>
              {lost ? '原来选的标签已被删除，请重新选择' : valid.length ? `已选 ${valid.length} 个标签` : `当前：${noneLabel}`}
            </span>
            <button className="btn btn-lg" onClick={onClose}>
              取消
            </button>
            <button className="btn btn-primary btn-lg" onClick={() => onApply(valid)} disabled={lost}>
              确定
            </button>
          </>
        }
      >
        {help && <p className="tag-help">{help}</p>}
        <div className="tag-choices" role="group" aria-label="选择标签">
          <button className={`tag-choice is-none ${noneOn ? 'is-on' : ''}`} aria-pressed={noneOn} onClick={() => choose(() => [])}>
            <span className="tag-check">{noneOn && <Check size={16} />}</span>
            <span className="tag-choice-name">{noneLabel}</span>
          </button>
          {snap.tags.map(t => {
            const on = valid.includes(t.id)
            return (
              <button key={t.id} className={`tag-choice c-${t.color} ${on ? 'is-on' : ''}`} aria-pressed={on} onClick={() => toggle(t.id)}>
                <span className="tag-check">{on && <Check size={16} />}</span>
                <i className="tag-dot" aria-hidden />
                <span className="tag-choice-name">{t.name}</span>
                <span className="tag-choice-n">{num(usage.get(t.id) ?? 0)} 个分类</span>
              </button>
            )
          })}
        </div>
        <button className="link tag-manage-link" onClick={() => setManaging(true)}>
          新建或管理标签…
        </button>
      </Modal>
      {managing && <TagManager onClose={() => setManaging(false)} />}
    </>
  )
}

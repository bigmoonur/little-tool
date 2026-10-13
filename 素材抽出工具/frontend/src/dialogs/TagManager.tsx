import { useEffect, useState, type ReactNode } from 'react'
import { Check, Pencil, Plus, Tag as TagIcon, Trash2 } from 'lucide-react'
import { api, errorText } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import type { Tag } from '../types'
import { Modal } from '../ui/Modal'

/** 每个标签贴在多少条文本上。逐个标签向后端统计，标签或素材变化后重新计算；统计不出来时不显示数字。 */
export function useTagUsage() {
  const { snap } = useApp()
  const key = `${snap.tags.map(t => t.id).join(',')}|${snap.lists.map(l => `${l.id}:${l.updatedAt}`).join(',')}`
  const [usage, setUsage] = useState<{ key: string; counts: Map<string, number> } | null>(null)
  useEffect(() => {
    let alive = true
    Promise.all(snap.tags.map(async t => [t.id, (await api.availableLists([t.id])).reduce((s, l) => s + l.count, 0)] as const)).then(
      pairs => alive && setUsage({ key, counts: new Map(pairs) }),
      () => {},
    )
    return () => {
      alive = false
    }
  }, [key])
  return usage?.key === key ? usage.counts : null
}

/** 输入名称直接新建标签；重名等错误显示在输入框下面。 */
function NewTag({ onCreated }: { onCreated?: (id: string) => void }) {
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
    <div className="tag-new">
      <div className="tag-new-row">
        <input
          className={`input ${error ? 'is-invalid' : ''}`}
          value={name}
          maxLength={40}
          placeholder="新标签的名称，例如：新品、常用"
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
        <button className="btn btn-tags" onClick={create} disabled={busy}>
          <Plus size={18} />
          {busy ? '正在新建…' : '新建标签'}
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

/** 全局标签：新建、改名、删除。标签贴在每一条文本上，在分类的“文本标签”里设置。 */
export function TagManager({ onClose }: { onClose: () => void }) {
  const { snap, prompt, confirm, run, setSnap, commit, tagFilter } = useApp()
  const usage = useTagUsage()
  const [busy, setBusy] = useState('')

  const rename = async (t: Tag) => {
    const name = await prompt({
      title: '修改标签名称',
      label: '标签名称',
      initial: t.name,
      confirmText: '保存',
      hint: '只改名称，已经贴上这个标签的文本不受影响。',
    })
    if (!name || name === t.name) return
    setBusy(t.id)
    const s = await run(api.renameTag(t.id, name))
    setBusy('')
    if (s) setSnap(s)
  }

  const remove = async (t: Tag) => {
    const used = usage?.get(t.id) ?? 0
    const inUse = [
      ...(tagFilter.includes(t.id) ? ['抽取页正在按这个标签筛选'] : []),
      ...(snap.presets.some(p => Object.values(p.picks).some(v => v.tagIds?.includes(t.id))) ? ['有方案用到了这个标签'] : []),
    ]
    const ok = await confirm({
      title: `删除标签「${t.name}」？`,
      message: (
        <>
          {used > 0 && `有 ${num(used)} 条文本贴着这个标签。`}文本本身会保留，只是不再带这个标签。删除后可以马上撤销。
          {inUse.length > 0 && (
            <p className="warn-text">{inUse.join('，')}。删除后不会自动改成抽取全部文本，需要重新选择标签才能继续抽取。</p>
          )}
        </>
      ),
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
      <p className="tag-help">
        标签贴在每一条文本上。先在这里建好标签，再到素材库某个分类的“文本标签”里给文本贴上；抽取时可以用“标签筛选”只抽带某些标签的文本。
      </p>
      <NewTag />
      {snap.tags.length === 0 ? (
        <p className="tag-empty">还没有标签。在上面填写名称，新建第一个标签。</p>
      ) : (
        <ul className="tag-catalog">
          {snap.tags.map(t => {
            const n = usage?.get(t.id)
            return (
              <li key={t.id}>
                <TagIcon size={18} className="tag-catalog-icon" />
                <div className="tag-catalog-main">
                  <b title={t.name}>{t.name}</b>
                  <span className="dim">{n === undefined ? ' ' : n > 0 ? `贴在 ${num(n)} 条文本上` : '还没有文本使用'}</span>
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
  )
}

/** 选择一个或多个标签：抽取页的标签筛选、单条文本的标签、批量添加或移除都用它。 */
export function TagPicker({ title, help, initial, only, noneLabel, canCreate = false, confirmText = '确定', busy = false, onApply, onClose }: {
  title: string
  help?: ReactNode
  initial: string[]
  /** 只列出这些标签（批量移除时只列出选中文本上已有的标签）。 */
  only?: string[]
  /** 一个都不选时的含义，例如“全部文本”“不设标签”；不提供时必须至少选一个。 */
  noneLabel?: string
  /** 允许在这里直接新建标签，新建后自动选中。 */
  canCreate?: boolean
  confirmText?: string
  busy?: boolean
  onApply: (ids: string[]) => void
  onClose: () => void
}) {
  const { snap } = useApp()
  const usage = useTagUsage()
  const [selected, setSelected] = useState<string[]>(initial)
  const [touched, setTouched] = useState(false)
  const [managing, setManaging] = useState(false)
  const tags = only ? snap.tags.filter(t => only.includes(t.id)) : snap.tags
  // 已删除的标签不算在选择里。
  const valid = selected.filter(id => tags.some(t => t.id === id))
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
        onClose={() => !busy && onClose()}
        className="tag-picker"
        initialFocus=".tag-choice"
        footer={
          <>
            <span className={`tag-picked ${lost ? 'err-text' : ''}`}>
              {lost ? '原来选的标签已被删除，请重新选择' : valid.length ? `已选 ${valid.length} 个标签` : noneLabel ? `当前：${noneLabel}` : '请至少选一个标签'}
            </span>
            <button className="btn btn-lg" onClick={onClose} disabled={busy}>
              取消
            </button>
            <button className="btn btn-primary btn-lg" onClick={() => onApply(valid)} disabled={busy || lost || (!noneLabel && !valid.length)}>
              {busy ? '正在保存…' : confirmText}
            </button>
          </>
        }
      >
        {help && <p className="tag-help">{help}</p>}
        {(tags.length > 0 || noneLabel) && (
          <div className="tag-choices" role="group" aria-label="选择标签">
            {noneLabel && (
              <button className={`tag-choice is-none ${noneOn ? 'is-on' : ''}`} aria-pressed={noneOn} onClick={() => choose(() => [])}>
                <span className="tag-check">{noneOn && <Check size={16} />}</span>
                <span className="tag-choice-name">{noneLabel}</span>
              </button>
            )}
            {tags.map(t => {
              const on = valid.includes(t.id)
              const n = usage?.get(t.id)
              return (
                <button key={t.id} className={`tag-choice ${on ? 'is-on' : ''}`} aria-pressed={on} onClick={() => toggle(t.id)}>
                  <span className="tag-check">{on && <Check size={16} />}</span>
                  <span className="tag-choice-name">{t.name}</span>
                  {n !== undefined && <span className="tag-choice-n">{num(n)} 条</span>}
                </button>
              )
            })}
          </div>
        )}
        {!tags.length && (
          <p className="tag-empty">
            {only ? '选中的文本上没有可以移除的标签。' : canCreate ? '还没有标签。在下面填写名称，新建第一个标签。' : '还没有标签。先新建标签，再到素材库某个分类的“文本标签”里给文本贴上。'}
          </p>
        )}
        {canCreate && <NewTag onCreated={id => choose(s => [...s, id])} />}
        {!only && (
          <button className="link tag-manage-link" onClick={() => setManaging(true)}>
            {canCreate ? '改名或删除标签…' : '新建或管理标签…'}
          </button>
        )}
      </Modal>
      {managing && <TagManager onClose={() => setManaging(false)} />}
    </>
  )
}

import { Check } from 'lucide-react'
import { useApp } from '../ctx'
import type { MenuAnchor } from '../ui/Menu'
import { Popover } from '../ui/Popover'
import { NewTag } from './TagManager'

/** 给一个分类贴标签的小面板：点一下标签立即贴上或取下，不用再点保存。 */
export function TagPopover({ anchor, title, checked, locked = false, onToggle, onManage, onClose }: {
  anchor: MenuAnchor
  title: string
  checked: ReadonlySet<string>
  /** 正在保存时暂时不能点。 */
  locked?: boolean
  onToggle: (tagId: string, on: boolean) => void
  onManage: () => void
  onClose: () => void
}) {
  const { snap } = useApp()

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const nodes = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('.tag-pop-item:not([disabled])'))
    const i = nodes.indexOf(document.activeElement as HTMLElement)
    if (i < 0) return
    e.preventDefault()
    nodes[(i + (e.key === 'ArrowDown' ? 1 : nodes.length - 1)) % nodes.length]?.focus()
  }

  return (
    <Popover anchor={anchor} onClose={onClose} width={300} label={title} className="tag-pop">
      <div className="pop-title">{title}</div>
      {snap.tags.length > 0 ? (
        <div className="tag-pop-list" role="group" aria-label="标签" onKeyDown={onKeyDown}>
          {snap.tags.map(t => {
            const on = checked.has(t.id)
            return (
              <button
                key={t.id}
                className={`tag-pop-item c-${t.color} ${on ? 'is-on' : ''}`}
                role="checkbox"
                aria-checked={on}
                disabled={locked}
                onClick={() => onToggle(t.id, !on)}
              >
                <span className="tag-check">{on && <Check size={16} />}</span>
                <i className="tag-dot" aria-hidden />
                <span className="menu-label">{t.name}</span>
              </button>
            )
          })}
        </div>
      ) : (
        <p className="pop-empty">还没有标签，在下面新建第一个。</p>
      )}
      <NewTag compact onCreated={id => onToggle(id, true)} />
      {snap.tags.length > 0 && (
        <button className="link pop-link" onClick={onManage}>
          改名、换颜色或删除标签…
        </button>
      )}
    </Popover>
  )
}

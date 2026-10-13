import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

export type MenuItem =
  | {
      label: string
      icon?: ReactNode
      onSelect: () => void
      danger?: boolean
      disabled?: boolean
      checked?: boolean
      hint?: string
      trailing?: ReactNode
    }
  | { divider: true }
  | { heading: string }
  | { custom: ReactNode }

export interface MenuAnchor {
  x: number
  y: number
  /** 锚点元素的底边和右边，用于向上翻转或右对齐。 */
  top?: number
  right?: number
  align?: 'start' | 'end'
}

export function anchorOf(el: Element, align: 'start' | 'end' = 'start'): MenuAnchor {
  const r = el.getBoundingClientRect()
  return { x: r.left, y: r.bottom + 4, top: r.top - 4, right: r.right, align }
}

export function Menu({ anchor, items, onClose, minWidth = 200 }: {
  anchor: MenuAnchor
  items: MenuItem[]
  onClose: () => void
  minWidth?: number
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const { width, height } = el.getBoundingClientRect()
    let left = anchor.align === 'end' && anchor.right != null ? anchor.right - width : anchor.x
    let top = anchor.y
    if (left + width > window.innerWidth - 8) left = window.innerWidth - width - 8
    if (top + height > window.innerHeight - 8) {
      top = anchor.top != null ? anchor.top - height : window.innerHeight - height - 8
    }
    setPos({ left: Math.max(8, left), top: Math.max(8, top) })
  }, [anchor])

  useEffect(() => {
    const down = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
      }
    }
    const blur = () => onClose()
    document.addEventListener('mousedown', down, true)
    document.addEventListener('keydown', key, true)
    window.addEventListener('blur', blur)
    window.addEventListener('resize', blur)
    return () => {
      document.removeEventListener('mousedown', down, true)
      document.removeEventListener('keydown', key, true)
      window.removeEventListener('blur', blur)
      window.removeEventListener('resize', blur)
    }
  }, [onClose])

  // 定位完成、菜单可见之后再聚焦第一项，方便直接用方向键和回车操作。
  const visible = pos != null
  useEffect(() => {
    if (visible) ref.current?.querySelector<HTMLElement>('.menu-item:not([disabled])')?.focus()
  }, [visible])

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    const nodes = Array.from(ref.current?.querySelectorAll<HTMLElement>('.menu-item:not([disabled])') ?? [])
    const i = nodes.indexOf(document.activeElement as HTMLElement)
    const next = e.key === 'ArrowDown' ? nodes[(i + 1) % nodes.length] : nodes[(i - 1 + nodes.length) % nodes.length]
    next?.focus()
  }

  return createPortal(
    <div
      ref={ref}
      className="menu"
      role="menu"
      style={{ minWidth, left: pos?.left ?? anchor.x, top: pos?.top ?? anchor.y, visibility: pos ? 'visible' : 'hidden' }}
      onKeyDown={onKeyDown}
    >
      {items.map((it, i) => {
        if ('divider' in it) return <div key={i} className="menu-divider" />
        if ('heading' in it) return <div key={i} className="menu-heading">{it.heading}</div>
        if ('custom' in it) return <div key={i} className="menu-custom">{it.custom}</div>
        return (
          <button
            key={i}
            className={`menu-item ${it.danger ? 'is-danger' : ''} ${it.checked ? 'is-checked' : ''}`}
            role="menuitem"
            disabled={it.disabled}
            onClick={() => {
              onClose()
              it.onSelect()
            }}
          >
            <span className="menu-icon">{it.icon}</span>
            <span className="menu-label">{it.label}</span>
            {it.hint && <span className="menu-hint">{it.hint}</span>}
            {it.trailing}
          </button>
        )
      })}
    </div>,
    document.body,
  )
}

/** 管理单个弹出菜单的打开状态。 */
export function useMenu() {
  const [state, setState] = useState<{ anchor: MenuAnchor; items: MenuItem[]; minWidth?: number } | null>(null)
  const close = () => setState(null)
  const node = state ? <Menu anchor={state.anchor} items={state.items} minWidth={state.minWidth} onClose={close} /> : null
  return {
    open: (anchor: MenuAnchor, items: MenuItem[], minWidth?: number) => setState({ anchor, items, minWidth }),
    close,
    isOpen: state != null,
    node,
  }
}

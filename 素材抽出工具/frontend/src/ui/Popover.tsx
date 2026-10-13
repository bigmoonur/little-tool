import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import type { MenuAnchor } from './Menu'

/** 贴着锚点弹出的小面板。和菜单不同，点里面的内容不会关闭，可以连续操作；点面板外面或按 Esc 关闭。 */
export function Popover({ anchor, onClose, children, width = 280, label, className = '' }: {
  anchor: MenuAnchor
  onClose: () => void
  children: ReactNode
  width?: number
  label: string
  className?: string
}) {
  const ref = useRef<HTMLDivElement>(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)

  // 内容变高（例如新建了标签）后重新摆放，不让面板伸出窗口。
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const place = () => {
      const { width: w, height: h } = el.getBoundingClientRect()
      let left = anchor.align === 'end' && anchor.right != null ? anchor.right - w : anchor.x
      let top = anchor.y
      if (left + w > window.innerWidth - 8) left = window.innerWidth - w - 8
      if (top + h > window.innerHeight - 8) top = anchor.top != null && anchor.top - h >= 8 ? anchor.top - h : window.innerHeight - h - 8
      setPos({ left: Math.max(8, left), top: Math.max(8, top) })
    }
    place()
    const ro = new ResizeObserver(place)
    ro.observe(el)
    return () => ro.disconnect()
  }, [anchor])

  useEffect(() => {
    const down = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) closeRef.current()
    }
    // 在捕获阶段处理 Esc：面板开在弹窗里时，只关面板，不把弹窗一起关掉。
    const key = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.stopPropagation()
      closeRef.current()
    }
    const away = () => closeRef.current()
    document.addEventListener('mousedown', down, true)
    document.addEventListener('keydown', key, true)
    window.addEventListener('blur', away)
    window.addEventListener('resize', away)
    return () => {
      document.removeEventListener('mousedown', down, true)
      document.removeEventListener('keydown', key, true)
      window.removeEventListener('blur', away)
      window.removeEventListener('resize', away)
    }
  }, [])

  // 关闭后把焦点还给打开它的地方。
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null
    return () => prev?.focus?.()
  }, [])

  const visible = pos != null
  useEffect(() => {
    if (visible) ref.current?.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled])')?.focus()
  }, [visible])

  return createPortal(
    <div
      ref={ref}
      className={`popover ${className}`}
      role="dialog"
      aria-label={label}
      style={{ width, left: pos?.left ?? anchor.x, top: pos?.top ?? anchor.y, visibility: pos ? 'visible' : 'hidden' }}
    >
      {children}
    </div>,
    document.body,
  )
}

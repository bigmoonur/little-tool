import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'

const FOCUSABLE = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

interface ModalProps {
  title: ReactNode
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  width?: number
  /** 打开时聚焦的元素选择器，默认聚焦第一个输入框或按钮。 */
  initialFocus?: string
  className?: string
}

export function Modal({ title, onClose, children, footer, width = 460, initialFocus, className }: ModalProps) {
  const ref = useRef<HTMLDivElement>(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose

  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null
    const el = ref.current
    if (el) {
      const target =
        (initialFocus && el.querySelector<HTMLElement>(initialFocus)) ||
        el.querySelector<HTMLElement>('.modal-body input, .modal-body textarea, .modal-body select') ||
        el.querySelector<HTMLElement>('.modal-foot .btn-primary') ||
        el.querySelector<HTMLElement>(FOCUSABLE)
      target?.focus()
    }
    return () => prev?.focus?.()
  }, [])

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      closeRef.current()
    }
    if (e.key === 'Tab' && ref.current) {
      const items = Array.from(ref.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(n => n.offsetParent !== null)
      if (!items.length) return
      const first = items[0]
      const last = items[items.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  return createPortal(
    <div className="modal-layer" onMouseDown={e => e.target === e.currentTarget && onClose()}>
      <div
        ref={ref}
        className={`modal ${className ?? ''}`}
        style={{ width }}
        role="dialog"
        aria-modal="true"
        onKeyDown={onKeyDown}
      >
        <div className="modal-head">
          <h2>{title}</h2>
          <button className="icon-btn" onClick={onClose} aria-label="关闭">
            <X size={18} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  )
}

// ---------- 确认框与输入框 ----------

export interface ConfirmButton {
  key: string
  label: string
  kind?: 'primary' | 'danger' | 'plain'
}

export interface ConfirmOptions {
  title: string
  message?: ReactNode
  buttons?: ConfirmButton[]
  confirmText?: string
  danger?: boolean
}

export interface PromptOptions {
  title: string
  label?: string
  initial?: string
  placeholder?: string
  confirmText?: string
  hint?: ReactNode
  validate?: (v: string) => string
}

type Request =
  | { kind: 'confirm'; opts: ConfirmOptions; resolve: (v: string | null) => void }
  | { kind: 'prompt'; opts: PromptOptions; resolve: (v: string | null) => void }

export function useDialogs() {
  const [queue, setQueue] = useState<Request[]>([])
  const push = (r: Request) => setQueue(q => [...q, r])
  const confirm = (opts: ConfirmOptions) =>
    new Promise<string | null>(resolve => push({ kind: 'confirm', opts, resolve }))
  const prompt = (opts: PromptOptions) =>
    new Promise<string | null>(resolve => push({ kind: 'prompt', opts, resolve }))

  const current = queue[0]
  const done = (v: string | null) => {
    current?.resolve(v)
    setQueue(q => q.slice(1))
  }

  const node = current ? (
    current.kind === 'confirm' ? (
      <ConfirmDialog key={queue.length} opts={current.opts} done={done} />
    ) : (
      <PromptDialog key={queue.length} opts={current.opts} done={done} />
    )
  ) : null

  return { confirm, prompt, node }
}

function ConfirmDialog({ opts, done }: { opts: ConfirmOptions; done: (v: string | null) => void }) {
  const buttons: ConfirmButton[] = opts.buttons ?? [
    { key: 'cancel', label: '取消', kind: 'plain' },
    { key: 'ok', label: opts.confirmText ?? '确定', kind: opts.danger ? 'danger' : 'primary' },
  ]
  return (
    <Modal
      title={opts.title}
      onClose={() => done(null)}
      footer={buttons.map(b => (
        <button
          key={b.key}
          className={`btn ${b.kind === 'danger' ? 'btn-danger' : b.kind === 'primary' ? 'btn-primary' : ''}`}
          onClick={() => done(b.key === 'cancel' ? null : b.key)}
        >
          {b.label}
        </button>
      ))}
    >
      {opts.message && <div className="confirm-text">{opts.message}</div>}
    </Modal>
  )
}

function PromptDialog({ opts, done }: { opts: PromptOptions; done: (v: string | null) => void }) {
  const [value, setValue] = useState(opts.initial ?? '')
  const [touched, setTouched] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const error = opts.validate?.(value.trim()) ?? (value.trim() ? '' : '请填写名称')

  useEffect(() => inputRef.current?.select(), [])

  const submit = () => {
    setTouched(true)
    if (!error) done(value.trim())
  }

  return (
    <Modal
      title={opts.title}
      onClose={() => done(null)}
      footer={
        <>
          <button className="btn" onClick={() => done(null)}>
            取消
          </button>
          <button className="btn btn-primary" onClick={submit}>
            {opts.confirmText ?? '确定'}
          </button>
        </>
      }
    >
      <label className="field">
        {opts.label && <span className="field-label">{opts.label}</span>}
        <input
          ref={inputRef}
          className={`input ${touched && error ? 'is-invalid' : ''}`}
          value={value}
          maxLength={40}
          placeholder={opts.placeholder}
          onChange={e => setValue(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter') {
              e.preventDefault()
              submit()
            }
          }}
        />
      </label>
      {touched && error ? <p className="field-error">{error}</p> : opts.hint && <p className="field-hint">{opts.hint}</p>}
    </Modal>
  )
}

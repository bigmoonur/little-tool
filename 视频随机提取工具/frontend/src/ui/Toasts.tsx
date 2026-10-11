import { useCallback, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { CircleAlert, CircleCheck, Info, X } from 'lucide-react'

export type ToastKind = 'ok' | 'error' | 'info'

export interface ToastOptions {
  kind?: ToastKind
  action?: { label: string; onClick: () => void }
  duration?: number
}

interface Toast extends ToastOptions {
  id: number
  text: string
}

export function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const seq = useRef(0)

  const dismiss = useCallback((id: number) => setToasts(t => t.filter(x => x.id !== id)), [])

  const toast = useCallback(
    (text: string, opts: ToastOptions = {}) => {
      const id = ++seq.current
      // 同时最多显示 3 条，带撤销的提示停留更久。
      setToasts(t => [...t.slice(-2), { id, text, ...opts }])
      const ms = opts.duration ?? (opts.action ? 7000 : opts.kind === 'error' ? 6000 : 3200)
      window.setTimeout(() => dismiss(id), ms)
      return id
    },
    [dismiss],
  )

  const node = createPortal(
    <div className="toasts" role="status" aria-live="polite">
      {toasts.map(t => (
        <div key={t.id} className={`toast toast-${t.kind ?? 'info'}`}>
          <span className="toast-icon">
            {t.kind === 'ok' ? <CircleCheck size={17} /> : t.kind === 'error' ? <CircleAlert size={17} /> : <Info size={17} />}
          </span>
          <span className="toast-text">{t.text}</span>
          {t.action && (
            <button
              className="toast-action"
              onClick={() => {
                dismiss(t.id)
                t.action!.onClick()
              }}
            >
              {t.action.label}
            </button>
          )}
          <button className="toast-close" aria-label="关闭提示" onClick={() => dismiss(t.id)}>
            <X size={14} />
          </button>
        </div>
      ))}
    </div>,
    document.getElementById('overlay-root') ?? document.body,
  )

  return { toast, node }
}

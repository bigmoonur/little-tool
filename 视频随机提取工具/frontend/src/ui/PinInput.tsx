import { useEffect, useRef } from 'react'

export const PIN_LENGTH = 4

/**
 * 四格密码输入：真正接收键盘的是一个看不见的输入框，四个格子只负责显示。
 * 用密码类型的输入框，输入法不会介入，直接敲数字就行。
 */
export function PinInput({ value, onChange, onComplete, state = 'idle', disabled = false, label, focusKey }: {
  value: string
  onChange: (v: string) => void
  /** 输满四位时调用一次。 */
  onComplete: (v: string) => void
  /** error：输错，整行轻晃；ok：输对，墨点变成藤黄。 */
  state?: 'idle' | 'error' | 'ok'
  disabled?: boolean
  label: string
  /** 变化时重新聚焦（换了一步、错误提示结束等）。 */
  focusKey?: unknown
}) {
  const input = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!disabled) input.current?.focus()
  }, [disabled, focusKey])

  return (
    <label className={`passcode is-${state} ${disabled ? 'is-disabled' : ''}`}>
      <input
        ref={input}
        className="passcode-field"
        type="password"
        inputMode="numeric"
        autoComplete="off"
        aria-label={label}
        maxLength={PIN_LENGTH}
        value={value}
        disabled={disabled}
        onChange={e => {
          const v = e.target.value.replace(/\D/g, '').slice(0, PIN_LENGTH)
          onChange(v)
          if (v.length === PIN_LENGTH && v !== value) onComplete(v)
        }}
      />
      {Array.from({ length: PIN_LENGTH }, (_, i) => (
        <span
          key={i}
          className={`passcode-cell ${i < value.length ? 'is-filled' : ''} ${i === value.length ? 'is-current' : ''}`}
          style={{ '--i': i } as React.CSSProperties}
          aria-hidden
        >
          <i className="passcode-dot" />
        </span>
      ))}
    </label>
  )
}

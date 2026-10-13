import { useEffect, useState } from 'react'
import { api, errorText } from '../api'
import type { LockStatus } from '../types'
import { Logo } from '../ui/Logo'
import { PinInput } from '../ui/PinInput'

/** 连续输错这么多次之后，每次再试都要先等一会儿（与后端一致）。 */
const FREE_TRIES = 5

/**
 * 锁屏：一层微透明的纸盖住整个窗口，输对四位密码后纸面向上揭开。
 * dropIn 为 true 时（手动锁定）纸从上面落下来；刚打开软件时直接盖着，不让后面的内容先露出来。
 */
export function LockScreen({ status, dropIn, onStatus, onUnlocked }: {
  status: LockStatus
  dropIn: boolean
  onStatus: (s: LockStatus) => void
  onUnlocked: (reset: boolean) => void
}) {
  const [pin, setPin] = useState('')
  const [phase, setPhase] = useState<'idle' | 'checking' | 'error' | 'ok' | 'leaving'>('idle')
  const [now, setNow] = useState(Date.now())
  const [forgot, setForgot] = useState(false)
  const [key, setKey] = useState('')
  const [keyError, setKeyError] = useState('')
  const [resetting, setResetting] = useState(false)

  const waiting = status.lockedUntil > now
  const seconds = Math.max(1, Math.ceil((status.lockedUntil - now) / 1000))

  // 锁上的那一刻向后端要一次最新状态：在设置里输错过的次数、还要等多久，都以后端为准。
  useEffect(() => {
    api.lockStatus().then(st => {
      onStatus(st)
      setNow(Date.now())
    }, () => {})
  }, [])

  // 等待期间每秒更新一次倒计时。
  useEffect(() => {
    if (status.lockedUntil <= Date.now()) return
    const t = window.setInterval(() => setNow(Date.now()), 250)
    return () => window.clearInterval(t)
  }, [status.lockedUntil])

  const leave = (reset: boolean) => {
    setPhase('leaving')
    window.setTimeout(() => onUnlocked(reset), 520)
  }

  const submit = async (value: string) => {
    if (phase !== 'idle') return
    setPhase('checking')
    let res: { ok: boolean; status: LockStatus }
    try {
      res = await api.unlock(value)
    } catch {
      res = { ok: false, status }
    }
    onStatus(res.status)
    setNow(Date.now())
    if (res.ok) {
      setPhase('ok')
      window.setTimeout(() => leave(false), 360)
      return
    }
    setPhase('error')
    window.setTimeout(() => {
      setPin('')
      setPhase('idle')
    }, 460)
  }

  const reset = async () => {
    if (resetting || !key.trim()) return
    setResetting(true)
    try {
      onStatus(await api.resetLockWithLicense(key))
      leave(true)
    } catch (e) {
      setKeyError(errorText(e))
    }
    setResetting(false)
  }

  const left = FREE_TRIES - status.failures
  const message = waiting
    ? `输错次数太多，请 ${seconds} 秒后再试`
    : phase === 'error' || (status.failures > 0 && phase !== 'ok')
      ? left > 0
        ? `密码不对，还可以再试 ${left} 次`
        : '密码不对'
      : '输入 4 位密码'

  return (
    <div className={`lock-overlay ${dropIn ? 'is-dropping' : ''} ${phase === 'leaving' ? 'is-leaving' : ''}`} role="dialog" aria-modal="true" aria-label="已锁定">
      <div className="lock-card">
        <Logo className="lock-logo" />
        <h1 className="lock-title">已锁定</h1>
        {forgot ? (
          <div className="lock-reset">
            <p className="lock-message">输入购买时的授权码，可以关闭锁屏密码。</p>
            <input
              className={`input lock-key ${keyError ? 'is-invalid' : ''}`}
              value={key}
              autoFocus
              spellCheck={false}
              placeholder="授权码"
              aria-label="授权码"
              onChange={e => {
                setKey(e.target.value)
                setKeyError('')
              }}
              onKeyDown={e => e.key === 'Enter' && reset()}
            />
            {keyError && (
              <p className="lock-message is-error" role="alert">
                {keyError}
              </p>
            )}
            <div className="lock-reset-actions">
              <button className="btn btn-ghost" onClick={() => setForgot(false)}>
                返回
              </button>
              <button className="btn btn-primary" onClick={reset} disabled={resetting || !key.trim()}>
                {resetting ? '正在验证…' : '重置密码'}
              </button>
            </div>
          </div>
        ) : (
          <>
            <PinInput
              label="锁屏密码"
              value={pin}
              onChange={setPin}
              onComplete={submit}
              state={phase === 'error' ? 'error' : phase === 'ok' || phase === 'leaving' ? 'ok' : 'idle'}
              disabled={waiting || phase === 'checking' || phase === 'ok' || phase === 'leaving'}
              focusKey={`${phase}:${waiting}`}
            />
            <p className={`lock-message ${waiting || phase === 'error' || status.failures > 0 ? 'is-error' : ''}`} role="status">
              {phase === 'ok' || phase === 'leaving' ? '已解锁' : message}
            </p>
            <button className="link lock-forgot" onClick={() => setForgot(true)}>
              忘记密码？
            </button>
          </>
        )}
      </div>
    </div>
  )
}

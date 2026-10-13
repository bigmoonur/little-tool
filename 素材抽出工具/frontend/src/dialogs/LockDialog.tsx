import { useState } from 'react'
import { api, errorText } from '../api'
import type { LockStatus } from '../types'
import { Modal } from '../ui/Modal'
import { PinInput } from '../ui/PinInput'

type Step = 'current' | 'new' | 'confirm'

const TITLES = { set: '设置锁屏密码', change: '修改锁屏密码', disable: '关闭锁屏密码' }

/** 设置、修改或关闭锁屏密码：一步只问一件事，四位输满自动进入下一步。 */
export function LockDialog({ mode, onDone, onClose }: {
  mode: 'set' | 'change' | 'disable'
  onDone: (s: LockStatus) => void
  onClose: () => void
}) {
  const [step, setStep] = useState<Step>(mode === 'set' ? 'new' : 'current')
  const [pin, setPin] = useState('')
  const [current, setCurrent] = useState('')
  const [fresh, setFresh] = useState('')
  const [error, setError] = useState('')
  const [shake, setShake] = useState(false)
  const [busy, setBusy] = useState(false)

  const fail = (message: string, back?: Step) => {
    setError(message)
    setShake(true)
    window.setTimeout(() => {
      setShake(false)
      setPin('')
      if (back) setStep(back)
    }, 460)
  }

  const next = (to: Step) => {
    setError('')
    setPin('')
    setStep(to)
  }

  const complete = async (value: string) => {
    if (busy || shake) return
    if (step === 'new') {
      setFresh(value)
      return next('confirm')
    }
    setBusy(true)
    try {
      if (step === 'current' && mode === 'change') {
        // 先核对当前密码，再让用户输入新的。
        const res = await api.unlock(value)
        if (!res.ok) {
          const wait = Math.ceil((res.status.lockedUntil - Date.now()) / 1000)
          fail(wait > 0 ? `输错次数太多，请 ${wait} 秒后再试` : '当前密码不对')
        } else {
          setCurrent(value)
          next('new')
        }
      } else if (step === 'current') {
        onDone(await api.disableLock(value))
      } else if (value !== fresh) {
        fail('两次输入的不一样，请重新设置', 'new')
      } else {
        onDone(await api.setLock(current, value))
      }
    } catch (e) {
      fail(errorText(e))
    }
    setBusy(false)
  }

  const prompt = step === 'current' ? '输入当前的密码' : step === 'new' ? '输入新的 4 位数字密码' : '再输入一次确认'

  return (
    <Modal title={TITLES[mode]} width={400} onClose={onClose} className="lock-dialog">
      <p className="lock-step">{prompt}</p>
      <PinInput label={prompt} value={pin} onChange={setPin} onComplete={complete} state={shake ? 'error' : 'idle'} disabled={busy} focusKey={`${step}:${shake}`} />
      <p className={`lock-note ${error ? 'is-error' : ''}`} role={error ? 'alert' : undefined}>
        {error || (mode === 'disable' ? '关闭后，打开软件不再需要密码。' : '忘记密码时，可以在锁屏上用授权码重置。')}
      </p>
    </Modal>
  )
}

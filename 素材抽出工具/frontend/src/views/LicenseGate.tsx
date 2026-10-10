import { useEffect, useState } from 'react'
import { CircleAlert, RefreshCw, WifiOff } from 'lucide-react'
import { api, errorText } from '../api'
import type { LicenseStatus } from '../types'
import { Logo } from '../ui/Logo'

/** 未激活时的全屏页面：输入授权码激活，或在网络恢复后重新验证。 */
export function LicenseGate({ status, version, onActivated }: {
  status: LicenseStatus
  version: string
  onActivated: (s: LicenseStatus) => void
}) {
  const [key, setKey] = useState(status.key)
  const [busy, setBusy] = useState<'' | 'activate' | 'retry'>('')
  const [error, setError] = useState(status.message)
  const [network, setNetwork] = useState(status.network)

  useEffect(() => {
    setError(status.message)
    setNetwork(status.network)
  }, [status.message, status.network])

  const activate = async () => {
    if (!key.trim() || busy) return
    setBusy('activate')
    setError('')
    try {
      onActivated(await api.activateLicense(key))
    } catch (e) {
      const msg = errorText(e)
      setError(msg)
      setNetwork(msg.includes('无法连接'))
    } finally {
      setBusy('')
    }
  }

  const retry = async () => {
    if (busy) return
    setBusy('retry')
    setError('')
    try {
      const st = await api.refreshLicense()
      if (st.state === 'active') return onActivated(st)
      setError(st.message)
      setNetwork(st.network)
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="gate">
      <div className="gate-card">
        <Logo className="gate-logo" />
        <h1>激活素材抽出</h1>
        <label className="field">
          <span className="field-label">授权码</span>
          <input
            className="input gate-input"
            value={key}
            autoFocus
            spellCheck={false}
            autoComplete="off"
            placeholder="粘贴或输入授权码"
            onChange={e => setKey(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && activate()}
          />
        </label>
        {error && (
          <p className={`gate-msg ${network ? 'is-net' : ''}`} role="alert">
            {network ? <WifiOff size={16} /> : <CircleAlert size={16} />}
            <span>{error}</span>
          </p>
        )}
        <button className="btn btn-primary btn-lg gate-btn" onClick={activate} disabled={!key.trim() || !!busy}>
          {busy === 'activate' ? '正在激活…' : '激活'}
        </button>
        {status.key && (
          <button className="btn btn-ghost gate-retry" onClick={retry} disabled={!!busy}>
            <RefreshCw size={15} />
            {busy === 'retry' ? '正在验证…' : '重新联网验证'}
          </button>
        )}
        <p className="gate-foot">
          本机：{status.machine || '未知'} · 版本 {version}
        </p>
      </div>
    </div>
  )
}

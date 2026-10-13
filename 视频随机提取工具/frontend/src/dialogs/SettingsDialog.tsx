import { FolderOpen, KeyRound, RefreshCw } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import type { AfterCopy, Settings, Theme } from '../types'
import { Modal } from '../ui/Modal'
import { LockDialog } from './LockDialog'
import { day } from '../lib/format'
import { useRef, useState } from 'react'

function Segmented<T extends string>({ value, options, onChange, label }: {
  value: T
  options: [T, string][]
  onChange: (v: T) => void
  label: string
}) {
  return (
    <div className="segmented" role="radiogroup" aria-label={label}>
      {options.map(([v, text]) => (
        <button key={v} role="radio" aria-checked={value === v} className={value === v ? 'is-on' : ''} onClick={() => onChange(v)}>
          {text}
        </button>
      ))}
    </div>
  )
}

export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const { settings, setSettings, run, version, license, setLicense, confirm, copy, lock, setLock, toast } = useApp()
  const [lockDialog, setLockDialog] = useState<'set' | 'change' | 'disable' | null>(null)
  // 密码小窗关闭后把焦点还给打开它的开关或按钮：点复选框不一定会让它得到焦点，
  // 焦点留在窗口外的话，按 Esc 就关不掉设置了。
  const lockTrigger = useRef<HTMLElement | null>(null)
  const openLockDialog = (mode: 'set' | 'change' | 'disable', from: HTMLElement) => {
    lockTrigger.current = from
    setLockDialog(mode)
  }
  const closeLockDialog = () => {
    setLockDialog(null)
    requestAnimationFrame(() => lockTrigger.current?.focus())
  }
  const [checking, setChecking] = useState(false)

  const refreshLicense = async () => {
    if (checking) return
    setChecking(true)
    try {
      const st = await run(api.refreshLicense())
      if (st) setLicense(st)
    } finally { setChecking(false) }
  }

  const deactivate = async () => {
    const ok = await confirm({
      title: '解除本机授权？',
      message: '解除后需要重新输入授权码。两款工具共用本机设备名额，另一款下次联网可能自动重新登记；换电脑前请在两款工具中都解除授权。视频、设置和记录会保留。',
      confirmText: '解除授权',
      danger: true,
    })
    if (!ok) return
    const st = await run(api.deactivateLicense())
    if (st) setLicense(st)
  }

  const update = async (patch: Partial<Settings>) => {
    const next = { ...settings, ...patch }
    setSettings(next)
    const s = await run(api.updateSettings(next))
    if (s) setSettings(s)
  }

  return (
    <Modal
      title="设置"
      width={500}
      onClose={onClose}
      footer={
        <button className="btn btn-primary" onClick={onClose}>
          完成
        </button>
      }
    >
      <div className="settings">
        <section>
          <h3>窗口</h3>
          <label className="check-row">
            <input type="checkbox" checked={settings.alwaysOnTop} onChange={e => update({ alwaysOnTop: e.target.checked })} />
            窗口始终显示在最前面
          </label>
          <div className="setting-row">
            <span>外观</span>
            <Segmented<Theme>
              label="外观"
              value={settings.theme}
              onChange={v => update({ theme: v })}
              options={[
                ['system', '跟随系统'],
                ['light', '浅色'],
                ['dark', '深色'],
              ]}
            />
          </div>
        </section>

        <section>
          <h3>锁屏</h3>
          <label className="check-row">
            <input type="checkbox" checked={lock.enabled} onChange={e => openLockDialog(e.target.checked ? 'set' : 'disable', e.target)} />
            打开软件时需要输入 4 位密码
          </label>
          {lock.enabled && (
            <div className="setting-row">
              <span className="dim">开启后顶栏会多一个锁，可以随时锁定</span>
              <button className="btn" onClick={e => openLockDialog('change', e.currentTarget)}>
                修改密码
              </button>
            </div>
          )}
        </section>

        <section>
          <h3>抽取与复制</h3>
          <div className="setting-row">
            <span>复制完成后</span>
            <Segmented<AfterCopy>
              label="复制完成后"
              value={settings.afterCopy}
              onChange={v => update({ afterCopy: v })}
              options={[
                ['stay', '留在程序'],
                ['minimize', '最小化'],
                ['quit', '退出程序'],
              ]}
            />
          </div>
          <label className="check-row">
            <input type="checkbox" checked={settings.openAfter} onChange={e => update({ openAfter: e.target.checked })} />
            复制完成后自动打开目标文件夹
          </label>
          <label className="check-row">
            <input type="checkbox" checked={settings.rememberDraft} onChange={e => update({ rememberDraft: e.target.checked })} />
            下次打开时保留上次填写的数量
          </label>
        </section>

        <section>
          <h3>素材库</h3>
          <label className="check-row">
            <input type="checkbox" checked={settings.thumbnails} onChange={e => update({ thumbnails: e.target.checked })} />
            缓存视频缩略图
          </label>
          <p className="dim settings-note">
            开启后素材库可以按图标显示，列表里鼠标移到视频上会弹出预览。缩略图保存在数据文件夹里，视频删除或更换后会自动清理；关闭这个选项会删掉已经缓存的缩略图。
          </p>
        </section>

        <section>
          <h3>数据</h3>
          <p className="dim settings-note">选择过的文件夹、设置和复制记录保存在数据文件夹里，不会混进视频文件夹。</p>
          <div className="settings-actions">
            <button className="btn btn-ghost" onClick={() => run(api.openDataFolder())}>
              <FolderOpen size={16} />
              打开数据文件夹
            </button>
          </div>
        </section>

        <section>
          <h3>授权</h3>
          <dl className="license-info">
            <dt>授权码</dt><dd className="mono">{license.masked || '—'}</dd>
            <dt>有效期</dt><dd>{license.expiry ? `${day(license.expiry)} 到期` : '长期有效'}</dd>
            <dt>本机</dt><dd>{license.machine || '—'}</dd>
            {license.offline && <><dt>状态</dt><dd className="warn-text">暂时无法联网验证，可以离线使用到 {day(license.graceUntil)}</dd></>}
          </dl>
          <div className="settings-actions">
            <button className="btn btn-ghost" onClick={refreshLicense} disabled={checking}>
              <RefreshCw size={16} />{checking ? '正在验证…' : '重新联网验证'}
            </button>
            <button className="btn btn-ghost is-danger" onClick={deactivate} disabled={checking || copy.phase === 'copying'}>
              <KeyRound size={16} />解除本机授权
            </button>
          </div>
        </section>

        <p className="settings-version dim">视频分发工具 {version}</p>
      </div>
      {lockDialog && (
        <LockDialog
          mode={lockDialog}
          onClose={closeLockDialog}
          onDone={st => {
            const done = lockDialog
            setLock(st)
            closeLockDialog()
            toast(done === 'disable' ? '已关闭锁屏密码' : done === 'change' ? '已修改锁屏密码' : '已开启锁屏密码，下次打开软件时生效', { kind: 'ok' })
          }}
        />
      )}
    </Modal>
  )
}

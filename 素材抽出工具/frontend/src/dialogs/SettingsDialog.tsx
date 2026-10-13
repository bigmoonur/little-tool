import { useRef, useState } from 'react'
import { Download, FolderOpen, KeyRound, Upload } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import type { AfterCopy, Settings, Theme } from '../types'
import { day } from '../lib/format'
import { Modal } from '../ui/Modal'
import { LockDialog } from './LockDialog'

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
  const { snap, setSnap, run, toast, confirm, commit, info, setDraft, license, setLicense, leave, lock, setLock } = useApp()
  const s = snap.settings
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

  const update = async (patch: Partial<Settings>) => {
    setSnap({ ...snap, settings: { ...s, ...patch } })
    const next = await run(api.updateSettings({ ...s, ...patch }))
    if (next) setSnap(next)
  }

  const exportBackup = async () => {
    const path = await run(api.exportBackup())
    if (path) toast('备份已导出', { kind: 'ok' })
  }

  const restore = async () => {
    const ok = await confirm({
      title: '从备份恢复？',
      message: '素材库中的全部分类、分组和方案会被备份文件中的内容替换。恢复后可以撤销。',
      confirmText: '选择备份文件',
    })
    if (!ok) return
    const res = await run(api.restoreBackup())
    if (res?.restored) {
      setDraft({})
      commit(res.snapshot, '已从备份恢复')
    }
  }

  const deactivate = async () => {
    const ok = await confirm({
      title: '解除本机授权？',
      message: '解除后需要重新输入授权码。两款工具共用本机设备名额，另一款下次联网可能自动重新登记；换电脑前请在两款工具中都解除授权。素材库会保留。',
      confirmText: '解除授权',
      danger: true,
    })
    if (!ok || !(await leave())) return
    const st = await run(api.deactivateLicense())
    if (st) setLicense(st)
  }

  return (
    <Modal
      title="设置"
      width={520}
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
            <input type="checkbox" checked={s.alwaysOnTop} onChange={e => update({ alwaysOnTop: e.target.checked })} />
            窗口始终显示在最前面
          </label>
          <div className="setting-row">
            <span>复制成功后</span>
            <Segmented<AfterCopy>
              label="复制成功后"
              value={s.afterCopy}
              onChange={v => update({ afterCopy: v })}
              options={[
                ['stay', '保持窗口'],
                ['minimize', '最小化'],
                ['quit', '关闭程序'],
              ]}
            />
          </div>
          <div className="setting-row">
            <span>外观</span>
            <Segmented<Theme>
              label="外观"
              value={s.theme}
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
          <h3>抽取与添加</h3>
          <label className="check-row">
            <input type="checkbox" checked={s.rememberDraft} onChange={e => update({ rememberDraft: e.target.checked })} />
            下次打开时保留上次填写的数量
          </label>
          <label className="check-row">
            <input type="checkbox" checked={s.importDedupe} onChange={e => update({ importDedupe: e.target.checked })} />
            添加素材时默认跳过重复的行
          </label>
        </section>

        <section>
          <h3>数据</h3>
          <p className="dim settings-note">素材库会自动保存，并在修改前保留最近 20 份自动备份。换电脑时可以导出备份，在新电脑上恢复。</p>
          <div className="settings-actions">
            <button className="btn" onClick={exportBackup}>
              <Download size={16} />
              导出备份
            </button>
            <button className="btn" onClick={restore}>
              <Upload size={16} />
              从备份恢复
            </button>
            <button className="btn btn-ghost" onClick={() => run(api.openDataFolder())}>
              <FolderOpen size={16} />
              打开数据文件夹
            </button>
          </div>
        </section>

        <section>
          <h3>授权</h3>
          <dl className="license-info">
            <dt>授权码</dt>
            <dd className="mono">{license.masked || '—'}</dd>
            <dt>有效期</dt>
            <dd>{license.expiry ? `${day(license.expiry)} 到期` : '长期有效'}</dd>
            <dt>本机</dt>
            <dd>{license.machine || '—'}</dd>
            {license.offline && (
              <>
                <dt>状态</dt>
                <dd className="warn-text">暂时无法联网验证，可以离线使用到 {day(license.graceUntil)}</dd>
              </>
            )}
          </dl>
          <div className="settings-actions">
            <button className="btn btn-ghost is-danger" onClick={deactivate}>
              <KeyRound size={16} />
              解除本机授权
            </button>
          </div>
        </section>

        <p className="settings-version dim">链接抽取工具 {info.version}</p>
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

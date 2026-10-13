import { useState } from 'react'
import { api } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import type { Category } from '../types'
import { Modal } from '../ui/Modal'

export function DeleteCategoryDialog({ category, onClose }: { category: Category; onClose: () => void }) {
  const { settings, setSettings, run, setState, confirm, toast, copy, adding, refresh } = useApp()
  const [withVideos, setWithVideos] = useState(settings.deleteWithVideos)
  const [pending, setPending] = useState(false)
  const busy = pending || copy.phase === 'copying' || !!adding
  const remove = async () => {
    if (busy) return
    setPending(true)
    try {
      if (withVideos && category.count > 0) {
        const ok = await confirm({
          title: `确定删除 ${num(category.count)} 个视频？`,
          message: '这些视频会从磁盘上直接删除，不能恢复，也不会移到回收站。分类里的其他文件和子文件夹会保留。',
          confirmText: '删除视频',
          danger: true,
        })
        if (!ok) return
      }
      const res = await run(api.deleteCategory(category.name, withVideos))
      // 删除中途失败也可能已删掉部分视频，重新读取数量和记住的选项。
      const s = await run(api.init())
      if (s) setSettings(s.settings)
      if (!res) return await refresh()
      setState(res.state)
      const message = res.removed > 0
        ? `已删除 ${num(res.removed)} 个视频${res.kept ? '，文件夹里的其他内容保留在磁盘上' : '和分类文件夹'}`
        : res.kept ? `已移除分类「${category.name}」，文件夹保留在磁盘上` : `已删除空分类「${category.name}」`
      toast(message, { kind: 'ok' })
      onClose()
    } finally { setPending(false) }
  }
  return (
    <Modal
      title={`删除分类「${category.name}」？`}
      onClose={() => { if (!pending) onClose() }}
      footer={
        <>
          <button className="btn" onClick={onClose} disabled={pending}>取消</button>
          <button className="btn btn-danger" onClick={remove} disabled={busy}>删除</button>
        </>
      }
    >
      <label className="check-row">
        <input type="checkbox" checked={withVideos} disabled={busy} onChange={e => setWithVideos(e.target.checked)} />
        同时删除分类里的 {num(category.count)} 个视频
      </label>
      <p className="confirm-text delete-category-note">
        {withVideos
          ? `这 ${num(category.count)} 个视频会从磁盘上直接删除，不能恢复，也不会移到回收站。文件夹里的其他文件和子文件夹会保留。`
          : '分类只从程序里移除，文件夹和视频仍留在磁盘上；完全空的文件夹会删除。以后新建同名分类，就能把它带回来。'}
      </p>
    </Modal>
  )
}

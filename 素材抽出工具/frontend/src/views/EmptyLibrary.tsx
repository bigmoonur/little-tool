import { useState } from 'react'
import { ClipboardPaste, FileText, FolderPlus } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { loadFiles, pickFiles } from '../lib/files'

/** 素材库为空时的引导：说明 TXT 与分类的关系，可以先建分组，再用两种方式添加素材。 */
export function EmptyLibrary({ title, lead }: { title: string; lead: string }) {
  const { snap, setSnap, openImport, toast, prompt, run, go } = useApp()
  // 刚新建的分组（或唯一的分组）：接下来添加素材时默认放进去，不用先导入再移动。
  const [created, setCreated] = useState('')
  const group = snap.groups.find(g => g.id === created) ?? (snap.groups.length === 1 ? snap.groups[0] : undefined)
  const groupId = group?.id

  const chooseFiles = async () => {
    const files = await pickFiles()
    if (!files.length) return
    const { loaded, skipped } = await loadFiles(files)
    if (skipped.length) toast(`只支持 TXT 文本文件，已跳过：${skipped.join('、')}`, { kind: 'error' })
    if (loaded.length) openImport({ tab: 'files', files: loaded, groupId })
  }

  const newGroup = async () => {
    const name = await prompt({
      title: '新建分组',
      label: '分组名称',
      placeholder: '例如：服装类',
      confirmText: '新建',
      hint: '建好以后，添加素材时可以直接放进这个分组。',
    })
    if (!name) return
    const res = await run(api.createGroup(name))
    if (!res) return
    setSnap(res.snapshot)
    setCreated(res.id)
    toast(`已新建分组「${name}」`, { kind: 'ok' })
    go('library')
  }

  return (
    <div className="empty">
      <div className="empty-card">
        <div className="empty-art" aria-hidden>
          <span className="sheet s1" />
          <span className="sheet s2" />
          <span className="sheet s3" />
        </div>
        <h2>{title}</h2>
        <p className="empty-lead">{lead}</p>
        <p className="empty-rule">
          每个 TXT 文件会成为一个<b>分类</b>，文件里的每一行是一条<b>素材</b>。
        </p>
        <div className="empty-actions">
          <button className="btn btn-primary btn-lg" onClick={chooseFiles}>
            <FileText size={18} />
            选择 TXT 文件
          </button>
          <button className="btn btn-lg" onClick={() => openImport({ tab: 'paste', groupId })}>
            <ClipboardPaste size={18} />
            粘贴文本
          </button>
          <button className="btn btn-lg" onClick={newGroup}>
            <FolderPlus size={18} />
            新建分组
          </button>
        </div>
        <p className="empty-drop">{group ? `接下来添加的素材会默认放进分组「${group.name}」，添加时可以更改` : '也可以直接把 TXT 文件拖进窗口'}</p>
      </div>
    </div>
  )
}

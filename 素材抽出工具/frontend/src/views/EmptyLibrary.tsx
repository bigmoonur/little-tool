import { ClipboardPaste, FileText } from 'lucide-react'
import { useApp } from '../ctx'
import { loadFiles, pickFiles } from '../lib/files'

/** 素材库为空时的引导：说明 TXT 与分类的关系，并给出两种添加方式。 */
export function EmptyLibrary({ title, lead }: { title: string; lead: string }) {
  const { openImport, toast } = useApp()

  const chooseFiles = async () => {
    const files = await pickFiles()
    if (!files.length) return
    const { loaded, skipped } = await loadFiles(files)
    if (skipped.length) toast(`只支持 TXT 文本文件，已跳过：${skipped.join('、')}`, { kind: 'error' })
    if (loaded.length) openImport({ tab: 'files', files: loaded })
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
            <FileText size={17} />
            选择 TXT 文件
          </button>
          <button className="btn btn-lg" onClick={() => openImport({ tab: 'paste' })}>
            <ClipboardPaste size={17} />
            粘贴文本
          </button>
        </div>
        <p className="empty-drop">也可以直接把 TXT 文件拖进窗口</p>
      </div>
    </div>
  )
}

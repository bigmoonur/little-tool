import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronUp, Search, X } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { num } from '../lib/format'
import type { ListInfo } from '../types'

// 与 styles.css 中 .gutter / .editor-text 的行高一致。
const LINE_HEIGHT = 24

function lineOf(text: string, index: number) {
  let n = 0
  for (let i = text.indexOf('\n'); i >= 0 && i < index; i = text.indexOf('\n', i + 1)) n++
  return n
}

/**
 * 分类内容编辑器：整段文本，每行一条素材。
 * 修改后需要保存；切换分类、切换页面或关闭窗口前会提醒未保存的修改。
 */
export function ListEditor({ list }: { list: ListInfo }) {
  const { run, commit, setGuard, confirm } = useApp()
  const [original, setOriginal] = useState<string | null>(null)
  const [value, setValue] = useState('')
  const [scroll, setScroll] = useState(0)
  const [cursorLine, setCursorLine] = useState(0)
  const [findOpen, setFindOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [match, setMatch] = useState<{ line: number; index: number; total: number } | null>(null)
  const [saving, setSaving] = useState(false)
  const ta = useRef<HTMLTextAreaElement>(null)
  const findInput = useRef<HTMLInputElement>(null)

  const dirty = original !== null && value !== original
  const dirtyRef = useRef(dirty)
  dirtyRef.current = dirty

  // 首次打开或分类内容在别处被修改（添加素材、撤销等）时重新读取；有未保存的修改时不覆盖。
  useEffect(() => {
    if (dirtyRef.current) return
    let alive = true
    api.listText(list.id).then(text => {
      if (!alive || dirtyRef.current) return
      setOriginal(text)
      setValue(text)
    })
    return () => {
      alive = false
    }
  }, [list.id, list.updatedAt, list.count])

  const deferred = useDeferredValue(value)
  const lineCount = useMemo(() => {
    let n = 1
    for (let i = deferred.indexOf('\n'); i >= 0; i = deferred.indexOf('\n', i + 1)) n++
    return n
  }, [deferred])
  const gutter = useMemo(() => Array.from({ length: lineCount }, (_, i) => i + 1).join('\n'), [lineCount])

  const save = async () => {
    if (!dirtyRef.current || saving) return true
    setSaving(true)
    const s = await run(api.setListText(list.id, value))
    setSaving(false)
    if (!s) return false
    const text = await api.listText(list.id)
    setOriginal(text)
    setValue(text)
    commit(s, `已保存「${list.name}」`)
    return true
  }
  const saveRef = useRef(save)
  saveRef.current = save

  const discard = () => original !== null && setValue(original)

  // 注册离开确认。
  useEffect(() => {
    if (!dirty) {
      setGuard(null)
      return
    }
    setGuard(async () => {
      const choice = await confirm({
        title: `保存对「${list.name}」的修改？`,
        message: '不保存的话，这次修改会丢失。',
        buttons: [
          { key: 'cancel', label: '取消' },
          { key: 'discard', label: '不保存', kind: 'danger' },
          { key: 'save', label: '保存', kind: 'primary' },
        ],
      })
      if (choice === 'save') return saveRef.current()
      if (choice === 'discard') {
        dirtyRef.current = false
        if (original !== null) setValue(original)
        return true
      }
      return false
    })
  }, [dirty, list.name, original])
  useEffect(() => () => setGuard(null), [])

  const updateCursor = () => {
    const el = ta.current
    if (el) setCursorLine(lineOf(el.value, el.selectionStart))
  }

  const find = (dir: 1 | -1, from?: number) => {
    const el = ta.current
    const q = query.toLowerCase()
    if (!el || !q) return setMatch(null)
    const hay = value.toLowerCase()
    let idx = dir === 1 ? hay.indexOf(q, from ?? el.selectionEnd) : hay.lastIndexOf(q, Math.max(0, el.selectionStart - 1))
    if (idx < 0) idx = dir === 1 ? hay.indexOf(q) : hay.lastIndexOf(q)
    if (idx < 0) return setMatch({ line: -1, index: 0, total: 0 })
    let total = 0
    let ordinal = 0
    for (let i = hay.indexOf(q); i >= 0 && total < 9999; i = hay.indexOf(q, i + q.length)) {
      total++
      if (i === idx) ordinal = total
    }
    el.setSelectionRange(idx, idx + q.length)
    const line = lineOf(value, idx)
    el.scrollTop = Math.max(0, line * LINE_HEIGHT - el.clientHeight / 2)
    setMatch({ line, index: ordinal, total })
  }

  useEffect(() => {
    if (findOpen && query) find(1, 0)
    else setMatch(null)
  }, [query])

  const onKeyDown = (e: React.KeyboardEvent) => {
    const mod = e.ctrlKey || e.metaKey
    if (mod && e.key.toLowerCase() === 's') {
      e.preventDefault()
      save()
    } else if (mod && e.key.toLowerCase() === 'f') {
      e.preventDefault()
      setFindOpen(true)
      requestAnimationFrame(() => findInput.current?.select())
    } else if (e.key === 'F3') {
      e.preventDefault()
      find(e.shiftKey ? -1 : 1)
    }
  }

  const closeFind = () => {
    setFindOpen(false)
    setMatch(null)
    ta.current?.focus()
  }

  const confirmDiscard = async () => {
    const ok = await confirm({ title: '放弃修改？', message: '编辑器会恢复到上次保存的内容。', confirmText: '放弃修改', danger: true })
    if (ok) discard()
  }

  return (
    <div className="editor" onKeyDown={onKeyDown}>
      {findOpen && (
        <div className="find-bar">
          <Search size={15} className="dim" />
          <input
            ref={findInput}
            value={query}
            placeholder="查找内容"
            aria-label="查找内容"
            onChange={e => setQuery(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') {
                e.preventDefault()
                find(e.shiftKey ? -1 : 1)
              } else if (e.key === 'Escape') {
                e.stopPropagation()
                closeFind()
              }
            }}
          />
          <span className="find-count">
            {match ? (match.total ? `${match.index} / ${match.total >= 9999 ? '9999+' : match.total}` : '没有找到') : ''}
          </span>
          <button className="icon-btn sm" onClick={() => find(-1)} aria-label="上一个" title="上一个（Shift + 回车）">
            <ChevronUp size={16} />
          </button>
          <button className="icon-btn sm" onClick={() => find(1)} aria-label="下一个" title="下一个（回车）">
            <ChevronDown size={16} />
          </button>
          <button className="icon-btn sm" onClick={closeFind} aria-label="关闭查找">
            <X size={16} />
          </button>
        </div>
      )}
      <div className="editor-body">
        <pre className="gutter" aria-hidden style={{ transform: `translateY(${-scroll}px)` }}>
          {gutter}
        </pre>
        {match && match.line >= 0 && (
          <div className="find-band" style={{ top: 12 + match.line * LINE_HEIGHT - scroll }} aria-hidden />
        )}
        <textarea
          ref={ta}
          className="editor-text"
          value={value}
          spellCheck={false}
          wrap="off"
          disabled={original === null}
          placeholder={original === null ? '正在读取…' : '在这里粘贴或输入素材，每行一条，然后保存'}
          aria-label={`「${list.name}」的素材内容，每行一条`}
          onChange={e => setValue(e.target.value)}
          onScroll={e => setScroll(e.currentTarget.scrollTop)}
          onKeyUp={updateCursor}
          onClick={updateCursor}
        />
      </div>
      <footer className={`editor-foot ${dirty ? 'is-dirty' : ''}`}>
        {dirty ? (
          <>
            <span className="dirty-dot" />
            <span>有未保存的修改</span>
            <span className="toolbar-spacer" />
            <button className="btn btn-ghost" onClick={confirmDiscard}>
              放弃修改
            </button>
            <button className="btn btn-primary" onClick={save} disabled={saving}>
              保存
              <kbd>Ctrl S</kbd>
            </button>
          </>
        ) : (
          <>
            <span className="dim">第 {num(cursorLine + 1)} 行，共 {num(lineCount)} 行</span>
            <span className="toolbar-spacer" />
            <span className="dim">每行一条素材 · 保存时自动去掉空行和首尾空格 · Ctrl+F 查找</span>
          </>
        )}
      </footer>
    </div>
  )
}

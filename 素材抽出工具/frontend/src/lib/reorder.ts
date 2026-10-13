import { useRef, useState } from 'react'
import { api } from '../api'
import { useApp } from '../ctx'
import type { Group, ListInfo, Snapshot } from '../types'

export interface Section {
  group: Group | null
  lists: ListInfo[]
}

/** 按分组顺序整理分类；有分组时“未分组”始终排在最后，方便把分类拖出分组。 */
export function buildSections(snap: Snapshot): Section[] {
  const sections: Section[] = snap.groups.map(group => ({ group, lists: snap.lists.filter(l => l.groupId === group.id) }))
  const loose = snap.lists.filter(l => !l.groupId)
  if (loose.length || snap.groups.length || !sections.length) sections.push({ group: null, lists: loose })
  return sections
}

type Kind = 'list' | 'group'
type Drop = { kind: Kind; id: string; where: 'before' | 'after' } | { kind: 'group-into'; id: string }

/** 两个页面共用后端顺序，拖动分类也可以跨分组移动。 */
export function useReorder() {
  const { snap, run, setSnap } = useApp()
  const dragging = useRef<{ kind: Kind; id: string } | null>(null)
  const target = useRef<Drop | null>(null)
  const [drop, setDrop] = useState<Drop | null>(null)
  const [active, setActive] = useState<{ kind: Kind; id: string } | null>(null)
  const [pending, setPending] = useState(false)
  const mark = (d: Drop | null) => { target.current = d; setDrop(d) }
  const half = (e: React.DragEvent) => {
    const r = e.currentTarget.getBoundingClientRect()
    return e.clientY < r.top + r.height / 2 ? 'before' : 'after'
  }
  const start = (e: React.DragEvent, kind: Kind, id: string) => {
    if (pending) return e.preventDefault()
    e.stopPropagation()
    dragging.current = { kind, id }
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(`text/x-${kind}`, id)
    // 界面上的拖动状态推迟到拖动真正开始之后再更新：在 dragstart 里立刻改动页面，浏览器可能直接取消这次拖动。
    window.setTimeout(() => setActive(dragging.current))
  }
  const end = () => { dragging.current = null; setActive(null); mark(null) }
  // overList / overGroup 同时接在 dragenter 和 dragover 上：有的内核换到新元素时先只发 dragenter，
  // 紧接着松手的话还没等到 dragover，落点就没有记下，这次放下也不会被接受。
  const overList = (e: React.DragEvent, l: ListInfo) => {
    if (dragging.current?.kind !== 'list') return
    e.preventDefault(); e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    mark({ kind: 'list', id: l.id, where: half(e) })
  }
  const overGroup = (e: React.DragEvent, id: string) => {
    const d = dragging.current
    if (!d) return
    e.preventDefault(); e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    if (d.kind === 'list') return mark({ kind: 'group-into', id })
    // 拖到另一个分组上就换到它的位置：往下拖放到它后面，往上拖放到它前面。
    // 不按上下半区判断：分组标题在最上面，那样拖到紧挨着的下一个分组的标题上等于原地不动。
    const from = snap.groups.findIndex(g => g.id === d.id), to = snap.groups.findIndex(g => g.id === id)
    mark(to >= 0 && to !== from ? { kind: 'group', id, where: from < to ? 'after' : 'before' } : null)
  }
  // 指针真的离开整个列表时才取消落点。不看 relatedTarget：它在 dragleave 上不一定有值，
  // 没有值时在列表内部从一个元素换到另一个元素也会被当成离开，把刚记下的落点清掉。
  const leave = (e: React.DragEvent) => {
    const r = e.currentTarget.getBoundingClientRect()
    if (e.clientX <= r.left || e.clientX >= r.right || e.clientY <= r.top || e.clientY >= r.bottom) mark(null)
  }
  const send = async (request: Promise<Snapshot>) => {
    setPending(true)
    const s = await run(request)
    if (s) setSnap(s)
    setPending(false)
  }
  const onDrop = async (e: React.DragEvent) => {
    if (!dragging.current) return
    e.preventDefault(); e.stopPropagation()
    const d = dragging.current, t = target.current
    end()
    if (!t || pending) return
    let request: Promise<Snapshot> | undefined
    if (d.kind === 'list') {
      if (t.kind === 'group-into') request = api.moveList(d.id, t.id, '')
      else if (t.kind === 'list' && t.id !== d.id) {
        const l = snap.lists.find(x => x.id === t.id)
        if (!l) return
        const same = snap.lists.filter(x => x.groupId === l.groupId && x.id !== d.id)
        const next = same[same.findIndex(x => x.id === l.id) + 1]
        request = api.moveList(d.id, l.groupId, t.where === 'before' ? l.id : next?.id ?? '')
      }
    } else if (t.kind === 'group') {
      const rest = snap.groups.filter(g => g.id !== d.id)
      const i = rest.findIndex(g => g.id === t.id)
      if (i >= 0) request = api.moveGroup(d.id, i + (t.where === 'after' ? 1 : 0))
    }
    if (request) await send(request)
  }

  // ---------- 不用拖动的移动方式：手柄菜单和方向键 ----------

  /** 上移或下移一格；到了分组的头尾就进入相邻的分组（向上放到末尾，向下放到最前）。 */
  const moveListBy = async (id: string, delta: -1 | 1) => {
    if (pending) return
    const sections = buildSections(snap)
    const si = sections.findIndex(s => s.lists.some(l => l.id === id))
    if (si < 0) return
    const { group, lists } = sections[si]
    const j = lists.findIndex(l => l.id === id) + delta
    // 下移时放到“下一个的下一个”前面；后面没有了就传空，由后端放到分组末尾。
    if (j >= 0 && j < lists.length) return send(api.moveList(id, group?.id ?? '', delta < 0 ? lists[j].id : lists[j + 1]?.id ?? ''))
    const next = sections[si + delta]
    if (!next) return
    const gid = next.group?.id ?? ''
    // 目标分组是折叠的就先展开，否则分类移过去之后就看不到了。
    if (next.group?.collapsed) await api.setGroupCollapsed(gid, false)
    return send(api.moveList(id, gid, delta < 0 ? '' : next.lists[0]?.id ?? ''))
  }
  const moveListTo = async (id: string, groupId: string) => {
    if (pending) return
    if (snap.groups.find(g => g.id === groupId)?.collapsed) await api.setGroupCollapsed(groupId, false)
    await send(api.moveList(id, groupId, ''))
  }
  const moveGroupBy = async (id: string, delta: -1 | 1) => {
    const i = snap.groups.findIndex(g => g.id === id)
    if (pending || i < 0 || i + delta < 0 || i + delta >= snap.groups.length) return
    await send(api.moveGroup(id, i + delta))
  }

  const dropClass = (kind: Drop['kind'], id: string) => !drop || drop.id !== id || drop.kind !== kind ? '' : 'where' in drop ? `drop-${drop.where}` : 'drop-into'
  return { start, end, overList, overGroup, leave, onDrop, dropClass, clear: () => mark(null), pending, active, moveListBy, moveListTo, moveGroupBy }
}

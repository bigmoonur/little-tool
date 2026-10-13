import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { useApp } from '../ctx'
import type { Category, Group, State } from '../types'

export interface Section {
  group: Group | null
  categories: Category[]
}

/** 未分组始终在最后，也给拖出分组的分类留一个落点。 */
export function buildSections(state: State): Section[] {
  const sections: Section[] = state.groups.map(group => ({ group, categories: state.categories.filter(c => c.groupId === group.id) }))
  const loose = state.categories.filter(c => !c.groupId)
  if (loose.length || state.groups.length || !sections.length) sections.push({ group: null, categories: loose })
  return sections
}

type Kind = 'category' | 'group'
type Drag = { kind: Kind; id: string }
type Drop = { kind: Kind; id: string; where: 'before' | 'after' } | { kind: 'group-into'; id: string }

export function useReorder(blocked = false, expandOnDrop = true) {
  const { state, run, setState, copy, adding } = useApp()
  const dragging = useRef<Drag | null>(null)
  const target = useRef<Drop | null>(null)
  const [drop, setDrop] = useState<Drop | null>(null)
  const [active, setActive] = useState<Drag | null>(null)
  const [pending, setPending] = useState(false)
  const disabled = blocked || pending || copy.phase === 'copying' || !!adding
  const mark = (d: Drop | null) => {
    target.current = d
    setDrop(d)
  }
  const start = (e: React.DragEvent, kind: Kind, id: string) => {
    if (disabled) return e.preventDefault()
    e.stopPropagation()
    dragging.current = { kind, id }
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(`text/x-${kind}`, id)
    // 拖动真正开始后再更新界面，免得浏览器因 dragstart 里改动页面而取消拖动。
    window.setTimeout(() => setActive(dragging.current))
  }
  const end = () => {
    dragging.current = null
    setActive(null)
    mark(null)
  }
  useEffect(() => {
    if (disabled) end()
  }, [disabled])
  // dragenter 也记下落点，紧接着松手时不会漏掉移动。
  const overCategory = (e: React.DragEvent, c: Category) => {
    if (disabled || dragging.current?.kind !== 'category') return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    const r = e.currentTarget.getBoundingClientRect()
    mark({ kind: 'category', id: c.name, where: e.clientY < r.top + r.height / 2 ? 'before' : 'after' })
  }
  const overGroup = (e: React.DragEvent, id: string) => {
    const d = dragging.current
    if (disabled || !d) return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    if (d.kind === 'category') return mark({ kind: 'group-into', id })
    const from = state.groups.findIndex(g => g.id === d.id)
    const to = state.groups.findIndex(g => g.id === id)
    mark(to >= 0 && to !== from ? { kind: 'group', id, where: from < to ? 'after' : 'before' } : null)
  }
  const leave = (e: React.DragEvent) => {
    const r = e.currentTarget.getBoundingClientRect()
    if (e.clientX <= r.left || e.clientX >= r.right || e.clientY <= r.top || e.clientY >= r.bottom) mark(null)
  }
  const send = async (request: Promise<State>) => {
    setPending(true)
    try {
      const s = await run(request)
      if (s) setState(s)
    } finally { setPending(false) }
  }
  const moveCategory = async (name: string, groupId: string, beforeName: string) => {
    if (disabled) return
    setPending(true)
    try {
      if (state.groups.find(g => g.id === groupId)?.collapsed) {
        const expanded = await run(api.setGroupCollapsed(groupId, false))
        if (!expanded) return
      }
      const s = await run(api.moveCategory(name, groupId, beforeName))
      if (s) setState(s)
    } finally { setPending(false) }
  }
  const moveCategoryTo = (name: string, groupId: string) => moveCategory(name, groupId, '')
  const onDrop = async (e: React.DragEvent) => {
    if (!dragging.current) return
    e.preventDefault()
    e.stopPropagation()
    const d = dragging.current
    const t = target.current
    end()
    if (!t || disabled) return
    if (d.kind === 'category') {
      if (t.kind === 'group-into') {
        return expandOnDrop ? moveCategoryTo(d.id, t.id) : send(api.moveCategory(d.id, t.id, ''))
      }
      if (t.kind === 'category' && t.id !== d.id) {
        const c = state.categories.find(x => x.name === t.id)
        if (!c) return
        const same = state.categories.filter(x => x.groupId === c.groupId && x.name !== d.id)
        const next = same[same.findIndex(x => x.name === c.name) + 1]
        return send(api.moveCategory(d.id, c.groupId, t.where === 'before' ? c.name : next?.name ?? ''))
      }
    } else if (t.kind === 'group') {
      const rest = state.groups.filter(g => g.id !== d.id)
      const at = rest.findIndex(g => g.id === t.id)
      if (at >= 0) return send(api.moveGroup(d.id, rest[at + (t.where === 'after' ? 1 : 0)]?.id ?? ''))
    }
  }
  // ---------- 不用拖动的移动方式：手柄菜单和方向键 ----------

  /** 上移或下移一格；到了分组的头尾就进入相邻的分组。 */
  const moveCategoryBy = async (name: string, delta: -1 | 1) => {
    if (disabled) return
    const sections = buildSections(state)
    const si = sections.findIndex(s => s.categories.some(c => c.name === name))
    if (si < 0) return
    const { group, categories } = sections[si]
    const j = categories.findIndex(c => c.name === name) + delta
    // 下移时放到“下一个的下一个”前面；没有了就放到分组末尾。
    if (j >= 0 && j < categories.length) {
      return send(api.moveCategory(name, group?.id ?? '', delta < 0 ? categories[j].name : categories[j + 1]?.name ?? ''))
    }
    const next = sections[si + delta]
    if (!next) return
    // 跨组移动时先展开目标分组，移动后才能看见分类并取回焦点。
    return moveCategory(name, next.group?.id ?? '', delta < 0 ? '' : next.categories[0]?.name ?? '')
  }
  const moveGroupBy = async (id: string, delta: -1 | 1) => {
    const i = state.groups.findIndex(g => g.id === id)
    if (disabled || i < 0 || i + delta < 0 || i + delta >= state.groups.length) return
    const rest = state.groups.filter(g => g.id !== id)
    await send(api.moveGroup(id, rest[i + delta]?.id ?? ''))
  }
  const dropClass = (kind: Drop['kind'], id: string) => !drop || drop.id !== id || drop.kind !== kind ? '' : 'where' in drop ? `drop-${drop.where}` : 'drop-into'
  return { start, end, overCategory, overGroup, leave, onDrop, dropClass, disabled, active, moveCategoryBy, moveCategoryTo, moveGroupBy }
}

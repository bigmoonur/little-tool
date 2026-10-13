import { useRef, useState } from 'react'
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

export function useReorder() {
  const { state, run, setState, copy, adding } = useApp()
  const dragging = useRef<Drag | null>(null)
  const target = useRef<Drop | null>(null)
  const [drop, setDrop] = useState<Drop | null>(null)
  const [pending, setPending] = useState(false)
  const disabled = pending || copy.phase === 'copying' || !!adding
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
  }
  const end = () => {
    dragging.current = null
    mark(null)
  }
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
  const moveCategoryTo = async (name: string, groupId: string) => {
    if (disabled) return
    setPending(true)
    try {
      if (state.groups.find(g => g.id === groupId)?.collapsed) {
        const expanded = await run(api.setGroupCollapsed(groupId, false))
        if (!expanded) return
      }
      const s = await run(api.moveCategory(name, groupId, ''))
      if (s) setState(s)
    } finally { setPending(false) }
  }
  const onDrop = async (e: React.DragEvent) => {
    if (!dragging.current) return
    e.preventDefault()
    e.stopPropagation()
    const d = dragging.current
    const t = target.current
    end()
    if (!t || disabled) return
    if (d.kind === 'category') {
      if (t.kind === 'group-into') return moveCategoryTo(d.id, t.id)
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
  const dropClass = (kind: Drop['kind'], id: string) => !drop || drop.id !== id || drop.kind !== kind ? '' : 'where' in drop ? `drop-${drop.where}` : 'drop-into'
  return { start, end, overCategory, overGroup, leave, onDrop, dropClass, disabled, moveCategoryTo }
}

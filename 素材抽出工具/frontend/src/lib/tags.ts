import { useMemo } from 'react'
import { useApp } from '../ctx'
import type { Tag } from '../types'

/** 按 id 查标签。 */
export function useTagMap() {
  const { snap } = useApp()
  return useMemo(() => new Map<string, Tag>(snap.tags.map(t => [t.id, t])), [snap.tags])
}

/** 每个标签贴在多少个分类上。 */
export function useTagTotals() {
  const { snap } = useApp()
  return useMemo(() => {
    const totals = new Map<string, number>()
    for (const l of snap.lists) for (const id of l.tagIds) totals.set(id, (totals.get(id) ?? 0) + 1)
    return totals
  }, [snap.lists])
}

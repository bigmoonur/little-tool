import { useState } from 'react'
import { api } from '../api'
import { useApp } from '../ctx'
import type { Category } from '../types'
import { anchorOf, type Anchor } from '../ui/Popover'
import { TagManager } from './TagManager'
import { TagPopover } from './TagPopover'

/** 给分类贴标签：在按钮旁边弹出小面板，点一下贴上或取下。 */
export function useCategoryTagging() {
  const { state, setState, run, copy, adding } = useApp()
  const [target, setTarget] = useState<{ name: string; anchor: Anchor } | null>(null)
  const [busy, setBusy] = useState(false)
  const [managing, setManaging] = useState(false)
  const disabled = copy.phase === 'copying' || !!adding
  const category = target ? state.categories.find(c => c.name === target.name) : undefined

  const toggle = async (tagId: string, on: boolean) => {
    if (!category || busy || disabled) return
    setBusy(true)
    const s = await run(api.setCategoryTags(category.name, on ? [...category.tagIds, tagId] : category.tagIds.filter(id => id !== tagId)))
    setBusy(false)
    if (s) setState(s)
  }

  const node = (
    <>
      {target && category && (
        <TagPopover
          anchor={target.anchor}
          title={`「${category.name}」的标签`}
          checked={new Set(category.tagIds)}
          locked={busy || disabled}
          onToggle={toggle}
          onManage={() => {
            setTarget(null)
            setManaging(true)
          }}
          onClose={() => setTarget(null)}
        />
      )}
      {managing && <TagManager onClose={() => setManaging(false)} />}
    </>
  )

  return {
    /** 在 el 旁边打开这个分类的标签面板。 */
    open: (c: Category, el: Element) => setTarget({ name: c.name, anchor: anchorOf(el) }),
    /** 打开“管理标签”。 */
    manage: () => setManaging(true),
    openName: target?.name ?? '',
    node,
  }
}

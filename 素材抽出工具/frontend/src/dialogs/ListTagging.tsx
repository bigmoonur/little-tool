import { useState } from 'react'
import { api } from '../api'
import { useApp } from '../ctx'
import type { ListInfo } from '../types'
import { anchorOf } from '../ui/Menu'
import type { MenuAnchor } from '../ui/Menu'
import { TagManager } from './TagManager'
import { TagPopover } from './TagPopover'

/** 给分类贴标签：在按钮旁边弹出小面板，点一下贴上或取下。抽取页和素材库共用。 */
export function useListTagging() {
  const { snap, setSnap, run } = useApp()
  const [target, setTarget] = useState<{ listId: string; anchor: MenuAnchor } | null>(null)
  const [busy, setBusy] = useState(false)
  const [managing, setManaging] = useState(false)
  const list = target ? snap.lists.find(l => l.id === target.listId) : undefined

  const toggle = async (tagId: string, on: boolean) => {
    if (!list || busy) return
    setBusy(true)
    const s = await run(api.setListTags(list.id, on ? [...list.tagIds, tagId] : list.tagIds.filter(id => id !== tagId)))
    setBusy(false)
    if (s) setSnap(s)
  }

  const node = (
    <>
      {target && list && (
        <TagPopover
          anchor={target.anchor}
          title={`「${list.name}」的标签`}
          checked={new Set(list.tagIds)}
          locked={busy}
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
    open: (l: ListInfo, el: Element) => setTarget({ listId: l.id, anchor: anchorOf(el) }),
    openId: target?.listId ?? '',
    node,
  }
}

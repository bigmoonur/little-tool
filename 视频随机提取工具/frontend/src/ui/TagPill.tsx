import type { Tag } from '../types'

/** 一个标签：自己颜色的圆点加名称。 */
export function TagPill({ tag, className = '' }: { tag: Tag; className?: string }) {
  return (
    <span className={`tag-pill c-${tag.color} ${className}`} title={tag.name}>
      <i className="tag-dot" aria-hidden />
      <span className="tag-pill-name">{tag.name}</span>
    </span>
  )
}

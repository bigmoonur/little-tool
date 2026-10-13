import type { Tag } from '../types'

/** 一个标签：自己颜色的圆点加名称。标签已被删除时显示成红色。 */
export function TagPill({ tag, className = '' }: { tag: Tag | undefined; className?: string }) {
  if (!tag) return <span className={`tag-pill is-gone ${className}`}>已删除的标签</span>
  return (
    <span className={`tag-pill c-${tag.color} ${className}`} title={tag.name}>
      <i className="tag-dot" aria-hidden />
      <span className="tag-pill-name">{tag.name}</span>
    </span>
  )
}

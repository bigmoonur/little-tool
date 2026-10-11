// 本程序的标志：倾斜的胶片，中间被抽出的一格带播放三角。与「文本随机提取工具」同一套颜色，图形不同。
const HOLES = [3.2, 6.4, 9.6, 12.8, 16, 19.2, 22.4]

export function Logo({ className = '' }: { className?: string }) {
  return (
    <svg className={`logo ${className}`} viewBox="0 0 28 28" aria-hidden>
      <g transform="rotate(-14 14 14)">
        <rect x="8.4" y="1.2" width="11.2" height="25.6" rx="1.6" className="logo-strip" />
        {HOLES.map(y => (
          <g key={y} className="logo-hole">
            <rect x="9.2" y={y} width="1.3" height="1.6" rx="0.4" />
            <rect x="17.5" y={y} width="1.3" height="1.6" rx="0.4" />
          </g>
        ))}
      </g>
      <rect x="8" y="9" width="12" height="10.5" rx="1.8" className="logo-frame" />
      <path d="M12.6 11.6v5.2l4.4-2.6z" className="logo-play" />
    </svg>
  )
}

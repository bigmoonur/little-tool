// 回车键：左侧是“回车”二字，右侧是左上角缺一块的大回车键，键帽上画回车箭头。
const SHAPE =
  'M13.75 0.75H26.25A3 3 0 0 1 29.25 3.75V24.25A3 3 0 0 1 26.25 27.25H3.75A3 3 0 0 1 0.75 24.25V13.75' +
  'A3 3 0 0 1 3.75 10.75H7.75A3 3 0 0 0 10.75 7.75V3.75A3 3 0 0 1 13.75 0.75Z'

export function EnterKey({ className = '', label = true }: { className?: string; label?: boolean }) {
  return (
    <span className={`enter-key ${className}`} role="img" aria-label="回车">
      {label && <span className="ek-label" aria-hidden>回车</span>}
      <svg viewBox="0 0 30 29" aria-hidden>
        <path className="ek-shadow" d={SHAPE} transform="translate(0 1.5)" />
        <path className="ek-face" d={SHAPE} />
        <path className="ek-arrow" d="M21 6.5V18.5H8M11.5 15L8 18.5L11.5 22" />
      </svg>
    </span>
  )
}

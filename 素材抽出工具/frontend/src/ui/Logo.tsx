export function Logo({ className = '' }: { className?: string }) {
  return (
    <svg className={`logo ${className}`} viewBox="0 0 28 28" aria-hidden>
      <rect x="3" y="7" width="15" height="18" rx="2.5" className="logo-back" />
      <rect x="7" y="5" width="15" height="18" rx="2.5" className="logo-mid" transform="rotate(-6 14 14)" />
      <rect x="10" y="3" width="15" height="18" rx="2.5" className="logo-front" transform="rotate(8 17 12)" />
    </svg>
  )
}

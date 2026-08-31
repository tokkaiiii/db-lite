// PROTOTYPE — QueryPage 디자인 방향 탐색용 임시 컴포넌트. 방향이 정해지면
// 지운다. 프로덕션 빌드에서는 렌더링하지 않는다.
import { useEffect } from 'react'

type Props<T extends string> = {
  variants: readonly T[]
  labels: Record<T, string>
  current: T
  onChange: (next: T) => void
}

export function PrototypeSwitcher<T extends string>({ variants, labels, current, onChange }: Props<T>) {
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null
      if (target && ['INPUT', 'TEXTAREA'].includes(target.tagName)) return
      if (target?.isContentEditable) return
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return

      const idx = variants.indexOf(current)
      const delta = e.key === 'ArrowLeft' ? -1 : 1
      const next = variants[(idx + delta + variants.length) % variants.length]
      onChange(next)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [variants, current, onChange])

  if (import.meta.env.PROD) return null

  const idx = variants.indexOf(current)
  function cycle(delta: number) {
    onChange(variants[(idx + delta + variants.length) % variants.length])
  }

  return (
    <div className="proto-switcher">
      <button onClick={() => cycle(-1)} aria-label="이전 변형">
        ←
      </button>
      <span>
        {current} — {labels[current]}
      </span>
      <button onClick={() => cycle(1)} aria-label="다음 변형">
        →
      </button>
    </div>
  )
}

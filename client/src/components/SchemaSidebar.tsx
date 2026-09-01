import { useMemo, useState } from 'react'
import './schemaSidebar.css'

interface SchemaSidebarProps {
  schema: Record<string, string[]>
  onRefresh: () => void
}

export function SchemaSidebar({ schema, onRefresh }: SchemaSidebarProps) {
  const [open, setOpen] = useState(true)
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const tables = useMemo(() => {
    const q = query.trim().toLowerCase()
    return Object.entries(schema)
      .filter(([table]) => !q || table.toLowerCase().includes(q))
      .sort(([a], [b]) => a.localeCompare(b))
  }, [schema, query])

  function toggleTable(table: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(table)) next.delete(table)
      else next.add(table)
      return next
    })
  }

  if (!open) {
    return (
      <div className="schema-sidebar-rail">
        <button title="사이드바 펼치기" aria-label="사이드바 펼치기" onClick={() => setOpen(true)}>
          <ChevronDoubleIcon direction="right" />
        </button>
      </div>
    )
  }

  return (
    <aside className="schema-sidebar">
      <div className="schema-sidebar-header">
        <div className="schema-sidebar-search">
          <SearchIcon />
          <input
            type="text"
            placeholder="테이블 검색"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <button title="스키마 새로고침" aria-label="스키마 새로고침" onClick={onRefresh}>
          <RefreshIcon />
        </button>
        <button title="사이드바 접기" aria-label="사이드바 접기" onClick={() => setOpen(false)}>
          <ChevronDoubleIcon direction="left" />
        </button>
      </div>

      <div className="schema-sidebar-tree">
        {tables.length === 0 && <p className="schema-sidebar-empty">테이블이 없습니다</p>}
        {tables.map(([table, columns]) => {
          const isExpanded = expanded.has(table)
          return (
            <div key={table}>
              <div className="schema-sidebar-table" onClick={() => toggleTable(table)} title={table}>
                <ChevronIcon expanded={isExpanded} />
                <TableIcon />
                <span className="schema-sidebar-table-name">{table}</span>
                <span className="schema-sidebar-count">{columns.length}</span>
              </div>
              {isExpanded && (
                <div className="schema-sidebar-columns">
                  {columns.map((column) => (
                    <div key={column} className="schema-sidebar-column">
                      {column}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </aside>
  )
}

function SearchIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <circle cx="11" cy="11" r="7" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  )
}

function RefreshIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
  )
}

function ChevronDoubleIcon({ direction }: { direction: 'left' | 'right' }) {
  const points = direction === 'left' ? ['11 17 6 12 11 7', '18 17 13 12 18 7'] : ['13 17 18 12 13 7', '6 17 11 12 6 7']
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points={points[0]} />
      <polyline points={points[1]} />
    </svg>
  )
}

function ChevronIcon({ expanded }: { expanded: boolean }) {
  return (
    <svg
      width="11"
      height="11"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="schema-sidebar-chevron"
      style={{ transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)' }}
    >
      <polyline points="9 18 15 12 9 6" />
    </svg>
  )
}

function TableIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="4" width="18" height="16" rx="1.5" />
      <line x1="3" y1="10" x2="21" y2="10" />
      <line x1="9" y1="10" x2="9" y2="20" />
    </svg>
  )
}

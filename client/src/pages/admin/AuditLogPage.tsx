import { useEffect, useMemo, useState } from 'react'
import * as api from '../../api/client'
import type { AuditLogEntry, Connection, User } from '../../api/types'

type AllowedFilter = 'all' | 'allowed' | 'denied'

export function AuditLogPage() {
  const [entries, setEntries] = useState<AuditLogEntry[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [userFilter, setUserFilter] = useState<number | 'all'>('all')
  const [allowedFilter, setAllowedFilter] = useState<AllowedFilter>('all')
  const [search, setSearch] = useState('')

  useEffect(() => {
    api.adminListAuditLog().then(setEntries)
    api.adminListUsers().then(setUsers)
    api.adminListConnections().then(setConnections)
  }, [])

  const usernameOf = useMemo(() => {
    const map = new Map(users.map((u) => [u.id, u.username]))
    return (id: number) => map.get(id) ?? `#${id}`
  }, [users])

  const connectionNameOf = useMemo(() => {
    const map = new Map(connections.map((c) => [c.id, c.name]))
    return (id: number) => map.get(id) ?? `#${id}`
  }, [connections])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return entries.filter((e) => {
      if (userFilter !== 'all' && e.userId !== userFilter) return false
      if (allowedFilter === 'allowed' && !e.allowed) return false
      if (allowedFilter === 'denied' && e.allowed) return false
      if (q && !e.statement.toLowerCase().includes(q) && !e.errorMessage?.toLowerCase().includes(q)) return false
      return true
    })
  }, [entries, userFilter, allowedFilter, search])

  return (
    <div>
      <h1>감사 로그 (쓰기 쿼리만 기록)</h1>
      <div className="inline-form">
        <select value={userFilter} onChange={(e) => setUserFilter(e.target.value === 'all' ? 'all' : Number(e.target.value))}>
          <option value="all">전체 사용자</option>
          {users.map((u) => (
            <option key={u.id} value={u.id}>
              {u.username}
            </option>
          ))}
        </select>
        <select value={allowedFilter} onChange={(e) => setAllowedFilter(e.target.value as AllowedFilter)}>
          <option value="all">허용/거부 전체</option>
          <option value="allowed">허용만</option>
          <option value="denied">거부만</option>
        </select>
        <input
          type="text"
          placeholder="쿼리·오류 메시지 검색"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ flex: 1, minWidth: '200px' }}
        />
      </div>
      {entries.length === 0 ? (
        <p>감사 로그가 없습니다.</p>
      ) : filtered.length === 0 ? (
        <p>조건에 맞는 로그가 없습니다.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>시각</th>
              <th>사용자</th>
              <th>Connection</th>
              <th>허용</th>
              <th>쿼리</th>
              <th>오류</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((e) => (
              <tr key={e.id}>
                <td>{e.executedAt}</td>
                <td>{usernameOf(e.userId)}</td>
                <td>{connectionNameOf(e.connectionId)}</td>
                <td>
                  <span className={e.allowed ? 'badge badge-allowed' : 'badge badge-denied'}>
                    {e.allowed ? '허용' : '거부'}
                  </span>
                </td>
                <td className="audit-statement">
                  <code>{e.statement}</code>
                </td>
                <td className="error">{e.errorMessage}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

import { useEffect, useState } from 'react'
import * as api from '../../api/client'
import type { AuditLogEntry } from '../../api/types'

export function AuditLogPage() {
  const [entries, setEntries] = useState<AuditLogEntry[]>([])

  useEffect(() => {
    api.adminListAuditLog().then(setEntries)
  }, [])

  return (
    <div>
      <h1>감사 로그 (쓰기 쿼리만 기록)</h1>
      <table>
        <thead>
          <tr>
            <th>시각</th>
            <th>사용자 ID</th>
            <th>Connection ID</th>
            <th>허용</th>
            <th>쿼리</th>
            <th>오류</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr key={e.id}>
              <td>{e.executedAt}</td>
              <td>{e.userId}</td>
              <td>{e.connectionId}</td>
              <td>{e.allowed ? '허용' : '거부'}</td>
              <td><code>{e.statement}</code></td>
              <td>{e.errorMessage}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

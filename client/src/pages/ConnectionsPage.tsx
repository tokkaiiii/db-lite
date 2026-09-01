import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import * as api from '../api/client'
import type { ConnectionWithLevel } from '../api/types'
import { DbKindBadge } from '../components/DbKindBadge'

export function ConnectionsPage() {
  const [connections, setConnections] = useState<ConnectionWithLevel[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .listConnections()
      .then(setConnections)
      .catch((err) => setError(err.message))
  }, [])

  if (error) return <p className="error">{error}</p>
  if (!connections) return <p>불러오는 중...</p>
  if (connections.length === 0) return <p>접근 가능한 Connection이 없습니다. 관리자에게 권한을 요청하세요.</p>

  return (
    <div>
      <h1>Connections</h1>
      <table>
        <thead>
          <tr>
            <th>이름</th>
            <th>종류</th>
            <th>호스트</th>
            <th>권한</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {connections.map((c) => (
            <tr key={c.id}>
              <td>{c.name}</td>
              <td><DbKindBadge kind={c.kind} /></td>
              <td>{c.host}:{c.port}</td>
              <td>{c.level}</td>
              <td>
                <Link to={`/connections/${c.id}/query`}>쿼리 실행</Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

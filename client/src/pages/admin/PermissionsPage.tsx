import { useEffect, useState } from 'react'
import * as api from '../../api/client'
import type { Connection, PermissionLevel, User } from '../../api/types'

const LEVELS: PermissionLevel[] = ['none', 'read', 'write']

export function PermissionsPage() {
  const [users, setUsers] = useState<User[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [userId, setUserId] = useState<number | null>(null)
  const [connectionId, setConnectionId] = useState<number | null>(null)
  const [level, setLevel] = useState<PermissionLevel>('read')
  const [status, setStatus] = useState<string | null>(null)

  useEffect(() => {
    api.adminListUsers().then((u) => {
      setUsers(u)
      setUserId(u[0]?.id ?? null)
    })
    api.adminListConnections().then((c) => {
      setConnections(c)
      setConnectionId(c[0]?.id ?? null)
    })
  }, [])

  async function apply() {
    if (userId == null || connectionId == null) return
    setStatus(null)
    await api.adminSetPermission(userId, connectionId, level)
    setStatus('저장되었습니다')
  }

  return (
    <div>
      <h1>권한 부여</h1>
      <div className="inline-form">
        <select value={userId ?? ''} onChange={(e) => setUserId(Number(e.target.value))}>
          {users.map((u) => (
            <option key={u.id} value={u.id}>
              {u.username}
            </option>
          ))}
        </select>
        <select value={connectionId ?? ''} onChange={(e) => setConnectionId(Number(e.target.value))}>
          {connections.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <select value={level} onChange={(e) => setLevel(e.target.value as PermissionLevel)}>
          {LEVELS.map((l) => (
            <option key={l} value={l}>
              {l}
            </option>
          ))}
        </select>
        <button onClick={apply}>저장</button>
      </div>
      {status && <p>{status}</p>}
    </div>
  )
}

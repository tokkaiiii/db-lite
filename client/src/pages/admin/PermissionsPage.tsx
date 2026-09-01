import { useEffect, useState } from 'react'
import * as api from '../../api/client'
import type { Connection, Permission, PermissionLevel, User } from '../../api/types'

const LEVELS: PermissionLevel[] = ['none', 'read', 'write']

export function PermissionsPage() {
  const [users, setUsers] = useState<User[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [userId, setUserId] = useState<number | null>(null)
  const [connectionId, setConnectionId] = useState<number | null>(null)
  const [level, setLevel] = useState<PermissionLevel>('read')
  const [status, setStatus] = useState<string | null>(null)

  function reloadPermissions() {
    api.adminListAllPermissions().then(setPermissions)
  }

  useEffect(() => {
    api.adminListUsers().then((u) => {
      setUsers(u)
      setUserId(u[0]?.id ?? null)
    })
    api.adminListConnections().then((c) => {
      setConnections(c)
      setConnectionId(c[0]?.id ?? null)
    })
    reloadPermissions()
  }, [])

  async function apply() {
    if (userId == null || connectionId == null) return
    setStatus(null)
    await api.adminSetPermission(userId, connectionId, level)
    setStatus('저장되었습니다')
    reloadPermissions()
  }

  async function revoke(p: Permission) {
    const confirmed = confirm(
      `${usernameOf(p.userId)}의 "${connectionNameOf(p.connectionId)}" 권한을 회수하시겠습니까?`,
    )
    if (!confirmed) return
    await api.adminSetPermission(p.userId, p.connectionId, 'none')
    reloadPermissions()
  }

  function usernameOf(id: number) {
    return users.find((u) => u.id === id)?.username ?? `#${id}`
  }

  function connectionNameOf(id: number) {
    return connections.find((c) => c.id === id)?.name ?? `#${id}`
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

      <h2>부여된 권한</h2>
      <table>
        <thead>
          <tr>
            <th>User</th>
            <th>Connection</th>
            <th>권한</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {permissions.map((p) => (
            <tr key={`${p.userId}-${p.connectionId}`}>
              <td>{usernameOf(p.userId)}</td>
              <td>{connectionNameOf(p.connectionId)}</td>
              <td>{p.level}</td>
              <td>
                <button className="button-danger" onClick={() => revoke(p)}>
                  회수
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

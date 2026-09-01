import { useEffect, useMemo, useState } from 'react'
import * as api from '../../api/client'
import type { Connection, Permission, PermissionLevel, User } from '../../api/types'
import { DbKindBadge } from '../../components/DbKindBadge'
import './permissionsPage.css'

const LEVELS: PermissionLevel[] = ['none', 'read', 'write']

const LEVEL_CLASS: Record<PermissionLevel, string> = {
  none: 'perm-none',
  read: 'perm-read',
  write: 'perm-write',
}

export function PermissionsPage() {
  const [users, setUsers] = useState<User[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [userQuery, setUserQuery] = useState('')

  function reloadPermissions() {
    api.adminListAllPermissions().then(setPermissions)
  }

  useEffect(() => {
    api.adminListUsers().then(setUsers)
    api.adminListConnections().then(setConnections)
    reloadPermissions()
  }, [])

  const levelMap = useMemo(() => {
    const map = new Map<string, PermissionLevel>()
    permissions.forEach((p) => map.set(`${p.userId}:${p.connectionId}`, p.level))
    return map
  }, [permissions])

  function levelOf(userId: number, connectionId: number): PermissionLevel {
    return levelMap.get(`${userId}:${connectionId}`) ?? 'none'
  }

  async function cycleCell(userId: number, connectionId: number) {
    const current = levelOf(userId, connectionId)
    const next = LEVELS[(LEVELS.indexOf(current) + 1) % LEVELS.length]
    await api.adminSetPermission(userId, connectionId, next)
    reloadPermissions()
  }

  const filteredUsers = useMemo(() => {
    const q = userQuery.trim().toLowerCase()
    return users.filter((u) => !q || u.username.toLowerCase().includes(q))
  }, [users, userQuery])

  return (
    <div>
      <h1>권한 부여</h1>
      <div className="permissions-toolbar">
        <div className="user-search">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <circle cx="11" cy="11" r="7" />
            <line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
          <input type="text" placeholder="사용자 검색" value={userQuery} onChange={(e) => setUserQuery(e.target.value)} />
        </div>
        <div className="permissions-legend">
          <span>
            <span className="legend-swatch none" /> none
          </span>
          <span>
            <span className="legend-swatch read" /> read
          </span>
          <span>
            <span className="legend-swatch write" /> write
          </span>
          <span>셀을 클릭하면 none → read → write 순서로 바뀝니다</span>
        </div>
      </div>

      {users.length === 0 ? (
        <p className="empty-state">등록된 사용자가 없습니다.</p>
      ) : connections.length === 0 ? (
        <p className="empty-state">등록된 Connection이 없습니다.</p>
      ) : filteredUsers.length === 0 ? (
        <p className="empty-state">조건에 맞는 사용자가 없습니다.</p>
      ) : (
        <div className="permissions-matrix-wrap">
          <table className="permissions-matrix">
            <thead>
              <tr>
                <th className="user-col">사용자</th>
                {connections.map((c) => (
                  <th key={c.id}>
                    <div className="kind-cell">
                      <span className="conn-name">{c.name}</span>
                      <DbKindBadge kind={c.kind} />
                    </div>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filteredUsers.map((u) => (
                <tr key={u.id}>
                  <td className="user-col">
                    {u.username}
                    {u.isAdmin && <span style={{ color: 'var(--text-dim)', fontSize: '0.72rem' }}> (admin)</span>}
                  </td>
                  {connections.map((c) => {
                    const level = levelOf(u.id, c.id)
                    return (
                      <td
                        key={c.id}
                        className="level-cell"
                        onClick={() => cycleCell(u.id, c.id)}
                        title="클릭해서 권한 변경"
                      >
                        <div className={`perm-cell-btn ${LEVEL_CLASS[level]}`}>{level}</div>
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

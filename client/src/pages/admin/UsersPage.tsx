import { useEffect, useState, type FormEvent } from 'react'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { User } from '../../api/types'

export function UsersPage() {
  const [users, setUsers] = useState<User[]>([])
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function reload() {
    api.adminListUsers().then(setUsers).catch((err) => setError(err.message))
  }

  useEffect(reload, [])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.adminCreateUser(username, password, isAdmin)
      setUsername('')
      setPassword('')
      setIsAdmin(false)
      reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '생성에 실패했습니다')
    }
  }

  return (
    <div>
      <h1>사용자 관리</h1>
      <form onSubmit={handleCreate} className="inline-form">
        <label>
          아이디
          <input placeholder="아이디" value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <label>
          비밀번호
          <input
            placeholder="비밀번호"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        <label className="checkbox-field">
          <input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)} />
          Admin
        </label>
        <button type="submit">추가</button>
      </form>
      {error && <p className="error">{error}</p>}
      {users.length === 0 ? (
        <p className="empty-state">등록된 사용자가 없습니다.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>아이디</th>
              <th>Admin</th>
              <th>생성일</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td>{u.id}</td>
                <td>{u.username}</td>
                <td>{u.isAdmin && <CheckIcon />}</td>
                <td>{u.createdAt}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function CheckIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="3"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="check-icon"
      aria-label="관리자"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}

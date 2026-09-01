import { useEffect, useState, type FormEvent } from 'react'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { useAuth } from '../../auth/AuthContext'

export function UsersPage() {
  const { claims } = useAuth()
  const [users, setUsers] = useState<User[]>([])
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [resetId, setResetId] = useState<number | null>(null)
  const [newPassword, setNewPassword] = useState('')

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

  function startReset(id: number) {
    setResetId(id)
    setNewPassword('')
    setError(null)
  }

  function cancelReset() {
    setResetId(null)
    setNewPassword('')
  }

  async function submitReset(id: number) {
    if (!newPassword) return
    setError(null)
    try {
      await api.adminResetUserPassword(id, newPassword)
      cancelReset()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '비밀번호 재설정에 실패했습니다')
    }
  }

  async function handleDelete(u: User) {
    if (!confirm(`"${u.username}" 사용자를 삭제하시겠습니까? 이 작업은 되돌릴 수 없습니다.`)) return
    setError(null)
    try {
      await api.adminDeleteUser(u.id)
      reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '삭제에 실패했습니다')
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
              <th></th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td>{u.id}</td>
                <td>{u.username}</td>
                <td>{u.isAdmin && <CheckIcon />}</td>
                <td>{u.createdAt}</td>
                <td>
                  {resetId === u.id ? (
                    <span className="inline-form" style={{ margin: 0 }}>
                      <input
                        type="password"
                        placeholder="새 비밀번호"
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        autoFocus
                        style={{ width: '140px' }}
                      />
                      <button type="button" onClick={() => submitReset(u.id)}>
                        저장
                      </button>
                      <button type="button" onClick={cancelReset}>
                        취소
                      </button>
                    </span>
                  ) : (
                    <>
                      <button type="button" onClick={() => startReset(u.id)}>
                        비밀번호 재설정
                      </button>
                      {u.id !== claims?.userId && (
                        <button type="button" className="button-danger" onClick={() => handleDelete(u)}>
                          삭제
                        </button>
                      )}
                    </>
                  )}
                </td>
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

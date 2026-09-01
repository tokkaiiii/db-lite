import { useEffect, useState, type FormEvent } from 'react'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { Connection, DBKind } from '../../api/types'

const KINDS: DBKind[] = ['mssql', 'mysql', 'postgres', 'oracle']
const DEFAULT_PORTS: Record<DBKind, number> = {
  mssql: 1433,
  mysql: 3306,
  postgres: 5432,
  oracle: 1521,
}

export function ConnectionsAdminPage() {
  const [connections, setConnections] = useState<Connection[]>([])
  const [editingId, setEditingId] = useState<number | null>(null)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<DBKind>('mssql')
  const [host, setHost] = useState('')
  const [port, setPort] = useState(DEFAULT_PORTS.mssql)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [serviceName, setServiceName] = useState('')
  const [error, setError] = useState<string | null>(null)

  function reload() {
    api.adminListConnections().then(setConnections).catch((err) => setError(err.message))
  }

  useEffect(reload, [])

  function resetForm() {
    setEditingId(null)
    setName('')
    setKind('mssql')
    setHost('')
    setPort(DEFAULT_PORTS.mssql)
    setUsername('')
    setPassword('')
    setServiceName('')
  }

  function startEdit(c: Connection) {
    setEditingId(c.id)
    setName(c.name)
    setKind(c.kind)
    setHost(c.host)
    setPort(c.port)
    setUsername(c.username)
    // Never prefilled — the server doesn't send it back, and leaving this
    // blank on submit means "keep the existing password" (see api/client.ts).
    setPassword('')
    setServiceName(c.serviceName)
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      if (editingId != null) {
        await api.adminUpdateConnection(editingId, { name, kind, host, port, username, password, serviceName })
      } else {
        await api.adminCreateConnection({ name, kind, host, port, username, password, serviceName })
      }
      resetForm()
      reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : editingId != null ? '수정에 실패했습니다' : '생성에 실패했습니다')
    }
  }

  async function handleDelete(c: Connection) {
    if (!confirm(`"${c.name}" Connection을 삭제하시겠습니까? 이 작업은 되돌릴 수 없습니다.`)) return
    await api.adminDeleteConnection(c.id)
    if (editingId === c.id) resetForm()
    reload()
  }

  return (
    <div>
      <h1>Connection 관리</h1>
      <form onSubmit={handleSubmit} className="inline-form">
        <label>
          이름
          <input placeholder="이름" value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          종류
          <select
            value={kind}
            onChange={(e) => {
              const k = e.target.value as DBKind
              setKind(k)
              setPort(DEFAULT_PORTS[k])
            }}
          >
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
        </label>
        <label>
          호스트
          <input placeholder="호스트" value={host} onChange={(e) => setHost(e.target.value)} />
        </label>
        <label>
          포트
          <input
            placeholder="포트"
            type="number"
            value={port}
            onChange={(e) => setPort(Number(e.target.value))}
          />
        </label>
        <label>
          계정
          <input placeholder="계정" value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <label>
          비밀번호
          <input
            placeholder={editingId != null ? '비우면 기존 값 유지' : '비밀번호'}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {kind === 'oracle' && (
          <label>
            서비스명/SID
            <input
              placeholder="서비스명/SID"
              value={serviceName}
              onChange={(e) => setServiceName(e.target.value)}
            />
          </label>
        )}
        <button type="submit">{editingId != null ? '수정 저장' : '추가'}</button>
        {editingId != null && (
          <button type="button" onClick={resetForm}>
            취소
          </button>
        )}
      </form>
      {error && <p className="error">{error}</p>}
      {connections.length === 0 ? (
        <p className="empty-state">등록된 Connection이 없습니다.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>이름</th>
              <th>종류</th>
              <th>호스트</th>
              <th>계정</th>
              <th>서비스명/SID</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {connections.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td>{c.kind}</td>
                <td>{c.host}:{c.port}</td>
                <td>{c.username}</td>
                <td>{c.serviceName}</td>
                <td>
                  <button onClick={() => startEdit(c)}>수정</button>
                  <button className="button-danger" onClick={() => handleDelete(c)}>
                    삭제
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

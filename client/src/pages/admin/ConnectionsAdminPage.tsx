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

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.adminCreateConnection({ name, kind, host, port, username, password, serviceName })
      setName('')
      setHost('')
      setUsername('')
      setPassword('')
      setServiceName('')
      reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '생성에 실패했습니다')
    }
  }

  async function handleDelete(id: number) {
    await api.adminDeleteConnection(id)
    reload()
  }

  return (
    <div>
      <h1>Connection 관리</h1>
      <form onSubmit={handleCreate} className="inline-form">
        <input placeholder="이름" value={name} onChange={(e) => setName(e.target.value)} />
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
        <input placeholder="호스트" value={host} onChange={(e) => setHost(e.target.value)} />
        <input
          placeholder="포트"
          type="number"
          value={port}
          onChange={(e) => setPort(Number(e.target.value))}
        />
        <input placeholder="계정" value={username} onChange={(e) => setUsername(e.target.value)} />
        <input
          placeholder="비밀번호"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {kind === 'oracle' && (
          <input
            placeholder="서비스명/SID"
            value={serviceName}
            onChange={(e) => setServiceName(e.target.value)}
          />
        )}
        <button type="submit">추가</button>
      </form>
      {error && <p className="error">{error}</p>}
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
                <button onClick={() => handleDelete(c.id)}>삭제</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

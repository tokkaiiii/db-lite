import { useState } from 'react'
import { useParams } from 'react-router-dom'
import * as api from '../api/client'
import type { QueryResult } from '../api/types'
import { ApiError } from '../api/client'

export function QueryPage() {
  const { connectionId } = useParams()
  const [statement, setStatement] = useState('SELECT 1')
  const [result, setResult] = useState<QueryResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  async function run() {
    if (!connectionId) return
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const res = await api.executeQuery(Number(connectionId), statement)
      setResult(res)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '실행에 실패했습니다')
    } finally {
      setRunning(false)
    }
  }

  return (
    <div>
      <h1>쿼리 실행 (Connection #{connectionId})</h1>
      <textarea
        rows={8}
        value={statement}
        onChange={(e) => setStatement(e.target.value)}
        spellCheck={false}
      />
      <div>
        <button onClick={run} disabled={running}>
          실행
        </button>
      </div>
      {error && <p className="error">{error}</p>}
      {result && <QueryResultView result={result} />}
    </div>
  )
}

function QueryResultView({ result }: { result: QueryResult }) {
  if (result.rowsAffected !== undefined && !result.columns) {
    return <p>{result.rowsAffected}행이 영향을 받았습니다.</p>
  }
  if (!result.columns || !result.rows) {
    return <p>결과가 없습니다.</p>
  }
  return (
    <div>
      {result.truncated && <p className="warning">결과가 최대 행 수로 잘렸습니다.</p>}
      <table>
        <thead>
          <tr>
            {result.columns.map((c) => (
              <th key={c}>{c}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {result.rows.map((row, i) => (
            <tr key={i}>
              {row.map((cell, j) => (
                <td key={j}>{cell === null ? 'NULL' : String(cell)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

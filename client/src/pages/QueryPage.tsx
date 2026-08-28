import { useEffect, useMemo, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import CodeMirror from '@uiw/react-codemirror'
import { MSSQL, MySQL, PLSQL, PostgreSQL, sql, type SQLDialect } from '@codemirror/lang-sql'
import * as api from '../api/client'
import type { DBKind, QueryResult } from '../api/types'
import { ApiError } from '../api/client'

// PLSQL is the closest lang-sql dialect to Oracle's SQL flavor.
const DIALECTS: Record<DBKind, SQLDialect> = {
  mysql: MySQL,
  postgres: PostgreSQL,
  mssql: MSSQL,
  oracle: PLSQL,
}

export function QueryPage() {
  const { connectionId } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const catalog = searchParams.get('catalog') ?? ''

  const [kind, setKind] = useState<DBKind | null>(null)
  const [catalogs, setCatalogs] = useState<string[]>([])
  const [catalogsLoaded, setCatalogsLoaded] = useState(false)
  const [schema, setSchema] = useState<Record<string, string[]>>({})
  const [statement, setStatement] = useState('SELECT 1')
  const [result, setResult] = useState<QueryResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  useEffect(() => {
    if (!connectionId) return
    api.listConnections().then((conns) => {
      const conn = conns.find((c) => c.id === Number(connectionId))
      if (conn) setKind(conn.kind)
    })
  }, [connectionId])

  useEffect(() => {
    if (!connectionId) return
    setCatalogsLoaded(false)
    api
      .listCatalogs(Number(connectionId))
      .then(({ catalogs }) => {
        setCatalogs(catalogs)
        // Every DB kind that has a Catalog concept needs one to connect to,
        // so default to the first one rather than leaving the picker empty.
        if (catalogs.length > 0 && !searchParams.get('catalog')) {
          setSearchParams({ catalog: catalogs[0] }, { replace: true })
        }
        setCatalogsLoaded(true)
      })
      .catch(() => {
        setCatalogs([])
        setCatalogsLoaded(true)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connectionId])

  function loadSchema() {
    if (!connectionId) return
    api
      .describeSchema(Number(connectionId), catalog)
      .then(({ schema }) => setSchema(schema))
      .catch(() => setSchema({}))
  }

  useEffect(() => {
    if (!connectionId || !catalogsLoaded) return
    // If this Connection has Catalogs, wait for the default-selection
    // redirect above to land in the URL before fetching — otherwise this
    // fires once against no catalog at all.
    if (catalogs.length > 0 && !catalog) return
    loadSchema()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connectionId, catalogsLoaded, catalogs.length, catalog])

  const extensions = useMemo(
    () => [sql({ dialect: kind ? DIALECTS[kind] : undefined, schema, upperCaseKeywords: true })],
    [kind, schema],
  )

  async function run() {
    if (!connectionId) return
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const res = await api.executeQuery(Number(connectionId), statement, catalog)
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
      {catalogs.length > 0 && (
        <div>
          <label>
            Catalog:{' '}
            <select
              value={catalog}
              onChange={(e) => setSearchParams({ catalog: e.target.value })}
            >
              {catalogs.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
        </div>
      )}
      <CodeMirror value={statement} height="200px" extensions={extensions} onChange={setStatement} />
      <div>
        <button onClick={run} disabled={running}>
          실행
        </button>
        <button type="button" onClick={loadSchema}>
          스키마 새로고침
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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror'
import { keymap } from '@codemirror/view'
import { Prec, type EditorState } from '@codemirror/state'
import { syntaxTree } from '@codemirror/language'
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

// Finds the statement surrounding cursorPos using lang-sql's own parser
// (its grammar tokenizes String/LineComment/BlockComment separately from
// the ';' that ends a Statement node), rather than a plain ';' scan — so a
// ';' inside a string literal or a comment no longer incorrectly splits a
// statement.
function statementAtCursor(state: EditorState, cursorPos: number): string {
  let node = syntaxTree(state).resolveInner(cursorPos, -1)
  while (node && node.name !== 'Statement' && node.parent) node = node.parent
  if (!node || node.name !== 'Statement') return state.doc.toString().trim()
  return state.sliceDoc(node.from, node.to).trim()
}

export function QueryPage() {
  const { connectionId } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const catalog = searchParams.get('catalog') ?? ''

  const editorRef = useRef<ReactCodeMirrorRef>(null)
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

  // Multiple ';'-separated statements in the editor previously got sent to
  // the DB as one blob and failed with a confusing driver-level syntax
  // error. Instead, run only the statement the cursor is in (or the
  // selection, if there is one) — the same convention as most SQL editors
  // (DBeaver, SSMS, DataGrip).
  function statementToRun(): string {
    const view = editorRef.current?.view
    if (!view) return statement
    const sel = view.state.selection.main
    if (!sel.empty) return view.state.sliceDoc(sel.from, sel.to).trim()
    return statementAtCursor(view.state, sel.head)
  }

  async function run() {
    if (!connectionId) return
    const toRun = statementToRun()
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const res = await api.executeQuery(Number(connectionId), toRun, catalog)
      setResult(res)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '실행에 실패했습니다')
    } finally {
      setRunning(false)
    }
  }

  // Keeping a ref to the latest `run` lets the Ctrl/Cmd+Enter keymap below
  // stay in one stable extension instance instead of being rebuilt (and
  // briefly detached/reattached) on every keystroke.
  const runRef = useRef(run)
  runRef.current = run

  const extensions = useMemo(
    () => [
      sql({ dialect: kind ? DIALECTS[kind] : undefined, schema, upperCaseKeywords: true }),
      // Prec.highest so this wins over CodeMirror's own Enter-inserts-a-
      // newline binding, which otherwise fires first for Ctrl/Cmd+Enter too.
      Prec.highest(
        keymap.of([
          {
            key: 'Mod-Enter',
            run: () => {
              runRef.current()
              return true
            },
          },
        ]),
      ),
    ],
    [kind, schema],
  )

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
      <CodeMirror
        ref={editorRef}
        value={statement}
        height="200px"
        extensions={extensions}
        onChange={setStatement}
      />
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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror'
import { EditorView, keymap } from '@codemirror/view'
import { Prec, type EditorState } from '@codemirror/state'
import { syntaxTree } from '@codemirror/language'
import { acceptCompletion, type Completion } from '@codemirror/autocomplete'
import { MSSQL, MySQL, PLSQL, PostgreSQL, sql, type SQLDialect, type SQLNamespace } from '@codemirror/lang-sql'
import { formatDialect, mysql as sqlFormatterMysql, plsql, postgresql, transactsql } from 'sql-formatter'
import * as api from '../api/client'
import type { DBKind, QueryResult } from '../api/types'
import { ApiError } from '../api/client'
import './queryPage.css'

// PLSQL is the closest lang-sql dialect to Oracle's SQL flavor.
const DIALECTS: Record<DBKind, SQLDialect> = {
  mysql: MySQL,
  postgres: PostgreSQL,
  mssql: MSSQL,
  oracle: PLSQL,
}

const FORMAT_DIALECTS: Record<DBKind, Parameters<typeof formatDialect>[1]['dialect']> = {
  mysql: sqlFormatterMysql,
  postgres: postgresql,
  mssql: transactsql,
  oracle: plsql,
}

// Picks a short alias for table — initials of each `_`-separated word
// (order_items -> oi), or its first letter for a single-word name — then
// disambiguates against whatever's already in the document (by simple
// substring-of-a-word search) so completing a second table whose name
// starts the same way doesn't silently collide with the first one's alias.
function generateAlias(table: string, docText: string): string {
  const initials = table
    .split(/[_\s]+/)
    .map((part) => part[0]?.toLowerCase())
    .filter(Boolean)
    .join('')
  const base = initials || table[0]?.toLowerCase() || 't'
  const isTaken = (candidate: string) => new RegExp(`\\b${candidate}\\b`, 'i').test(docText)
  let candidate = base
  for (let n = 2; isTaken(candidate); n++) candidate = base + n
  return candidate
}

// Wraps each table's column list in a { self, children } SQLNamespace entry
// so selecting the table from autocomplete inserts "table alias" instead of
// just "table" — the alias-aware `alias.column` completion lang-sql already
// does (see CONTEXT.md's Catalog entry / dbtool#autocomplete work) then
// resolves it normally, since it re-parses "FROM table alias" from the
// document text rather than caring how it got typed.
function schemaWithAliasCompletion(schema: Record<string, string[]>): SQLNamespace {
  const namespace: SQLNamespace = {}
  for (const [table, columns] of Object.entries(schema)) {
    const self: Completion = {
      label: table,
      type: 'type',
      apply: (view: EditorView, _completion: Completion, from: number, to: number) => {
        const alias = generateAlias(table, view.state.doc.toString())
        const insert = `${table} ${alias}`
        view.dispatch({
          changes: { from, to, insert },
          selection: { anchor: from + insert.length },
        })
      },
    }
    namespace[table] = { self, children: columns }
  }
  return namespace
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

// DataGrip's "Complete Current Statement" (Mod-Shift-Enter): finds the end
// of the statement the cursor is in, ignoring trailing whitespace, and
// inserts a ';' there if one isn't already present — so you don't have to
// jump to the end of the line yourself just to terminate the statement.
function completeCurrentStatement(view: EditorView) {
  const { state } = view
  const pos = state.selection.main.head
  let node = syntaxTree(state).resolveInner(pos, -1)
  while (node && node.name !== 'Statement' && node.parent) node = node.parent
  const nodeEnd = node && node.name === 'Statement' ? node.to : state.doc.length
  let end = nodeEnd
  while (end > 0 && /\s/.test(state.doc.sliceString(end - 1, end))) end--

  if (state.doc.sliceString(Math.max(0, end - 1), end) === ';') {
    view.dispatch({ selection: { anchor: end } })
    return true
  }
  view.dispatch({
    changes: { from: end, to: end, insert: ';' },
    selection: { anchor: end + 1 },
  })
  return true
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

  const cmSchema = useMemo(() => schemaWithAliasCompletion(schema), [schema])

  // DataGrip's Reformat Code (Mod-Alt-l): formats the selection, or the
  // whole document if there is none.
  function formatEditor() {
    const view = editorRef.current?.view
    if (!view || !kind) return true
    const sel = view.state.selection.main
    const [from, to] = sel.empty ? [0, view.state.doc.length] : [sel.from, sel.to]
    const formatted = formatDialect(view.state.sliceDoc(from, to), { dialect: FORMAT_DIALECTS[kind] })
    view.dispatch({ changes: { from, to, insert: formatted } })
    return true
  }

  const extensions = useMemo(
    () => [
      sql({ dialect: kind ? DIALECTS[kind] : undefined, schema: cmSchema, upperCaseKeywords: true }),
      // Prec.highest so these win over CodeMirror's own bindings for the
      // same keys — Enter-inserts-a-newline (which otherwise fires first
      // for Ctrl/Cmd+Enter too) and Tab-indents. acceptCompletion() itself
      // returns false when no completion popup is open, so Tab still falls
      // through to the default indent behavior the rest of the time.
      Prec.highest(
        keymap.of([
          {
            key: 'Mod-Enter',
            run: () => {
              runRef.current()
              return true
            },
          },
          { key: 'Tab', run: acceptCompletion },
          // Matches JetBrains DataGrip's default bindings, per user request.
          { key: 'Mod-Alt-l', run: formatEditor },
          { key: 'Mod-Shift-Enter', run: completeCurrentStatement },
        ]),
      ),
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [kind, cmSchema],
  )

  return (
    <div className="query-page">
      <div className="query-breadcrumb">
        Connection #{connectionId}
        {catalogs.length > 0 && (
          <>
            {' / '}
            <select value={catalog} onChange={(e) => setSearchParams({ catalog: e.target.value })}>
              {catalogs.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </>
        )}
      </div>
      <div className="query-editor-wrap">
        <div className="query-floating-toolbar">
          <button className="run" onClick={run} disabled={running} title="실행 (Ctrl/Cmd+Enter)">
            ▶
          </button>
          <button type="button" onClick={formatEditor} title="포맷팅 (Ctrl/Cmd+Alt+L)">
            포맷
          </button>
          <button
            type="button"
            onClick={() => {
              const view = editorRef.current?.view
              if (view) completeCurrentStatement(view)
            }}
            title="문장 완성 (Ctrl/Cmd+Shift+Enter)"
          >
            ;
          </button>
          <button type="button" onClick={loadSchema} title="스키마 새로고침">
            ↻
          </button>
        </div>
        <CodeMirror ref={editorRef} value={statement} height="320px" theme="dark" extensions={extensions} onChange={setStatement} />
      </div>
      <div className="query-results">
        {error && <p className="error" style={{ padding: '0.5rem 0.75rem' }}>{error}</p>}
        {result && (
          <QueryResultView
            result={result}
            connectionId={connectionId ? Number(connectionId) : undefined}
            catalog={catalog}
          />
        )}
      </div>
      <div className="query-status">
        <span>{catalog || '-'}</span>
        <span>
          {result?.truncated
            ? '결과가 최대 행 수로 잘렸습니다'
            : result
              ? `${result.rows?.length ?? result.rowsAffected ?? 0}행`
              : '대기 중'}
        </span>
      </div>
    </div>
  )
}

function QueryResultView({
  result,
  connectionId,
  catalog,
}: {
  result: QueryResult
  connectionId?: number
  catalog: string
}) {
  if (result.rowsAffected !== undefined && !result.columns) {
    return <p>{result.rowsAffected}행이 영향을 받았습니다.</p>
  }
  if (!result.columns || !result.rows) {
    return <p>결과가 없습니다.</p>
  }

  const columns = result.columns
  // ADR 0009 / ADR 0011: whether a given column's download button shows up
  // is a per-cell decision now (JOINs can trace some columns' origin table
  // but not others), not a whole-result one — see QueryResult.columnOrigins.
  const origins = result.columnOrigins
  // 셀 값이 길어 한 줄로 잘려 보일 때, 클릭해서 그 셀만 펼쳐 전체 값을 볼 수
  // 있게 한다 — 원본 다운로드(서버 재조회)와 달리 이미 받아온 값을 그대로
  // 보여주는 것뿐이라 Cell Truncation(2KB)까지만 보인다.
  const [expandedCells, setExpandedCells] = useState<Set<string>>(new Set())

  function toggleExpand(key: string) {
    setExpandedCells((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  async function downloadCell(row: unknown[], columnIndex: number, expectedValue: string) {
    const origin = origins?.[columnIndex]
    if (!origin) return
    const primaryKey: Record<string, unknown> = {}
    origin.primaryKeyColumns.forEach((pk, i) => {
      primaryKey[pk] = row[origin.primaryKeyRowIndexes[i]]
    })
    try {
      await api.downloadCellFile(connectionId!, { catalog, table: origin.table, column: columns[columnIndex], primaryKey, expectedValue })
    } catch (e) {
      alert(e instanceof ApiError ? e.message : '다운로드에 실패했습니다.')
    }
  }

  return (
    <div>
      {result.truncated && <p className="warning">결과가 최대 행 수로 잘렸습니다.</p>}
      <table>
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c}>{c}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {result.rows.map((row, i) => (
            <tr key={i}>
              {columns.map((_, j) => {
                const cell = row[j]
                const cellKey = `${i}-${j}`
                const text = cell === null ? 'NULL' : String(cell)
                const expanded = expandedCells.has(cellKey)
                const downloadEnabled = connectionId !== undefined && !!origins?.[j]
                return (
                  <td
                    key={j}
                    className={expanded ? 'cell-expanded' : undefined}
                    onClick={() => toggleExpand(cellKey)}
                    title={expanded ? undefined : '클릭하면 전체 값 보기'}
                  >
                    <span className="cell-value">{text}</span>
                    {downloadEnabled && (
                      <button
                        type="button"
                        className="cell-download"
                        title="원본 값 다운로드"
                        onClick={(e) => {
                          e.stopPropagation()
                          downloadCell(row, j, text)
                        }}
                      >
                        ⭳
                      </button>
                    )}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
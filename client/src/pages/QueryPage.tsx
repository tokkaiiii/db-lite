import { useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror'
import { EditorView, keymap } from '@codemirror/view'
import { Prec, type EditorState, type Extension } from '@codemirror/state'
import { syntaxTree } from '@codemirror/language'
import { acceptCompletion, type Completion } from '@codemirror/autocomplete'
import { MSSQL, MySQL, PLSQL, PostgreSQL, sql, type SQLDialect, type SQLNamespace } from '@codemirror/lang-sql'
import { formatDialect, mysql as sqlFormatterMysql, plsql, postgresql, transactsql } from 'sql-formatter'
import * as api from '../api/client'
import type { DBKind, QueryResult } from '../api/types'
import { ApiError } from '../api/client'
import { PrototypeSwitcher } from '../components/PrototypeSwitcher'
import './queryPagePrototype.css'

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

// PROTOTYPE — DataGrip 스타일 방향 탐색용 variant 키. 방향이 정해지면
// VariantA/B/C와 이 타입, updateSearchParams의 variant 관련 부분을 지운다.
type ProtoVariant = 'A' | 'B' | 'C'
const PROTO_VARIANTS: readonly ProtoVariant[] = ['A', 'B', 'C']
const PROTO_LABELS: Record<ProtoVariant, string> = {
  A: '사이드바 트리 (라이트)',
  B: '다크 + 쿼리 탭',
  C: '다크 + 플로팅 툴바',
}

export function QueryPage() {
  const { connectionId } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const catalog = searchParams.get('catalog') ?? ''
  const variant = (searchParams.get('variant') as ProtoVariant | null) ?? 'A'

  // 기존 setSearchParams({ catalog: ... }) 호출이 전체 파라미터를 덮어써
  // variant가 URL에서 사라지던 것을 막기 위한 prototype용 병합 헬퍼.
  function updateSearchParams(patch: Record<string, string>, opts?: { replace?: boolean }) {
    const next = new URLSearchParams(searchParams)
    for (const [k, v] of Object.entries(patch)) next.set(k, v)
    setSearchParams(next, opts)
  }

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
          updateSearchParams({ catalog: catalogs[0] }, { replace: true })
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

  const variantProps: VariantProps = {
    connectionId,
    kind,
    catalogs,
    catalog,
    onCatalogChange: (v) => updateSearchParams({ catalog: v }),
    schema,
    editorRef,
    statement,
    setStatement,
    extensions,
    run,
    running,
    formatEditor,
    onCompleteStatement: () => {
      const view = editorRef.current?.view
      if (view) completeCurrentStatement(view)
    },
    loadSchema,
    error,
    result,
  }

  return (
    <>
      {variant === 'B' && <VariantB {...variantProps} />}
      {variant === 'C' && <VariantC {...variantProps} />}
      {variant !== 'B' && variant !== 'C' && <VariantA {...variantProps} />}
      <PrototypeSwitcher
        variants={PROTO_VARIANTS}
        labels={PROTO_LABELS}
        current={variant}
        onChange={(v) => updateSearchParams({ variant: v })}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// PROTOTYPE — DataGrip 스타일 방향 탐색용 세 가지 variant. 방향이 정해지면
// 승자만 남기고 이 구역(useCellDownload 제외)과 queryPagePrototype.css,
// PrototypeSwitcher.tsx를 지운다.
// ---------------------------------------------------------------------------

type VariantProps = {
  connectionId?: string
  kind: DBKind | null
  catalogs: string[]
  catalog: string
  onCatalogChange: (v: string) => void
  schema: Record<string, string[]>
  editorRef: RefObject<ReactCodeMirrorRef | null>
  statement: string
  setStatement: (v: string) => void
  extensions: Extension[]
  run: () => void
  running: boolean
  formatEditor: () => boolean
  onCompleteStatement: () => void
  loadSchema: () => void
  error: string | null
  result: QueryResult | null
}

// canDownloadCell mirrors the ADR 0009 gate: the server only fills in
// Table/PrimaryKey when the statement was a plain `SELECT * FROM <table>`
// on a table that has a primary key.
function canDownloadCell(connectionId: number | undefined, result: QueryResult): boolean {
  return connectionId !== undefined && !!result.table && !!result.primaryKey?.length
}

// 세 variant가 공유하는 "셀 원본 다운로드" 로직 — 레이아웃(JSX)은 각
// variant가 따로 그리되, 다운로드 판단/요청 로직만 공유한다.
function useCellDownload(connectionId: number | undefined, catalog: string, result: QueryResult) {
  const downloadEnabled = canDownloadCell(connectionId, result)
  const columns = result.columns ?? []
  const pkIndexes = downloadEnabled ? result.primaryKey!.map((pk) => columns.indexOf(pk)) : []

  async function downloadCell(row: unknown[], column: string) {
    const primaryKey: Record<string, unknown> = {}
    result.primaryKey!.forEach((pk, i) => {
      primaryKey[pk] = row[pkIndexes[i]]
    })
    try {
      await api.downloadCellFile(connectionId!, { catalog, table: result.table!, column, primaryKey })
    } catch (e) {
      alert(e instanceof ApiError ? e.message : '다운로드에 실패했습니다.')
    }
  }

  return { downloadEnabled, downloadCell }
}

function CatalogPicker({ catalogs, catalog, onCatalogChange }: Pick<VariantProps, 'catalogs' | 'catalog' | 'onCatalogChange'>) {
  if (catalogs.length === 0) return null
  return (
    <select value={catalog} onChange={(e) => onCatalogChange(e.target.value)}>
      {catalogs.map((c) => (
        <option key={c} value={c}>
          {c}
        </option>
      ))}
    </select>
  )
}

// ---------- Variant A: 라이트 + 왼쪽 스키마 트리 ----------
function VariantA(props: VariantProps) {
  const { connectionId, catalogs, catalog, onCatalogChange, schema, editorRef, statement, setStatement, extensions, run, running, formatEditor, onCompleteStatement, loadSchema, error, result } = props
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  function toggle(table: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(table)) next.delete(table)
      else next.add(table)
      return next
    })
  }

  return (
    <div className="proto-a">
      <aside className="proto-a-sidebar">
        <div className="proto-a-sidebar-title">Connection #{connectionId}</div>
        {Object.entries(schema).map(([table, columns]) => (
          <div key={table}>
            <div className="proto-a-table-row" onClick={() => toggle(table)}>
              {expanded.has(table) ? '▾' : '▸'} {table}
            </div>
            {expanded.has(table) &&
              columns.map((col) => (
                <div key={col} className="proto-a-column-row proto-mono">
                  {col}
                </div>
              ))}
          </div>
        ))}
        <div style={{ padding: '0.4rem 0.6rem' }}>
          <button type="button" onClick={loadSchema}>
            새로고침
          </button>
        </div>
      </aside>
      <div className="proto-a-main">
        <div className="proto-a-toolbar">
          <button className="run" onClick={run} disabled={running} title="Ctrl/Cmd+Enter">
            ▶ 실행
          </button>
          <button type="button" onClick={formatEditor} title="Ctrl/Cmd+Alt+L">
            포맷팅
          </button>
          <button type="button" onClick={onCompleteStatement} title="Ctrl/Cmd+Shift+Enter">
            문장 완성(;)
          </button>
          {catalogs.length > 0 && <CatalogPicker catalogs={catalogs} catalog={catalog} onCatalogChange={onCatalogChange} />}
        </div>
        <div className="proto-a-editor">
          <CodeMirror ref={editorRef} value={statement} height="280px" extensions={extensions} onChange={setStatement} />
        </div>
        <div className="proto-a-results">
          {error && <p className="error">{error}</p>}
          {result && <ResultTableA result={result} connectionId={connectionId ? Number(connectionId) : undefined} catalog={catalog} />}
        </div>
      </div>
    </div>
  )
}

function ResultTableA({ result, connectionId, catalog }: { result: QueryResult; connectionId?: number; catalog: string }) {
  if (result.rowsAffected !== undefined && !result.columns) return <p style={{ padding: '0.6rem' }}>{result.rowsAffected}행이 영향을 받았습니다.</p>
  if (!result.columns || !result.rows) return <p style={{ padding: '0.6rem' }}>결과가 없습니다.</p>
  const { downloadEnabled, downloadCell } = useCellDownload(connectionId, catalog, result)

  return (
    <>
      {result.truncated && <p className="warning" style={{ padding: '0 0.6rem' }}>결과가 최대 행 수로 잘렸습니다.</p>}
      <table>
        <thead>
          <tr>
            <th className="rownum">#</th>
            {result.columns.map((c) => (
              <th key={c}>{c}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {result.rows.map((row, i) => (
            <tr key={i}>
              <td className="rownum">{i + 1}</td>
              {row.map((cell, j) => (
                <td key={j} className="proto-mono">
                  {cell === null ? 'NULL' : String(cell)}
                  {downloadEnabled && (
                    <button type="button" className="cell-download" title="원본 값 다운로드" onClick={() => downloadCell(row, result.columns![j])}>
                      ⬇
                    </button>
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}

// ---------- Variant B: 다크 + 쿼리 탭 ----------
function VariantB(props: VariantProps) {
  const { connectionId, catalogs, catalog, onCatalogChange, editorRef, statement, setStatement, extensions, run, running, formatEditor, onCompleteStatement, loadSchema, error, result } = props

  return (
    <div className="proto-b">
      <div className="proto-b-tabs">
        <div className="proto-b-tab active proto-mono">Query 1</div>
        <div className="proto-b-tab">+</div>
      </div>
      <div className="proto-b-toolbar">
        <button className="run" onClick={run} disabled={running} title="Ctrl/Cmd+Enter">
          ▶ 실행
        </button>
        <button type="button" onClick={formatEditor} title="Ctrl/Cmd+Alt+L">
          포맷팅
        </button>
        <button type="button" onClick={onCompleteStatement} title="Ctrl/Cmd+Shift+Enter">
          문장 완성(;)
        </button>
        <button type="button" onClick={loadSchema}>
          스키마 새로고침
        </button>
        {catalogs.length > 0 && <CatalogPicker catalogs={catalogs} catalog={catalog} onCatalogChange={onCatalogChange} />}
      </div>
      <CodeMirror ref={editorRef} value={statement} height="260px" theme="dark" extensions={extensions} onChange={setStatement} />
      <div className="proto-b-results">
        {error && <p className="error" style={{ padding: '0.5rem' }}>{error}</p>}
        {result && <ResultTableB result={result} connectionId={connectionId ? Number(connectionId) : undefined} catalog={catalog} />}
      </div>
      <div className="proto-b-status proto-mono">
        {result?.truncated ? '결과가 최대 행 수로 잘렸습니다.' : result ? `${result.rows?.length ?? result.rowsAffected ?? 0}행` : '대기 중'}
      </div>
    </div>
  )
}

function ResultTableB({ result, connectionId, catalog }: { result: QueryResult; connectionId?: number; catalog: string }) {
  if (result.rowsAffected !== undefined && !result.columns) return <p style={{ padding: '0.6rem' }}>{result.rowsAffected}행이 영향을 받았습니다.</p>
  if (!result.columns || !result.rows) return <p style={{ padding: '0.6rem' }}>결과가 없습니다.</p>
  const { downloadEnabled, downloadCell } = useCellDownload(connectionId, catalog, result)

  return (
    <table>
      <thead>
        <tr>
          {result.columns.map((c) => (
            <th key={c} className="proto-mono">
              {c}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {result.rows.map((row, i) => (
          <tr key={i}>
            {row.map((cell, j) => (
              <td key={j} className="proto-mono">
                {cell === null ? 'NULL' : String(cell)}
                {downloadEnabled && (
                  <button type="button" className="cell-download" title="원본 값 다운로드" onClick={() => downloadCell(row, result.columns![j])}>
                    ⬇
                  </button>
                )}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// ---------- Variant C: 다크 + 사이드바 없음 + 플로팅 툴바 ----------
function VariantC(props: VariantProps) {
  const { connectionId, catalogs, catalog, onCatalogChange, editorRef, statement, setStatement, extensions, run, running, formatEditor, onCompleteStatement, loadSchema, error, result } = props

  return (
    <div className="proto-c">
      <div className="proto-c-breadcrumb proto-mono">
        Connection #{connectionId}
        {catalogs.length > 0 && (
          <>
            {' / '}
            <CatalogPicker catalogs={catalogs} catalog={catalog} onCatalogChange={onCatalogChange} />
          </>
        )}
      </div>
      <div className="proto-c-editor-wrap">
        <div className="proto-c-floating-toolbar">
          <button className="run" onClick={run} disabled={running} title="Ctrl/Cmd+Enter">
            ▶
          </button>
          <button type="button" onClick={formatEditor} title="Ctrl/Cmd+Alt+L">
            포맷
          </button>
          <button type="button" onClick={onCompleteStatement} title="Ctrl/Cmd+Shift+Enter">
            ;
          </button>
          <button type="button" onClick={loadSchema}>
            ↻
          </button>
        </div>
        <CodeMirror ref={editorRef} value={statement} height="240px" theme="dark" extensions={extensions} onChange={setStatement} />
      </div>
      <div className="proto-c-results">
        {error && <p className="error" style={{ padding: '0.5rem' }}>{error}</p>}
        {result && <ResultTableC result={result} connectionId={connectionId ? Number(connectionId) : undefined} catalog={catalog} />}
      </div>
      <div className="proto-c-status proto-mono">
        <span>{catalog || '-'}</span>
        <span>{result?.truncated ? '결과 잘림' : result ? `${result.rows?.length ?? result.rowsAffected ?? 0}행` : '대기 중'}</span>
      </div>
    </div>
  )
}

function ResultTableC({ result, connectionId, catalog }: { result: QueryResult; connectionId?: number; catalog: string }) {
  if (result.rowsAffected !== undefined && !result.columns) return <p style={{ padding: '0.6rem' }}>{result.rowsAffected}행이 영향을 받았습니다.</p>
  if (!result.columns || !result.rows) return <p style={{ padding: '0.6rem' }}>결과가 없습니다.</p>
  const { downloadEnabled, downloadCell } = useCellDownload(connectionId, catalog, result)

  return (
    <table>
      <thead>
        <tr>
          {result.columns.map((c) => (
            <th key={c} className="proto-mono">
              {c}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {result.rows.map((row, i) => (
          <tr key={i}>
            {row.map((cell, j) => (
              <td key={j} className="proto-mono">
                {cell === null ? 'NULL' : String(cell)}
                {downloadEnabled && (
                  <button type="button" className="cell-download" title="원본 값 다운로드" onClick={() => downloadCell(row, result.columns![j])}>
                    ⬇
                  </button>
                )}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}


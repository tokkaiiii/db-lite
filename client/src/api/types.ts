export type DBKind = 'mssql' | 'mysql' | 'postgres' | 'oracle'
export type PermissionLevel = 'none' | 'read' | 'write'

export interface User {
  id: number
  username: string
  isAdmin: boolean
  createdAt: string
}

export interface Connection {
  id: number
  name: string
  kind: DBKind
  host: string
  port: number
  username: string
  serviceName: string
  createdAt: string
}

export interface ConnectionWithLevel extends Connection {
  level: PermissionLevel
}

export interface Permission {
  userId: number
  connectionId: number
  level: PermissionLevel
}

export interface AuditLogEntry {
  id: number
  userId: number
  connectionId: number
  statement: string
  allowed: boolean
  errorMessage?: string
  executedAt: string
}

// ColumnOrigin says which table one result column's value came from and how
// to find that exact row for a cell-value download (ADR 0009 / ADR 0011).
// primaryKeyRowIndexes may point past the end of `columns` — a row's raw
// array can carry hidden PK-carrier cells a JOIN rewrite appended purely so
// a download can re-fetch the row; those are never rendered.
export interface ColumnOrigin {
  table: string
  primaryKeyColumns: string[]
  primaryKeyRowIndexes: number[]
}

export interface QueryResult {
  columns?: string[]
  rows?: unknown[][]
  truncated?: boolean
  rowsAffected?: number
  // One entry per `columns` entry; null where the origin couldn't be
  // pinned down (an aggregate/expression result, an unqualified column in
  // a JOIN, or a table without a primary key) — see ADR 0009 / ADR 0011.
  columnOrigins?: (ColumnOrigin | null)[]
}

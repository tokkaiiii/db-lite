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

export interface QueryResult {
  columns?: string[]
  rows?: unknown[][]
  truncated?: boolean
  rowsAffected?: number
  // Set only when the statement was a plain `SELECT * FROM <table>` on a
  // table with a primary key — see ADR 0009. Their presence is what
  // enables the per-cell "원본 다운로드" button.
  table?: string
  primaryKey?: string[]
}

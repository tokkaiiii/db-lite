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
  createdAt: string
}

export interface ConnectionWithLevel extends Connection {
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
}

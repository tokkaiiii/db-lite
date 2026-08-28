import type {
  AuditLogEntry,
  Connection,
  ConnectionWithLevel,
  DBKind,
  Permission,
  PermissionLevel,
  QueryResult,
  User,
} from './types'

const TOKEN_KEY = 'dbtool.token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken()
  const headers = new Headers(options.headers)
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(path, { ...options, headers })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new ApiError(res.status, body.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export { ApiError }

export function login(username: string, password: string) {
  return request<{ token: string }>('/api/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function listConnections() {
  return request<ConnectionWithLevel[]>('/api/connections')
}

export function executeQuery(connectionId: number, statement: string, catalog: string) {
  return request<QueryResult>(`/api/connections/${connectionId}/query`, {
    method: 'POST',
    body: JSON.stringify({ statement, catalog }),
  })
}

export function listCatalogs(connectionId: number) {
  return request<{ catalogs: string[] }>(`/api/connections/${connectionId}/catalogs`)
}

export function describeSchema(connectionId: number, catalog: string) {
  const qs = catalog ? `?catalog=${encodeURIComponent(catalog)}` : ''
  return request<{ schema: Record<string, string[]> }>(`/api/connections/${connectionId}/schema${qs}`)
}

export function adminListUsers() {
  return request<User[]>('/api/admin/users')
}

export function adminCreateUser(username: string, password: string, isAdmin: boolean) {
  return request<User>('/api/admin/users', {
    method: 'POST',
    body: JSON.stringify({ username, password, isAdmin }),
  })
}

export interface ConnectionInput {
  name: string
  kind: DBKind
  host: string
  port: number
  username: string
  password: string
  serviceName?: string
}

export function adminCreateConnection(input: ConnectionInput) {
  return request<Connection>('/api/admin/connections', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

// input.password: leave "" to keep the Connection's existing password —
// it's never sent back by the server (Connection.Password is json:"-"), so
// the edit form can't prefill it for the user to leave unchanged verbatim.
export function adminUpdateConnection(id: number, input: ConnectionInput) {
  return request<Connection>(`/api/admin/connections/${id}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

export function adminListConnections() {
  return request<Connection[]>('/api/admin/connections')
}

export function adminDeleteConnection(id: number) {
  return request<void>(`/api/admin/connections/${id}`, { method: 'DELETE' })
}

export function adminSetPermission(userId: number, connectionId: number, level: PermissionLevel) {
  return request<void>('/api/admin/permissions', {
    method: 'PUT',
    body: JSON.stringify({ userId, connectionId, level }),
  })
}

export function adminListAllPermissions() {
  return request<Permission[]>('/api/admin/permissions')
}

export function adminListAuditLog() {
  return request<AuditLogEntry[]>('/api/admin/audit-log')
}

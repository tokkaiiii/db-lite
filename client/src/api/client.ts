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

// Called when a request that carried a token still comes back 401 — that
// means the Session expired (or was otherwise invalidated) server-side, as
// opposed to a 401 from /api/login itself (wrong credentials, no token
// sent). AuthProvider registers this once to clear its state and bounce
// the user to the login screen instead of leaving every page to show a
// generic "요청 실패" error for what's really an expired session.
let unauthorizedHandler: (() => void) | null = null

export function onUnauthorized(handler: () => void) {
  unauthorizedHandler = handler
}

async function throwApiError(res: Response, token: string | null): Promise<never> {
  const body = await res.json().catch(() => ({ error: res.statusText }))
  if (res.status === 401 && token) {
    clearToken()
    unauthorizedHandler?.()
  }
  throw new ApiError(res.status, body.error ?? res.statusText)
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken()
  const headers = new Headers(options.headers)
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(path, { ...options, headers })
  if (!res.ok) await throwApiError(res, token)
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

// downloadCellFile re-fetches one cell's untruncated original value (ADR
// 0009) and saves it as a file. A plain <a href> download can't carry the
// Authorization header this app authenticates with, so it fetches the
// file as a Blob and triggers the save via a synthetic, hidden <a
// download> click instead of exposing the JWT in a URL.
//
// expectedValue is the text the grid cell already shows (NULL -> "NULL",
// else String(cell)) — the server cross-checks it against the freshly
// re-fetched value (ADR 0011) so a JOIN download whose column-origin
// tracking was wrong fails loudly instead of silently returning the wrong
// file.
export async function downloadCellFile(
  connectionId: number,
  params: { catalog: string; table: string; column: string; primaryKey: Record<string, unknown>; expectedValue: string },
) {
  const token = getToken()
  const headers = new Headers()
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`/api/connections/${connectionId}/cell`, {
    method: 'POST',
    headers,
    body: JSON.stringify(params),
  })
  if (!res.ok) await throwApiError(res, token)

  const blob = await res.blob()
  const filename = filenameFromContentDisposition(res.headers.get('Content-Disposition')) ?? 'download'

  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

function filenameFromContentDisposition(header: string | null): string | null {
  if (!header) return null
  // Prefer the RFC 5987 filename* form — the plain filename= fallback
  // has non-ASCII characters replaced with "_" (see contentDisposition
  // in query_handlers.go), so it's the star form that has the real name.
  const star = /filename\*=UTF-8''([^;]+)/i.exec(header)
  if (star) return decodeURIComponent(star[1])
  const plain = /filename="?([^";]+)"?/i.exec(header)
  return plain ? plain[1] : null
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

export function adminResetUserPassword(id: number, password: string) {
  return request<User>(`/api/admin/users/${id}/password`, {
    method: 'PUT',
    body: JSON.stringify({ password }),
  })
}

export function adminDeleteUser(id: number) {
  return request<void>(`/api/admin/users/${id}`, { method: 'DELETE' })
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

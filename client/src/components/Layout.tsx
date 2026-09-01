import { Link, Outlet, useMatch } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export function Layout() {
  const { claims, logout } = useAuth()
  // QueryPage는 스키마 사이드바를 두고 화면 폭을 그대로 쓴다 — 다른 화면은
  // 기존 max-width 중앙 정렬 레이아웃을 유지한다.
  const isQueryPage = useMatch('/connections/:connectionId/query')

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <header className="topbar">
        <nav style={{ alignItems: 'center' }}>
          <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-bright)', fontWeight: 600, marginRight: '0.5rem' }}>
            DB Lite
          </span>
          <Link to="/">Connections</Link>
          {claims?.isAdmin && (
            <>
              <Link to="/admin/users">Users</Link>
              <Link to="/admin/connections">Manage Connections</Link>
              <Link to="/admin/permissions">Permissions</Link>
              <Link to="/admin/audit-log">Audit Log</Link>
            </>
          )}
        </nav>
        {claims && (
          <div className="user-info">
            <span>{claims.username}{claims.isAdmin ? ' (admin)' : ''}</span>
            <button onClick={logout}>로그아웃</button>
          </div>
        )}
      </header>
      <main className={isQueryPage ? 'full-bleed' : undefined}>
        <Outlet />
      </main>
    </div>
  )
}

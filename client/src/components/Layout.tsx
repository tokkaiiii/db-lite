import { Link, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export function Layout() {
  const { claims, logout } = useAuth()

  return (
    <div>
      <header className="topbar">
        <nav>
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
      <main>
        <Outlet />
      </main>
    </div>
  )
}

import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export function ProtectedRoute() {
  const { claims } = useAuth()
  if (!claims) return <Navigate to="/login" replace />
  return <Outlet />
}

export function AdminRoute() {
  const { claims } = useAuth()
  if (!claims) return <Navigate to="/login" replace />
  if (!claims.isAdmin) return <Navigate to="/" replace />
  return <Outlet />
}

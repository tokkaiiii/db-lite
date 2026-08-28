import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { AdminRoute, ProtectedRoute } from './components/ProtectedRoute'
import { Layout } from './components/Layout'
import { LoginPage } from './pages/LoginPage'
import { ConnectionsPage } from './pages/ConnectionsPage'
import { QueryPage } from './pages/QueryPage'
import { UsersPage } from './pages/admin/UsersPage'
import { ConnectionsAdminPage } from './pages/admin/ConnectionsAdminPage'
import { PermissionsPage } from './pages/admin/PermissionsPage'
import { AuditLogPage } from './pages/admin/AuditLogPage'

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />

          <Route element={<ProtectedRoute />}>
            <Route element={<Layout />}>
              <Route path="/" element={<ConnectionsPage />} />
              <Route path="/connections/:connectionId/query" element={<QueryPage />} />

              <Route element={<AdminRoute />}>
                <Route path="/admin/users" element={<UsersPage />} />
                <Route path="/admin/connections" element={<ConnectionsAdminPage />} />
                <Route path="/admin/permissions" element={<PermissionsPage />} />
                <Route path="/admin/audit-log" element={<AuditLogPage />} />
              </Route>
            </Route>
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import * as api from '../api/client'

interface Claims {
  userId: number
  username: string
  isAdmin: boolean
}

interface AuthState {
  claims: Claims | null
  sessionMessage: string | null
  clearSessionMessage: () => void
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

function decodeClaims(token: string): Claims | null {
  try {
    const payload = token.split('.')[1]
    const json = atob(payload.replace(/-/g, '+').replace(/_/g, '/'))
    const parsed = JSON.parse(json)
    return { userId: parsed.userId, username: parsed.username, isAdmin: parsed.isAdmin }
  } catch {
    return null
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [claims, setClaims] = useState<Claims | null>(() => {
    const token = api.getToken()
    return token ? decodeClaims(token) : null
  })
  const [sessionMessage, setSessionMessage] = useState<string | null>(null)

  useEffect(() => {
    api.onUnauthorized(() => {
      setClaims(null)
      setSessionMessage('세션이 만료되었습니다. 다시 로그인해주세요.')
    })
  }, [])

  const value = useMemo<AuthState>(
    () => ({
      claims,
      sessionMessage,
      clearSessionMessage: () => setSessionMessage(null),
      login: async (username, password) => {
        const { token } = await api.login(username, password)
        api.setToken(token)
        setClaims(decodeClaims(token))
      },
      logout: () => {
        api.clearToken()
        setClaims(null)
      },
    }),
    [claims, sessionMessage],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

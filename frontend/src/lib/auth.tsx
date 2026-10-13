import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { ApiError, request, requestDownload, saveDownload, sessionSchema, type DownloadFormat, type RequestOptions, type Session, type User } from './api'

type AuthStatus = 'loading' | 'authenticated' | 'anonymous' | 'error'
type AuthContextValue = {
  user: User | null
  status: AuthStatus
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refresh: () => Promise<void>
  updateUser: (user: User) => void
  api: <T>(path: string, schema: z.ZodType<T>, options?: RequestOptions) => Promise<T>
  download: (path: string, format: DownloadFormat, signal?: AbortSignal) => Promise<void>
}
const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [session, setSession] = useState<Session | null>(null)
  const [status, setStatus] = useState<AuthStatus>('loading')
  const generation = useRef(0)
  const currentToken = useRef<string | null>(null)
  const currentUser = useRef<User | null>(null)
  const invalidate = useCallback(() => {
    generation.current++
    queryClient.clear()
    currentToken.current = null
    currentUser.current = null
    setSession(null)
    setStatus('anonymous')
  }, [queryClient])
  const refresh = useCallback(async () => {
    const current = ++generation.current
    setStatus(previous => previous === 'error' ? 'loading' : previous)
    try {
      const next = await request('/auth/me', sessionSchema)
      if (generation.current !== current) return
      if (!next.user.is_active) { invalidate(); return }
      if (currentUser.current && (currentUser.current.id !== next.user.id || currentUser.current.role !== next.user.role)) queryClient.clear()
      currentToken.current = next.csrf_token
      currentUser.current = next.user
      setSession(next)
      setStatus('authenticated')
    } catch (error) {
      if (generation.current !== current) return
      if (error instanceof ApiError && error.status === 401) invalidate()
      else {
        // A failed bootstrap must never show cached identity or private data.
        queryClient.clear()
        currentToken.current = null
        currentUser.current = null
        setSession(null)
        setStatus('error')
      }
    }
  }, [invalidate, queryClient])
  useEffect(() => {
    void refresh()
    return () => { generation.current++ }
  }, [refresh])
  useEffect(() => {
    if (status !== 'authenticated') return
    const onFocus = () => { void refresh() }
    window.addEventListener('focus', onFocus)
    const interval = window.setInterval(onFocus, 60_000)
    return () => { window.removeEventListener('focus', onFocus); window.clearInterval(interval) }
  }, [refresh, status])
  const login = useCallback(async (email: string, password: string) => {
    const next = await request('/auth/login', sessionSchema, { method: 'POST', body: { email, password } })
    generation.current++
    queryClient.clear()
    currentToken.current = next.csrf_token
    currentUser.current = next.user
    setSession(next)
    setStatus('authenticated')
  }, [queryClient])
  const api = useCallback(async <T,>(path: string, schema: z.ZodType<T>, options: RequestOptions = {}) => {
    try {
      return await request(path, schema, { ...options, csrf: session?.csrf_token })
    } catch (error) {
      if (error instanceof ApiError && (error.status === 401 || error.code === 'csrf_failed') && currentToken.current === session?.csrf_token) invalidate()
      throw error
    }
  }, [session?.csrf_token, invalidate])
  const logout = useCallback(async () => {
    await api('/auth/logout', z.undefined(), { method: 'POST' })
    invalidate()
  }, [api, invalidate])
  const download = useCallback(async (path: string, format: DownloadFormat, signal?: AbortSignal) => {
    const identity = currentUser.current
    if (!identity?.is_active || (identity.role !== 'ADMIN' && identity.role !== 'IT_ENGINEER')) throw new ApiError(403, 'forbidden')
    const started = generation.current, token = currentToken.current
    const unchanged = () => started === generation.current && token === currentToken.current && identity.id === currentUser.current?.id && identity.role === currentUser.current?.role
    try {
      const blob = await requestDownload(path, format, signal)
      if (signal?.aborted || !unchanged()) throw new ApiError(0, 'request_cancelled')
      saveDownload(blob, format)
    } catch (error) {
      if (signal?.aborted || !unchanged()) throw new ApiError(0, 'request_cancelled')
      if (error instanceof ApiError && error.status === 401) invalidate()
      throw error
    }
  }, [invalidate])
  const updateUser = useCallback((user: User) => {
    if (currentUser.current?.id !== user.id) return
    currentUser.current = { ...currentUser.current, display_name: user.display_name }
    setSession(previous => previous?.user.id === user.id ? { ...previous, user: { ...previous.user, display_name: user.display_name } } : previous)
  }, [])
  return <AuthContext.Provider value={{ user: session?.user ?? null, status, login, logout, refresh, updateUser, api, download }}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const auth = useContext(AuthContext)
  if (!auth) throw new Error('AuthProvider is required')
  return auth
}

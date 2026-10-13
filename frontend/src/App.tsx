import { lazy, Suspense, type ReactNode } from 'react'
import { Route, Routes, Link, Navigate, Outlet } from 'react-router-dom'
import { Shell } from './components/Shell'
import { Login } from './pages/Login'
import { Button } from './components/ui/button'
import { useLocale } from './lib/locale'
import { AuthProvider, useAuth } from './lib/auth'
import { ToastProvider } from './lib/notifications'
import { hasStaffRole } from './lib/knowledge'

const Overview = lazy(() => import('./pages/Overview').then(module => ({ default: module.Overview })))

const Profile = lazy(() => import('./pages/Profile').then(module => ({ default: module.Profile })))

const UsersPage = lazy(() => import('./pages/Users').then(module => ({ default: module.UsersPage })))

const DepartmentsPage = lazy(() => import('./pages/Departments').then(module => ({ default: module.DepartmentsPage })))
const Knowledge = lazy(() => import('./pages/Knowledge').then(module => ({ default: module.Knowledge })))
const Article = lazy(() => import('./pages/Article').then(module => ({ default: module.Article })))
const ArticleEditor = lazy(() => import('./pages/ArticleEditor').then(module => ({ default: module.ArticleEditor })))
const Tickets = lazy(() => import('./pages/Tickets').then(module => ({ default: module.Tickets })))
const TicketCreate = lazy(() => import('./pages/TicketCreate').then(module => ({ default: module.TicketCreate })))
const TicketDetail = lazy(() => import('./pages/TicketDetail').then(module => ({ default: module.TicketDetail })))
const Categories = lazy(() => import('./pages/Categories').then(module => ({ default: module.Categories })))
const Analytics = lazy(() => import('./pages/Analytics').then(module => ({ default: module.Analytics })))
const Audit = lazy(() => import('./pages/Audit').then(module => ({ default: module.Audit })))
const Settings = lazy(() => import('./pages/Settings').then(module => ({ default: module.Settings })))

function NotFound() {
  const { t } = useLocale()
  return <div className="not-found"><span>404</span><h1>{t.notFoundTitle}</h1><p>{t.notFoundDescription}</p><Button asChild><Link to="/">{t.backOverview}</Link></Button></div>
}

function RequireSession() {
  const auth = useAuth()
  const { t } = useLocale()
  if (auth.status === 'loading') return <div className="session-state" role="status" aria-busy="true">{t.sessionLoading}</div>
  if (auth.status === 'error') return <div className="session-state"><p role="alert">{t.sessionUnavailable}</p><Button onClick={() => void auth.refresh()}>{t.retry}</Button></div>
  if (!auth.user) return <Navigate to="/login" replace />
  return <Outlet key={`${auth.user.id}:${auth.user.role}`} />
}

function RequireAdmin() {
  const { user } = useAuth()
  const { t } = useLocale()
  if (user?.role !== 'ADMIN') return <div className="not-found"><span>403</span><h1>{t.forbiddenTitle}</h1><p>{t.forbiddenDescription}</p><Button asChild><Link to="/">{t.backOverview}</Link></Button></div>
  return <Outlet />
}

function Page({ children }: { children: ReactNode }) {
  const { t } = useLocale()
  return <Suspense fallback={<div className="session-state" role="status">{t.loading}</div>}>{children}</Suspense>
}

function RequireStaff() {
  const { user } = useAuth()
  const { t } = useLocale()
  if (!hasStaffRole(user?.role)) return <div className="not-found"><span>403</span><h1>{t.forbiddenTitle}</h1><p>{t.staffForbidden}</p><Button asChild><Link to="/knowledge">{t.backKnowledge}</Link></Button></div>
  return <Outlet />
}

function ProfileRoute() {
  const { user } = useAuth()
  return <Page><Profile key={user?.id} /></Page>
}

export function App() {
  return <AuthProvider><ToastProvider><Routes><Route element={<Shell />}><Route path="/login" element={<Login />} /><Route element={<RequireSession />}><Route path="/" element={<Page><Overview /></Page>} /><Route path="/profile" element={<ProfileRoute />} /><Route path="/knowledge" element={<Page><Knowledge /></Page>} /><Route path="/tickets" element={<Page><Tickets /></Page>} /><Route path="/tickets/new" element={<Page><TicketCreate /></Page>} /><Route path="/tickets/:id" element={<Page><TicketDetail /></Page>} /><Route path="/search" element={<Page><Knowledge /></Page>} /><Route path="/knowledge/articles/:id" element={<Page><Article /></Page>} /><Route element={<RequireStaff />}><Route path="/analytics" element={<Page><Analytics /></Page>} /><Route path="/engineer/tickets" element={<Page><Tickets /></Page>} /><Route path="/engineer/knowledge" element={<Page><Knowledge manage /></Page>} /><Route path="/engineer/knowledge/new" element={<Page><ArticleEditor /></Page>} /><Route path="/engineer/knowledge/:id/edit" element={<Page><ArticleEditor /></Page>} /></Route><Route element={<RequireAdmin />}><Route path="/admin/audit" element={<Page><Audit /></Page>} /><Route path="/admin/settings" element={<Page><Settings /></Page>} /><Route path="/admin/users" element={<Page><UsersPage /></Page>} /><Route path="/admin/departments" element={<Page><DepartmentsPage /></Page>} /><Route path="/admin/categories" element={<Page><Categories /></Page>} /></Route><Route path="*" element={<NotFound />} /></Route></Route></Routes></ToastProvider></AuthProvider>
}

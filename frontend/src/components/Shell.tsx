import { useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'
import * as Dialog from '@radix-ui/react-dialog'
import { Activity, BookOpen, Building2, ChevronRight, CircleHelp, LayoutDashboard, LockKeyhole, LogIn, LogOut, Menu, Plus, Settings2, ScrollText, ShieldCheck, Ticket, UserRound, Users, X } from 'lucide-react'
import { useLocale } from '../lib/locale'
import { useAuth } from '../lib/auth'
import { useErrorMessage, useToast } from '../lib/notifications'
import { cn } from '../lib/utils'
import { Button } from './ui/button'
import { hasStaffRole } from '../lib/knowledge'
import { useSchoolAccess } from '../lib/school'
import { useSettings } from '../lib/settings'

function Brand() {
  const { t } = useLocale()
  const settings = useSettings()
  const inSchool = useLocation().pathname.startsWith('/school')
  const { locale } = useLocale()
  return <Link className="brand" to={inSchool ? '/school' : '/'} aria-label="MedDesk"><span className="brand-mark"><Plus size={26} strokeWidth={3} aria-hidden="true" /></span><span><span className="brand-name">MedDesk<span className="brand-dot">.</span></span><span className="brand-subtitle" title={inSchool ? undefined : settings.data?.settings.clinic_name}>{inSchool ? (locale === 'kk' ? 'Мектеп · IT қызметі' : 'Школа · IT-служба') : settings.data?.settings.clinic_name ?? t.clinic}</span></span></Link>
}

function SidebarContent({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useLocale()
  const { user } = useAuth()
  const school = useSchoolAccess()
  const inSchool = useLocation().pathname.startsWith('/school') || user?.account_scope === 'SCHOOL'
  const { locale } = useLocale()
  const linkClass = ({ isActive }: { isActive: boolean }) => cn('nav-link', isActive && 'active')
  return <><Brand /><div className="workspace-label">{t.workspace}</div><nav aria-label={t.mainNavigation} className="sidebar-nav">
 {school.data?.allowed && <NavLink to="/school" onClick={onNavigate} className={linkClass}><Building2 size={19} aria-hidden="true" />{user?.account_scope === 'SCHOOL' ? (locale === 'kk' ? 'Мектеп өтінімдері' : 'Заявки школы') : (locale === 'kk' ? 'Мектеп · жеке кеңістік' : 'Школа · личное пространство')}</NavLink>}
    {inSchool && user?.account_scope !== 'SCHOOL' && <NavLink to="/" onClick={onNavigate} className={linkClass}><Building2 size={19} aria-hidden="true" />{locale === 'kk' ? 'Емхана кеңістігіне оралу' : 'Вернуться в поликлинику'}</NavLink>}
    {!inSchool && (user ? <><NavLink to="/" end onClick={onNavigate} className={linkClass}><LayoutDashboard size={19} aria-hidden="true" />{t.overview}<ChevronRight className="nav-chevron" size={15} aria-hidden="true" /></NavLink><NavLink to="/tickets" onClick={onNavigate} className={linkClass}><Ticket size={19} aria-hidden="true" />{t.tickets}</NavLink><NavLink to="/knowledge" onClick={onNavigate} className={linkClass}><BookOpen size={19} aria-hidden="true" />{t.knowledge}</NavLink>{hasStaffRole(user.role) && <NavLink to="/engineer/knowledge" onClick={onNavigate} className={linkClass}><BookOpen size={19} aria-hidden="true" />{t.knowledgeManage}</NavLink>}<NavLink to="/profile" onClick={onNavigate} className={linkClass}><UserRound size={19} aria-hidden="true" />{t.profile}</NavLink>{user.role === 'ADMIN' && <><NavLink to="/admin/users" onClick={onNavigate} className={linkClass}><Users size={19} aria-hidden="true" />{t.users}</NavLink><NavLink to="/admin/departments" onClick={onNavigate} className={linkClass}><Building2 size={19} aria-hidden="true" />{t.departments}</NavLink><NavLink to="/admin/categories" onClick={onNavigate} className={linkClass}><BookOpen size={19} aria-hidden="true" />{t.categories}</NavLink></>}</> : <NavLink to="/login" onClick={onNavigate} className={linkClass}><LogIn size={19} aria-hidden="true" />{t.login}</NavLink>)}
    {!inSchool && user && hasStaffRole(user.role) && <NavLink to="/analytics" onClick={onNavigate} className={linkClass}><Activity size={19} aria-hidden="true" />{t.analytics}</NavLink>}
    {!inSchool && user?.role === 'ADMIN' && <><NavLink to="/admin/audit" onClick={onNavigate} className={linkClass}><ScrollText size={19} aria-hidden="true" />{t.audit}</NavLink><NavLink to="/admin/settings" onClick={onNavigate} className={linkClass}><Settings2 size={19} aria-hidden="true" />{t.settings}</NavLink></>}
  </nav>
    <div className="sidebar-bottom"><div className="sidebar-note"><CircleHelp size={20} aria-hidden="true" /><p>{t.privacyTitle}</p><span>{t.buildFooter}</span></div><div className="signed-out"><span className="signed-out-icon">{user ? <UserRound size={15} aria-hidden="true" /> : <LockKeyhole size={15} aria-hidden="true" />}</span><span>{user?.display_name ?? t.notSignedIn}</span></div></div>
  </>
}

export function Shell() {
  const { t, locale, setLocale } = useLocale()
  const auth = useAuth()
  const notify = useToast()
  const errorMessage = useErrorMessage()
  const [menuOpen, setMenuOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const { pathname } = useLocation()
  const names: Record<string, string> = { '/school': locale === 'kk' ? 'Мектеп' : 'Школа', '/login': t.login, '/profile': t.profile, '/admin/users': t.users, '/admin/departments': t.departments, '/admin/categories': t.categories, '/admin/audit': t.audit, '/admin/settings': t.settings, '/analytics': t.analytics, '/knowledge': t.knowledge, '/search': t.searchResults, '/engineer/knowledge': t.knowledgeManage, '/tickets': t.tickets, '/tickets/new': t.newTicket, '/engineer/tickets': t.tickets }
  const pageName = names[pathname] ?? (pathname.startsWith('/tickets/') ? t.tickets : pathname.startsWith('/engineer/knowledge') ? t.knowledgeManage : pathname.startsWith('/knowledge/') ? t.knowledge : t.overview)
  const logout = async () => {
    setLoggingOut(true)
    try { await auth.logout() }
    catch (error) { notify({ kind: 'error', message: errorMessage(error) }) }
    finally { setLoggingOut(false) }
  }
  return <div className="app-shell"><a href="#main-content" className="skip-link">{t.skip}</a><aside className="desktop-sidebar"><SidebarContent /></aside><div className="main-shell"><header className="topbar"><div className="topbar-start"><Dialog.Root open={menuOpen} onOpenChange={setMenuOpen}><Dialog.Trigger asChild><Button className="mobile-menu" size="icon" variant="ghost" aria-label={t.openMenu}><Menu aria-hidden="true" /></Button></Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="drawer-overlay" /><Dialog.Content className="drawer-content" aria-describedby={undefined}><Dialog.Title className="sr-only">{t.mainNavigation}</Dialog.Title><Dialog.Close asChild><Button className="drawer-close" variant="ghost" size="icon" aria-label={t.closeMenu}><X aria-hidden="true" /></Button></Dialog.Close><SidebarContent onNavigate={() => setMenuOpen(false)} /></Dialog.Content></Dialog.Portal></Dialog.Root><div className="breadcrumb"><span>{t.workspace}</span><ChevronRight size={14} aria-hidden="true" /><strong>{pageName}</strong></div></div><div className="topbar-end"><span className="internal-label"><ShieldCheck size={15} aria-hidden="true" />{t.internalPortal}</span><div className="language-toggle" role="group" aria-label={t.chooseLanguage}><button type="button" aria-label="Қазақша" aria-pressed={locale === 'kk'} onClick={() => setLocale('kk')} className={cn(locale === 'kk' && 'selected')}>ҚАЗ</button><button type="button" aria-label="Русский" aria-pressed={locale === 'ru'} onClick={() => setLocale('ru')} className={cn(locale === 'ru' && 'selected')}>РУС</button></div>{auth.user && <div className="topbar-account"><Link to={auth.user.account_scope === 'SCHOOL' ? '/school' : '/profile'} aria-label={t.profile}><UserRound size={18} aria-hidden="true" /><span>{auth.user.display_name}</span></Link><Button size="sm" variant="ghost" onClick={() => void logout()} disabled={loggingOut} aria-label={t.logout}><LogOut size={17} aria-hidden="true" /><span>{t.logout}</span></Button></div>}</div></header><main id="main-content" className="page-content" tabIndex={-1}><Outlet /></main><footer className="page-footer"><span>{pathname === '/school' ? (locale === 'kk' ? 'MedDesk · Мектептің IT қолдауы' : 'MedDesk · IT-поддержка школы') : t.footer}</span><span>{pathname === '/school' ? (locale === 'kk' ? 'Қолжетімділік қорғалған' : 'Доступ защищён') : t.buildFooter}</span></footer></div></div>
}

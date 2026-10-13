import { useQuery } from '@tanstack/react-query'
import { ArrowRight, Check, CheckCircle2, CircleDashed, Database, Layers3, RefreshCw, Server, ShieldCheck } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { fetchOperationalStatus } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { cn } from '../lib/utils'
import { hasStaffRole } from '../lib/knowledge'
import { analyticsSchema } from '../lib/analytics'
import { ticketsPageSchema } from '../lib/tickets'
import { AnalyticsSummary } from '../components/AnalyticsSummary'
import { useErrorMessage } from '../lib/notifications'

function OperationalCard({ endpoint }: { endpoint: 'health' | 'ready' }) {
  const { t } = useLocale()
  const query = useQuery({
    queryKey: ['operational', endpoint],
    queryFn: ({ signal }) => fetchOperationalStatus(endpoint, signal),
    refetchInterval: 30_000,
  })
  const isAPI = endpoint === 'health'
  const Icon = isAPI ? Server : Database
  const loading = query.isPending
  const error = query.isError
  const state = loading ? 'checking' : error ? 'unavailable' : 'available'
  const status = loading ? t.checking : error ? t.unavailable : isAPI ? t.online : t.ready
  const note = loading ? t.checkNote : error ? t.unavailableNote : isAPI ? t.apiReadyNote : t.databaseReadyNote
  return <article className={cn('operational-card', state)} aria-labelledby={`${endpoint}-title`}>
    <div className="operational-card-top"><span className="service-icon"><Icon size={22} strokeWidth={1.7} aria-hidden="true" /></span><Badge className={cn('service-badge', state)}><span className={cn('status-dot', loading && 'animate-pulse')} />{status}</Badge></div>
    <h3 id={`${endpoint}-title`}>{isAPI ? t.api : t.database}</h3>
    <p className="service-description">{isAPI ? t.apiDescription : t.databaseDescription}</p>
    <div className="service-result" aria-live="polite" role="status">{loading ? <CircleDashed size={16} className="animate-spin" aria-hidden="true" /> : error ? <CircleDashed size={16} aria-hidden="true" /> : <CheckCircle2 size={16} aria-hidden="true" />}<span>{note}</span></div>
  </article>
}

export function Overview() {
  const { t } = useLocale()
  const { user, api } = useAuth()
  const message = useErrorMessage()
  const staff = hasStaffRole(user?.role)
  const health = useQuery({ queryKey: ['operational', 'health'], queryFn: ({ signal }) => fetchOperationalStatus('health', signal), refetchInterval: 30_000 })
  const ready = useQuery({ queryKey: ['operational', 'ready'], queryFn: ({ signal }) => fetchOperationalStatus('ready', signal), refetchInterval: 30_000 })
  const report = useQuery({ queryKey: ['analytics', 'overview', user?.id, user?.role], enabled: staff, queryFn: ({ signal }) => api('/analytics', analyticsSchema, { signal }) })
  const personal = useQuery({ queryKey: ['tickets', 'overview', user?.id, user?.role], enabled: !staff, queryFn: async ({ signal }) => {
    const statuses = ['NEW', 'IN_PROGRESS', 'WAITING_FOR_USER', 'RESOLVED', 'CLOSED'] as const
    const pages = await Promise.all(statuses.map(status => api(`/tickets?status=${status}&limit=1&offset=0`, ticketsPageSchema, { signal })))
    return { total: pages.reduce((sum, page) => sum + page.total, 0), open: pages.slice(0, 3).reduce((sum, page) => sum + page.total, 0), resolved: pages[3].total + pages[4].total, closed: pages[4].total }
  } })
  const fetching = health.isFetching || ready.isFetching
  return <>
    <div className="page-heading"><div><div className="eyebrow"><span />{t.foundation}</div><h1>{t.welcome}, {user?.display_name}.</h1><p>{t.homeDescription}</p></div><Badge className="development-badge"><CircleDashed size={13} aria-hidden="true" />{t.development}</Badge></div>
    <section className="welcome-panel" aria-labelledby="foundation-title">
      <div className="welcome-main"><span className="welcome-icon"><Layers3 size={23} aria-hidden="true" /></span><h2 id="foundation-title">{t.foundationNotice}</h2><p>{t.foundationDescription}</p><Button asChild className="welcome-action"><Link to="/knowledge">{t.knowledge}<ArrowRight size={16} aria-hidden="true" /></Link></Button><Button asChild variant="outline" className="welcome-action"><Link to="/tickets/new">{t.newTicket}<ArrowRight size={16} aria-hidden="true" /></Link></Button></div>
      <div className="phase-card"><div className="phase-card-top"><span>{t.currentPhase}</span><span className="phase-check"><Check size={16} aria-hidden="true" /></span></div><div className="phase-number">{t.phaseOne}<span>/ 05</span></div><h3>{t.phaseOneTitle}</h3><p>{t.phaseOneDescription}</p><div className="phase-progress" aria-hidden="true"><span className="complete" /><span className="complete" /><span className="complete" /><span className="complete" /><span className="complete" /></div></div>
      <div className="hero-orbit orbit-one" aria-hidden="true" /><div className="hero-orbit orbit-two" aria-hidden="true" />
    </section>
    <section className="overview-metrics" aria-labelledby="overview-metrics-title"><div className="section-heading"><div><h2 id="overview-metrics-title">{staff ? t.supportOverview : t.personalOverview}</h2><p>{staff ? t.staffOverviewHint : t.personalOverviewHint}</p></div></div>{staff ? report.isPending ? <p className="table-state" role="status">{t.loading}</p> : report.isError ? <div className="content-card table-state" role="alert"><p>{message(report.error)}</p><Button variant="outline" onClick={() => void report.refetch()}>{t.retry}</Button></div> : report.data && <><p className="reporting-period">{report.data.period.from} — {report.data.period.to} · {report.data.period.timezone}</p><AnalyticsSummary summary={report.data.summary} /></> : personal.isPending ? <p className="table-state" role="status">{t.loading}</p> : personal.isError ? <div className="content-card table-state" role="alert"><p>{message(personal.error)}</p><Button variant="outline" onClick={() => void personal.refetch()}>{t.retry}</Button></div> : personal.data && <dl className="analytics-summary personal-summary">{[[t.totalTickets, personal.data.total], [t.openTickets, personal.data.open], [t.resolvedTickets, personal.data.resolved], [t.closedTickets, personal.data.closed]].map(([label, value]) => <div className="metric-card" key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>}</section>
    <section className="admin-workspace content-card"><div className="section-heading"><div><h2>{t.quickActions}</h2></div></div><div className="admin-actions"><Button asChild><Link to={staff ? '/engineer/tickets' : '/tickets'}>{staff ? t.engineerQueue : t.myTickets}<ArrowRight size={16} aria-hidden="true" /></Link></Button>{staff && <><Button asChild variant="outline"><Link to="/analytics">{t.analytics}<ArrowRight size={16} aria-hidden="true" /></Link></Button><Button asChild variant="outline"><Link to="/engineer/knowledge">{t.knowledgeManage}</Link></Button></>}{user?.role === 'ADMIN' && <><Button asChild variant="outline"><Link to="/admin/users">{t.users}</Link></Button><Button asChild variant="outline"><Link to="/admin/departments">{t.departments}</Link></Button><Button asChild variant="outline"><Link to="/admin/categories">{t.categories}</Link></Button><Button asChild variant="outline"><Link to="/admin/audit">{t.audit}</Link></Button><Button asChild variant="outline"><Link to="/admin/settings">{t.settings}</Link></Button></>}</div></section>
    <section className="system-section" aria-labelledby="system-title">
      <div className="section-heading"><div><h2 id="system-title">{t.connectedTitle}<span className="live-label"><span />{t.live}</span></h2><p>{t.connectedDescription}</p></div><Button variant="outline" size="sm" disabled={fetching} onClick={() => { void health.refetch(); void ready.refetch() }}><RefreshCw className={cn(fetching && 'animate-spin')} aria-hidden="true" />{fetching ? t.refreshing : t.refresh}</Button></div>
      <div className="operational-grid"><OperationalCard endpoint="health" /><OperationalCard endpoint="ready" /></div>
      <p className="auto-refresh"><RefreshCw size={12} aria-hidden="true" />{t.autoRefresh}</p>
    </section>
    <aside className="privacy-note"><span className="privacy-icon"><ShieldCheck size={20} aria-hidden="true" /></span><div><h2>{t.privacyTitle}</h2><p>{t.privacyDescription}</p></div></aside>
  </>
}

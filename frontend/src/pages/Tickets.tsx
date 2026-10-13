import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import { Download, Plus, Search, Ticket as TicketIcon } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { Pagination } from '../components/forms/Pagination'
import { ReportingFilters } from '../components/ReportingFilters'
import { KnowledgeState } from '../components/KnowledgeState'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage } from '../lib/notifications'
import { ApiError, queryString, type DownloadFormat } from '../lib/api'
import { validReportingWindow, type ReportingWindow } from '../lib/analytics'
import { hasStaffRole } from '../lib/knowledge'
import { useSettings } from '../lib/settings'
import { useCategories } from '../lib/useCategories'
import { assigneesSchema, ticketDepartmentsSchema, ticketPriorities, ticketPriorityKey, ticketStatuses, ticketStatusKey, ticketsPageSchema, type TicketStatus, type TicketPriority } from '../lib/tickets'

export function Tickets() {
  const { t, locale } = useLocale()
  const { api, download, user } = useAuth()
  const message = useErrorMessage()
  const staff = hasStaffRole(user?.role)
  const [params, setParams] = useSearchParams()
  const [search, setSearch] = useState(params.get('q') ?? '')
  const query = (params.get('q') ?? '').slice(0, 100)
  const status = ticketStatuses.includes(params.get('status') as TicketStatus) ? params.get('status')! : ''
  const priority = ticketPriorities.includes(params.get('priority') as TicketPriority) ? params.get('priority')! : ''
  const category = params.get('category_id') ?? ''
  const department = staff ? params.get('department_id') ?? '' : ''
  const assignee = staff ? params.get('assignee_id') ?? '' : ''
  const mine = params.get('mine') === 'true' ? 'true' : ''
  const from = params.get('from') ?? '', to = params.get('to') ?? '', explicitTimezone = params.get('timezone') ?? ''
  const settings = useSettings()
  const defaultTimezone = settings.data?.settings.reporting_timezone ?? 'Asia/Almaty'
  const timezone = explicitTimezone || defaultTimezone
  const periodValid = (!from && !to) || validReportingWindow({ from, to, timezone })
  const [periodDraft, setPeriodDraft] = useState<ReportingWindow>({ from, to, timezone })
  const [dateInvalid, setDateInvalid] = useState(false)
  const periodTouched = useRef(false)
  useEffect(() => { periodTouched.current = false; setPeriodDraft({ from, to, timezone: explicitTimezone || defaultTimezone }); setDateInvalid(false) }, [from, to, explicitTimezone])
  useEffect(() => { if (!periodTouched.current && !explicitTimezone) setPeriodDraft(previous => ({ ...previous, timezone: defaultTimezone })) }, [defaultTimezone, explicitTimezone])
  const pendingExport = useRef<AbortController | null>(null)
  const [exporting, setExporting] = useState(false)
  const [exportError, setExportError] = useState<unknown>(null)
  useEffect(() => () => { pendingExport.current?.abort(); pendingExport.current = null }, [])
  const requestedPage = Number(params.get('page') ?? 0)
  const page = Number.isInteger(requestedPage) && requestedPage >= 0 && requestedPage <= 5000 ? requestedPage : 0
  const limit = 20
  const appliedFilters = { q: query, status, priority, category_id: category, department_id: department, assignee_id: assignee, mine, from, to, timezone: from || to ? timezone : '' }
  const filterSignature = queryString(appliedFilters)
  useEffect(() => {
    if (pendingExport.current) { pendingExport.current.abort(); pendingExport.current = null; setExporting(false) }
  }, [filterSignature, locale])
  useEffect(() => { setExportError(null) }, [filterSignature])
  const categories = useCategories()
  const assignees = useQuery({ queryKey: ['tickets', 'assignees'], enabled: staff, queryFn: ({ signal }) => api('/tickets/assignees', assigneesSchema, { signal }) })
  const departments = useQuery({ queryKey: ['tickets', 'departments'], enabled: staff, queryFn: ({ signal }) => api('/tickets/departments', ticketDepartmentsSchema, { signal }) })
  const tickets = useQuery({ queryKey: ['tickets', 'list', appliedFilters, page], enabled: periodValid, queryFn: ({ signal }) => api(`/tickets?${queryString({ ...appliedFilters, limit, offset: page * limit })}`, ticketsPageSchema, { signal }) })
  function change(values: Record<string, string>) {
    const next = new URLSearchParams(params)
    Object.entries(values).forEach(([key, value]) => { if (value) next.set(key, value); else next.delete(key) })
    setExportError(null)
    setParams(next)
  }
  function applyPeriod() {
    const next = { ...periodDraft, timezone: periodDraft.timezone.trim() }
    if ((next.from || next.to) && !validReportingWindow(next)) { setDateInvalid(true); return }
    setDateInvalid(false)
    change({ from: next.from, to: next.to, timezone: next.from && next.to ? next.timezone : '', page: '' })
  }
  async function exportFile(format: DownloadFormat) {
    if (!staff || !periodValid || pendingExport.current) return
    const controller = new AbortController()
    pendingExport.current = controller
    setExporting(true); setExportError(null)
    try { await download(`/tickets/export?${queryString({ format, locale, ...appliedFilters })}`, format, controller.signal) }
    catch (error) { if (!controller.signal.aborted && !(error instanceof ApiError && error.code === 'request_cancelled')) setExportError(error) }
    finally { if (pendingExport.current === controller) { pendingExport.current = null; setExporting(false) } }
  }
  const filterError = categories.isError || (staff && assignees.isError) || (staff && departments.isError)
  return <>
    <div className="page-heading"><div><div className="eyebrow"><span />MedDesk · {t.tickets}</div><h1>{t.tickets}</h1><p>{staff ? t.ticketQueueDescription : t.ticketsDescription}</p></div><Button asChild className="heading-action"><Link to="/tickets/new"><Plus size={17} aria-hidden="true" />{t.newTicket}</Link></Button></div>
    <section className="knowledge-search content-card">
      <form className="knowledge-search-form" onSubmit={event => { event.preventDefault(); change({ q: search.trim(), page: '' }) }}><div className="search-input"><Search size={19} aria-hidden="true" /><input value={search} onChange={event => setSearch(event.target.value)} maxLength={100} aria-label={t.ticketSearch} placeholder={t.ticketSearch} /></div><Button type="submit">{t.search}</Button></form>
      <div className="knowledge-filters ticket-filters">
        <select aria-label={t.status} value={status} onChange={event => change({ status: event.target.value, page: '' })}><option value="">{t.allStatuses}</option>{ticketStatuses.map(value => <option key={value} value={value}>{t[ticketStatusKey[value]]}</option>)}</select>
        <select aria-label={t.priority} value={priority} onChange={event => change({ priority: event.target.value, page: '' })}><option value="">{t.allPriorities}</option>{ticketPriorities.map(value => <option key={value} value={value}>{t[ticketPriorityKey[value]]}</option>)}</select>
        <select aria-label={t.category} value={category} disabled={categories.isPending} onChange={event => change({ category_id: event.target.value, page: '' })}><option value="">{t.allCategories}</option>{categories.data?.map(value => <option key={value.id} value={value.id}>{locale === 'kk' ? value.name_kk : value.name_ru}</option>)}</select>
        {staff && <><select aria-label={t.assignee} value={assignee} disabled={assignees.isPending} onChange={event => change({ assignee_id: event.target.value, page: '' })}><option value="">{t.allAssignees}</option>{assignees.data?.items.map(value => <option key={value.id} value={value.id}>{value.display_name}</option>)}</select><select aria-label={t.department} value={department} disabled={departments.isPending} onChange={event => change({ department_id: event.target.value, page: '' })}><option value="">{t.allDepartments}</option>{departments.data?.items.map(value => <option key={value.id} value={value.id}>{value.name}</option>)}</select><label className="checkbox-field"><input type="checkbox" checked={mine === 'true'} onChange={event => change({ mine: event.target.checked ? 'true' : '', page: '' })} />{t.myTickets}</label></>}
        {params.size > 0 && <Button variant="ghost" onClick={() => { setSearch(''); periodTouched.current = false; setPeriodDraft({ from: '', to: '', timezone: defaultTimezone }); setDateInvalid(false); setExportError(null); setParams({}) }}>{t.clearSearch}</Button>}
      </div>
      {filterError && <p className="inline-error" role="alert">{t.requestFailed} <Button variant="ghost" size="sm" onClick={() => { void categories.refetch(); if (staff) void assignees.refetch(); if (staff) void departments.refetch() }}>{t.retry}</Button></p>}
      <form className="ticket-period-form" onSubmit={event => { event.preventDefault(); applyPeriod() }} noValidate>
        <h2>{t.ticketCreationPeriod}</h2><div className="reporting-filter-grid"><ReportingFilters prefix="tickets" value={periodDraft} onChange={value => { periodTouched.current = true; setPeriodDraft(value) }} invalid={dateInvalid || !periodValid} /></div><p className="field-hint">{t.ticketOptionalPeriod} {t.reportingInclusive}</p>{(dateInvalid || !periodValid) && <p className="inline-error" role="alert">{t.reportingValidation}</p>}<div className="form-actions"><Button type="submit" variant="outline">{t.applyFilters}</Button></div>
      </form>
    </section>
    {staff && <section className="content-card ticket-export" aria-labelledby="ticket-export-heading">
      <div><h2 id="ticket-export-heading">{t.ticketExport}</h2><p>{t.ticketExportHint}</p><p className="field-hint">{from && to ? `${from} — ${to} · ${timezone}` : t.ticketAllTime} · {t.department}: {departments.data?.items.find(item => item.id === department)?.name ?? (department || t.allDepartments)} · {t.assignee}: {assignees.data?.items.find(item => item.id === assignee)?.display_name ?? (assignee || t.allAssignees)}{mine && ` · ${t.myTickets}`}{tickets.data && ` · ${t.ticketExportCount}: ${tickets.data.total}`}</p></div>
      <div className="ticket-export-actions"><Button variant="outline" disabled={exporting || !periodValid} onClick={() => void exportFile('xlsx')}><Download size={16} aria-hidden="true" />{t.exportExcel}</Button><Button variant="outline" disabled={exporting || !periodValid} onClick={() => void exportFile('csv')}><Download size={16} aria-hidden="true" />{t.exportCSV}</Button></div>{exporting && <p role="status">{t.exportPreparing}</p>}{exportError != null && <p className="inline-error" role="alert">{message(exportError)}</p>}
    </section>}
    {periodValid && <KnowledgeState pending={tickets.isPending} error={tickets.error} retry={() => void tickets.refetch()}>{tickets.data && <section className="table-card ticket-list"><div className="table-scroll"><table><thead><tr><th scope="col">{t.ticketNumber}</th><th scope="col">{t.ticketTitle}</th><th scope="col">{t.status}</th><th scope="col">{t.priority}</th>{staff && <th scope="col">{t.requester}</th>}<th scope="col">{t.assignee}</th><th scope="col">{t.updated}</th></tr></thead><tbody>{tickets.data.items.map(ticket => <tr key={ticket.id}><td>#{ticket.number}</td><td><Link className="ticket-title-link" to={`/tickets/${ticket.id}`}>{ticket.title}</Link>{staff && <span className="table-secondary">{ticket.department_name}</span>}</td><td><Badge className={`ticket-status ticket-status-${ticket.status.toLowerCase()}`}>{t[ticketStatusKey[ticket.status]]}</Badge></td><td><Badge className={`ticket-priority ticket-priority-${ticket.priority.toLowerCase()}`}>{t[ticketPriorityKey[ticket.priority]]}</Badge></td>{staff && <td>{ticket.requester_name}</td>}<td>{ticket.assignee_name || t.unassigned}</td><td>{new Date(ticket.updated_at).toLocaleDateString(locale === 'kk' ? 'kk-KZ' : 'ru-RU')}</td></tr>)}</tbody></table></div>{tickets.data.items.length === 0 && <div className="table-state"><TicketIcon size={32} aria-hidden="true" /><p>{t.noTickets}</p></div>}<Pagination page={page} total={tickets.data.total} limit={limit} busy={tickets.isFetching} onChange={next => change({ page: String(next) })} /></section>}</KnowledgeState>}
  </>
}

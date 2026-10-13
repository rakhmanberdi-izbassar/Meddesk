import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'
import { LocaleProvider } from '../lib/locale'
import type { User } from '../lib/api'
import type { AnalyticsData } from '../lib/analytics'
import { validReportingWindow } from '../lib/analytics'
import type { ApplicationSettings } from '../lib/settings'
import { settingsQueryKey } from '../lib/settings'

const UUID = '11111111-1111-4111-8111-111111111111'
const employee: User = { id: UUID, email: 'employee@example.test', display_name: 'Айым Тест', role: 'EMPLOYEE', department_id: UUID, department_name: 'Тест бөлімі', is_active: true }
const engineer: User = { ...employee, id: '22222222-2222-4222-8222-222222222222', display_name: 'IT Тест', role: 'IT_ENGINEER' }
const admin: User = { ...employee, id: '33333333-3333-4333-8333-333333333333', display_name: 'Әкімші Тест', role: 'ADMIN' }
const token = 'synthetic-test-csrf-token-12345'
const period = { from: '2026-09-13', to: '2026-10-12', timezone: 'Asia/Almaty', start_at: '2026-09-12T19:00:00Z', end_at: '2026-10-12T19:00:00Z' }
const baseSettings: ApplicationSettings = { clinic_name: 'Тест клиникасы', reporting_timezone: 'Asia/Almaty', default_ticket_priority: 'HIGH', version: 4, updated_at: '2026-10-12T00:00:00Z' }
const baseReport: AnalyticsData = { period, summary: { tickets_total: 8, tickets_open: 3, tickets_resolved: 5, tickets_closed: 2, average_resolution_seconds: 7200, feedback_attempts: 4, helpful_attempts: 3, self_service_rate: .75 }, departments: [{ id: UUID, name: 'Тест бөлімі', total: 8, open: 3, resolved: 5 }], categories: [{ id: UUID, name_kk: 'Бұрынғы санат', name_ru: 'Архивная категория', total: 8, open: 3, resolved: 5 }], common_issues: [{ id: UUID, name_kk: 'Бұрынғы санат', name_ru: 'Архивная категория', total: 8, open: 3, resolved: 5 }], popular_articles: [{ id: UUID, title: 'Принтерді іске қосу', view_count: 9, helpful_count: 3, not_helpful_count: 1 }], trend: [{ date: period.from, tickets_created: 2, tickets_resolved: 1, feedback_attempts: 2, helpful_attempts: 1 }, { date: period.to, tickets_created: 6, tickets_resolved: 4, feedback_attempts: 2, helpful_attempts: 2 }] }
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const failure = (status: number, code: string) => json({ error: { code, message: 'private server detail' } }, status)
let identity: User
let settings: ApplicationSettings
let report: AnalyticsData
let override: ((path: string, init?: RequestInit) => Response | Promise<Response> | undefined) | undefined
const calls = (part: string, method?: string) => vi.mocked(fetch).mock.calls.filter(([path, init]) => String(path).includes(part) && (!method || init?.method === method))
function renderPage(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
  return { client, ...render(<QueryClientProvider client={client}><LocaleProvider><MemoryRouter initialEntries={[path]}><App /></MemoryRouter></LocaleProvider></QueryClientProvider>) }
}
beforeEach(() => {
  identity = engineer; settings = { ...baseSettings }; report = structuredClone(baseReport); override = undefined; localStorage.clear()
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url), response = override?.(path, init)
    if (response) return response
    if (path === '/api/v1/auth/me') return json({ user: identity, csrf_token: token })
    if (path === '/api/v1/settings' && init?.method === 'PATCH') { settings = { ...settings, ...JSON.parse(String(init.body)), version: settings.version + 1 }; return json({ settings }) }
    if (path === '/api/v1/settings') return json({ settings })
    if (path === '/api/v1/analytics/filters') return json({ departments: [{ id: UUID, name: 'Тест бөлімі' }], categories: [{ id: UUID, name_kk: 'Бұрынғы санат', name_ru: 'Архивная категория' }], assignees: [{ id: engineer.id, display_name: engineer.display_name }] })
    if (path === '/api/v1/analytics' || path.startsWith('/api/v1/analytics?')) return json(report)
    if (path === '/api/v1/admin/audit/filters') return json({ actions: ['ticket.created', 'settings.updated'], entity_types: ['ticket', 'application_settings'], actors: [{ id: admin.id, display_name: admin.display_name }] })
    if (path.startsWith('/api/v1/admin/audit?')) { const p = new URL(path, 'http://test').searchParams; const offset = Number(p.get('offset')); return json({ period, items: [{ id: offset + 1, actor_id: admin.id, actor_name: admin.display_name, action: 'ticket.created', entity_type: 'ticket', entity_id: UUID, metadata: { version: offset + 1, status: 'NEW' }, created_at: '2026-10-12T00:00:00Z' }], total: 21, limit: 20, offset }) }
    if (path.startsWith('/api/v1/knowledge/categories?')) return json({ items: [], total: 0, limit: 100, offset: 0 })
    if (path.startsWith('/api/v1/tickets/suggestions?')) return json({ items: [], total: 0, limit: 5, offset: 0 })
    if (path.startsWith('/api/v1/tickets?')) { const p = new URL(path, 'http://test').searchParams; const totals: Record<string, number> = { NEW: 1, IN_PROGRESS: 2, WAITING_FOR_USER: 3, RESOLVED: 4, CLOSED: 5 }; return json({ items: [], total: totals[p.get('status') ?? ''] ?? 0, limit: 1, offset: 0 }) }
    if (path === '/api/v1/health') return json({ status: 'ok' })
    if (path === '/api/v1/ready') return json({ status: 'ready' })
    return failure(404, 'not_found')
  }))
})

describe('phase 5 analytics and administration', () => {
  it.each(['/analytics', '/admin/audit', '/admin/settings'])('blocks employees before fetching protected data at %s', async path => {
    identity = employee; renderPage(path)
    await screen.findByRole('heading', { name: 'Қолжетімділік шектеулі' })
    expect(calls('/analytics')).toHaveLength(0); expect(calls('/admin/audit')).toHaveLength(0)
    expect(screen.queryByRole('link', { name: 'Аналитика' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Қолданба баптаулары' })).not.toBeInTheDocument()
    expect(calls('/settings', 'PATCH')).toHaveLength(0)
  })
  it('keeps administrators-only audit unavailable to engineers', async () => {
    renderPage('/admin/audit'); await screen.findByRole('heading', { name: 'Қолжетімділік шектеулі' })
    expect(calls('/admin/audit')).toHaveLength(0)
    expect(screen.getByRole('link', { name: 'Аналитика' })).toHaveAttribute('href', '/analytics')
  })
  it('renders server values, the honest percentage, accessible chart data and archived filter options', async () => {
    renderPage('/analytics'); await screen.findByText('75%')
    expect(screen.getByText('2 сағ')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Күндер бойынша өзгеріс' })).toBeInTheDocument()
    await userEvent.click(screen.getByText('Күндер бойынша кестені ашу'))
    expect(screen.getByRole('table', { name: 'Күндер бойынша өзгеріс' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Принтерді іске қосу' })).toHaveAttribute('href', `/knowledge/articles/${UUID}`)
    expect(screen.getByRole('option', { name: 'Бұрынғы санат' })).toBeInTheDocument()
    expect(screen.getByLabelText('Басталу күні')).toHaveValue(period.from)
    await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
    expect(screen.getByRole('heading', { name: 'Заявки по категориям' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Архивная категория' })).toBeInTheDocument()
    expect(screen.getByText('2 ч')).toBeInTheDocument()
  })
  it('distinguishes no measurement from zero feedback and does not invent successful self-service', async () => {
    report = { ...report, summary: { tickets_total: 0, tickets_open: 0, tickets_resolved: 0, tickets_closed: 0, average_resolution_seconds: null, feedback_attempts: 0, helpful_attempts: 0, self_service_rate: null }, departments: [], categories: [], common_issues: [], popular_articles: [], trend: [] }
    renderPage('/analytics'); await screen.findByText('Бұл кезеңде таңдалған сүзгілерге сәйкес өтінім немесе аяқталған нұсқаулық әрекеті жоқ.')
    expect(screen.getAllByText('Дерек жоқ')).toHaveLength(2)
    expect(screen.queryByText('0%')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
  it('applies date, timezone and every scoped filter only after submission', async () => {
    renderPage('/analytics'); await screen.findByText('75%')
    fireEvent.change(screen.getByLabelText('Басталу күні'), { target: { value: '2026-10-01' } })
    fireEvent.change(screen.getByLabelText('Аяқталу күні'), { target: { value: '2026-10-10' } })
    await userEvent.clear(screen.getByLabelText('Есептің уақыт белдеуі')); await userEvent.type(screen.getByLabelText('Есептің уақыт белдеуі'), 'UTC')
    await userEvent.selectOptions(screen.getByLabelText('Бөлім атауы'), UUID)
    await userEvent.selectOptions(screen.getByLabelText('Санат'), UUID)
    await userEvent.selectOptions(screen.getByLabelText('Орындаушы'), engineer.id)
    expect(calls('/analytics?')).toHaveLength(1)
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    await waitFor(() => expect(calls('/analytics?')).toHaveLength(2))
    const p = new URL(String(calls('/analytics?')[1][0]), 'http://test').searchParams
    expect(Object.fromEntries(p)).toEqual({ from: '2026-10-01', to: '2026-10-10', timezone: 'UTC', department_id: UUID, category_id: UUID, assignee_id: engineer.id })
  })
  it('rejects reversed dates and preserves edited filters on background refetch', async () => {
    const { client } = renderPage('/analytics'); await screen.findByText('75%')
    fireEvent.change(screen.getByLabelText('Басталу күні'), { target: { value: '2026-10-13' } })
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    expect(await screen.findByText('Дұрыс күндер мен уақыт белдеуін көрсетіңіз. Кезең 1–366 күн болуы керек.')).toBeInTheDocument()
    expect(calls('/analytics?')).toHaveLength(1)
    report = { ...report, period: { ...period, from: '2026-09-14' } }
    await act(async () => { await client.invalidateQueries({ queryKey: ['analytics', 'report'] }) })
    expect(screen.getByLabelText('Басталу күні')).toHaveValue('2026-10-13')
  })
  it('filters audit events and resets pagination while presenting safe structured values', async () => {
    identity = admin; renderPage('/admin/audit'); await screen.findByRole('link', { name: UUID })
    await userEvent.click(screen.getByRole('button', { name: 'Келесі' }))
    await waitFor(() => expect(calls('/admin/audit?').some(([path]) => String(path).includes('offset=20'))).toBe(true))
    await userEvent.selectOptions(screen.getByLabelText('Әрекет'), 'settings.updated')
    await userEvent.selectOptions(screen.getByLabelText('Әрекет иесі'), admin.id)
    await userEvent.selectOptions(screen.getByLabelText('Нысан түрі'), 'application_settings')
    await userEvent.type(screen.getByLabelText('Нысан ID'), UUID)
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    await waitFor(() => expect(calls('/admin/audit?')).toHaveLength(3))
    const params = new URL(String(calls('/admin/audit?')[2][0]), 'http://test').searchParams
    expect(Object.fromEntries(params)).toEqual({ from: period.from, to: period.to, timezone: period.timezone, action: 'settings.updated', actor_id: admin.id, entity_type: 'application_settings', entity_id: UUID, offset: '0', limit: '20' })
    expect(calls('/admin/audit?', 'PATCH')).toHaveLength(0)
  })
  it('escapes metadata strings, renders unknown/system actors and handles an empty audit', async () => {
    identity = admin
    override = path => path.startsWith('/api/v1/admin/audit?') ? json({ period, items: [{ id: 1, actor_id: null, actor_name: null, action: 'custom.action', entity_type: 'custom', entity_id: null, metadata: { safe: '<img src=x onerror=alert(1)>' }, created_at: period.start_at }], total: 1, limit: 20, offset: 0 }) : undefined
    const { client } = renderPage('/admin/audit'); await screen.findByText('Жүйе / белгісіз пайдаланушы')
    await userEvent.click(screen.getByText('Әрекет деректері', { selector: 'summary' }))
    expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    override = path => path.startsWith('/api/v1/admin/audit?') ? json({ period, items: [], total: 0, limit: 20, offset: 0 }) : undefined
    await act(async () => { await client.invalidateQueries({ queryKey: ['admin', 'audit', 'page'] }) })
    expect(await screen.findByText('Осы сүзгілерге сәйкес әрекет табылмады.')).toBeInTheDocument()
  })
  it('confirms a settings change with original version and CSRF and refreshes clinic branding', async () => {
    identity = admin; renderPage('/admin/settings'); const input = await screen.findByLabelText('Клиника атауы')
    await userEvent.clear(input); await userEvent.type(input, 'Жаңа клиника')
    await userEvent.selectOptions(screen.getByLabelText('Жаңа өтінімнің әдепкі басымдығы'), 'LOW')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    const confirmation = screen.getByRole('dialog', { name: 'Баптауларды сақтауды растаңыз' })
    expect(calls('/settings', 'PATCH')).toHaveLength(0)
    await userEvent.click(within(confirmation).getByRole('button', { name: 'Растау' }))
    await screen.findByText('Баптаулар сақталды.')
    const call = calls('/settings', 'PATCH')[0]
    expect(JSON.parse(String(call[1]?.body))).toEqual({ version: 4, clinic_name: 'Жаңа клиника', default_ticket_priority: 'LOW', reporting_timezone: 'Asia/Almaty' })
    expect(call[1]?.headers).toMatchObject({ 'X-CSRF-Token': token })
    expect(screen.getByText('Жаңа клиника', { selector: '.brand-subtitle' })).toBeInTheDocument()
  })
  it('retains settings input and version on background changes and 409 until an explicit reload', async () => {
    identity = admin; const { client } = renderPage('/admin/settings'); const input = await screen.findByLabelText('Клиника атауы')
    await userEvent.clear(input); await userEvent.type(input, 'Сақталмаған атау')
    settings = { ...settings, version: 5, clinic_name: 'Басқа әкімші атауы' }
    await act(async () => { await client.invalidateQueries({ queryKey: ['application-settings'] }) })
    expect(screen.getByLabelText('Клиника атауы')).toHaveValue('Сақталмаған атау')
    override = (path, init) => path === '/api/v1/settings' && init?.method === 'PATCH' ? failure(409, 'version_conflict') : undefined
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Растау' }))
    await screen.findByText('Баптаулар басқа жерде өзгертілді. Енгізген деректеріңіз сақталды. Жалғастыру үшін соңғы нұсқаны қарап, қайта жүктеңіз.')
    expect(JSON.parse(String(calls('/settings', 'PATCH')[0][1]?.body)).version).toBe(4)
    expect(screen.getByLabelText('Клиника атауы')).toHaveValue('Сақталмаған атау')
    expect(screen.getByRole('button', { name: 'Сақтау' })).toBeDisabled()
    await userEvent.click(screen.getAllByRole('button', { name: 'Баптаулардың соңғы нұсқасын жүктеу' })[0])
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Бас тарту' }))
    expect(screen.getByLabelText('Клиника атауы')).toHaveValue('Сақталмаған атау')
    await userEvent.click(screen.getAllByRole('button', { name: 'Баптаулардың соңғы нұсқасын жүктеу' })[0])
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Баптаулардың соңғы нұсқасын жүктеу' }))
    await waitFor(() => expect(screen.getByLabelText('Клиника атауы')).toHaveValue('Басқа әкімші атауы'))
  })
  it('ignores a late settings save for a previous account', async () => {
    identity = admin; let resolve!: (value: Response) => void
    override = (path, init) => path === '/api/v1/settings' && init?.method === 'PATCH' ? new Promise<Response>(done => { resolve = done }) : undefined
    const { client } = renderPage('/admin/settings'); const input = await screen.findByLabelText('Клиника атауы')
    await userEvent.type(input, ' ескі өзгеріс'); await userEvent.click(screen.getByRole('button', { name: 'Сақтау' })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Растау' }))
    identity = employee
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await screen.findByRole('heading', { name: 'Қолжетімділік шектеулі' })
    await act(async () => { resolve(json({ settings: { ...settings, clinic_name: 'Late old account', version: 5 } })) })
    expect(client.getQueryData(settingsQueryKey(admin.id, admin.role))).toBeUndefined()
    expect(screen.queryByText('Late old account')).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('removes administrator audit data and navigation after a role downgrade', async () => {
    identity = admin; const { client } = renderPage('/admin/audit'); await screen.findByRole('link', { name: UUID })
    identity = { ...admin, role: 'EMPLOYEE' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await screen.findByRole('heading', { name: 'Қолжетімділік шектеулі' })
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Әрекеттер журналы' })).not.toBeInTheDocument()
    expect(client.getQueryCache().findAll({ queryKey: ['admin', 'audit'] })).toHaveLength(0)
  })
  it('uses the settings priority once and preserves user choice through subsequent settings changes', async () => {
    identity = employee; const { client } = renderPage('/tickets/new'); await screen.findByLabelText('Мәселені сипаттаңыз')
    await userEvent.type(screen.getByLabelText('Мәселені сипаттаңыз'), 'Принтер іске қосылмайды')
    await userEvent.click(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' }))
    await screen.findByText('Ұқсас нұсқаулық табылмады. Өтінім жіберуге болады.')
    await userEvent.click(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' }))
    expect(screen.getByLabelText('Басымдық')).toHaveValue('HIGH')
    await userEvent.selectOptions(screen.getByLabelText('Басымдық'), 'LOW')
    settings = { ...settings, default_ticket_priority: 'CRITICAL', version: 5 }
    await act(async () => { await client.invalidateQueries({ queryKey: ['application-settings'] }) })
    expect(screen.getByLabelText('Басымдық')).toHaveValue('LOW')
  })
  it('preserves priority chosen before the initial settings response arrives', async () => {
    identity = employee; let resolve!: (value: Response) => void
    override = path => path === '/api/v1/settings' ? new Promise<Response>(done => { resolve = done }) : undefined
    renderPage('/tickets/new'); await screen.findByLabelText('Мәселені сипаттаңыз')
    await userEvent.type(screen.getByLabelText('Мәселені сипаттаңыз'), 'Принтер іске қосылмайды')
    await userEvent.click(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' }))
    await screen.findByText('Ұқсас нұсқаулық табылмады. Өтінім жіберуге болады.')
    await userEvent.click(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' }))
    expect(screen.getByRole('button', { name: 'Өтінім жіберу' })).toBeDisabled()
    await userEvent.selectOptions(screen.getByLabelText('Басымдық'), 'LOW')
    expect(screen.getByRole('button', { name: 'Өтінім жіберу' })).toBeEnabled()
    await act(async () => { resolve(json({ settings })) })
    expect(screen.getByLabelText('Басымдық')).toHaveValue('LOW')
  })
  it('derives employee overview counts from their actual ticket totals', async () => {
    identity = employee; renderPage('/'); await screen.findByRole('heading', { name: 'Менің өтінімдерімнің күйі' })
    await waitFor(() => expect(screen.getByText('Барлық өтінімдер').nextElementSibling).toHaveTextContent('15'))
    expect(screen.getByText('Ашық өтінімдер').nextElementSibling).toHaveTextContent('6')
    expect(calls('/analytics')).toHaveLength(0)
  })
})

it('validates inclusive reporting windows and IANA timezone without browser timezone dependence', () => {
  expect(validReportingWindow({ from: '2024-01-01', to: '2024-12-31', timezone: 'Asia/Almaty' })).toBe(true)
  expect(validReportingWindow({ from: '2024-01-01', to: '2025-01-01', timezone: 'UTC' })).toBe(false)
  expect(validReportingWindow({ from: '2026-02-30', to: '2026-03-05', timezone: 'UTC' })).toBe(false)
  expect(validReportingWindow({ from: '2026-10-12', to: '2026-10-12', timezone: 'Unknown/Zone' })).toBe(false)
})

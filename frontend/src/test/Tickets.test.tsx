import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'
import { LocaleProvider } from '../lib/locale'
import type { User } from '../lib/api'
import type { Article, Category } from '../lib/knowledge'
import { imageMaxBytes, validMediaFile } from '../lib/knowledge'
import type { Ticket, TicketDetail } from '../lib/tickets'

const employee: User = { id: 'employee-id', email: 'employee@example.test', display_name: 'Айым Тест', role: 'EMPLOYEE', department_id: 'department-id', department_name: 'Тест бөлімі', is_active: true }
const engineer: User = { ...employee, id: 'engineer-id', display_name: 'IT Тест', role: 'IT_ENGINEER' }
const category: Category = { id: 'category-id', slug: 'printers', name_kk: 'Принтер', name_ru: 'Принтеры', description: '', is_active: true, created_at: '2026-10-12T00:00:00Z', updated_at: '2026-10-12T00:00:00Z' }
const guide: Article = { id: 'article-id', version: 4, title: 'Принтерді іске қосу', slug: 'printer', description: 'Қосылымды тексеріңіз.', category_id: category.id, category_name_kk: category.name_kk, category_name_ru: category.name_ru, category_active: true, symptoms: 'Принтер жұмыс істемейді.', solution_steps: ['Қуатты тексеріңіз.', 'Қайта қосыңыз.'], tags: ['принтер'], author_id: engineer.id, author_name: engineer.display_name, video_id: null, image_ids: [], is_published: true, published_at: '2026-10-12T00:00:00Z', created_at: '2026-10-12T00:00:00Z', updated_at: '2026-10-12T00:00:00Z', view_count: 2, helpful_count: 1, not_helpful_count: 0 }
const example: Ticket = { id: 'ticket-id', number: 17, title: 'Принтер басып шығармайды', description: 'USB қосылған, принтер қосылмайды.', requester_id: employee.id, requester_name: employee.display_name, department_id: employee.department_id, department_name: employee.department_name, category_id: category.id, priority: 'MEDIUM', status: 'NEW', assignee_id: null, assignee_name: '', version: 4, created_at: '2026-10-12T00:00:00Z', updated_at: '2026-10-12T00:00:00Z', resolved_at: null, article_ids: [], attachment_ids: [], converted_article_id: null }
const token = 'synthetic-test-csrf-token-12345'
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const failure = (status: number, code: string) => json({ error: { code, message: 'private server detail' } }, status)
let identity: User
let ticket: Ticket
let detail: Omit<TicketDetail, 'ticket'>
let override: ((path: string, init?: RequestInit) => Response | Promise<Response> | undefined) | undefined
function renderPage(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
  return { client, ...render(<QueryClientProvider client={client}><LocaleProvider><MemoryRouter initialEntries={[path]}><App /></MemoryRouter></LocaleProvider></QueryClientProvider>) }
}
const calls = (part: string, method?: string) => vi.mocked(fetch).mock.calls.filter(([path, init]) => String(path).includes(part) && (!method || init?.method === method))
beforeEach(() => {
  identity = employee; ticket = { ...example }; detail = { comments: [], history: [{ id: 1, from_status: null, to_status: 'NEW', actor_name: employee.display_name, reason: '', created_at: example.created_at }] }; override = undefined; localStorage.clear()
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url), response = override?.(path, init)
    if (response) return response
    if (path === '/api/v1/auth/me') return json({ user: identity, csrf_token: token })
    if (path === '/api/v1/settings') return json({ settings: { clinic_name: 'Тест клиникасы', default_ticket_priority: 'MEDIUM', reporting_timezone: 'Asia/Almaty', version: 1, updated_at: example.created_at } })
    if (path.startsWith('/api/v1/tickets/export?')) { const format = new URL(path, 'https://example.test').searchParams.get('format'); return new Response('synthetic report', { headers: { 'Content-Type': format === 'csv' ? 'text/csv; charset=utf-8' : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' } }) }
    if (path.startsWith('/api/v1/knowledge/categories?')) return json({ items: [category], total: 1, limit: 100, offset: 0 })
    if (path === '/api/v1/tickets/assignees') return json({ items: [{ id: engineer.id, display_name: engineer.display_name }] })
    if (path === '/api/v1/tickets/departments') return json({ items: [{ id: employee.department_id, name: employee.department_name }] })
    if (path.startsWith('/api/v1/tickets/suggestions?')) return json({ items: [guide], total: 1, limit: 5, offset: 0 })
    if (path.startsWith('/api/v1/tickets?')) return json({ items: [ticket], total: 45, limit: 20, offset: Number(new URL(path, 'https://example.test').searchParams.get('offset')) })
    if (path === '/api/v1/tickets' && init?.method === 'POST') { ticket = { ...example, ...JSON.parse(String(init.body)) }; return json({ ticket }, 201) }
    if (path === '/api/v1/tickets/ticket-id' && init?.method === 'PATCH') { const body = JSON.parse(String(init.body)); ticket = { ...ticket, ...body, assignee_name: body.assignee_id === engineer.id ? engineer.display_name : ticket.assignee_name, version: ticket.version + 1 }; return json({ ticket }) }
    if (path === '/api/v1/tickets/ticket-id/comments') { const body = JSON.parse(String(init?.body)); const comment = { id: 'comment-id', author_id: identity.id, author_name: identity.display_name, body: body.body, is_internal: body.is_internal, created_at: example.created_at }; detail.comments.push(comment); ticket = { ...ticket, version: ticket.version + 1 }; return json({ ticket, comment }) }
    if (path === '/api/v1/tickets/ticket-id/confirm') { ticket = { ...ticket, status: 'CLOSED', version: ticket.version + 1 }; return json({ ticket }) }
    if (path === '/api/v1/tickets/ticket-id/reopen') { ticket = { ...ticket, status: 'IN_PROGRESS', version: ticket.version + 1 }; return json({ ticket }) }
    if (path === '/api/v1/tickets/ticket-id/attachments') { ticket = { ...ticket, attachment_ids: [...ticket.attachment_ids, ...JSON.parse(String(init?.body)).attachment_ids], version: ticket.version + 1 }; return json({ ticket }) }
    if (path === '/api/v1/tickets/ticket-id/article') return json({ article: { ...guide, is_published: false, ...JSON.parse(String(init?.body)) } }, 201)
    if (path === '/api/v1/tickets/ticket-id') return json({ ticket, ...detail })
    if (path === '/api/v1/knowledge/articles/article-id') return json({ article: guide })
    if (path.endsWith('/feedback')) return json({ recorded: true, helpful_count: 2, not_helpful_count: 1 })
    if (path === '/api/v1/media?purpose=TICKET_ATTACHMENT') return json({ media: { id: 'image-id', original_filename: 'screen.png', content_type: 'image/png', byte_count: 4, purpose: 'TICKET_ATTACHMENT', created_at: example.created_at } }, 201)
    return failure(404, 'not_found')
  }))
})
afterEach(() => vi.restoreAllMocks())

function mockDownload() {
  const create = vi.fn(() => 'blob:synthetic-report'), revoke = vi.fn()
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: create })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revoke })
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  return { create, revoke, click }
}

async function suggestions() {
  await screen.findByLabelText('Мәселені сипаттаңыз')
  await userEvent.type(screen.getByLabelText('Мәселені сипаттаңыз'), 'принтер жұмыс істемейді')
  await userEvent.click(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' }))
  await screen.findByRole('heading', { name: guide.title })
}

describe('ticket workflows', () => {
  it('lists employee tickets with real filters and bounded pagination', async () => {
    renderPage('/tickets')
    // Cold lazy routes can compile alongside other test files; await the page before its request result.
    await screen.findByRole('heading', { name: 'Қолдау өтінімдері' }, { timeout: 5000 })
    await screen.findByRole('link', { name: example.title })
    expect(screen.queryByLabelText('Орындаушы')).not.toBeInTheDocument()
    expect(calls('/tickets/assignees')).toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Excel жүктеу' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'CSV жүктеу' })).not.toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: 'Өтінімдерді іздеу' }), 'принтер')
    await userEvent.click(screen.getByRole('button', { name: 'Іздеу' }))
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Күйі' }), 'NEW')
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Басымдық' }), 'HIGH')
    await userEvent.click(screen.getByRole('button', { name: 'Келесі' }))
    await waitFor(() => expect(calls('/tickets?').at(-1)?.[0]).toContain('status=NEW&priority=HIGH'))
    expect(calls('/tickets?').at(-1)?.[0]).toContain('offset=20')
    expect(screen.getByRole('link', { name: example.title })).toHaveAttribute('href', '/tickets/ticket-id')
  })
  it('lets engineers filter the queue by assignee, department, category and ownership', async () => {
    identity = engineer; renderPage('/engineer/tickets')
    await screen.findByRole('link', { name: example.title })
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Орындаушы' }), engineer.id)
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Бөлім' }), employee.department_id)
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Санат' }), category.id)
    await userEvent.click(screen.getByRole('checkbox', { name: 'Менің өтінімдерім' }))
    await waitFor(() => expect(calls('/tickets?').at(-1)?.[0]).toContain('mine=true'))
    expect(calls('/tickets?').at(-1)?.[0]).toContain('department_id=department-id&assignee_id=engineer-id')
    expect(calls('/admin/departments')).toHaveLength(0)
  })
  it('localizes ticket status, priorities and creation in Russian', async () => {
    renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
    expect(screen.getByRole('heading', { name: 'Заявки в поддержку' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Ожидает ответа сотрудника' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Срочный' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Новая заявка' })).toHaveAttribute('href', '/tickets/new')
  })
  it('blocks ticket creation while helpful feedback is pending and ends in self-service', async () => {
    let complete!: (response: Response) => void
    override = path => path.endsWith('/feedback') ? new Promise(resolve => { complete = resolve }) : undefined
    renderPage('/tickets/new'); await suggestions()
    await userEvent.click(screen.getByRole('button', { name: 'Иә, көмектесті' }))
    expect(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' })).toBeDisabled()
    await act(async () => complete(json({ recorded: true, helpful_count: 3, not_helpful_count: 0 })))
    await screen.findByRole('heading', { name: 'Мәселе өздігінен шешілді. Жауабыңыз тіркелді.' })
    expect(calls('/api/v1/tickets', 'POST')).toHaveLength(0)
  })
  it('requires retrying an ambiguous helpful answer before leaving self-service', async () => {
    let attempts = 0
    override = path => path.endsWith('/feedback') && ++attempts === 1 ? failure(503, 'request_failed') : undefined
    renderPage('/tickets/new'); await suggestions()
    await userEvent.click(screen.getByRole('button', { name: 'Иә, көмектесті' }))
    const retry = await screen.findByRole('button', { name: 'Жауапты қайта жіберу' })
    expect(retry).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' })).toBeDisabled()
    await userEvent.click(retry)
    await screen.findByRole('heading', { name: 'Мәселе өздігінен шешілді. Жауабыңыз тіркелді.' })
    expect(calls('/api/v1/tickets', 'POST')).toHaveLength(0)
    const answers = calls('/feedback').map(([, init]) => JSON.parse(String(init?.body)))
    expect(answers[0]).toEqual(answers[1]); expect(answers[0].helpful).toBe(true)
  })
  it('keeps feedback answer and attempt stable after an ambiguous failure', async () => {
    let attempts = 0
    override = path => path.endsWith('/feedback') && ++attempts === 1 ? failure(503, 'request_failed') : undefined
    renderPage('/tickets/new'); await suggestions()
    await userEvent.click(screen.getByRole('button', { name: 'Жоқ, көмектеспеді' }))
    await screen.findByRole('button', { name: 'Жауапты қайта жіберу' })
    expect(screen.getByRole('button', { name: 'Иә, көмектесті' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Жауапты қайта жіберу' }))
    await screen.findByText('Мәселе шешілмеді. IT маманына өтінім дайындаңыз.')
    const bodies = calls('/feedback').map(([, init]) => JSON.parse(String(init?.body)))
    expect(bodies[0]).toEqual(bodies[1]); expect(bodies[0]).toMatchObject({ attempt_id: expect.stringMatching(/^[0-9a-f-]{36}$/), helpful: false })
    expect(screen.queryByText('private server detail')).not.toBeInTheDocument()
  })
  it('creates an unresolved ticket with reviewed guides, private PNG upload and CSRF', async () => {
    renderPage('/tickets/new'); await suggestions()
    await userEvent.click(screen.getByRole('button', { name: 'Жоқ, көмектеспеді' }))
    await screen.findByText('Мәселе шешілмеді. IT маманына өтінім дайындаңыз.')
    await userEvent.click(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' }))
    await userEvent.type(screen.getByLabelText('Мәселенің атауы'), 'Принтер мәселесі')
    await userEvent.selectOptions(screen.getByLabelText('Санат'), category.id)
    await userEvent.selectOptions(screen.getByLabelText('Басымдық'), 'HIGH')
    await userEvent.upload(screen.getByLabelText('Скриншот қосу'), new File(['test'], 'screen.png', { type: 'image/png' }))
    await screen.findByRole('img', { name: 'Скриншоттар 1' })
    await userEvent.click(screen.getByRole('button', { name: 'Өтінім жіберу' }))
    await screen.findByRole('heading', { name: 'Принтер мәселесі' })
    const call = calls('/api/v1/tickets', 'POST').find(([path]) => path === '/api/v1/tickets')!
    expect(JSON.parse(String(call[1]?.body))).toEqual({ title: 'Принтер мәселесі', description: 'принтер жұмыс істемейді', priority: 'HIGH', category_id: category.id, article_ids: [guide.id], attachment_ids: ['image-id'] })
    expect(call[1]?.headers).toMatchObject({ 'X-CSRF-Token': token })
    const upload = calls('/media?purpose=TICKET_ATTACHMENT')[0]
    expect(upload[1]?.body).toBeInstanceOf(FormData)
    expect(upload[1]?.headers).not.toHaveProperty('Content-Type')
    expect(calls('/tickets/suggestions?')[0][0]).not.toContain('limit=')
  })
  it('does not offer creation until suggestions load successfully', async () => {
    override = path => path.includes('/tickets/suggestions?') ? failure(503, 'request_failed') : undefined
    renderPage('/tickets/new'); await screen.findByLabelText('Мәселені сипаттаңыз')
    await userEvent.type(screen.getByLabelText('Мәселені сипаттаңыз'), 'Жаңа мәселе')
    await userEvent.click(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' }))
    await screen.findByText('Нұсқаулықтарды жүктеу мүмкін болмады. Қайта тексеріңіз.')
    expect(screen.queryByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' })).not.toBeInTheDocument()
    override = path => path.includes('/tickets/suggestions?') ? json({ items: [], total: 0, limit: 5, offset: 0 }) : undefined
    await userEvent.click(screen.getByRole('button', { name: 'Қайталау' }))
    await screen.findByText('Ұқсас нұсқаулық табылмады. Өтінім жіберуге болады.')
    expect(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' })).toBeEnabled()
  })
  it('normalizes multiline descriptions for suggestions while preserving the original ticket text', async () => {
    renderPage('/tickets/new'); await screen.findByLabelText('Мәселені сипаттаңыз')
    await userEvent.type(screen.getByLabelText('Мәселені сипаттаңыз'), 'Принтер істемейді\nҚайта іске қосылды')
    await userEvent.click(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' }))
    await screen.findByRole('heading', { name: guide.title })
    const query = new URL(String(calls('/tickets/suggestions?')[0][0]), 'https://example.test').searchParams.get('q')
    expect(query).toBe('Принтер істемейді Қайта іске қосылды')
    expect(screen.getByLabelText('Мәселені сипаттаңыз')).toHaveValue('Принтер істемейді\nҚайта іске қосылды')
  })
  it('removes previous reviewed guide references when starting a new search', async () => {
    renderPage('/tickets/new'); await suggestions()
    await userEvent.click(screen.getByRole('button', { name: 'Жоқ, көмектеспеді' }))
    await screen.findByText('Мәселе шешілмеді. IT маманына өтінім дайындаңыз.')
    override = path => path.includes('/tickets/suggestions?') ? json({ items: [], total: 0, limit: 5, offset: 0 }) : undefined
    await userEvent.clear(screen.getByLabelText('Мәселені сипаттаңыз'))
    await userEvent.type(screen.getByLabelText('Мәселені сипаттаңыз'), 'Интернет істемейді')
    await userEvent.click(screen.getByRole('button', { name: 'Нұсқаулықтарды табу' }))
    await screen.findByText('Ұқсас нұсқаулық табылмады. Өтінім жіберуге болады.')
    await userEvent.click(screen.getByRole('button', { name: 'Мәселе шешілмеді — өтінім дайындау' }))
    expect(screen.queryByRole('heading', { name: 'Қаралған нұсқаулықтар' })).not.toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Мәселенің атауы'), 'Интернет мәселесі')
    await userEvent.click(screen.getByRole('button', { name: 'Өтінім жіберу' }))
    await screen.findByRole('heading', { name: 'Интернет мәселесі' })
    const call = calls('/api/v1/tickets', 'POST').find(([path]) => path === '/api/v1/tickets')!
    expect(JSON.parse(String(call[1]?.body)).article_ids).toEqual([])
  })
  it('changes priority without resubmitting an unchanged inactive assignee', async () => {
    identity = engineer; ticket = { ...ticket, assignee_id: 'inactive-id', assignee_name: 'Inactive engineer' }
    renderPage('/tickets/ticket-id'); await screen.findByRole('heading', { name: example.title })
    await userEvent.selectOptions(screen.getByLabelText('Басымдық'), 'HIGH')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByText('Өтінім жаңартылды.')
    expect(JSON.parse(String(calls('/tickets/ticket-id', 'PATCH')[0][1]?.body))).toEqual({ version: 4, priority: 'HIGH' })
  })
  it('shows employees public comments and confirms resolved tickets through the dedicated action', async () => {
    ticket = { ...ticket, status: 'RESOLVED', resolved_at: example.updated_at }
    detail.comments = [{ id: 'public-id', author_id: engineer.id, author_name: engineer.display_name, body: 'Қуат сымы ауыстырылды.', is_internal: false, created_at: example.created_at }, { id: 'internal-id', author_id: engineer.id, author_name: engineer.display_name, body: 'Private internal note', is_internal: true, created_at: example.created_at }]
    renderPage('/tickets/ticket-id'); await screen.findByRole('heading', { name: example.title })
    expect(screen.getByText('Қуат сымы ауыстырылды.')).toBeInTheDocument()
    expect(screen.queryByText('Private internal note')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Ішкі хабарлама (тек IT мамандары)')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Мәселе шешілгенін растау' }))
    const modal = screen.getByRole('dialog')
    await userEvent.click(within(modal).getByRole('button', { name: 'Өтінімді жабу' }))
    await waitFor(() => expect(screen.queryByLabelText('Хабарлама мәтіні')).not.toBeInTheDocument())
    expect(JSON.parse(String(calls('/confirm')[0][1]?.body))).toEqual({ version: 4 })
    expect(calls('/tickets/ticket-id', 'PATCH')).toHaveLength(0)
  })
  it('updates staff assignment and priority and uses the next version for a valid transition', async () => {
    identity = engineer; renderPage('/tickets/ticket-id'); await screen.findByRole('heading', { name: example.title })
    expect(screen.getByRole('heading', { name: 'Өтінім мәртебесі' })).toBeInTheDocument()
    expect(screen.getByText('Жаңа өтінімді «Орындалуда» күйіне ауыстырыңыз. Мәселе шешілген соң «Шешілген» күйін таңдаңыз.')).toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('Басымдық'), 'CRITICAL')
    await userEvent.click(screen.getByRole('button', { name: 'Өзіме тағайындау' }))
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByText('Өтінім жаңартылды.')
    expect(JSON.parse(String(calls('/tickets/ticket-id', 'PATCH')[0][1]?.body))).toEqual({ version: 4, priority: 'CRITICAL', assignee_id: engineer.id })
    expect(screen.queryByRole('option', { name: 'Жабылған' })).not.toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('Жаңа күй'), 'IN_PROGRESS')
    await userEvent.click(screen.getByRole('button', { name: 'Күйді өзгерту' }))
    await waitFor(() => expect(calls('/tickets/ticket-id', 'PATCH')).toHaveLength(2))
    expect(JSON.parse(String(calls('/tickets/ticket-id', 'PATCH')[1][1]?.body))).toMatchObject({ version: 5, status: 'IN_PROGRESS' })
  })
  it('preserves inputs and original version on background refresh, then requires explicit conflict refresh', async () => {
    identity = engineer
    const { client } = renderPage('/tickets/ticket-id'); await screen.findByRole('heading', { name: example.title })
    await userEvent.type(screen.getByLabelText('Хабарлама мәтіні'), 'Сақталмаған хабарлама')
    await userEvent.selectOptions(screen.getByLabelText('Басымдық'), 'HIGH')
    ticket = { ...ticket, status: 'IN_PROGRESS', version: 5, priority: 'LOW' }
    await act(async () => { await client.invalidateQueries({ queryKey: ['tickets', 'detail'] }) })
    expect(screen.getByLabelText('Хабарлама мәтіні')).toHaveValue('Сақталмаған хабарлама')
    expect(screen.getByLabelText('Басымдық')).toHaveValue('HIGH')
    override = path => path.endsWith('/comments') ? failure(409, 'version_conflict') : undefined
    await userEvent.click(screen.getByRole('button', { name: 'Хабарлама жіберу' }))
    await screen.findByText('Өтінім өзгертілген. Енгізілген деректер сақталды. Соңғы нұсқаны қарап, қайта жүктеу туралы шешім қабылдаңыз.')
    expect(JSON.parse(String(calls('/comments')[0][1]?.body))).toMatchObject({ version: 4, body: 'Сақталмаған хабарлама' })
    expect(screen.getByRole('button', { name: 'Хабарлама жіберу' })).toBeDisabled()
    await userEvent.click(screen.getAllByRole('button', { name: 'Соңғы нұсқаны жүктеу' })[0])
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Бас тарту' }))
    expect(screen.getByLabelText('Хабарлама мәтіні')).toHaveValue('Сақталмаған хабарлама')
    await userEvent.click(screen.getAllByRole('button', { name: 'Соңғы нұсқаны жүктеу' })[0])
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Соңғы нұсқаны жүктеу' }))
    await waitFor(() => expect(screen.getByLabelText('Хабарлама мәтіні')).toHaveValue(''))
    expect(screen.getByLabelText('Басымдық')).toHaveValue('LOW')
    expect(screen.getByRole('button', { name: 'Хабарлама жіберу' })).toBeEnabled()
  })
  it('allows internal messages and adds uploaded screenshots with the current ticket version', async () => {
    identity = engineer; renderPage('/tickets/ticket-id'); await screen.findByRole('heading', { name: example.title })
    await userEvent.type(screen.getByLabelText('Хабарлама мәтіні'), 'Қосалқы принтер тексерілді.')
    await userEvent.click(screen.getByLabelText('Ішкі хабарлама (тек IT мамандары)'))
    await userEvent.click(screen.getByRole('button', { name: 'Хабарлама жіберу' }))
    await screen.findByText('Қосалқы принтер тексерілді.')
    expect(JSON.parse(String(calls('/comments')[0][1]?.body))).toMatchObject({ is_internal: true, version: 4 })
    await userEvent.upload(screen.getByLabelText('Скриншот қосу'), new File(['test'], 'screen.png', { type: 'image/png' }))
    await screen.findByRole('img', { name: 'Скриншоттар 1' })
    await userEvent.click(screen.getByRole('button', { name: 'Скриншоттарды сақтау' }))
    await waitFor(() => expect(calls('/attachments')).toHaveLength(1))
    expect(JSON.parse(String(calls('/attachments')[0][1]?.body))).toEqual({ version: 5, attachment_ids: ['image-id'] })
  })
  it('keeps closed tickets immutable and requires an administrator reopen reason', async () => {
    identity = { ...engineer, role: 'ADMIN' }; ticket = { ...ticket, status: 'CLOSED' }
    renderPage('/tickets/ticket-id'); await screen.findByRole('heading', { name: example.title })
    expect(screen.queryByLabelText('Хабарлама мәтіні')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Скриншот қосу')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Өтінімді қайта ашу' }))
    expect(await screen.findByText('Себебін көрсетіңіз (2000 таңбаға дейін).')).toBeInTheDocument()
    expect(calls('/reopen')).toHaveLength(0)
    await userEvent.type(screen.getByLabelText('Өзгерту себебі'), 'Мәселе қайталанды')
    await userEvent.click(screen.getByRole('button', { name: 'Өтінімді қайта ашу' }))
    await screen.findByLabelText('Хабарлама мәтіні')
    expect(JSON.parse(String(calls('/reopen')[0][1]?.body))).toEqual({ version: 4, reason: 'Мәселе қайталанды' })
  })
  it('converts a resolved ticket into a draft with reviewed fields and then opens the article editor', async () => {
    identity = engineer; ticket = { ...ticket, status: 'RESOLVED', resolved_at: example.updated_at }
    renderPage('/engineer/knowledge/new?ticket_id=ticket-id')
    await screen.findByLabelText('Мақала атауы')
    expect(screen.getByLabelText('Мақала атауы')).toHaveValue(ticket.title)
    expect(screen.queryByLabelText('Қызметкерлерге жариялау')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Сурет жүктеу')).not.toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Мекенжай атауы (slug)'), 'printer-fix')
    await userEvent.type(screen.getByLabelText('Қадам 1'), 'Қуат сымын ауыстырыңыз.')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByRole('heading', { name: 'Мақаланы өзгерту' })
    const body = JSON.parse(String(calls('/article', 'POST')[0][1]?.body))
    expect(body).toMatchObject({ version: 4, slug: 'printer-fix', title: example.title, solution_steps: ['Қуат сымын ауыстырыңыз.'], tags: [] })
    expect(body).not.toHaveProperty('is_published'); expect(body).not.toHaveProperty('image_ids')
  })
  it('retains unsaved conversion fields and its original version when a ticket is reopened in the background', async () => {
    identity = engineer; ticket = { ...ticket, status: 'RESOLVED' }
    const { client } = renderPage('/engineer/knowledge/new?ticket_id=ticket-id')
    await screen.findByLabelText('Мақала атауы')
    await userEvent.type(screen.getByLabelText('Мекенжай атауы (slug)'), 'printer-fix')
    await userEvent.type(screen.getByLabelText('Қадам 1'), 'Сақталмаған шешім')
    ticket = { ...ticket, status: 'IN_PROGRESS', version: 5 }
    await act(async () => { await client.invalidateQueries({ queryKey: ['tickets', 'detail'] }) })
    expect(screen.getByLabelText('Қадам 1')).toHaveValue('Сақталмаған шешім')
    override = path => path === '/api/v1/tickets/ticket-id/article' ? failure(409, 'version_conflict') : undefined
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByRole('link', { name: 'Соңғы нұсқаны ашу' })
    expect(JSON.parse(String(calls('/article', 'POST')[0][1]?.body)).version).toBe(4)
    expect(screen.getByLabelText('Қадам 1')).toHaveValue('Сақталмаған шешім')
    expect(screen.getByRole('button', { name: 'Сақтау' })).toBeDisabled()
  })
  it('blocks employee conversion before fetching ticket data and clears staff identity caches after role changes', async () => {
    const first = renderPage('/engineer/knowledge/new?ticket_id=ticket-id')
    await screen.findByText('Бұл бетті IT маманы немесе әкімші ғана аша алады.')
    expect(calls('/tickets/ticket-id')).toHaveLength(0)
    first.unmount()
    identity = engineer; const second = renderPage('/tickets/ticket-id')
    await screen.findByLabelText('Ішкі хабарлама (тек IT мамандары)')
    await userEvent.type(screen.getByLabelText('Хабарлама мәтіні'), 'Private unsaved staff draft')
    identity = employee
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await waitFor(() => expect(screen.queryByLabelText('Ішкі хабарлама (тек IT мамандары)')).not.toBeInTheDocument())
    expect(screen.getByLabelText('Хабарлама мәтіні')).toHaveValue('')
    expect(second.client.getQueryData(['tickets', 'assignees'])).toBeUndefined()
  })
})

describe('ticket reporting export', () => {
  it.each(['IT_ENGINEER', 'ADMIN'] as const)('exports all applied filters without pagination for %s', async role => {
    identity = { ...engineer, role }; const { create, click } = mockDownload()
    renderPage('/tickets?q=printer&status=NEW&priority=HIGH&category_id=category-id&department_id=department-id&assignee_id=engineer-id&mine=true&from=2026-10-01&to=2026-10-10&timezone=UTC&page=2')
    await screen.findByRole('link', { name: example.title })
    expect(screen.getByText(/Экспортталатын өтінімдер: 45/)).toHaveTextContent('Бөлім: Тест бөлімі · Орындаушы: IT Тест')
    await userEvent.type(screen.getByRole('textbox', { name: 'Өтінімдерді іздеу' }), ' unsent draft')
    fireEvent.change(screen.getByLabelText('Басталу күні'), { target: { value: '2026-10-02' } })
    await userEvent.click(screen.getByRole('button', { name: 'Excel жүктеу' }))
    await waitFor(() => expect(click).toHaveBeenCalledOnce())
    const filters = new URL(String(calls('/tickets/export?')[0][0]), 'https://example.test').searchParams
    expect(Object.fromEntries(filters)).toEqual({ format: 'xlsx', locale: 'kk', q: 'printer', status: 'NEW', priority: 'HIGH', category_id: 'category-id', department_id: 'department-id', assignee_id: 'engineer-id', mine: 'true', from: '2026-10-01', to: '2026-10-10', timezone: 'UTC' })
    expect(filters.has('offset')).toBe(false); expect(filters.has('limit')).toBe(false); expect(filters.has('page')).toBe(false)
    expect(create).toHaveBeenCalledOnce()
    await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
    await userEvent.click(screen.getByRole('button', { name: 'Скачать CSV' }))
    await waitFor(() => expect(calls('/tickets/export?')).toHaveLength(2))
    expect(calls('/tickets/export?')[1][0]).toContain('format=csv&locale=ru')
  })
  it('preserves all-time queries and validates optional date ranges before applying and resetting the page', async () => {
    identity = engineer; renderPage('/tickets?page=2')
    await screen.findByRole('link', { name: example.title })
    expect(calls('/tickets?')[0][0]).toBe('/api/v1/tickets?limit=20&offset=40')
    expect(screen.getByLabelText('Басталу күні')).toHaveValue('')
    expect(screen.getByLabelText('Аяқталу күні')).toHaveValue('')
    fireEvent.change(screen.getByLabelText('Басталу күні'), { target: { value: '2026-10-01' } })
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    expect(await screen.findByText('Дұрыс күндер мен уақыт белдеуін көрсетіңіз. Кезең 1–366 күн болуы керек.')).toBeInTheDocument()
    expect(calls('/tickets?')).toHaveLength(1)
    fireEvent.change(screen.getByLabelText('Аяқталу күні'), { target: { value: '2027-10-02' } })
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    expect(calls('/tickets?')).toHaveLength(1)
    fireEvent.change(screen.getByLabelText('Аяқталу күні'), { target: { value: '2026-10-10' } })
    fireEvent.change(screen.getByLabelText('Есептің уақыт белдеуі'), { target: { value: 'Invalid/Zone' } })
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    expect(calls('/tickets?')).toHaveLength(1)
    fireEvent.change(screen.getByLabelText('Есептің уақыт белдеуі'), { target: { value: 'UTC' } })
    await userEvent.click(screen.getByRole('button', { name: 'Сүзгілерді қолдану' }))
    await waitFor(() => expect(calls('/tickets?')).toHaveLength(2))
    expect(calls('/tickets?')[1][0]).toBe('/api/v1/tickets?from=2026-10-01&to=2026-10-10&timezone=UTC&limit=20&offset=0')
    await userEvent.click(screen.getByRole('button', { name: 'Іздеуді тазалау' }))
    await waitFor(() => expect(calls('/tickets?').at(-1)?.[0]).toBe('/api/v1/tickets?limit=20&offset=0'))
  })
  it('explains oversized exports safely in both languages', async () => {
    identity = engineer; const { create } = mockDownload()
    override = path => path.startsWith('/api/v1/tickets/export?') ? failure(409, 'export_limit') : undefined
    renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'CSV жүктеу' }))
    await screen.findByText('Есеп тым үлкен (шегі: 5000 өтінім немесе 20 МБ). Кезеңді немесе басқа сүзгілерді тарылтып, қайта жүктеңіз.')
    expect(screen.queryByText('private server detail')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
    expect(screen.getByText('Отчёт слишком большой (лимит: 5000 заявок или 20 МБ). Сузьте период или другие фильтры и скачайте повторно.')).toBeInTheDocument()
    expect(create).not.toHaveBeenCalled()
  })
  it('exports an empty result without disabling the report button or inventing a count', async () => {
    identity = engineer; const { click } = mockDownload()
    override = path => path.startsWith('/api/v1/tickets?') ? json({ items: [], total: 0, limit: 20, offset: 0 }) : undefined
    renderPage('/tickets'); await screen.findByText('Өтінім табылмады. Сүзгілерді өзгертіңіз немесе жаңа өтінім жіберіңіз.')
    expect(screen.getByText(/Экспортталатын өтінімдер: 0/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Excel жүктеу' }))
    await waitFor(() => expect(click).toHaveBeenCalledOnce())
  })
  it('allows only one export at a time and aborts it when the page unmounts', async () => {
    identity = engineer; const { create } = mockDownload()
    let complete!: (response: Response) => void
    override = path => path.startsWith('/api/v1/tickets/export?') ? new Promise<Response>(done => { complete = done }) : undefined
    const view = renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'Excel жүктеу' }))
    expect(screen.getByRole('button', { name: 'Excel жүктеу' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'CSV жүктеу' })).toBeDisabled()
    const signal = calls('/tickets/export?')[0][1]?.signal
    view.unmount(); expect(signal?.aborted).toBe(true)
    await act(async () => { complete(new Response('synthetic report', { headers: { 'Content-Type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' } })) })
    expect(create).not.toHaveBeenCalled()
  })
  it('cancels an in-flight export when applied filters are changed', async () => {
    identity = engineer; const { create } = mockDownload()
    let complete!: (response: Response) => void
    override = path => path.startsWith('/api/v1/tickets/export?') ? new Promise<Response>(done => { complete = done }) : undefined
    renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'CSV жүктеу' }))
    const signal = calls('/tickets/export?')[0][1]?.signal
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Күйі' }), 'NEW')
    await waitFor(() => expect(screen.getByRole('button', { name: 'CSV жүктеу' })).toBeEnabled())
    expect(signal?.aborted).toBe(true)
    await act(async () => { complete(new Response('synthetic report', { headers: { 'Content-Type': 'text/csv' } })) })
    expect(create).not.toHaveBeenCalled()
    expect(screen.queryByText('Есеп дайындалуда…')).not.toBeInTheDocument()
  })
  it.each(['response', 'blob'] as const)('discards an old export %s when the identity changes', async delayed => {
    identity = engineer; const { create } = mockDownload()
    let completeResponse!: (response: Response) => void, completeBlob!: (blob: Blob) => void
    const response = new Response('synthetic report', { headers: { 'Content-Type': 'text/csv' } })
    if (delayed === 'blob') Object.defineProperty(response, 'blob', { value: () => new Promise<Blob>(done => { completeBlob = done }) })
    override = path => path.startsWith('/api/v1/tickets/export?') ? delayed === 'response' ? new Promise<Response>(done => { completeResponse = done }) : response : undefined
    renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'CSV жүктеу' }))
    identity = employee
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await waitFor(() => expect(screen.queryByRole('button', { name: 'CSV жүктеу' })).not.toBeInTheDocument())
    await act(async () => { if (delayed === 'response') completeResponse(response); else completeBlob(new Blob(['synthetic report'])) })
    expect(create).not.toHaveBeenCalled()
    expect(screen.queryByRole('heading', { name: 'MedDesk жүйесіне кіру' })).not.toBeInTheDocument()
  })
  it('does not log out a newer identity when an older export fails with 401', async () => {
    identity = engineer; mockDownload()
    let complete!: (response: Response) => void
    override = path => path.startsWith('/api/v1/tickets/export?') ? new Promise<Response>(done => { complete = done }) : undefined
    renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'CSV жүктеу' }))
    identity = { ...engineer, id: 'another-engineer', display_name: 'Басқа маман' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await screen.findByRole('link', { name: example.title })
    await act(async () => { complete(failure(401, 'unauthenticated')) })
    expect(screen.getByRole('button', { name: 'CSV жүктеу' })).toBeEnabled()
    expect(screen.queryByRole('heading', { name: 'MedDesk жүйесіне кіру' })).not.toBeInTheDocument()
  })
  it('expires only the current session after an export 401', async () => {
    identity = engineer; mockDownload()
    override = path => path.startsWith('/api/v1/tickets/export?') ? failure(401, 'unauthenticated') : undefined
    const { client } = renderPage('/tickets'); await screen.findByRole('link', { name: example.title })
    client.setQueryData(['private-test'], 'synthetic private data')
    await userEvent.click(screen.getByRole('button', { name: 'CSV жүктеу' }))
    await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })
    expect(client.getQueryData(['private-test'])).toBeUndefined()
  })
})

it('accepts only nonempty PNG/JPEG ticket screenshots up to 10 MiB', () => {
  expect(validMediaFile(new File(['test'], 'screen.png', { type: 'image/png' }), 'TICKET_ATTACHMENT')).toBe(true)
  expect(validMediaFile(new File(['test'], 'screen.jpg', { type: 'image/jpeg' }), 'TICKET_ATTACHMENT')).toBe(true)
  expect(validMediaFile(new File(['test'], 'screen.gif', { type: 'image/gif' }), 'TICKET_ATTACHMENT')).toBe(false)
  expect(validMediaFile(new File([], 'empty.png', { type: 'image/png' }), 'TICKET_ATTACHMENT')).toBe(false)
  expect(validMediaFile(new File([new Uint8Array(imageMaxBytes + 1)], 'large.png', { type: 'image/png' }), 'TICKET_ATTACHMENT')).toBe(false)
})

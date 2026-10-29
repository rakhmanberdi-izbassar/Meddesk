import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, expect, it, vi } from 'vitest'
import { App } from '../App'
import { LocaleProvider } from '../lib/locale'
const now = '2026-10-29T08:00:00Z'
const ticket = { id: '00000000-0000-4000-8000-000000000001', number: 1, title: 'Камера көрсетпейді', description: 'Дәліздегі камера', requester: 'Мұғалім', room: 'Дәліз', category: 'SURVEILLANCE', priority: 'HIGH', status: 'NEW', lesson_blocked: true, asset_id: null, resolution: '', version: 1, created_at: now, updated_at: now, resolved_at: null }
const reply = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const device = { id: '00000000-0000-4000-8000-000000000002', name: 'Камера 101', kind: 'CAMERA', location: '101', model: '', inventory_number: 'CAM-101', ip_address: '192.168.1.10', notes: '', version: 1, created_at: now, updated_at: now }
let schoolEmployee = false
let withDevices = false
let allowed = true
beforeEach(() => {
 allowed = true; withDevices = false; schoolEmployee = false
 vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
  const path = String(url)
  if (path === '/api/v1/auth/me') return reply({ user: { id: 'owner-id', email: 'owner@example.test', display_name: 'Owner', role: schoolEmployee ? 'EMPLOYEE' : 'ADMIN', account_scope: schoolEmployee ? 'SCHOOL' : 'CLINIC', department_id: 'dept', department_name: 'IT', is_active: true }, csrf_token: 'synthetic-csrf-token-12345' })
  if (path.startsWith('/api/v1/school/requests?')) return reply({ items: [{ ...ticket, public_reply: 'Жөнделді' }], total: 1, limit: 20, offset: 0 })
  if (path === '/api/v1/school/requests' && init?.method === 'POST') return reply({ ...ticket, ...JSON.parse(String(init.body)), public_reply: '' }, 201)
  if (path.startsWith('/api/v1/school/members?')) return reply({ items: [], total: 0, limit: 20, offset: 0 })
  if (path === '/api/v1/school/members' && init?.method === 'POST') return reply({ id: 'member-id', ...JSON.parse(String(init.body)), is_active: true, version: 1 }, 201)
  if (path === '/api/v1/school/access') return reply({ allowed })
  if (path.startsWith('/api/v1/school/tickets?')) return reply({ items: [ticket], total: 1, limit: 20, offset: 0 })
  if (path === `/api/v1/school/assets/${device.id}`) return reply(device)
  if (path.startsWith('/api/v1/school/assets?')) return reply({ items: withDevices ? [device] : [], total: withDevices ? 1 : 0, limit: 20, offset: 0 })
  if (path === '/api/v1/school/tickets' && init?.method === 'POST') return reply({ ...ticket, ...JSON.parse(String(init.body)) }, 201)
  return reply({ error: { code: 'not_found' } }, 404)
 }))
})
function page(path = '/school') { return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })}><LocaleProvider><MemoryRouter initialEntries={[path]}><App /></MemoryRouter></LocaleProvider></QueryClientProvider>) }
it('denies a clinic administrator without explicit school ownership', async () => {
 allowed = false;page()
 await screen.findByText('Мектеп кеңістігі тек оның иесіне қолжетімді.')
 expect(screen.queryByRole('link', { name: 'Мектеп · жеке кеңістік' })).not.toBeInTheDocument()
 expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).startsWith('/api/v1/school/tickets'))).toBe(false)
})
it('renders school surveillance requests and switches category language', async () => {
 page();await screen.findByText(/SCH-1/)
 expect(screen.getByText('Сабаққа кедергі келтіріп тұр')).toBeInTheDocument()
 expect(screen.getByRole('link', { name: 'Мектеп · жеке кеңістік' })).toBeInTheDocument()
 await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
 expect(screen.getByRole('heading', { name: 'Школа · IT-служба' })).toBeInTheDocument()
 expect(screen.getByText(/Видеонаблюдение/)).toBeInTheDocument()
})
it('validates and submits a school request with CSRF using the private endpoint', async () => {
 page();await screen.findByText(/SCH-1/)
 await userEvent.click(screen.getByRole('button', { name: 'Қосу' }))
 await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
 expect(screen.getByRole('alert')).toBeInTheDocument()
 expect(vi.mocked(fetch).mock.calls.some(([,init]) => init?.method === 'POST')).toBe(false)
 await userEvent.type(screen.getByRole('textbox', { name: 'Мәселе' }), 'Wi-Fi істемейді')
 await userEvent.type(screen.getByRole('textbox', { name: 'Сипаттамасы' }), 'Бірінші қабатта желі жоқ')
 await userEvent.type(screen.getByRole('textbox', { name: 'Хабарлаған қызметкер' }), 'Мұғалім')
 await userEvent.type(screen.getByRole('textbox', { name: 'Кабинет / орын' }), '101')
 await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Санаты' }), 'NETWORK')
 await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
 await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url,init]) => url === '/api/v1/school/tickets' && init?.method === 'POST')).toBe(true))
 const [,init] = vi.mocked(fetch).mock.calls.find(([,init]) => init?.method === 'POST')!
 expect(JSON.parse(String(init?.body))).toMatchObject({ category: 'NETWORK', requester: 'Мұғалім', room: '101' })
 expect(init?.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token-12345' })
})

it('links a device selected by its human-readable name', async () => {
 withDevices = true;page();await screen.findByText(/SCH-1/)
 await userEvent.click(screen.getByRole('button', { name: 'Қосу' }))
 await screen.findByRole('option', { name: 'Камера 101 · 101 · CAM-101' })
 await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Құрылғы (міндетті емес)' }), device.id)
 await userEvent.type(screen.getByRole('textbox', { name: 'Мәселе' }), 'Камера істемейді')
 await userEvent.type(screen.getByRole('textbox', { name: 'Сипаттамасы' }), 'Кескін жоқ')
 await userEvent.type(screen.getByRole('textbox', { name: 'Хабарлаған қызметкер' }), 'Мұғалім')
 await userEvent.type(screen.getByRole('textbox', { name: 'Кабинет / орын' }), '101')
 await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
 await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url,init]) => url === '/api/v1/school/tickets' && init?.method === 'POST')).toBe(true))
 const [,init] = vi.mocked(fetch).mock.calls.find(([,init]) => init?.method === 'POST')!
 expect(JSON.parse(String(init?.body))).toMatchObject({ asset_id: device.id })
})

it('routes a school employee away from clinic pages without fetching clinic resources', async () => {
 schoolEmployee = true;page('/knowledge')
 await screen.findByRole('heading', { name: 'Мектеп · менің өтінімдерім' })
 expect(await screen.findByText('Жөнделді')).toBeInTheDocument()
 expect(screen.queryByRole('button', { name: 'Қызметкерлер' })).not.toBeInTheDocument()
 expect(screen.queryByRole('button', { name: 'Құрылғылар' })).not.toBeInTheDocument()
 expect(screen.queryByRole('link', { name: 'Емхана кеңістігіне оралу' })).not.toBeInTheDocument()
 expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).startsWith('/api/v1/knowledge') || url === '/api/v1/settings' || String(url).startsWith('/api/v1/school/assets'))).toBe(false)
})
it('lets a school employee submit an identity-bound request without requester or device fields', async () => {
 schoolEmployee = true;page()
 await screen.findByRole('heading', { name: 'Мектеп · менің өтінімдерім' })
 await userEvent.click(screen.getByRole('button', { name: 'Өтінім жіберу' }))
 await userEvent.type(screen.getByRole('textbox', { name: 'Мәселе' }), 'Интернет жоқ')
 await userEvent.type(screen.getByRole('textbox', { name: 'Сипаттамасы' }), '101 кабинетте байланыс жоқ')
 await userEvent.type(screen.getByRole('textbox', { name: 'Кабинет / орын' }), '101')
 expect(screen.queryByRole('textbox', { name: 'Хабарлаған қызметкер' })).not.toBeInTheDocument()
 await userEvent.click(screen.getByRole('button', { name: 'Жіберу' }))
 await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url,init]) => url === '/api/v1/school/requests' && init?.method === 'POST')).toBe(true))
 const [,init] = vi.mocked(fetch).mock.calls.find(([url,init]) => url === '/api/v1/school/requests' && init?.method === 'POST')!
 const data = JSON.parse(String(init?.body))
 expect(data).toMatchObject({ title: 'Интернет жоқ', room: '101' })
 expect(data).not.toHaveProperty('requester_id');expect(data).not.toHaveProperty('asset_id')
})
it('lets the owner create a school account with the private members endpoint', async () => {
 page();await screen.findByText(/SCH-1/)
 await userEvent.click(screen.getByRole('button', { name: 'Қызметкерлер' }))
 await userEvent.click(await screen.findByRole('button', { name: 'Қызметкер қосу' }))
 await userEvent.type(screen.getByRole('textbox', { name: 'Аты-жөні' }), 'Тест мұғалім')
 await userEvent.type(screen.getByRole('textbox', { name: 'Email' }), 'teacher@example.test')
 await userEvent.type(screen.getByLabelText('Құпиясөз', { exact: true }), 'Synthetic-password-2026')
 await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
 await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url,init]) => url === '/api/v1/school/members' && init?.method === 'POST')).toBe(true))
 expect(vi.mocked(fetch).mock.calls.some(([url,init]) => url === '/api/v1/users' && init?.method === 'POST')).toBe(false)
})

import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'
import { LocaleProvider, localeStorageKey } from '../lib/locale'
import type { Department, Session, User } from '../lib/api'

const employee: User = { id: 'employee-id', email: 'employee@example.test', display_name: 'Айым Тест', role: 'EMPLOYEE', department_id: 'department-id', department_name: 'Тест бөлімі', is_active: true }
const admin: User = { ...employee, id: 'admin-id', display_name: 'Әкімші Тест', role: 'ADMIN', email: 'admin@example.test' }
const department: Department = { id: 'department-id', name: 'Тест бөлімі', is_active: true, created_at: '2026-10-09T00:00:00Z', updated_at: '2026-10-09T00:00:00Z' }
const token = 'synthetic-test-csrf-token-12345'
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const failure = (status: number, code: string) => json({ error: { code, message: 'private server detail must never appear' } }, status)
let session: Session | null
let users: User[]
let override: ((path: string, init?: RequestInit) => Response | Promise<Response> | undefined) | undefined

function mockServer() {
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url), result = override?.(path, init)
    if (result) return result
    if (path === '/api/v1/auth/me') return session ? json(session) : failure(401, 'unauthenticated')
    if (path === '/api/v1/auth/login') { session = { user: employee, csrf_token: token }; return json(session) }
    if (path === '/api/v1/auth/logout') { session = null; return new Response(null, { status: 204 }) }
    if (path === '/api/v1/health') return json({ status: 'ok' })
    if (path === '/api/v1/ready') return json({ status: 'ready' })
    if (path === '/api/v1/profile' && init?.method === 'PATCH') {
      session = { user: { ...session!.user, ...JSON.parse(String(init.body)) }, csrf_token: token }
      return json({ user: session.user })
    }
    if (path.startsWith('/api/v1/users?')) {
      const p = new URL(path, 'http://meddesk.test').searchParams, offset = Number(p.get('offset')), limit = Number(p.get('limit'))
      return json({ items: users.slice(offset, offset + limit), total: users.length, limit, offset })
    }
    if (path.startsWith('/api/v1/departments?')) return json({ items: [department], total: 1, limit: 100, offset: 0 })
    if (path === '/api/v1/departments' && init?.method === 'POST') return json({ department: { ...department, name: JSON.parse(String(init.body)).name } }, 201)
    if (path === '/api/v1/users' && init?.method === 'POST') return json({ user: employee }, 201)
    if (path === '/api/v1/users/employee-id' && init?.method === 'PATCH') return json({ user: { ...employee, ...JSON.parse(String(init.body)) } })
    return failure(404, 'not_found')
  }))
}
function renderApp(path = '/') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
  render(<QueryClientProvider client={client}><LocaleProvider><MemoryRouter initialEntries={[path]}><App /></MemoryRouter></LocaleProvider></QueryClientProvider>)
  return client
}
beforeEach(() => { session = { user: employee, csrf_token: token }; users = [employee, admin]; override = undefined; localStorage.clear(); mockServer() })

describe('identity and management', () => {
  it('bootstraps real identity, Kazakh locale, actual operational states and completed workflows', async () => {
    renderApp()
    expect(await screen.findByRole('heading', { name: 'Қош келдіңіз, Айым Тест.' })).toBeInTheDocument()
    expect(document.documentElement.lang).toBe('kk')
    expect(await screen.findByText('Дерекқор қосылған, схема дайын.')).toBeInTheDocument()
    expect(screen.queryByText('Жоспарланған')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Қолдау өтінімдері' })).toHaveAttribute('href', '/tickets')
    expect(screen.getByRole('link', { name: 'Жаңа өтінім' })).toHaveAttribute('href', '/tickets/new')
    expect(screen.queryByRole('link', { name: 'Қызметкерлер' })).not.toBeInTheDocument()
    expect(localStorage.getItem('csrf_token')).toBeNull()
  })
  it('protects private pages and renders a credential form when anonymous', async () => {
    session = null; renderApp('/profile')
    expect(await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })).toBeInTheDocument()
    expect(screen.getByLabelText('Электрондық пошта')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Менің профилім' })).not.toBeInTheDocument()
  })
  it('does not expose admin data while bootstrap is pending', async () => {
    let resolve!: (value: Response) => void
    override = path => path === '/api/v1/auth/me' ? new Promise<Response>(done => { resolve = done }) : undefined
    renderApp('/admin/users')
    expect(screen.getByText('Есептік жазба тексерілуде…')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    resolve(failure(401, 'unauthenticated'))
    expect(await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })).toBeInTheDocument()
  })
  it('validates login and sends the exact password without trimming or CSRF', async () => {
    session = null; renderApp('/login')
    await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })
    await userEvent.click(screen.getByRole('button', { name: 'Кіру' }))
    expect(await screen.findByText('Дұрыс электрондық пошта мекенжайын енгізіңіз.')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Электрондық пошта'), employee.email)
    await userEvent.type(screen.getByLabelText('Құпиясөз', { exact: true }), ' synthetic password ')
    await userEvent.click(screen.getByRole('button', { name: 'Кіру' }))
    expect(await screen.findByRole('heading', { name: 'Қош келдіңіз, Айым Тест.' })).toBeInTheDocument()
    const call = vi.mocked(fetch).mock.calls.find(([path]) => path === '/api/v1/auth/login')!
    expect(call[1]).toMatchObject({ method: 'POST', credentials: 'same-origin', body: JSON.stringify({ email: employee.email, password: ' synthetic password ' }) })
    expect((call[1]?.headers as Record<string, string>)['X-CSRF-Token']).toBeUndefined()
  })
  it('translates failed login safely, including switching locale after the error', async () => {
    session = null; override = path => path === '/api/v1/auth/login' ? failure(401, 'invalid_credentials') : undefined
    renderApp('/login'); await screen.findByLabelText('Электрондық пошта')
    await userEvent.type(screen.getByLabelText('Электрондық пошта'), employee.email)
    await userEvent.type(screen.getByLabelText('Құпиясөз', { exact: true }), 'synthetic-password')
    await userEvent.click(screen.getByRole('button', { name: 'Кіру' }))
    expect(await screen.findByText('Пошта немесе құпиясөз қате.')).toBeInTheDocument()
    expect(screen.queryByText(/private server detail/)).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
    expect(screen.getByText('Неверная почта или пароль.')).toBeInTheDocument()
    expect(localStorage.getItem(localeStorageKey)).toBe('ru')
  })
  it('restores Russian locale on the login page', async () => {
    session = null; localStorage.setItem(localeStorageKey, 'ru'); renderApp('/login')
    expect(await screen.findByRole('heading', { name: 'Вход в MedDesk' })).toBeInTheDocument()
    expect(document.documentElement.lang).toBe('ru')
  })
  it('saves profile with memory CSRF and refreshes displayed identity', async () => {
    renderApp('/profile'); const input = await screen.findByLabelText('Аты-жөні')
    await userEvent.clear(input); await userEvent.type(input, 'Жаңа Аты')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    expect(await screen.findByText('Профиль жаңартылды.')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Жаңа Аты' })).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/profile', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ display_name: 'Жаңа Аты' }), headers: expect.objectContaining({ 'X-CSRF-Token': token }) }))
    expect(Object.keys(localStorage)).toEqual([localeStorageKey])
  })
  it('clears cached private data and redirects to login on any protected 401', async () => {
    override = path => path === '/api/v1/profile' ? failure(401, 'unauthenticated') : undefined
    const client = renderApp('/profile'), input = await screen.findByLabelText('Аты-жөні')
    client.setQueryData(['private', 'users'], ['synthetic private data'])
    await userEvent.type(input, ' Жаңарту'); await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    expect(await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })).toBeInTheDocument()
    expect(client.getQueryData(['private', 'users'])).toBeUndefined()
    expect(screen.queryByText(employee.email)).not.toBeInTheDocument()
  })
  it('logs out using CSRF and removes private navigation', async () => {
    renderApp(); await screen.findByRole('heading', { name: 'Қош келдіңіз, Айым Тест.' })
    await userEvent.click(screen.getByRole('button', { name: 'Шығу' }))
    expect(await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/auth/logout', expect.objectContaining({ method: 'POST', headers: expect.objectContaining({ 'X-CSRF-Token': token }) }))
  })
  it('prevents employee access to admin pages without fetching admin data', async () => {
    renderApp('/admin/users')
    expect(await screen.findByRole('heading', { name: 'Қолжетімділік шектеулі' })).toBeInTheDocument()
    expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path).startsWith('/api/v1/users'))).toBe(false)
  })
  it('sends admin user filters and pagination to the API', async () => {
    session = { user: admin, csrf_token: token }
    users = Array.from({ length: 21 }, (_, n) => ({ ...employee, id: `employee-${n}`, display_name: `Қызметкер ${n}` }))
    renderApp('/admin/users'); await screen.findByText('Қызметкер 0')
    await userEvent.click(screen.getByRole('button', { name: 'Келесі' })); await screen.findByText('Қызметкер 20')
    expect(fetch).toHaveBeenCalledWith('/api/v1/users?limit=20&offset=20', expect.anything())
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Рөл' }), 'IT_ENGINEER')
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/v1/users?role=IT_ENGINEER&limit=20&offset=0', expect.anything()))
  })
  it('creates departments with accessible forms and CSRF', async () => {
    session = { user: admin, csrf_token: token }; renderApp('/admin/departments'); await screen.findByText(department.name)
    await userEvent.click(screen.getByRole('button', { name: 'Бөлім қосу' }))
    const dialog = screen.getByRole('dialog', { name: 'Бөлім қосу' })
    await userEvent.type(within(dialog).getByLabelText('Бөлім атауы'), 'Жаңа бөлім')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Қосу' }))
    expect(await screen.findByText('Бөлім сақталды.')).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/departments', expect.objectContaining({ method: 'POST', body: JSON.stringify({ name: 'Жаңа бөлім' }), headers: expect.objectContaining({ 'X-CSRF-Token': token }) }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('requires confirmation before changing a role and revoking sessions', async () => {
    session = { user: admin, csrf_token: token }; renderApp('/admin/users'); await screen.findByText(employee.email)
    await userEvent.click(screen.getByRole('button', { name: 'Өзгерту: Айым Тест' }))
    const dialog = screen.getByRole('dialog', { name: 'Қызметкерді өзгерту' })
    await userEvent.selectOptions(within(dialog).getByLabelText('Рөл'), 'IT_ENGINEER')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Сақтау' }))
    const confirmation = screen.getByRole('dialog', { name: 'Өзгерістерді растаңыз' })
    expect(vi.mocked(fetch).mock.calls.some(([path, init]) => path === '/api/v1/users/employee-id' && init?.method === 'PATCH')).toBe(false)
    await userEvent.click(within(confirmation).getByRole('button', { name: 'Растау' }))
    expect(await screen.findByText('Қызметкер сақталды.')).toBeInTheDocument()
    const call = vi.mocked(fetch).mock.calls.find(([path, init]) => path === '/api/v1/users/employee-id' && init?.method === 'PATCH')!
    expect(JSON.parse(String(call[1]?.body))).toMatchObject({ role: 'IT_ENGINEER', is_active: true })
  })
  it('confirms department deactivation and displays an active-user conflict', async () => {
    session = { user: admin, csrf_token: token }
    override = (path, init) => path === '/api/v1/departments/department-id' && init?.method === 'PATCH' ? failure(409, 'department_has_active_users') : undefined
    renderApp('/admin/departments'); await screen.findByText(department.name)
    await userEvent.click(screen.getByRole('button', { name: 'Өзгерту: Тест бөлімі' }))
    const dialog = screen.getByRole('dialog', { name: 'Бөлімді өзгерту' })
    await userEvent.click(within(dialog).getByLabelText('Белсенді'))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Сақтау' }))
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Өзгерістерді растаңыз' })).getByRole('button', { name: 'Растау' }))
    expect(await screen.findByText('Бұл бөлімде белсенді қызметкерлер бар. Алдымен оларды басқа бөлімге ауыстырыңыз.')).toBeInTheDocument()
    expect(screen.getByRole('dialog', { name: 'Бөлімді өзгерту' })).toBeInTheDocument()
  })
  it('supports accessible mobile navigation and closes it after navigating', async () => {
    renderApp(); await screen.findByRole('heading', { name: 'Қош келдіңіз, Айым Тест.' })
    await userEvent.click(screen.getByRole('button', { name: 'Мәзірді ашу' }))
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Негізгі навигация' })).getByRole('link', { name: 'Менің профилім' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Менің профилім' })).toBeInTheDocument()
  })
  it('clears private cache and resets an unsaved form after a cross-tab account switch', async () => {
    const client = renderApp('/profile'), input = await screen.findByLabelText('Аты-жөні')
    await userEvent.clear(input); await userEvent.type(input, 'Ескі өзгеріс')
    client.setQueryData(['private', 'users'], ['account-one'])
    session = { user: admin, csrf_token: token + '-new' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    expect(await screen.findByRole('heading', { name: admin.display_name })).toBeInTheDocument()
    expect(screen.getByLabelText('Аты-жөні')).toHaveValue(admin.display_name)
    expect(client.getQueryData(['private', 'users'])).toBeUndefined()
  })
  it('removes admin data and navigation when /me returns a different role', async () => {
    session = { user: admin, csrf_token: token }
    const client = renderApp('/admin/users')
    await screen.findByText(employee.email)
    session = { user: { ...admin, role: 'EMPLOYEE' }, csrf_token: token + '-new' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    expect(await screen.findByRole('heading', { name: 'Қолжетімділік шектеулі' })).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(client.getQueryCache().findAll({ queryKey: ['admin'] })).toHaveLength(0)
    expect(screen.queryByRole('link', { name: 'Қызметкерлер' })).not.toBeInTheDocument()
  })
  it('ignores a late profile response belonging to the previous account', async () => {
    let resolve!: (value: Response) => void
    override = (path, init) => path === '/api/v1/profile' && init?.method === 'PATCH' ? new Promise<Response>(done => { resolve = done }) : undefined
    renderApp('/profile'); const input = await screen.findByLabelText('Аты-жөні')
    await userEvent.type(input, ' Жаңарту'); await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    session = { user: admin, csrf_token: token + '-new' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await screen.findByRole('heading', { name: admin.display_name })
    await act(async () => { resolve(json({ user: { ...employee, display_name: 'Ескі есептік жазба' } })) })
    expect(screen.getByRole('heading', { name: admin.display_name })).toBeInTheDocument()
    expect(screen.getByLabelText('Аты-жөні')).toHaveValue(admin.display_name)
    expect(screen.queryByText('Ескі есептік жазба')).not.toBeInTheDocument()
  })
  it('keeps private pages unavailable until a failed bootstrap is retried successfully', async () => {
    override = path => path === '/api/v1/auth/me' ? failure(503, 'not_ready') : undefined
    renderApp('/profile')
    expect(await screen.findByText('Сеансты тексеру мүмкін болмады.')).toBeInTheDocument()
    expect(screen.queryByLabelText('Аты-жөні')).not.toBeInTheDocument()
    override = undefined
    await userEvent.click(screen.getByRole('button', { name: 'Қайталау' }))
    expect(await screen.findByLabelText('Аты-жөні')).toHaveValue(employee.display_name)
  })
  it('returns keyboard focus to the opener after a modal is canceled', async () => {
    session = { user: admin, csrf_token: token }; renderApp('/admin/departments'); await screen.findByText(department.name)
    const opener = screen.getByRole('button', { name: 'Бөлім қосу' })
    await userEvent.click(opener)
    expect(within(screen.getByRole('dialog', { name: 'Бөлім қосу' })).getByLabelText('Бөлім атауы')).toHaveFocus()
    await userEvent.click(within(screen.getByRole('dialog', { name: 'Бөлім қосу' })).getByRole('button', { name: 'Бас тарту' }))
    await waitFor(() => expect(opener).toHaveFocus())
  })
  it('creates a user with a multibyte password matching the backend byte bounds', async () => {
    session = { user: admin, csrf_token: token }; renderApp('/admin/users'); await screen.findByText(employee.email)
    await userEvent.click(screen.getByRole('button', { name: 'Қызметкер қосу' }))
    const dialog = screen.getByRole('dialog', { name: 'Қызметкер қосу' })
    await within(dialog).findByRole('option', { name: department.name })
    await userEvent.type(within(dialog).getByLabelText('Аты-жөні'), 'Тест қызметкер')
    await userEvent.type(within(dialog).getByLabelText('Электрондық пошта'), 'worker@example.test')
    await userEvent.selectOptions(within(dialog).getByLabelText('Бөлім'), department.id)
    await userEvent.type(within(dialog).getByLabelText('Құпиясөз', { exact: true }), 'Қазақша')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Қосу' }))
    expect(await screen.findByText('Қызметкер сақталды.')).toBeInTheDocument()
    const call = vi.mocked(fetch).mock.calls.find(([path, init]) => path === '/api/v1/users' && init?.method === 'POST')!
    expect(JSON.parse(String(call[1]?.body))).toEqual({ display_name: 'Тест қызметкер', email: 'worker@example.test', role: 'EMPLOYEE', department_id: department.id, password: 'Қазақша' })
  })
  it('validates and confirms password resets while blocking self-admin changes', async () => {
    session = { user: admin, csrf_token: token }; renderApp('/admin/users'); await screen.findByText(employee.email)
    await userEvent.click(screen.getByRole('button', { name: 'Өзгерту: Әкімші Тест' }))
    let dialog = screen.getByRole('dialog', { name: 'Қызметкерді өзгерту' })
    expect(within(dialog).getByLabelText('Рөл')).toBeDisabled()
    expect(within(dialog).getByLabelText('Белсенді')).toBeDisabled()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Бас тарту' }))
    await userEvent.click(screen.getByRole('button', { name: 'Өзгерту: Айым Тест' }))
    dialog = screen.getByRole('dialog', { name: 'Қызметкерді өзгерту' })
    await userEvent.type(within(dialog).getByLabelText('Жаңа құпиясөз орнату'), 'short')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Сақтау' }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('12–72')
    await userEvent.clear(within(dialog).getByLabelText('Жаңа құпиясөз орнату'))
    await userEvent.type(within(dialog).getByLabelText('Жаңа құпиясөз орнату'), 'synthetic reset password')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Сақтау' }))
    expect(screen.getByRole('dialog', { name: 'Өзгерістерді растаңыз' })).toBeInTheDocument()
    expect(vi.mocked(fetch).mock.calls.some(([path, init]) => path === '/api/v1/users/employee-id' && init?.method === 'PATCH')).toBe(false)
  })
  it('loads all active department pages for user selection', async () => {
    session = { user: admin, csrf_token: token }
    override = path => {
      if (!path.startsWith('/api/v1/departments?')) return
      const offset = Number(new URL(path, 'http://meddesk.test').searchParams.get('offset'))
      return json({ items: offset === 0 ? Array.from({ length: 100 }, (_, n) => ({ ...department, id: `d-${n}`, name: `Бөлім ${n}` })) : [{ ...department, id: 'last', name: 'Соңғы бөлім' }], total: 101, limit: 100, offset })
    }
    renderApp('/admin/users'); await screen.findByText(employee.email)
    await userEvent.click(screen.getByRole('button', { name: 'Қызметкер қосу' }))
    expect(await within(screen.getByRole('dialog', { name: 'Қызметкер қосу' })).findByRole('option', { name: 'Соңғы бөлім' })).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/departments?is_active=true&limit=100&offset=100', expect.anything())
  })

  it('closes an admin editor when another account becomes active in this tab', async () => {
    session = { user: admin, csrf_token: token }; renderApp('/admin/users'); await screen.findByText(employee.email)
    await userEvent.click(screen.getByRole('button', { name: 'Өзгерту: Айым Тест' }))
    await userEvent.type(within(screen.getByRole('dialog', { name: 'Қызметкерді өзгерту' })).getByLabelText('Аты-жөні'), ' Жоба')
    session = { user: { ...admin, id: 'second-admin', display_name: 'Екінші әкімші' }, csrf_token: token + '-new' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    expect(await screen.findAllByText('Екінші әкімші')).toHaveLength(2)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('does not allow a delayed profile snapshot to restore an old role or email', async () => {
    let resolve!: (value: Response) => void
    override = (path, init) => path === '/api/v1/profile' && init?.method === 'PATCH' ? new Promise<Response>(done => { resolve = done }) : undefined
    renderApp('/profile'); await userEvent.type(await screen.findByLabelText('Аты-жөні'), ' Жаңарту')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    session = { user: { ...employee, role: 'IT_ENGINEER', email: 'new@example.test' }, csrf_token: token + '-new' }
    await act(async () => { window.dispatchEvent(new Event('focus')) })
    await screen.findByText('new@example.test')
    await act(async () => { resolve(json({ user: { ...employee, display_name: 'Жаңартылған аты' } })) })
    expect(screen.getByText('new@example.test')).toBeInTheDocument()
    expect(screen.getByText('IT маманы')).toBeInTheDocument()
    expect(screen.queryByText(employee.email)).not.toBeInTheDocument()
  })
  it('clears stale identity and private data when a protected mutation fails CSRF validation', async () => {
    override = path => path === '/api/v1/profile' ? failure(403, 'csrf_failed') : undefined
    const client = renderApp('/profile'); await userEvent.type(await screen.findByLabelText('Аты-жөні'), ' Жаңарту')
    client.setQueryData(['private', 'users'], ['synthetic private data'])
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    expect(await screen.findByRole('heading', { name: 'MedDesk жүйесіне кіру' })).toBeInTheDocument()
    expect(client.getQueryData(['private', 'users'])).toBeUndefined()
  })

})

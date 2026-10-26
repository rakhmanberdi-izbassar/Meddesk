import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'
import { LocaleProvider } from '../lib/locale'
import type { User } from '../lib/api'
import { validMediaFile, type Article, type Category } from '../lib/knowledge'

const employee: User = { id: 'employee-id', email: 'employee@example.test', display_name: 'Айым Тест', role: 'EMPLOYEE', department_id: 'department-id', department_name: 'Тест бөлімі', is_active: true }
const engineer: User = { ...employee, role: 'IT_ENGINEER' }
const category: Category = { id: 'category-id', slug: 'signatures', name_kk: 'Электрондық қолтаңба', name_ru: 'Электронная подпись', description: '', is_active: true, created_at: '2026-10-09T00:00:00Z', updated_at: '2026-10-09T00:00:00Z' }
const example: Article = { id: 'article-id', version: 4, title: 'NCALayer бағдарламасын іске қосу', slug: 'ncalayer', description: 'Қолтаңба мәселесін шешіңіз.', category_id: category.id, category_name_kk: category.name_kk, category_name_ru: category.name_ru, category_active: true, symptoms: 'NCALayer жұмыс істемейді.', solution_steps: ['Бағдарламаны ашыңыз.', 'Қайта іске қосыңыз.'], tags: ['NCALayer', 'ЭЦҚ'], author_id: 'engineer-id', author_name: 'IT Тест', video_id: null, image_ids: [], is_published: true, published_at: '2026-10-09T00:00:00Z', created_at: '2026-10-09T00:00:00Z', updated_at: '2026-10-09T00:00:00Z', view_count: 2, helpful_count: 1, not_helpful_count: 0 }
const token = 'synthetic-test-csrf-token-12345'
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const failure = (status: number, code: string) => json({ error: { code, message: 'private server detail' } }, status)
let identity: User
let article: Article
let override: ((path: string, init?: RequestInit) => Response | Promise<Response> | undefined) | undefined
function renderPage(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
  return { client, ...render(<QueryClientProvider client={client}><LocaleProvider><MemoryRouter initialEntries={[path]}><App /></MemoryRouter></LocaleProvider></QueryClientProvider>) }
}
beforeEach(() => {
  identity = employee; article = { ...example }; override = undefined; localStorage.clear()
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url), response = override?.(path, init)
    if (response) return response
    if (path === '/api/v1/auth/me') return json({ user: identity, csrf_token: token })
    if (path.startsWith('/api/v1/knowledge/categories?')) return json({ items: [category], total: 1, limit: 100, offset: 0 })
    if (path === '/api/v1/knowledge/categories' && init?.method === 'POST') return json({ category: { ...category, ...JSON.parse(String(init.body)) } }, 201)
    if (path.startsWith('/api/v1/knowledge/articles?')) return json({ items: [article], total: 1, limit: 12, offset: 0 })
    if (path === '/api/v1/knowledge/articles/article-id/views') return json({ recorded: true, view_count: 3 })
    if (path === '/api/v1/knowledge/articles/article-id/feedback') return json({ recorded: true, helpful_count: 2, not_helpful_count: 0 })
    if (path === '/api/v1/knowledge/articles/article-id' && init?.method === 'DELETE') return new Response(null, { status: 204 })
    if (path === '/api/v1/knowledge/articles/article-id' && init?.method === 'PATCH') { article = { ...article, ...JSON.parse(String(init.body)), version: article.version + 1 }; return json({ article }) }
    if (path === '/api/v1/knowledge/articles/article-id') return json({ article })
    if (path === '/api/v1/knowledge/articles' && init?.method === 'POST') { article = { ...example, ...JSON.parse(String(init.body)) }; return json({ article }, 201) }
    if (path === '/api/v1/media?purpose=ARTICLE_IMAGE') return json({ media: { id: 'new-media-id', original_filename: 'image.png', content_type: 'image/png', byte_count: 9, purpose: 'ARTICLE_IMAGE', created_at: '2026-10-09T00:00:00Z' } }, 201)
    return failure(404, 'not_found')
  }))
})

describe('knowledge workflows', () => {
  it('searches multilingual content and filters published articles with real API queries', async () => {
    renderPage('/knowledge'); await screen.findByRole('heading', { name: example.title })
    await userEvent.type(screen.getByRole('textbox', { name: 'Нұсқаулықтардан іздеу' }), 'принтер')
    await userEvent.click(screen.getByRole('button', { name: 'Іздеу' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/v1/knowledge/articles?q=%D0%BF%D1%80%D0%B8%D0%BD%D1%82%D0%B5%D1%80&published=true&limit=12&offset=0', expect.anything()))
    expect(screen.getByRole('heading', { name: 'Іздеу нәтижелері' })).toBeInTheDocument()
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Санат' }), category.id)
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path).includes('category_id=category-id'))).toBe(true))
    expect(screen.queryByRole('link', { name: 'Мақалаларды басқару' })).not.toBeInTheDocument()
  })
  it('shows Russian category labels after locale switching', async () => {
    renderPage('/knowledge'); await screen.findByRole('heading', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
    expect(screen.getByRole('option', { name: category.name_ru })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'База знаний' })).toBeInTheDocument()
  })
  it('blocks employee editing without fetching any draft data', async () => {
    renderPage('/engineer/knowledge/article-id/edit')
    expect(await screen.findByText('Бұл бетті IT маманы немесе әкімші ғана аша алады.')).toBeInTheDocument()
    expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path).startsWith('/api/v1/knowledge/articles'))).toBe(false)
  })
  it('records a view once and submits a helpful attempt with an opaque UUID and CSRF', async () => {
    renderPage('/knowledge/articles/article-id'); await screen.findByRole('heading', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'Иә, көмектесті' }))
    expect(await screen.findByText('Мәселе өздігінен шешілді. Жауабыңыз тіркелді.')).toBeInTheDocument()
    const views = vi.mocked(fetch).mock.calls.filter(([path]) => String(path).endsWith('/views'))
    expect(views).toHaveLength(1)
    const feedback = vi.mocked(fetch).mock.calls.find(([path]) => String(path).endsWith('/feedback'))!
    expect(JSON.parse(String(feedback[1]?.body))).toMatchObject({ helpful: true, attempt_id: expect.stringMatching(/^[0-9a-f-]{36}$/) })
    expect(feedback[1]?.headers).toMatchObject({ 'X-CSRF-Token': token })
    expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path).includes('/tickets'))).toBe(false)
  })
  it('keeps the same answer and attempt ID across retries after an ambiguous failure', async () => {
    let calls = 0
    override = path => path.endsWith('/feedback') && ++calls === 1 ? failure(503, 'request_failed') : undefined
    renderPage('/knowledge/articles/article-id'); await screen.findByRole('heading', { name: example.title })
    await userEvent.click(screen.getByRole('button', { name: 'Жоқ, көмектеспеді' }))
    await screen.findByRole('button', { name: 'Жауапты қайта жіберу' })
    expect(screen.getByRole('button', { name: 'Иә, көмектесті' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Жауапты қайта жіберу' }))
    expect(await screen.findByText('Мәселе шешілмеді. IT маманына өтінім дайындаңыз.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Жаңа өтінім' })).toHaveAttribute('href', '/tickets/new?article_id=article-id')
    const attempts = vi.mocked(fetch).mock.calls.filter(([path]) => String(path).endsWith('/feedback')).map(([, init]) => JSON.parse(String(init?.body)))
    expect(attempts).toHaveLength(2); expect(attempts[1]).toEqual(attempts[0]); expect(attempts[0].helpful).toBe(false)
    expect(screen.queryByText('private server detail')).not.toBeInTheDocument()
  })
  it.each([{ is_published: false }, { category_active: false }])('keeps staff previews without employee metrics or feedback: %j', async visibility => {
    identity = engineer; article = { ...example, ...visibility }
    renderPage('/knowledge/articles/article-id'); await screen.findByRole('heading', { name: example.title })
    expect(screen.queryByRole('heading', { name: 'Бұл нұсқаулық көмектесті ме?' })).not.toBeInTheDocument()
    expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path).endsWith('/views'))).toBe(false)
  })
  it('renders plain instructions and cookie-protected media without HTML injection', async () => {
    article = { ...example, symptoms: '<script>private</script>', video_id: 'video-id', image_ids: ['image-id'] }
    renderPage('/knowledge/articles/article-id'); await screen.findByRole('heading', { name: example.title })
    expect(screen.getByText('<script>private</script>')).toBeInTheDocument()
    expect(screen.getByLabelText('Бейненұсқаулық')).toHaveAttribute('src', '/api/v1/media/video-id')
    expect(screen.getByRole('img')).toHaveAttribute('src', '/api/v1/media/image-id')
    expect(document.querySelector('.article-detail script')).toBeNull()
  })
  it('creates a draft with ordered steps, tags, and CSRF', async () => {
    identity = engineer; renderPage('/engineer/knowledge/new')
    await screen.findByLabelText('Мақала атауы')
    await userEvent.type(screen.getByLabelText('Мақала атауы'), 'Тест нұсқаулық')
    await userEvent.type(screen.getByLabelText('Мекенжай атауы (slug)'), 'test-guide')
    await userEvent.selectOptions(screen.getByLabelText('Санат'), category.id)
    await userEvent.type(screen.getByLabelText('Қысқаша сипаттама'), 'Тест шешім')
    await userEvent.type(screen.getByLabelText('Мәселенің белгілері'), 'Тест мәселе')
    await userEvent.type(screen.getByLabelText('Қадам 1'), 'Бірінші қадам')
    await userEvent.click(screen.getByRole('button', { name: 'Қадам қосу' }))
    await userEvent.type(screen.getByLabelText('Қадам 2'), 'Екінші қадам')
    await userEvent.type(screen.getByLabelText('Тегтер'), 'тест, NCALayer')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    expect(await screen.findByText('Мақала сақталды.')).toBeInTheDocument()
    const call = vi.mocked(fetch).mock.calls.find(([path, init]) => path === '/api/v1/knowledge/articles' && init?.method === 'POST')!
    expect(JSON.parse(String(call[1]?.body))).toMatchObject({ title: 'Тест нұсқаулық', solution_steps: ['Бірінші қадам', 'Екінші қадам'], tags: ['тест', 'NCALayer'], is_published: false, video_id: null, image_ids: [] })
    expect(call[1]?.headers).toMatchObject({ 'X-CSRF-Token': token })
  })
  it('preserves unsaved edits on a version conflict and prevents accidental overwrite', async () => {
    identity = engineer
    override = (path, init) => path === '/api/v1/knowledge/articles/article-id' && init?.method === 'PATCH' ? failure(409, 'version_conflict') : undefined
    renderPage('/engineer/knowledge/article-id/edit'); const input = await screen.findByLabelText('Мақала атауы')
    await userEvent.clear(input); await userEvent.type(input, 'Сақталмаған өзгеріс')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    expect(await screen.findByRole('link', { name: 'Соңғы нұсқаны ашу' })).toHaveAttribute('target', '_blank')
    expect(screen.getByLabelText('Мақала атауы')).toHaveValue('Сақталмаған өзгеріс')
    expect(screen.getByRole('button', { name: 'Сақтау' })).toBeDisabled()
    const call = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PATCH')!
    expect(JSON.parse(String(call[1]?.body)).version).toBe(4)
  })
  it('keeps the original editing version when a background query refreshes newer content', async () => {
    identity = engineer
    const { client } = renderPage('/engineer/knowledge/article-id/edit')
    const input = await screen.findByLabelText('Мақала атауы')
    await userEvent.clear(input); await userEvent.type(input, 'Өзгеріс сақталуы керек')
    article = { ...example, title: 'Басқа маманның жаңа нұсқасы', version: 5 }
    await act(async () => { await client.invalidateQueries({ queryKey: ['knowledge', 'article', 'article-id'] }) })
    expect(screen.getByLabelText('Мақала атауы')).toHaveValue('Өзгеріс сақталуы керек')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByText('Мақала сақталды.')
    const call = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PATCH')!
    expect(JSON.parse(String(call[1]?.body))).toMatchObject({ title: 'Өзгеріс сақталуы керек', version: 4 })
  })
  it('allows an existing draft in an inactive category to be edited when no active categories exist', async () => {
    identity = engineer; article = { ...example, is_published: false, category_active: false }
    override = path => path.startsWith('/api/v1/knowledge/categories?') ? json({ items: [], total: 0, limit: 100, offset: 0 }) : undefined
    renderPage('/engineer/knowledge/article-id/edit')
    await screen.findByText('Белсенді санат жоқ. Әкімшіден санат қосуды сұраңыз.')
    const input = screen.getByLabelText('Мақала атауы')
    await userEvent.clear(input); await userEvent.type(input, 'Жасырын жоба жаңартылды')
    expect(screen.getByRole('button', { name: 'Сақтау' })).toBeEnabled()
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByText('Мақала сақталды.')
    const call = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PATCH')!
    expect(JSON.parse(String(call[1]?.body))).toMatchObject({ category_id: category.id, is_published: false })
  })
  it('preserves a dirty editor when a background article refresh temporarily fails', async () => {
    identity = engineer
    const { client } = renderPage('/engineer/knowledge/article-id/edit')
    const input = await screen.findByLabelText('Мақала атауы')
    await userEvent.clear(input); await userEvent.type(input, 'Уақытша қате кезінде сақталады')
    override = path => path === '/api/v1/knowledge/articles/article-id' ? failure(503, 'request_failed') : undefined
    await act(async () => { await client.invalidateQueries({ queryKey: ['knowledge', 'article', 'article-id'] }) })
    expect(screen.getByLabelText('Мақала атауы')).toHaveValue('Уақытша қате кезінде сақталады')
    expect(await screen.findByRole('button', { name: 'Қайталау' })).toBeInTheDocument()
    override = undefined
    await userEvent.click(screen.getByRole('button', { name: 'Қайталау' }))
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Қайталау' })).not.toBeInTheDocument())
    expect(screen.getByLabelText('Мақала атауы')).toHaveValue('Уақытша қате кезінде сақталады')
  })
  it('uploads a valid screenshot then includes its ID only when the article is saved', async () => {
    identity = engineer; renderPage('/engineer/knowledge/article-id/edit'); await screen.findByLabelText('Мақала атауы')
    await userEvent.upload(screen.getByLabelText('Сурет жүктеу'), new File(['synthetic'], 'image.png', { type: 'image/png' }))
    await screen.findByText('Файл жүктелді. Мақаланы сақтау арқылы оны қосыңыз.')
    const call = vi.mocked(fetch).mock.calls.find(([path]) => path === '/api/v1/media?purpose=ARTICLE_IMAGE')!
    expect(call[1]?.body).toBeInstanceOf(FormData); expect((call[1]?.headers as Record<string, string>)['Content-Type']).toBeUndefined()
    expect(screen.getByRole('img')).toHaveAttribute('src', '/api/v1/media/new-media-id')
    await userEvent.click(screen.getByRole('button', { name: 'Сақтау' }))
    await screen.findByText('Мақала сақталды.')
    const save = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PATCH')!
    expect(JSON.parse(String(save[1]?.body)).image_ids).toEqual(['new-media-id'])
  })
  it('requires explicit confirmation before a versioned article deletion', async () => {
    identity = engineer; renderPage('/engineer/knowledge/article-id/edit'); await screen.findByLabelText('Мақала атауы')
    await userEvent.click(screen.getByRole('button', { name: 'Мақаланы жою' }))
    const modal = screen.getByRole('dialog', { name: 'Мақаланы жою' })
    expect(vi.mocked(fetch).mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false)
    await userEvent.click(within(modal).getByRole('button', { name: 'Растау' }))
    await screen.findByText('Мақала жойылды.')
    expect(fetch).toHaveBeenCalledWith('/api/v1/knowledge/articles/article-id', expect.objectContaining({ method: 'DELETE', body: '{"version":4}' }))
  })
  it('creates bilingual categories through an admin-only form', async () => {
    identity = { ...employee, role: 'ADMIN' }; renderPage('/admin/categories'); await screen.findByText(category.name_kk)
    await userEvent.click(screen.getByRole('button', { name: 'Санат қосу' }))
    const dialog = screen.getByRole('dialog', { name: 'Санат қосу' })
    await userEvent.type(within(dialog).getByLabelText('Қазақша атауы'), 'Тест санат')
    await userEvent.type(within(dialog).getByLabelText('Орысша атауы'), 'Тест категория')
    await userEvent.type(within(dialog).getByLabelText('Мекенжай атауы (slug)'), 'test-category')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Сақтау' }))
    expect(await screen.findByText('Санат сақталды.')).toBeInTheDocument()
    expect(fetch).toHaveBeenCalledWith('/api/v1/knowledge/categories', expect.objectContaining({ method: 'POST', body: JSON.stringify({ slug: 'test-category', name_kk: 'Тест санат', name_ru: 'Тест категория', description: '', is_active: true }) }))
  })
  it('restricts client media formats and sizes before uploading', () => {
    expect(validMediaFile(new File(['x'], 'image.svg', { type: 'image/svg+xml' }), 'ARTICLE_IMAGE')).toBe(false)
    expect(validMediaFile(new File(['x'], 'image.jpg', { type: 'image/jpeg' }), 'ARTICLE_IMAGE')).toBe(true)
    expect(validMediaFile(new File([], 'empty.png', { type: 'image/png' }), 'ARTICLE_IMAGE')).toBe(false)
    expect(validMediaFile(new File(['x'], 'video.webm', { type: 'video/webm' }), 'ARTICLE_VIDEO')).toBe(true)
    const file = new File(['x'], 'big.mp4', { type: 'video/mp4' }); Object.defineProperty(file, 'size', { value: 100 * 1024 * 1024 + 1 })
    expect(validMediaFile(file, 'ARTICLE_VIDEO')).toBe(false)
  })
})

it('switches complete article content without recording another view', async () => {
 article.translation_ru = { title: 'Запуск программы', description: 'Описание проблемы', symptoms: 'Программа не отвечает', solution_steps: ['Откройте программу', 'Повторите попытку'] }
 renderPage('/knowledge/articles/article-id')
 await screen.findByRole('heading', { name: example.title })
 await userEvent.click(screen.getByRole('button', { name: 'Русский' }))
 expect(screen.getByRole('heading', { name: 'Запуск программы' })).toBeInTheDocument()
 expect(screen.getByText('Описание проблемы')).toBeInTheDocument()
 expect(screen.getByText('Программа не отвечает')).toBeInTheDocument()
 expect(screen.getByText('Откройте программу')).toBeInTheDocument()
 expect(vi.mocked(fetch).mock.calls.filter(([path]) => String(path).endsWith('/views'))).toHaveLength(1)
})

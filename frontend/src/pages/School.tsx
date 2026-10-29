import { SchoolEmployee } from './SchoolEmployee'
import { SchoolMembers } from './SchoolMembers'
import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { request } from '../lib/api'
import { useToast, useErrorMessage } from '../lib/notifications'
import { Button } from '../components/ui/button'
import { useSchoolAccess, assetsSchema, ticketsSchema, assetSchema, ticketSchema, categories, kinds, statuses, priorities, labels, type SchoolAsset, type SchoolTicket } from '../lib/school'

export function School() {
 const { user } = useAuth()
 return user?.account_scope === 'SCHOOL' ? <SchoolEmployee /> : <OwnerSchool />
}
function OwnerSchool() {
 const { locale, t } = useLocale()
 const l = (kk: string, ru: string) => locale === 'kk' ? kk : ru
 const label = (v: string) => labels[v]?.[locale === 'kk' ? 0 : 1] ?? v
 const access = useSchoolAccess()
 const { user } = useAuth()
 const [tab, setTab] = useState<'tickets' | 'assets' | 'members'>('tickets')
 const [offset, setOffset] = useState(0)
 const [status, setStatus] = useState('')
 const [form, setForm] = useState<'ticket' | 'asset' | null>(null)
 const [editing, setEditing] = useState<SchoolAsset | SchoolTicket | null>(null)
 const [assetFilter, setAssetFilter] = useState('')
 const [assetSearch, setAssetSearch] = useState('')
 const allowed = !!access.data?.allowed
 const tickets = useQuery({ queryKey: ['school', user?.id, 'tickets', offset, status, assetFilter], queryFn: ({ signal }) => request(`/school/tickets?limit=20&offset=${offset}&status=${status}&asset_id=${encodeURIComponent(assetFilter)}`, ticketsSchema, { signal }), enabled: allowed && tab === 'tickets' })
 const assets = useQuery({ queryKey: ['school', user?.id, 'assets', offset, assetSearch], queryFn: ({ signal }) => request(`/school/assets?limit=20&offset=${offset}&q=${encodeURIComponent(assetSearch)}`, assetsSchema, { signal }), enabled: allowed && tab === 'assets' })
 const selectedDevice = useQuery({ queryKey: ['school', user?.id, 'device', assetFilter], queryFn: ({ signal }) => request(`/school/assets/${encodeURIComponent(assetFilter)}`, assetSchema, { signal }), enabled: allowed && !!assetFilter })
 if (access.isPending) return <p role="status">{t.loading}</p>
 if (access.isError) return <div role="alert"><p>{t.requestFailed}</p><Button onClick={() => void access.refetch()}>{t.retry}</Button></div>
 if (!allowed) return <div className="not-found"><h1>{t.forbiddenTitle}</h1><p>{l('Мектеп кеңістігі тек оның иесіне қолжетімді.', 'Школьное пространство доступно только его владельцу.')}</p></div>
 const current = tab === 'tickets' ? tickets : assets
 const total = current.data?.total ?? 0
 return <section className="school-workspace">
 <header className="page-heading"><div><p className="eyebrow">{l('Жеке жұмыс кеңістігі', 'Личное рабочее пространство')}</p><h1>{l('Мектеп · IT қызметі', 'Школа · IT-служба')}</h1><p>{l('Емханадан бөлек өтінімдер, интернет, бейнебақылау және құрылғылар.', 'Отдельные от поликлиники заявки, сеть, видеонаблюдение и оборудование.')}</p></div></header>
 <div className="school-toolbar"><Button variant={tab === 'tickets' ? 'default' : 'outline'} onClick={() => { setTab('tickets'); setOffset(0); setForm(null); setEditing(null) }}>{l('Өтінімдер', 'Заявки')}</Button><Button variant={tab === 'assets' ? 'default' : 'outline'} onClick={() => { setTab('assets'); setOffset(0); setForm(null); setEditing(null) }}>{l('Құрылғылар', 'Оборудование')}</Button><Button onClick={() => { setTab('members'); setForm(null); setEditing(null) }}>{l('Қызметкерлер', 'Сотрудники')}</Button>{tab !== 'members' && <Button onClick={() => { setEditing(null); setForm(tab === 'tickets' ? 'ticket' : 'asset') }}>{l('Қосу', 'Добавить')}</Button>}</div>
 {tab === 'members' ? <SchoolMembers /> : <>
 {form && <SchoolForm key={editing?.id ?? form} kind={form} initialAssetID={assetFilter} editing={editing} close={() => { setForm(null); setEditing(null) }} />}
 {tab === 'tickets' && <div className="school-toolbar"><label>{l('Күйі', 'Статус')} <select value={status} onChange={e => { setStatus(e.target.value); setOffset(0) }}><option value="">{l('Барлығы', 'Все')}</option>{statuses.map(s => <option key={s} value={s}>{label(s)}</option>)}</select></label>{assetFilter && <Button variant="outline" onClick={() => { setAssetFilter(''); setOffset(0) }}>{l('Құрылғы сүзгісін тазалау', 'Сбросить фильтр устройства')}</Button>}</div>}
 {tab === 'assets' && <label>{l('Құрылғылардан іздеу', 'Поиск оборудования')}<input className="school-search" maxLength={100} value={assetSearch} onChange={event => { setAssetSearch(event.target.value); setOffset(0) }} /></label>}
 {assetFilter && tab === 'tickets' && selectedDevice.data && <h2>{selectedDevice.data.name} · {selectedDevice.data.location}</h2>}
 {current.isPending ? <p role="status">{t.loading}</p> : current.isError ? <div role="alert"><p>{t.requestFailed}</p><Button onClick={() => void current.refetch()}>{t.retry}</Button></div> : <>
 <p>{l('Барлығы', 'Всего')}: {total}</p>
 <div className="school-records">{tab === 'tickets' ? tickets.data?.items.map(item => <article className="school-record" key={item.id}><header><strong>SCH-{item.number} · {item.title}</strong><span>{label(item.status)} · {label(item.priority)}</span></header><p>{label(item.category)} · {item.room} · {item.requester}</p>{item.lesson_blocked && <p className="school-warning">{l('Сабаққа кедергі келтіріп тұр', 'Мешает проведению урока')}</p>}<p className="school-text">{item.description}</p>{item.asset_id && <p>{l('Құрылғы', 'Устройство')}: <Button variant="outline" onClick={() => { setAssetFilter(item.asset_id!); setOffset(0); setStatus('') }}>{l('Құрылғы тарихы', 'История устройства')}</Button></p>}{item.resolution && <p className="school-text">{l('Ішкі жазба', 'Внутренняя запись')}: {item.resolution}</p>}{item.public_reply && <p className="school-text">{l('Қызметкерге жауап', 'Ответ сотруднику')}: {item.public_reply}</p>}<time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString(locale === 'kk' ? 'kk-KZ' : 'ru-RU')}</time>{item.status !== 'CLOSED' && <Button variant="outline" onClick={() => { setEditing(item); setForm('ticket') }}>{l('Өңдеу', 'Обработать')}</Button>}</article>) : assets.data?.items.map(item => <article className="school-record" key={item.id}><header><strong>{item.name}</strong><span>{label(item.kind)}</span></header><p>{item.location} · {item.model} · {item.inventory_number}</p><p>IP: {item.ip_address ?? '—'}</p><p className="school-text">{item.notes}</p><div className="school-toolbar"><Button variant="outline" onClick={() => { setEditing(item); setForm('asset') }}>{l('Өңдеу', 'Изменить')}</Button><Button variant="outline" onClick={() => { setForm(null); setEditing(null); setAssetFilter(item.id); setTab('tickets'); setOffset(0); setStatus('') }}>{l('Өтінімдері', 'Заявки')}</Button></div></article>)}</div>
 {total === 0 && <p>{l('Әзірге жазба жоқ. Жоғарыдағы «Қосу» батырмасын басыңыз.', 'Записей пока нет. Нажмите «Добавить» выше.')}</p>}
 <div className="school-toolbar"><Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))}>{l('Алдыңғы', 'Назад')}</Button><span>{Math.floor(offset / 20) + 1}</span><Button variant="outline" disabled={offset + 20 >= total} onClick={() => setOffset(offset + 20)}>{l('Келесі', 'Далее')}</Button></div>
 </>}
 </>}
 </section>
}

function SchoolForm({ kind, editing, close, initialAssetID }: { initialAssetID: string; kind: 'ticket' | 'asset'; editing: SchoolAsset | SchoolTicket | null; close: () => void }) {
 const { locale, t } = useLocale()
 const l = (kk: string, ru: string) => locale === 'kk' ? kk : ru
 const label = (v: string) => labels[v]?.[locale === 'kk' ? 0 : 1] ?? v
 const { api } = useAuth()
 const cache = useQueryClient(), notify = useToast(), errorMessage = useErrorMessage()
 const [busy, setBusy] = useState(false)
 const required = (max: number) => z.string().trim().min(1, t.validationFailed).max(max, t.validationFailed)
 const assetInput = z.object({ name: required(150), kind: z.enum(kinds), location: required(200), model: z.string().max(200), inventory_number: z.string().max(100), ip_address: z.string().max(64), notes: z.string().max(3000) })
 const ticketInput = z.object({ title: required(250), description: required(10000), requester: required(150), room: required(200), category: z.enum(categories), priority: z.enum(priorities), lesson_blocked: z.boolean(), asset_id: z.string().refine(v => v === '' || z.string().uuid().safeParse(v).success, t.validationFailed) })
 const updateInput = z.object({ status: z.enum(statuses), priority: z.enum(priorities), resolution: z.string().trim().max(5000), public_reply: z.string().trim().max(5000) }).superRefine((v,c) => { if ((v.status === 'RESOLVED' || v.status === 'CLOSED') && !v.resolution) c.addIssue({ code: 'custom', path: ['resolution'], message: t.validationFailed }) })
 const schema = kind === 'asset' ? assetInput : editing ? updateInput : ticketInput
 // A single form shape keeps editing values intact when the locale changes.
 type Values = { name: string; kind: typeof kinds[number]; location: string; model: string; inventory_number: string; ip_address: string; notes: string; title: string; description: string; requester: string; room: string; category: typeof categories[number]; priority: typeof priorities[number]; lesson_blocked: boolean; asset_id: string; status: typeof statuses[number]; resolution: string; public_reply: string }
 const asset = kind === 'asset' ? editing as SchoolAsset | null : null
 const ticket = kind === 'ticket' ? editing as SchoolTicket | null : null
 const defaults: Values = { name: asset?.name ?? '', kind: (asset?.kind as Values['kind']) ?? 'COMPUTER', location: asset?.location ?? '', model: asset?.model ?? '', inventory_number: asset?.inventory_number ?? '', ip_address: asset?.ip_address ?? '', notes: asset?.notes ?? '', title: '', description: '', requester: '', room: '', category: 'COMPUTERS', priority: (ticket?.priority as Values['priority']) ?? 'MEDIUM', lesson_blocked: false, asset_id: initialAssetID, status: (ticket?.status as Values['status']) ?? 'NEW', resolution: ticket?.resolution ?? '', public_reply: ticket?.public_reply ?? '' }
 const form = useForm<Values>({ defaultValues: defaults })
 const [validation, setValidation] = useState('')
 const field = (name: keyof Values, title: string, multiline = false) => <label>{title}{multiline ? <textarea {...form.register(name)} rows={4} /> : <input {...form.register(name)} />}</label>
 const select = (name: 'kind' | 'category' | 'priority' | 'status', title: string, values: readonly string[]) => <label>{title}<select {...form.register(name)}>{values.map(v => <option key={v} value={v}>{label(v)}</option>)}</select></label>
 const submit = form.handleSubmit(async values => {
  const parsed = schema.safeParse(values)
  if (!parsed.success) { setValidation(t.validationFailed); return }
  setBusy(true);setValidation('')
  try {
   const path = `/school/${kind === 'asset' ? 'assets' : 'tickets'}${editing ? `/${editing.id}` : ''}`
   await api(path, z.union([assetSchema, ticketSchema]), { method: editing ? 'PATCH' : 'POST', body: { ...parsed.data, ...(editing ? { version: editing.version } : {}) } })
   await cache.invalidateQueries({ queryKey: ['school'] });notify({ kind: 'success', message: l('Сақталды', 'Сохранено') });close()
  } catch (err) { notify({ kind: 'error', message: errorMessage(err) }) } finally { setBusy(false) }
 })
 const transitions: Record<string, string[]> = { NEW: ['NEW','IN_PROGRESS'], IN_PROGRESS: ['IN_PROGRESS','WAITING_FOR_USER','RESOLVED'], WAITING_FOR_USER: ['WAITING_FOR_USER','IN_PROGRESS','RESOLVED'], RESOLVED: ['RESOLVED','IN_PROGRESS','CLOSED'], CLOSED: ['CLOSED'] }
 return <form className="school-form" onSubmit={submit}><h2>{l('Жазба', 'Запись')}</h2><p>{l('Құпиясөздер мен камера жазбаларын енгізбеңіз.', 'Не добавляйте пароли и записи с камер.')}</p>
 <fieldset disabled={busy}>{kind === 'asset' ? <>{field('name',l('Атауы','Название'))}{select('kind',l('Түрі','Тип'),kinds)}{field('location',l('Орналасқан жері','Расположение'))}{field('model',l('Моделі','Модель'))}{field('inventory_number',l('Инвентарлық нөмір','Инвентарный номер'))}{field('ip_address','IP')}{field('notes',l('Ескертпелер','Примечания'),true)}</> : editing ? <>{select('status',l('Күйі','Статус'),transitions[ticket!.status])}{select('priority',l('Басымдығы','Приоритет'),priorities)}{field('resolution',l('Ішкі техникалық жазба','Внутренняя техническая запись'),true)}{field('public_reply',l('Қызметкерге жауап','Ответ сотруднику'),true)}</> : <>{field('title',l('Мәселе','Проблема'))}{field('description',l('Сипаттамасы','Описание'),true)}{field('requester',l('Хабарлаған қызметкер','Кто сообщил'))}{field('room',l('Кабинет / орын','Кабинет / место'))}{select('category',l('Санаты','Категория'),categories)}{select('priority',l('Басымдығы','Приоритет'),priorities)}<DevicePicker value={form.watch('asset_id')} onChange={value => form.setValue('asset_id', value)} /><label><input type="checkbox" {...form.register('lesson_blocked')} />{l('Сабаққа кедергі келтіріп тұр','Мешает проведению урока')}</label></>}
 {validation && <p role="alert">{validation}</p>}<div className="school-toolbar"><Button type="submit">{busy ? t.loading : l('Сақтау','Сохранить')}</Button><Button type="button" variant="outline" onClick={close}>{l('Болдырмау','Отмена')}</Button></div></fieldset></form>
}


function DevicePicker({ value, onChange }: { value: string; onChange: (value: string) => void }) {
 const { user } = useAuth(), { locale, t } = useLocale()
 const l = (kk: string, ru: string) => locale === 'kk' ? kk : ru
 const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [offset, setOffset] = useState(0)
 useEffect(() => { const timer = setTimeout(() => { setQuery(search.trim()); setOffset(0) }, 250); return () => clearTimeout(timer) }, [search])
 const devices = useQuery({ queryKey: ['school', user?.id, 'device-picker', query, offset], queryFn: ({ signal }) => request(`/school/assets?limit=20&offset=${offset}&q=${encodeURIComponent(query)}`, assetsSchema, { signal }) })
 const selected = useQuery({ queryKey: ['school', user?.id, 'device', value], queryFn: ({ signal }) => request(`/school/assets/${encodeURIComponent(value)}`, assetSchema, { signal }), enabled: !!value })
 const options = devices.data?.items ?? []
 const title = (asset: SchoolAsset) => [asset.name, asset.location, asset.inventory_number].filter(Boolean).join(' · ')
 return <div className="school-device-picker">
 <label>{l('Құрылғыны атауы немесе инвентарлық нөмірімен іздеу', 'Поиск устройства по названию или инвентарному номеру')}<input maxLength={100} value={search} onChange={event => setSearch(event.target.value)} /></label>
 <label>{l('Құрылғы (міндетті емес)', 'Устройство (необязательно)')}<select value={value} onChange={event => onChange(event.target.value)}><option value="">{l('Құрылғысыз', 'Без устройства')}</option>{value && !options.some(asset => asset.id === value) && <option value={value}>{selected.data ? title(selected.data) : t.loading}</option>}{options.map(asset => <option key={asset.id} value={asset.id}>{title(asset)}</option>)}</select></label>
 {(devices.isError || selected.isError) && <p role="alert">{t.requestFailed}</p>}
 {devices.isPending && <p role="status">{t.loading}</p>}
 <div className="school-toolbar"><Button type="button" variant="outline" disabled={offset === 0 || devices.isFetching} onClick={() => setOffset(Math.max(0, offset - 20))}>{l('Алдыңғы құрылғылар', 'Предыдущие устройства')}</Button><Button type="button" variant="outline" disabled={devices.isFetching || !devices.data || offset + 20 >= devices.data.total} onClick={() => setOffset(offset + 20)}>{l('Келесі құрылғылар', 'Следующие устройства')}</Button></div>
 </div>
}

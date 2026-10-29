import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useToast, useErrorMessage } from '../lib/notifications'
import { useSchoolAccess, schoolRequestsSchema, schoolRequestSchema, labels, categories, priorities } from '../lib/school'
import { Button } from '../components/ui/button'
import { FormField } from '../components/forms/FormField'
export function SchoolEmployee() {
 const { user, api } = useAuth(), { locale, t } = useLocale(), access = useSchoolAccess(), cache = useQueryClient(), notify = useToast(), message = useErrorMessage()
 const l = (kk: string, ru: string) => locale === 'kk' ? kk : ru
 const label = (value: string) => labels[value]?.[locale === 'kk' ? 0 : 1] ?? value
 const [offset, setOffset] = useState(0), [creating, setCreating] = useState(false)
 const query = useQuery({ queryKey: ['school', user?.id, 'own-requests', offset], queryFn: ({ signal }) => api(`/school/requests?limit=20&offset=${offset}`, schoolRequestsSchema, { signal }), enabled: !!access.data?.allowed })
 const schema = z.object({ title: z.string().trim().min(1,t.required).max(250), description: z.string().trim().min(1,t.required).max(10000), room: z.string().trim().min(1,t.required).max(200), category: z.enum(categories), priority: z.enum(priorities), lesson_blocked: z.boolean() })
 const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { title: '', description: '', room: '', category: 'COMPUTERS', priority: 'MEDIUM', lesson_blocked: false } })
 const submit = form.handleSubmit(async body => {
  try { await api('/school/requests', schoolRequestSchema, { method: 'POST', body });form.reset();setCreating(false);setOffset(0);await cache.invalidateQueries({ queryKey: ['school'] });notify({ kind: 'success', message: l('Өтінім жіберілді', 'Заявка отправлена') }) } catch (err) { notify({ kind: 'error', message: message(err) }) }
 })
 if (access.isPending) return <p role="status">{t.loading}</p>
 if (access.isError || !access.data?.allowed) return <div role="alert"><p>{access.isError ? t.requestFailed : t.permissionDenied}</p><Button onClick={() => void access.refetch()}>{t.retry}</Button></div>
 return <section><header className="page-heading"><h1>{l('Мектеп · менің өтінімдерім', 'Школа · мои заявки')}</h1><p>{l('Сіз жіберген өтінімдер мен инженердің жауаптары.', 'Ваши обращения и ответы инженера.')}</p></header><Button onClick={() => setCreating(value => !value)}>{creating ? t.cancel : l('Өтінім жіберу', 'Отправить заявку')}</Button>
 {creating && <form className="school-form" onSubmit={submit} noValidate><fieldset disabled={form.formState.isSubmitting}>
 <FormField id="school-request-title" label={l('Мәселе', 'Проблема')} error={form.formState.errors.title?.message}><input id="school-request-title" {...form.register('title')} /></FormField>
 <FormField id="school-request-description" label={l('Сипаттамасы', 'Описание')} error={form.formState.errors.description?.message}><textarea id="school-request-description" rows={4} {...form.register('description')} /></FormField>
 <FormField id="school-request-room" label={l('Кабинет / орын', 'Кабинет / место')} error={form.formState.errors.room?.message}><input id="school-request-room" {...form.register('room')} /></FormField>
 <label>{l('Санаты', 'Категория')}<select {...form.register('category')}>{categories.map(value => <option key={value} value={value}>{label(value)}</option>)}</select></label><label>{l('Басымдығы', 'Приоритет')}<select {...form.register('priority')}>{priorities.map(value => <option key={value} value={value}>{label(value)}</option>)}</select></label>
 <label><input type="checkbox" {...form.register('lesson_blocked')} />{l('Сабаққа кедергі келтіріп тұр', 'Мешает проведению урока')}</label><p>{l('Құпиясөздер мен оқушылардың жеке деректерін жазбаңыз.', 'Не указывайте пароли и персональные данные учеников.')}</p><Button type="submit">{form.formState.isSubmitting ? t.saving : l('Жіберу', 'Отправить')}</Button></fieldset></form>}
 {query.isPending ? <p role="status">{t.loading}</p> : query.isError ? <div role="alert"><p>{t.requestFailed}</p><Button onClick={() => void query.refetch()}>{t.retry}</Button></div> : <>
 {query.data.items.length === 0 && <p>{l('Әзірге өтінім жоқ.', 'Заявок пока нет.')}</p>}
 <div className="school-records">{query.data.items.map(item => <article className="school-record" key={item.id}><header><h2>SCH-{item.number} · {item.title}</h2><span>{label(item.status)} · {label(item.priority)}</span></header><p>{item.room} · {label(item.category)}</p><p className="school-text">{item.description}</p>{item.public_reply && <div><h3>{l('Инженердің жауабы', 'Ответ инженера')}</h3><p className="school-text">{item.public_reply}</p></div>}<time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString(locale === 'kk' ? 'kk-KZ' : 'ru-RU')}</time></article>)}</div>
 <div className="school-toolbar"><Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0,offset - 20))}>{l('Алдыңғы', 'Назад')}</Button><span>{offset / 20 + 1}</span><Button variant="outline" disabled={offset + 20 >= query.data.total} onClick={() => setOffset(offset + 20)}>{l('Келесі', 'Далее')}</Button></div></>}
 </section>
}

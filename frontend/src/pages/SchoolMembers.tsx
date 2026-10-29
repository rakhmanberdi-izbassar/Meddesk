import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useToast, useErrorMessage } from '../lib/notifications'
import { membersSchema, memberSchema, type SchoolMember } from '../lib/school'
import { passwordIsValid, nameSchema } from '../lib/validation'
import { Button } from '../components/ui/button'
import { Modal } from '../components/forms/Modal'
import { FormField } from '../components/forms/FormField'

export function SchoolMembers() {
 const { api, user } = useAuth(), { locale, t } = useLocale()
 const l = (kk: string, ru: string) => locale === 'kk' ? kk : ru
 const [offset, setOffset] = useState(0), [editing, setEditing] = useState<SchoolMember | null | undefined>(undefined)
 const query = useQuery({ queryKey: ['school', user?.id, 'members', offset], queryFn: ({ signal }) => api(`/school/members?limit=20&offset=${offset}`, membersSchema, { signal }) })
 return <section><h2>{l('Мектеп қызметкерлері', 'Сотрудники школы')}</h2><p>{l('Бұл аккаунттар тек мектепке өтінім жібереді және өз өтінімдерін көреді.', 'Эти аккаунты отправляют заявки только в школу и видят только свои обращения.')}</p><Button onClick={() => setEditing(null)}>{l('Қызметкер қосу', 'Добавить сотрудника')}</Button>
 {editing !== undefined && <MemberEditor member={editing} close={() => setEditing(undefined)} />}
 {query.isPending ? <p role="status">{t.loading}</p> : query.isError ? <div role="alert"><p>{t.requestFailed}</p><Button onClick={() => void query.refetch()}>{t.retry}</Button></div> : <>
 {query.data.items.length === 0 && <p>{l('Әзірге қызметкер жоқ.', 'Сотрудников пока нет.')}</p>}
 <div className="school-records">{query.data.items.map(member => <article className="school-record" key={member.id}><h3>{member.display_name}</h3><p>{member.email} · {member.is_active ? l('Белсенді', 'Активен') : l('Өшірілген', 'Отключён')}</p><Button variant="outline" onClick={() => setEditing(member)}>{l('Аккаунтты өңдеу', 'Изменить аккаунт')}</Button></article>)}</div>
 <div className="school-toolbar"><Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))}>{l('Алдыңғы', 'Назад')}</Button><span>{offset / 20 + 1}</span><Button variant="outline" disabled={offset + 20 >= query.data.total} onClick={() => setOffset(offset + 20)}>{l('Келесі', 'Далее')}</Button></div></>}
 </section>
}
function MemberEditor({ member, close }: { member: SchoolMember | null; close: () => void }) {
 const { api } = useAuth(), { t, locale } = useLocale(), notify = useToast(), message = useErrorMessage(), cache = useQueryClient()
 const l = (kk: string, ru: string) => locale === 'kk' ? kk : ru
 const [busy, setBusy] = useState(false), [confirmed, setConfirmed] = useState<Values | null>(null)
 const schema = z.object({ email: z.string().trim().email(t.emailInvalid).max(254), display_name: nameSchema(t.nameRule), password: z.string(), is_active: z.boolean() }).superRefine((value, ctx) => { if ((!member || value.password !== '') && !passwordIsValid(value.password)) ctx.addIssue({ code: 'custom', path: ['password'], message: t.passwordRule }) })
 type Values = z.infer<typeof schema>
 const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { email: member?.email ?? '', display_name: member?.display_name ?? '', password: '', is_active: member?.is_active ?? true } })
 async function save(values: Values) {
  setBusy(true)
  try {
   await api(`/school/members${member ? `/${member.id}` : ''}`, memberSchema, { method: member ? 'PATCH' : 'POST', body: member ? { display_name: values.display_name, is_active: values.is_active, version: member.version, ...(values.password ? { password: values.password } : {}) } : { email: values.email, display_name: values.display_name, password: values.password } })
   await cache.invalidateQueries({ queryKey: ['school'] });notify({ kind: 'success', message: l('Аккаунт сақталды', 'Аккаунт сохранён') });close()
  } catch (err) { notify({ kind: 'error', message: err instanceof ApiError && err.code === 'conflict' ? l('Бұл email басқа аккаунтта қолданылып тұр.', 'Этот email уже используется другим аккаунтом.') : message(err) });setConfirmed(null) } finally { setBusy(false) }
 }
 const submit = form.handleSubmit(values => { if (member && (values.is_active !== member.is_active || values.password !== '')) setConfirmed(values); else void save(values) })
 return <Modal title={confirmed ? t.confirmChanges : member ? l('Мектеп аккаунтын өңдеу', 'Изменение школьного аккаунта') : l('Мектеп аккаунтын ашу', 'Создание школьного аккаунта')} description={confirmed ? l('Құпиясөзді жаңарту немесе аккаунтты өшіру ағымдағы сессияларды тоқтатады.', 'Смена пароля или отключение аккаунта завершает текущие сеансы.') : l('Қызметкер осы email мен құпиясөз арқылы сайтқа кіреді.', 'Сотрудник входит на сайт с этим email и паролем.')} onClose={close} busy={busy}>
 {confirmed ? <div className="school-toolbar"><Button variant="outline" disabled={busy} onClick={() => setConfirmed(null)}>{t.back}</Button><Button disabled={busy} onClick={() => void save(confirmed)}>{t.confirm}</Button></div> : <form className="editor-form" onSubmit={submit} noValidate>
 <fieldset disabled={busy}><FormField id="school-member-name" label={l('Аты-жөні', 'Имя сотрудника')} error={form.formState.errors.display_name?.message}><input id="school-member-name" {...form.register('display_name')} /></FormField>
 <FormField id="school-member-email" label="Email" error={form.formState.errors.email?.message}><input id="school-member-email" type="email" readOnly={!!member} autoComplete="off" {...form.register('email')} /></FormField>
 <FormField id="school-member-password" label={member ? l('Жаңа құпиясөз (бос қалса өзгермейді)', 'Новый пароль (пустой — без изменений)') : l('Құпиясөз', 'Пароль')} error={form.formState.errors.password?.message}><input id="school-member-password" type="password" autoComplete="new-password" {...form.register('password')} /></FormField><p>{t.passwordRule}</p>
 {member && <label><input type="checkbox" {...form.register('is_active')} />{l('Аккаунт белсенді', 'Аккаунт активен')}</label>}
 <div className="school-toolbar"><Button type="submit">{busy ? t.saving : t.save}</Button><Button type="button" variant="outline" onClick={close}>{t.cancel}</Button></div></fieldset></form>}
 </Modal>
}

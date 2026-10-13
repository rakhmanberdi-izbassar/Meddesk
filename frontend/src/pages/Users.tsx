import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Plus, Users, Search } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { FormField, fieldDescription } from '../components/forms/FormField'
import { Modal } from '../components/forms/Modal'
import { Pagination } from '../components/forms/Pagination'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { departmentsPageSchema, queryString, roleSchema, userResultSchema, usersPageSchema, type Department, type Role, type User } from '../lib/api'
import { nameSchema, passwordIsValid } from '../lib/validation'

function UserEditor({ user, onClose }: { user: User | null; onClose: () => void }) {
  const { t } = useLocale()
  const auth = useAuth()
  const notify = useToast()
  const message = useErrorMessage()
  const cache = useQueryClient()
  const [busy, setBusy] = useState(false)
  const schema = z.object({
    email: z.string().trim().email(t.emailInvalid).max(254, t.emailInvalid), display_name: nameSchema(t.nameRule),
    role: roleSchema, department_id: z.string().min(1, t.required), is_active: z.boolean(), password: z.string(),
  }).superRefine((values, context) => {
    if ((!user || values.password !== '') && !passwordIsValid(values.password)) context.addIssue({ code: z.ZodIssueCode.custom, path: ['password'], message: t.passwordRule })
  })
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: {
    email: user?.email ?? '', display_name: user?.display_name ?? '', role: user?.role ?? 'EMPLOYEE',
    department_id: user?.department_id ?? '', is_active: user?.is_active ?? true, password: '',
  } })
  const [confirmation, setConfirmation] = useState<Values | null>(null)
  const isSelf = user?.id === auth.user?.id
  const departments = useQuery({ queryKey: ['admin', 'active-departments'], queryFn: async ({ signal }) => {
    const result: Department[] = []
    let offset = 0
    while (true) {
      const page = await auth.api(`/departments?${queryString({ is_active: 'true', limit: 100, offset })}`, departmentsPageSchema, { signal })
      result.push(...page.items)
      offset += page.items.length
      if (offset >= page.total || page.items.length === 0) return result
    }
  } })
  const choices = departments.data ?? []
  const roles: { value: Role; label: string }[] = [{ value: 'EMPLOYEE', label: t.employeeRole }, { value: 'IT_ENGINEER', label: t.engineerRole }, { value: 'ADMIN', label: t.adminRole }]
  const save = async (values: Values) => {
    setBusy(true)
    try {
      const body = { email: values.email, display_name: values.display_name, role: values.role, department_id: values.department_id,
        ...(user ? { is_active: values.is_active } : {}), ...(values.password !== '' ? { password: values.password } : {}) }
      await auth.api(user ? `/users/${user.id}` : '/users', userResultSchema, { method: user ? 'PATCH' : 'POST', body })
      await cache.invalidateQueries({ queryKey: ['admin', 'users'] })
      notify({ kind: 'success', message: t.userSaved })
      onClose()
      if (isSelf) await auth.refresh()
    } catch (error) { notify({ kind: 'error', message: message(error) }); setConfirmation(null) }
    finally { setBusy(false) }
  }
  const submit = form.handleSubmit(async values => {
    if (user && (values.email !== user.email || values.role !== user.role || values.is_active !== user.is_active || values.password !== '')) setConfirmation(values)
    else await save(values)
  })
  return <Modal title={confirmation ? t.confirmChanges : user ? t.editUser : t.addUser} description={confirmation ? t.confirmUserDescription : t.editorDescription} onClose={onClose} busy={busy}>
    {confirmation ? <div className="confirm-content"><p><strong>{user?.display_name}</strong> · {user?.email}</p><div className="form-actions"><Button autoFocus variant="outline" disabled={busy} onClick={() => setConfirmation(null)}>{t.back}</Button><Button disabled={busy} onClick={() => void save(confirmation)}>{busy ? t.saving : t.confirm}</Button></div></div> : <form className="editor-form" onSubmit={submit} noValidate>
      <FormField id="user-name" label={t.displayName} error={form.formState.errors.display_name?.message}><input id="user-name" {...form.register('display_name')} autoComplete="off" disabled={busy} aria-invalid={!!form.formState.errors.display_name} aria-describedby={fieldDescription('user-name', form.formState.errors.display_name?.message)} /></FormField>
      <FormField id="user-email" label={t.email} error={form.formState.errors.email?.message}><input id="user-email" type="email" autoComplete="off" {...form.register('email')} disabled={busy} aria-invalid={!!form.formState.errors.email} aria-describedby={fieldDescription('user-email', form.formState.errors.email?.message)} /></FormField>
      <div className="form-two-columns"><FormField id="user-role" label={t.role}><select id="user-role" {...form.register('role')} disabled={busy || isSelf}>{roles.map(role => <option key={role.value} value={role.value}>{role.label}</option>)}</select></FormField><FormField id="user-department" label={t.department} error={form.formState.errors.department_id?.message}><select id="user-department" {...form.register('department_id')} disabled={busy || departments.isPending || departments.isError} aria-invalid={!!form.formState.errors.department_id} aria-describedby={fieldDescription('user-department', form.formState.errors.department_id?.message)}><option value="">{departments.isPending ? t.loading : t.selectDepartment}</option>{user && !choices.some(choice => choice.id === user.department_id) && <option value={user.department_id}>{user.department_name} ({t.inactive})</option>}{choices.map(department => <option value={department.id} key={department.id}>{department.name}</option>)}</select></FormField></div>
      {departments.isError && <div className="inline-error" role="alert">{message(departments.error)} <Button variant="ghost" size="sm" onClick={() => void departments.refetch()} type="button">{t.retry}</Button></div>}
      {!departments.isPending && !departments.isError && choices.length === 0 && <p className="inline-error" role="alert">{t.noDepartmentChoices}</p>}
      <FormField id="user-password" label={user ? t.resetPassword : t.password} error={form.formState.errors.password?.message} hint={user ? t.passwordOptional : t.passwordRule}><input id="user-password" type="password" autoComplete="new-password" {...form.register('password')} disabled={busy} aria-invalid={!!form.formState.errors.password} aria-describedby={fieldDescription('user-password', form.formState.errors.password?.message, 'hint')} /></FormField>
      {user && <label className="checkbox-field"><input type="checkbox" {...form.register('is_active')} disabled={busy || isSelf} />{t.active}</label>}
      {isSelf && <p className="card-note">{t.selfAdminNotice}</p>}
      <div className="form-actions"><Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t.cancel}</Button><Button type="submit" disabled={busy || departments.isPending || departments.isError || (!user && choices.length === 0)}>{busy ? t.saving : user ? t.save : t.create}</Button></div>
    </form>}
  </Modal>
}

export function UsersPage() {
  const { t } = useLocale()
  const { api } = useAuth()
  const message = useErrorMessage()
  const [search, setSearch] = useState('')
  const [role, setRole] = useState('')
  const [active, setActive] = useState('')
  const [page, setPage] = useState(0)
  const [editor, setEditor] = useState<{ user: User | null } | null>(null)
  const limit = 20
  const query = useQuery({ queryKey: ['admin', 'users', search, role, active, page], queryFn: ({ signal }) => api(`/users?${queryString({ q: search, role, is_active: active, limit, offset: page * limit })}`, usersPageSchema, { signal }) })
  const roleName = (value: Role) => value === 'ADMIN' ? t.adminRole : value === 'IT_ENGINEER' ? t.engineerRole : t.employeeRole
  return <><div className="page-heading"><div><div className="eyebrow"><span />{t.adminWorkspace}</div><h1>{t.users}</h1><p>{t.usersDescription}</p></div><Button className="heading-action" onClick={() => setEditor({ user: null })}><Plus size={17} aria-hidden="true" />{t.addUser}</Button></div>
    <section className="table-card" aria-label={t.users}><div className="table-filters"><div className="search-input"><Search size={17} aria-hidden="true" /><input value={search} maxLength={100} aria-label={t.searchUsers} placeholder={t.searchUsers} onChange={event => { setSearch(event.target.value); setPage(0) }} /></div><select aria-label={t.role} value={role} onChange={event => { setRole(event.target.value); setPage(0) }}><option value="">{t.allRoles}</option><option value="EMPLOYEE">{t.employeeRole}</option><option value="IT_ENGINEER">{t.engineerRole}</option><option value="ADMIN">{t.adminRole}</option></select><select aria-label={t.status} value={active} onChange={event => { setActive(event.target.value); setPage(0) }}><option value="">{t.allStatuses}</option><option value="true">{t.active}</option><option value="false">{t.inactive}</option></select></div>
      {query.isPending ? <div className="table-state" role="status">{t.loading}</div> : query.isError ? <div className="table-state"><p role="alert">{message(query.error)}</p><Button variant="outline" onClick={() => void query.refetch()}>{t.retry}</Button></div> : <><div className="table-scroll" tabIndex={0} role="region" aria-label={t.users}><table><thead><tr><th>{t.displayName}</th><th>{t.role}</th><th>{t.department}</th><th>{t.status}</th><th>{t.actions}</th></tr></thead><tbody>{query.data.items.map(user => <tr key={user.id}><td><strong>{user.display_name}</strong><span className="table-secondary">{user.email}</span></td><td>{roleName(user.role)}</td><td>{user.department_name}</td><td><Badge className={`account-status ${user.is_active ? 'active-status' : 'inactive-status'}`}>{user.is_active ? t.active : t.inactive}</Badge></td><td><Button size="sm" variant="ghost" onClick={() => setEditor({ user })} aria-label={`${t.edit}: ${user.display_name}`}>{t.edit}</Button></td></tr>)}</tbody></table>{query.data.items.length === 0 && <div className="table-state"><Users size={28} aria-hidden="true" /><p>{t.noUsers}</p></div>}</div><Pagination page={page} limit={limit} total={query.data.total} busy={query.isFetching} onChange={setPage} /></>}
    </section>{editor && <UserEditor user={editor.user} onClose={() => setEditor(null)} />}
  </>
}

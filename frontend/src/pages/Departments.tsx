import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Building2, Plus, Search } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { FormField, fieldDescription } from '../components/forms/FormField'
import { Modal } from '../components/forms/Modal'
import { Pagination } from '../components/forms/Pagination'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { departmentResultSchema, departmentsPageSchema, queryString, type Department } from '../lib/api'
import { nameSchema } from '../lib/validation'

function DepartmentEditor({ department, onClose }: { department: Department | null; onClose: () => void }) {
  const { t } = useLocale()
  const { api } = useAuth()
  const notify = useToast()
  const message = useErrorMessage()
  const cache = useQueryClient()
  const schema = z.object({ name: nameSchema(t.nameRule), is_active: z.boolean() })
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: department?.name ?? '', is_active: department?.is_active ?? true } })
  const [confirmation, setConfirmation] = useState<Values | null>(null)
  const [busy, setBusy] = useState(false)
  const save = async (values: Values) => {
    setBusy(true)
    try {
      await api(department ? `/departments/${department.id}` : '/departments', departmentResultSchema, { method: department ? 'PATCH' : 'POST', body: department ? values : { name: values.name } })
      await cache.invalidateQueries({ queryKey: ['admin', 'departments'] })
      await cache.invalidateQueries({ queryKey: ['admin', 'active-departments'] })
      await cache.invalidateQueries({ queryKey: ['admin', 'users'] })
      notify({ kind: 'success', message: t.departmentSaved })
      onClose()
    } catch (error) { notify({ kind: 'error', message: message(error) }); setConfirmation(null) }
    finally { setBusy(false) }
  }
  const submit = form.handleSubmit(async values => {
    if (department?.is_active && !values.is_active) setConfirmation(values)
    else await save(values)
  })
  return <Modal title={confirmation ? t.confirmChanges : department ? t.editDepartment : t.addDepartment} description={confirmation ? t.confirmDepartmentDescription : t.departmentEditorDescription} onClose={onClose} busy={busy}>
    {confirmation ? <div className="confirm-content"><p><strong>{department?.name}</strong></p><div className="form-actions"><Button autoFocus variant="outline" disabled={busy} onClick={() => setConfirmation(null)}>{t.back}</Button><Button disabled={busy} onClick={() => void save(confirmation)}>{busy ? t.saving : t.confirm}</Button></div></div> : <form className="editor-form" onSubmit={submit} noValidate><FormField id="department-name" label={t.departmentName} error={form.formState.errors.name?.message}><input id="department-name" {...form.register('name')} disabled={busy} aria-invalid={!!form.formState.errors.name} aria-describedby={fieldDescription('department-name', form.formState.errors.name?.message)} /></FormField>{department && <label className="checkbox-field"><input type="checkbox" {...form.register('is_active')} disabled={busy} />{t.active}</label>}<div className="form-actions"><Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t.cancel}</Button><Button type="submit" disabled={busy}>{busy ? t.saving : department ? t.save : t.create}</Button></div></form>}
  </Modal>
}

export function DepartmentsPage() {
  const { t } = useLocale()
  const { api } = useAuth()
  const message = useErrorMessage()
  const [search, setSearch] = useState('')
  const [active, setActive] = useState('')
  const [page, setPage] = useState(0)
  const [editor, setEditor] = useState<{ department: Department | null } | null>(null)
  const limit = 20
  const query = useQuery({ queryKey: ['admin', 'departments', search, active, page], queryFn: ({ signal }) => api(`/departments?${queryString({ q: search, is_active: active, limit, offset: page * limit })}`, departmentsPageSchema, { signal }) })
  return <><div className="page-heading"><div><div className="eyebrow"><span />{t.adminWorkspace}</div><h1>{t.departments}</h1><p>{t.departmentsDescription}</p></div><Button className="heading-action" onClick={() => setEditor({ department: null })}><Plus size={17} aria-hidden="true" />{t.addDepartment}</Button></div><section className="table-card" aria-label={t.departments}><div className="table-filters"><div className="search-input"><Search size={17} aria-hidden="true" /><input value={search} maxLength={100} aria-label={t.searchDepartments} placeholder={t.searchDepartments} onChange={event => { setSearch(event.target.value); setPage(0) }} /></div><select aria-label={t.status} value={active} onChange={event => { setActive(event.target.value); setPage(0) }}><option value="">{t.allStatuses}</option><option value="true">{t.active}</option><option value="false">{t.inactive}</option></select></div>{query.isPending ? <div className="table-state" role="status">{t.loading}</div> : query.isError ? <div className="table-state"><p role="alert">{message(query.error)}</p><Button variant="outline" onClick={() => void query.refetch()}>{t.retry}</Button></div> : <><div className="table-scroll" tabIndex={0} role="region" aria-label={t.departments}><table><thead><tr><th>{t.departmentName}</th><th>{t.status}</th><th>{t.actions}</th></tr></thead><tbody>{query.data.items.map(department => <tr key={department.id}><td><strong>{department.name}</strong></td><td><Badge className={`account-status ${department.is_active ? 'active-status' : 'inactive-status'}`}>{department.is_active ? t.active : t.inactive}</Badge></td><td><Button size="sm" variant="ghost" onClick={() => setEditor({ department })} aria-label={`${t.edit}: ${department.name}`}>{t.edit}</Button></td></tr>)}</tbody></table>{query.data.items.length === 0 && <div className="table-state"><Building2 size={28} aria-hidden="true" /><p>{t.noDepartments}</p></div>}</div><Pagination page={page} limit={limit} total={query.data.total} busy={query.isFetching} onChange={setPage} /></>}</section>{editor && <DepartmentEditor department={editor.department} onClose={() => setEditor(null)} />}</>
}

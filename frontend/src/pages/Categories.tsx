import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { FolderOpen, Plus, Search } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { FormField, fieldDescription } from '../components/forms/FormField'
import { Modal } from '../components/forms/Modal'
import { Pagination } from '../components/forms/Pagination'
import { KnowledgeState } from '../components/KnowledgeState'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { queryString } from '../lib/api'
import { nameSchema } from '../lib/validation'
import { categoriesPageSchema, categoryResultSchema, knowledgePath, type Category } from '../lib/knowledge'

function CategoryEditor({ category, onClose }: { category: Category | null; onClose: () => void }) {
  const { t } = useLocale()
  const { api } = useAuth()
  const notify = useToast()
  const message = useErrorMessage()
  const cache = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [confirmation, setConfirmation] = useState(false)
  const schema = z.object({ slug: z.string().trim().max(100, t.slugRule).regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, t.slugRule), name_kk: nameSchema(t.nameRule), name_ru: nameSchema(t.nameRule), description: z.string().trim().max(2000, t.articleValidation), is_active: z.boolean() })
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: category ? { slug: category.slug, name_kk: category.name_kk, name_ru: category.name_ru, description: category.description, is_active: category.is_active } : { slug: '', name_kk: '', name_ru: '', description: '', is_active: true } })
  const save = async (values: Values) => {
    setBusy(true)
    try {
      await api(category ? `${knowledgePath}/categories/${category.id}` : `${knowledgePath}/categories`, categoryResultSchema, { method: category ? 'PATCH' : 'POST', body: values })
      await cache.invalidateQueries({ queryKey: ['knowledge'] })
      notify({ kind: 'success', message: t.categorySaved }); onClose()
    } catch (error) { notify({ kind: 'error', message: message(error) }); setConfirmation(false) }
    finally { setBusy(false) }
  }
  const submit = form.handleSubmit(async values => {
    if (category?.is_active && !values.is_active) setConfirmation(true)
    else await save(values)
  })
  return <Modal title={confirmation ? t.confirmChanges : category ? t.editCategory : t.addCategory} description={confirmation ? t.categoryConfirm : t.categoriesDescription} onClose={onClose} busy={busy}>{confirmation ? <div className="confirm-content"><p><strong>{category?.name_kk}</strong> · {category?.name_ru}</p><div className="form-actions"><Button autoFocus variant="outline" onClick={() => setConfirmation(false)} disabled={busy}>{t.back}</Button><Button onClick={() => void save(form.getValues())} disabled={busy}>{busy ? t.saving : t.confirm}</Button></div></div> : <form className="editor-form" onSubmit={submit} noValidate>{(['name_kk', 'name_ru', 'slug'] as const).map(key => <FormField key={key} id={`category-${key}`} label={key === 'name_kk' ? t.categoryNameKK : key === 'name_ru' ? t.categoryNameRU : t.slug} error={form.formState.errors[key]?.message}><input id={`category-${key}`} {...form.register(key)} disabled={busy} aria-invalid={!!form.formState.errors[key]} aria-describedby={fieldDescription(`category-${key}`, form.formState.errors[key]?.message)} /></FormField>)}<FormField id="category-description" label={t.description} error={form.formState.errors.description?.message}><textarea id="category-description" rows={3} {...form.register('description')} disabled={busy} aria-invalid={!!form.formState.errors.description} /></FormField><label className="checkbox-field"><input type="checkbox" {...form.register('is_active')} disabled={busy} />{t.active}</label><div className="form-actions"><Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t.cancel}</Button><Button disabled={busy} type="submit">{busy ? t.saving : t.save}</Button></div></form>}</Modal>
}

export function Categories() {
  const { t } = useLocale()
  const { api } = useAuth()
  const [search, setSearch] = useState('')
  const [active, setActive] = useState('')
  const [page, setPage] = useState(0)
  const [editor, setEditor] = useState<{ category: Category | null } | null>(null)
  const limit = 20
  const categories = useQuery({ queryKey: ['knowledge', 'admin-categories', search, active, page], queryFn: ({ signal }) => api(`${knowledgePath}/categories?${queryString({ q: search, is_active: active, limit, offset: page * limit })}`, categoriesPageSchema, { signal }) })
  return <><div className="page-heading"><div><div className="eyebrow"><span />{t.adminWorkspace}</div><h1>{t.categories}</h1><p>{t.categoriesDescription}</p></div><Button className="heading-action" onClick={() => setEditor({ category: null })}><Plus size={17} aria-hidden="true" />{t.addCategory}</Button></div><section className="table-card"><div className="table-filters"><div className="search-input"><Search size={17} aria-hidden="true" /><input value={search} maxLength={100} aria-label={t.search} placeholder={t.search} onChange={event => { setSearch(event.target.value); setPage(0) }} /></div><select aria-label={t.status} value={active} onChange={event => { setActive(event.target.value); setPage(0) }}><option value="">{t.allStatuses}</option><option value="true">{t.active}</option><option value="false">{t.inactive}</option></select></div><KnowledgeState pending={categories.isPending} error={categories.error} retry={() => void categories.refetch()}>{categories.data && <><div className="table-scroll" tabIndex={0} role="region" aria-label={t.categories}><table><thead><tr><th>{t.categoryNameKK}</th><th>{t.categoryNameRU}</th><th>{t.status}</th><th>{t.actions}</th></tr></thead><tbody>{categories.data.items.map(category => <tr key={category.id}><td><strong>{category.name_kk}</strong><span className="table-secondary">{category.slug}</span></td><td>{category.name_ru}</td><td><Badge className={category.is_active ? 'active-status' : 'inactive-status'}>{category.is_active ? t.active : t.inactive}</Badge></td><td><Button size="sm" variant="ghost" onClick={() => setEditor({ category })} aria-label={`${t.edit}: ${category.name_kk}`}>{t.edit}</Button></td></tr>)}</tbody></table>{categories.data.items.length === 0 && <div className="table-state"><FolderOpen size={28} aria-hidden="true" /><p>{t.noCategories}</p></div>}</div><Pagination page={page} limit={limit} total={categories.data.total} busy={categories.isFetching} onChange={setPage} /></>}</KnowledgeState></section>{editor && <CategoryEditor category={editor.category} onClose={() => setEditor(null)} />}</>
}

import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useFieldArray, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ArrowLeft, ImagePlus, Plus, Trash2, Upload, X } from 'lucide-react'
import { Button } from '../components/ui/button'
import { FormField, fieldDescription } from '../components/forms/FormField'
import { Modal } from '../components/forms/Modal'
import { KnowledgeState } from '../components/KnowledgeState'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { ApiError } from '../lib/api'
import { articleResultSchema, knowledgePath, mediaResultSchema, mediaURL, validMediaFile, type Article, type MediaPurpose } from '../lib/knowledge'
import { useCategories } from '../lib/useCategories'
import { ticketDetailSchema, type Ticket } from '../lib/tickets'

function EditorForm({ article, sourceTicket, onRefresh }: { article: Article | null; sourceTicket?: Ticket; onRefresh?: () => Promise<void> }) {
  const { t, locale } = useLocale()
  const { api } = useAuth()
  const notify = useToast()
  const message = useErrorMessage()
  const navigate = useNavigate()
  const cache = useQueryClient()
  const categories = useCategories()
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState<MediaPurpose | null>(null)
  const [conflict, setConflict] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [videoID, setVideoID] = useState(article?.video_id ?? null)
  const [imageIDs, setImageIDs] = useState(article?.image_ids ?? [])
  const [videoName, setVideoName] = useState('')
  // This version belongs to the form's initial values, not later query refetches.
  const editingVersion = useRef(sourceTicket?.version ?? article?.version).current
  const controller = useRef(new AbortController())
  useEffect(() => { const current = new AbortController(); controller.current = current; return () => current.abort() }, [])
  const text = (max: number, required = true) => z.string().trim().refine(value => (!required || value.length > 0) && Array.from(value).length <= max && !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(value), t.articleValidation)
  const schema = z.object({ ru_title: text(250, false).refine(value => !/[\u0000-\u001f\u007f]/.test(value), t.articleValidation), ru_description: text(2000, false), ru_symptoms: text(10_000, false), ru_steps: text(50_000, false), title: text(250).refine(value => !/[\u0000-\u001f\u007f]/.test(value), t.articleValidation), slug: z.string().trim().max(200, t.slugRule).regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, t.slugRule), description: text(2000), category_id: z.string().min(1, t.categoryRequired), symptoms: text(10_000), steps: z.array(z.object({ value: text(5000) })).min(1, t.articleValidation).max(100, t.articleValidation), tags: z.string().refine(value => { const items = value.split(',').map(tag => tag.trim()).filter(Boolean); return items.length <= 10 && items.every(tag => Array.from(tag).length <= 60 && !/[\u0000-\u001f\u007f]/.test(tag)) }, t.articleValidation), is_published: z.boolean() }).superRefine((values, context) => {
    const required = !!article?.translation_ru || [values.ru_title, values.ru_description, values.ru_symptoms, values.ru_steps].some(value => value.trim().length > 0)
    if (!required) return
    for (const key of ['ru_title', 'ru_description', 'ru_symptoms', 'ru_steps'] as const) {
      if (!values[key].trim()) context.addIssue({ code: 'custom', path: [key], message: locale === 'kk' ? 'Орысша нұсқаның барлық өрісін толтырыңыз.' : 'Заполните все поля русского перевода.' })
    }
    const translatedSteps = values.ru_steps.split('\n').map(step => step.trim()).filter(Boolean)
    if (translatedSteps.length < 1 || translatedSteps.length > 100 || translatedSteps.some(step => Array.from(step).length > 5000)) context.addIssue({ code: 'custom', path: ['ru_steps'], message: t.articleValidation })
  })
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { ru_title: article?.translation_ru?.title ?? '', ru_description: article?.translation_ru?.description ?? '', ru_symptoms: article?.translation_ru?.symptoms ?? '', ru_steps: article?.translation_ru?.solution_steps.join('\n') ?? '', title: article?.title ?? sourceTicket?.title ?? '', slug: article?.slug ?? '', description: article?.description ?? Array.from(sourceTicket?.description ?? '').slice(0, 2000).join(''), category_id: article?.category_id ?? sourceTicket?.category_id ?? '', symptoms: article?.symptoms ?? Array.from(sourceTicket?.description ?? '').slice(0, 10_000).join(''), steps: article?.solution_steps.map(value => ({ value })) ?? [{ value: '' }], tags: article?.tags.join(', ') ?? '', is_published: article?.is_published ?? false } })
  const steps = useFieldArray({ control: form.control, name: 'steps' })
  const disabled = busy || uploading !== null
  const upload = async (file: File | undefined, purpose: MediaPurpose) => {
    if (!file) return
    if (!validMediaFile(file, purpose) || (purpose === 'ARTICLE_IMAGE' && imageIDs.length >= 10)) { notify({ kind: 'error', message: t.fileInvalid }); return }
    setUploading(purpose)
    try {
      const body = new FormData(); body.append('file', file)
      const result = await api(`/media?purpose=${purpose}`, mediaResultSchema, { method: 'POST', body, signal: controller.current.signal })
      if (purpose === 'ARTICLE_IMAGE') setImageIDs(previous => [...previous, result.media.id])
      else { setVideoID(result.media.id); setVideoName(result.media.original_filename) }
      notify({ kind: 'success', message: t.fileUploaded })
    } catch (error) { if (!controller.current.signal.aborted) notify({ kind: 'error', message: message(error) }) }
    finally { setUploading(null) }
  }
  const save = form.handleSubmit(async values => {
    const body = { ...(!sourceTicket && values.ru_title.trim() ? { translation_ru: { title: values.ru_title.trim(), description: values.ru_description.trim(), symptoms: values.ru_symptoms.trim(), solution_steps: values.ru_steps.split('\n').map(step => step.trim()).filter(Boolean) } } : {}), title: values.title, slug: values.slug, description: values.description, category_id: values.category_id, symptoms: values.symptoms, solution_steps: values.steps.map(step => step.value), tags: [...new Set(values.tags.split(',').map(tag => tag.trim()).filter(Boolean))], ...(sourceTicket ? { version: editingVersion } : { video_id: videoID, image_ids: imageIDs, is_published: values.is_published, ...(article ? { version: editingVersion } : {}) }) }
    if (new TextEncoder().encode(JSON.stringify(body)).length > 64 * 1024) { notify({ kind: 'error', message: t.articleValidation }); return }
    setBusy(true)
    try {
      const result = await api(sourceTicket ? `/tickets/${sourceTicket.id}/article` : article ? `${knowledgePath}/articles/${article.id}` : `${knowledgePath}/articles`, articleResultSchema, { method: article ? 'PATCH' : 'POST', body })
      await cache.invalidateQueries({ queryKey: ['knowledge'] })
      if (sourceTicket) await cache.invalidateQueries({ queryKey: ['tickets'] })
      notify({ kind: 'success', message: sourceTicket ? t.ticketDraftCreated : t.articleSaved })
      navigate(sourceTicket ? `/engineer/knowledge/${result.article.id}/edit` : `/knowledge/articles/${result.article.id}`)
    } catch (error) {
      if (error instanceof ApiError && error.code === 'version_conflict') setConflict(true)
      else notify({ kind: 'error', message: message(error) })
    } finally { setBusy(false) }
  })
  const remove = async () => {
    if (!article) return
    setBusy(true)
    try {
      await api(`${knowledgePath}/articles/${article.id}`, z.undefined(), { method: 'DELETE', body: { version: editingVersion } })
      await cache.invalidateQueries({ queryKey: ['knowledge'] })
      notify({ kind: 'success', message: t.articleDeleted }); navigate('/engineer/knowledge')
    } catch (error) {
      if (error instanceof ApiError && error.code === 'version_conflict') setConflict(true)
      else notify({ kind: 'error', message: message(error) })
      setDeleting(false)
    } finally { setBusy(false) }
  }
  return <><div className="article-navigation"><Link to={sourceTicket ? `/tickets/${sourceTicket.id}` : "/engineer/knowledge"}><ArrowLeft size={16} aria-hidden="true" />{sourceTicket ? t.backTickets : t.knowledgeManage}</Link>{article && <Button variant="outline" size="sm" onClick={() => setDeleting(true)} disabled={disabled}><Trash2 size={14} aria-hidden="true" />{t.deleteArticle}</Button>}</div><div className="page-heading"><div><h1>{sourceTicket ? t.ticketToArticle : article ? t.editArticle : t.addArticle}</h1><p>{sourceTicket ? t.ticketToArticleDescription : t.articleSafety}</p></div></div>{conflict && <div className="version-conflict" role="alert"><p>{sourceTicket ? t.ticketVersionConflict : t.versionConflict}</p><a href={sourceTicket ? `/tickets/${sourceTicket.id}` : `/knowledge/articles/${article!.id}`} target="_blank" rel="noopener noreferrer">{t.viewLatest}</a>{sourceTicket && <Button type="button" variant="outline" size="sm" onClick={() => setRefreshing(true)}>{t.refreshTicket}</Button>}</div>}<form className="article-editor content-card" onSubmit={save} noValidate>
    <h2>{locale === 'kk' ? 'Қазақша нұсқа' : 'Казахская версия'}</h2>
    <FormField id="article-title" label={t.articleTitle} error={form.formState.errors.title?.message}><input id="article-title" {...form.register('title')} disabled={disabled} aria-invalid={!!form.formState.errors.title} aria-describedby={fieldDescription('article-title', form.formState.errors.title?.message)} /></FormField>
    <div className="form-two-columns"><FormField id="article-slug" label={t.slug} hint={t.slugRule} error={form.formState.errors.slug?.message}><input id="article-slug" {...form.register('slug')} disabled={disabled} aria-invalid={!!form.formState.errors.slug} aria-describedby={fieldDescription('article-slug', form.formState.errors.slug?.message, 'hint')} /></FormField><FormField id="article-category" label={t.category} error={form.formState.errors.category_id?.message}><select id="article-category" {...form.register('category_id')} disabled={disabled || categories.isPending || categories.isError} aria-invalid={!!form.formState.errors.category_id}><option value="">{t.categoryRequired}</option>{article && !categories.data?.some(category => category.id === article.category_id) && <option value={article.category_id}>{locale === 'kk' ? article.category_name_kk : article.category_name_ru} ({t.inactive})</option>}{categories.data?.map(category => <option key={category.id} value={category.id}>{locale === 'kk' ? category.name_kk : category.name_ru}</option>)}</select></FormField></div>
    {categories.isError && <p role="alert" className="inline-error">{message(categories.error)} <Button type="button" variant="ghost" size="sm" onClick={() => void categories.refetch()}>{t.retry}</Button></p>}{categories.data?.length === 0 && <p className="inline-error" role="alert">{t.noCategories}</p>}
    <FormField id="article-description" label={t.description} error={form.formState.errors.description?.message}><textarea id="article-description" rows={3} {...form.register('description')} disabled={disabled} aria-invalid={!!form.formState.errors.description} aria-describedby={fieldDescription('article-description', form.formState.errors.description?.message)} /></FormField>
    <FormField id="article-symptoms" label={t.symptoms} error={form.formState.errors.symptoms?.message}><textarea id="article-symptoms" rows={4} {...form.register('symptoms')} disabled={disabled} aria-invalid={!!form.formState.errors.symptoms} aria-describedby={fieldDescription('article-symptoms', form.formState.errors.symptoms?.message)} /></FormField>
    <fieldset className="editor-steps"><legend>{t.solution}</legend>{steps.fields.map((step, index) => <div className="editor-step" key={step.id}><FormField id={`article-step-${index}`} label={`${t.step} ${index + 1}`} error={form.formState.errors.steps?.[index]?.value?.message}><textarea id={`article-step-${index}`} rows={3} {...form.register(`steps.${index}.value`)} disabled={disabled} aria-invalid={!!form.formState.errors.steps?.[index]?.value} aria-describedby={fieldDescription(`article-step-${index}`, form.formState.errors.steps?.[index]?.value?.message)} /></FormField><Button variant="ghost" size="icon" type="button" disabled={disabled || steps.fields.length <= 1} aria-label={`${t.removeStep} ${index + 1}`} onClick={() => steps.remove(index)}><X size={17} aria-hidden="true" /></Button></div>)}<Button variant="outline" size="sm" type="button" disabled={disabled || steps.fields.length >= 100} onClick={() => steps.append({ value: '' })}><Plus size={15} aria-hidden="true" />{t.addStep}</Button></fieldset>
    {!sourceTicket && <section className="translation-editor"><h2>{locale === 'kk' ? 'Орысша нұсқа' : 'Русская версия'}</h2><p className="field-hint">{locale === 'kk' ? 'Тіл ауысқанда осы мәтін көрсетіледі. Аударма қоссаңыз, барлық өрісін толтырыңыз; екі тілдегі мазмұнды бірге жаңартыңыз.' : 'Этот текст показывается при выборе русского языка. Заполните перевод полностью; обновляйте обе языковые версии вместе.'}</p>
      <FormField id="article-ru-title" label={locale === 'kk' ? 'Мақала атауы (орысша)' : 'Заголовок (русский)'} error={form.formState.errors.ru_title?.message}><input id="article-ru-title" {...form.register('ru_title')} disabled={disabled} aria-invalid={!!form.formState.errors.ru_title} /></FormField>
      <FormField id="article-ru-description" label={locale === 'kk' ? 'Қысқаша сипаттама (орысша)' : 'Описание (русский)'} error={form.formState.errors.ru_description?.message}><textarea id="article-ru-description" rows={3} {...form.register('ru_description')} disabled={disabled} aria-invalid={!!form.formState.errors.ru_description} /></FormField>
      <FormField id="article-ru-symptoms" label={locale === 'kk' ? 'Мәселенің белгілері (орысша)' : 'Симптомы (русский)'} error={form.formState.errors.ru_symptoms?.message}><textarea id="article-ru-symptoms" rows={4} {...form.register('ru_symptoms')} disabled={disabled} aria-invalid={!!form.formState.errors.ru_symptoms} /></FormField>
      <FormField id="article-ru-steps" label={locale === 'kk' ? 'Шешу қадамдары (орысша)' : 'Шаги решения (русский)'} hint={locale === 'kk' ? 'Әр қадамды жаңа жолдан жазыңыз, нөмір қоспаңыз.' : 'Каждый шаг с новой строки, без нумерации.'} error={form.formState.errors.ru_steps?.message}><textarea id="article-ru-steps" rows={8} {...form.register('ru_steps')} disabled={disabled} aria-invalid={!!form.formState.errors.ru_steps} /></FormField>
    </section>}
    <FormField id="article-tags" label={t.tags} hint={t.tagsHint} error={form.formState.errors.tags?.message}><input id="article-tags" {...form.register('tags')} disabled={disabled} aria-invalid={!!form.formState.errors.tags} aria-describedby={fieldDescription('article-tags', form.formState.errors.tags?.message, 'hint')} /></FormField>
    {!sourceTicket && <div className="media-editor-grid"><section className="media-editor"><h2>{t.video}</h2><p className="field-hint">{t.videoRule}</p><label className={`upload-field ${disabled ? 'disabled' : ''}`}><Upload size={18} aria-hidden="true" /><span>{uploading === 'ARTICLE_VIDEO' ? t.uploading : t.uploadVideo}</span><input type="file" accept="video/mp4,video/webm" aria-label={t.uploadVideo} disabled={disabled} onChange={event => { void upload(event.target.files?.[0], 'ARTICLE_VIDEO'); event.target.value = '' }} /></label>{videoID && <><video controls preload="metadata" className="article-video" src={mediaURL(videoID)} aria-label={t.video} />{videoName && <p className="field-hint">{videoName}</p>}<Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => { setVideoID(null); setVideoName('') }}><X size={13} aria-hidden="true" />{t.removeMedia}</Button></>}</section><section className="media-editor"><h2>{t.screenshots}</h2><p className="field-hint">{t.imageRule}</p><label className={`upload-field ${disabled ? 'disabled' : ''}`}><ImagePlus size={18} aria-hidden="true" /><span>{uploading === 'ARTICLE_IMAGE' ? t.uploading : t.uploadImage}</span><input type="file" accept="image/png,image/jpeg" aria-label={t.uploadImage} disabled={disabled || imageIDs.length >= 10} onChange={event => { void upload(event.target.files?.[0], 'ARTICLE_IMAGE'); event.target.value = '' }} /></label><div className="editor-images">{imageIDs.map((id, index) => <div key={id}><img src={mediaURL(id)} alt={`${t.screenshots} ${index + 1}`} /><Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => setImageIDs(previous => previous.filter(value => value !== id))} aria-label={`${t.removeMedia} ${index + 1}`}><X size={13} aria-hidden="true" />{t.removeMedia}</Button></div>)}</div></section></div>}{!sourceTicket && <><p className="field-hint">{t.unpublishedMedia}</p><label className="checkbox-field"><input type="checkbox" {...form.register('is_published')} disabled={disabled} />{t.publishArticle}</label></>}<div className="form-actions"><Button asChild variant="outline"><Link to={sourceTicket ? `/tickets/${sourceTicket.id}` : "/engineer/knowledge"}>{t.cancel}</Link></Button><Button type="submit" disabled={disabled || categories.isPending || categories.isError || (!article && categories.data?.length === 0) || conflict}>{busy ? t.saving : t.save}</Button></div>
  </form>{deleting && <Modal title={t.deleteArticle} description={t.deleteArticleDescription} onClose={() => setDeleting(false)} busy={busy}><div className="confirm-content"><p><strong>{article?.title}</strong></p><div className="form-actions"><Button autoFocus variant="outline" onClick={() => setDeleting(false)} disabled={busy}>{t.cancel}</Button><Button onClick={() => void remove()} disabled={busy}>{busy ? t.saving : t.confirm}</Button></div></div></Modal>}{refreshing && <Modal title={t.refreshTicket} description={t.refreshTicketDescription} onClose={() => setRefreshing(false)} busy={busy}><div className="form-actions"><Button autoFocus variant="outline" disabled={busy} onClick={() => setRefreshing(false)}>{t.cancel}</Button><Button disabled={busy} onClick={async () => { setBusy(true); try { await onRefresh?.() } catch (error) { notify({ kind: "error", message: message(error) }); setBusy(false) } }}>{busy ? t.loading : t.refreshTicket}</Button></div></Modal>}</>
}

function TicketArticleEditor({ ticket, onRefresh }: { ticket: Ticket; onRefresh: () => Promise<void> }) {
  const { t } = useLocale()
  const initial = useRef(ticket).current
  if (initial.converted_article_id) return <div className="table-state"><p>{t.ticketAlreadyConverted}</p><Button asChild><Link to={`/engineer/knowledge/${initial.converted_article_id}/edit`}>{t.openTicketDraft}</Link></Button></div>
  if (!['RESOLVED', 'CLOSED'].includes(initial.status)) return <div className="table-state"><p role="alert">{t.invalidTransition}</p><Link to={`/tickets/${initial.id}`}>{t.backTickets}</Link></div>
  return <>{ticket.version > initial.version && <p className="version-conflict" role="status">{t.ticketNewVersion}</p>}<EditorForm article={null} sourceTicket={initial} onRefresh={onRefresh} /></>
}

export function ArticleEditor() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const sourceID = params.get('ticket_id') ?? ''
  const [refreshKey, setRefreshKey] = useState(0)
  const { api } = useAuth()
  const { t } = useLocale()
  const message = useErrorMessage()
  const article = useQuery({ queryKey: ['knowledge', 'article', id], enabled: !!id, queryFn: ({ signal }) => api(`${knowledgePath}/articles/${encodeURIComponent(id!)}`, articleResultSchema, { signal }) })
  const source = useQuery({ queryKey: ['tickets', 'detail', sourceID], enabled: !id && !!sourceID, queryFn: ({ signal }) => api(`/tickets/${encodeURIComponent(sourceID)}`, ticketDetailSchema, { signal }) })
  if (!id && sourceID) {
    const sourceTransient = !!source.data && source.error instanceof ApiError && (source.error.status === 0 || source.error.status >= 500)
    return <>{sourceTransient && <p role="alert" className="inline-error">{message(source.error)}</p>}<KnowledgeState pending={source.isPending} error={sourceTransient ? undefined : source.error} retry={() => void source.refetch()}>{source.data && <TicketArticleEditor key={`${source.data.ticket.id}:${refreshKey}`} ticket={source.data.ticket} onRefresh={async () => { const result = await source.refetch(); if (result.error) throw result.error; setRefreshKey(value => value + 1) }} />}</KnowledgeState></>
  }
  if (!id) return <EditorForm article={null} />
  const transient = !!article.data && article.error instanceof ApiError && (article.error.status === 0 || article.error.status >= 500)
  return <>{transient && <p className="inline-error" role="alert">{message(article.error)} <Button variant="ghost" size="sm" onClick={() => void article.refetch()}>{t.retry}</Button></p>}<KnowledgeState pending={article.isPending} error={transient ? undefined : article.error} retry={() => void article.refetch()}>{article.data && <EditorForm key={article.data.article.id} article={article.data.article} />}</KnowledgeState></>
}

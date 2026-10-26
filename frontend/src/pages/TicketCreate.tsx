import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, BookOpen, CheckCircle2, Search, ThumbsDown, X, ThumbsUp } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { FormField } from '../components/forms/FormField'
import { TicketUploads } from '../components/TicketUploads'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { queryString } from '../lib/api'
import { articleResultSchema, localizeArticle, articlesPageSchema, feedbackSchema, knowledgePath, type Article } from '../lib/knowledge'
import { useCategories } from '../lib/useCategories'
import { ticketPriorities, ticketPriorityKey, ticketResultSchema, validTicketText, type TicketPriority } from '../lib/tickets'
import { useSettings } from '../lib/settings'

function SuggestedArticle({ article: original, attempt, disabled, onResolved, onReviewed, onBusyChange, onChoice }: {
  article: Article; attempt: string; disabled: boolean; onResolved: () => void; onReviewed: (id: string) => void; onBusyChange: (busy: boolean) => void; onChoice: (helpful: boolean) => void
}) {
  const { api } = useAuth()
  const { t, locale } = useLocale()
  const article = localizeArticle(original, locale)
  const message = useErrorMessage()
  const [choice, setChoice] = useState<boolean | null>(null)
  const [recorded, setRecorded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [failure, setFailure] = useState<unknown>(null)
  const answer = async (helpful: boolean) => {
    setChoice(helpful); setBusy(true); setFailure(null); onBusyChange(true); onChoice(helpful)
    try {
      await api(`${knowledgePath}/articles/${article.id}/feedback`, feedbackSchema, { method: 'POST', body: { attempt_id: attempt, helpful } })
      setRecorded(true); onReviewed(article.id)
      if (helpful) onResolved()
    } catch (error) { setFailure(error) }
    finally { setBusy(false); onBusyChange(false) }
  }
  return <article className="article-card suggestion-card"><Badge>{locale === 'kk' ? article.category_name_kk : article.category_name_ru}</Badge><h3>{article.title}</h3><p>{article.description}</p><details onToggle={event => { if (event.currentTarget.open) onReviewed(article.id) }}><summary><BookOpen size={15} aria-hidden="true" />{t.openArticle}</summary><p className="preserve-lines">{article.symptoms}</p><ol>{article.solution_steps.map((step, index) => <li key={index} className="preserve-lines">{step}</li>)}</ol><Link to={`/knowledge/articles/${article.id}`} target="_blank" rel="noopener noreferrer" onClick={() => onReviewed(article.id)}>{t.openArticle} ↗</Link></details><div className="suggestion-feedback"><p>{t.helpfulQuestion}</p>{recorded ? <p role="status">{choice ? t.helpfulResolved : t.helpfulUnresolved}</p> : <><div className="feedback-buttons"><Button type="button" variant="outline" size="sm" disabled={disabled || busy || choice !== null} onClick={() => void answer(true)}><ThumbsUp size={14} aria-hidden="true" />{t.helpfulYes}</Button><Button type="button" variant="outline" size="sm" disabled={disabled || busy || choice !== null} onClick={() => void answer(false)}><ThumbsDown size={14} aria-hidden="true" />{t.helpfulNo}</Button></div>{failure != null && <p className="inline-error" role="alert">{message(failure)} <Button type="button" variant="ghost" size="sm" disabled={disabled || busy} onClick={() => void answer(choice!)}>{t.feedbackRetry}</Button></p>}</>}</div></article>
}

export function TicketCreate() {
  const { t, locale } = useLocale()
  const { api } = useAuth()
  const cache = useQueryClient()
  const notify = useToast()
  const message = useErrorMessage()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const location = useLocation()
  const seed = location.state as { title?: string; description?: string; category_id?: string } | null
  const sourceID = params.get('article_id') ?? ''
  const source = useQuery({ queryKey: ['knowledge', 'article', sourceID], enabled: !!sourceID, queryFn: ({ signal }) => api(`${knowledgePath}/articles/${encodeURIComponent(sourceID)}`, articleResultSchema, { signal }) })
  const categories = useCategories()
  const settings = useSettings()
  const [title, setTitle] = useState(seed?.title ?? '')
  const [description, setDescription] = useState(seed?.description ?? '')
  const [category, setCategory] = useState(seed?.category_id ?? '')
  const [priority, setPriority] = useState<TicketPriority>('MEDIUM')
  const priorityChosen = useRef(false)
  const [priorityReady, setPriorityReady] = useState(false)
  useEffect(() => { if (settings.data && !priorityChosen.current) { setPriority(settings.data.settings.default_ticket_priority); priorityChosen.current = true; setPriorityReady(true) } }, [settings.data])
  const [stage, setStage] = useState<'describe' | 'suggestions' | 'create' | 'resolved'>('describe')
  const [search, setSearch] = useState<{ q: string; category: string; attempt: string } | null>(null)
  const [related, setRelated] = useState<string[]>([])
  const [includeSource, setIncludeSource] = useState(true)
  const [attachments, setAttachments] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [feedbackBusy, setFeedbackBusy] = useState(false)
  const [helpfulArticle, setHelpfulArticle] = useState('')
  const [invalid, setInvalid] = useState(false)
  const submitting = useRef(false)
  const suggestions = useQuery({ queryKey: ['tickets', 'suggestions', search?.q, search?.category, search?.attempt], enabled: !!search, queryFn: ({ signal }) => api(`/tickets/suggestions?${queryString({ q: search!.q, category_id: search!.category })}`, articlesPageSchema, { signal }) })
  const find = () => {
    if (busy || uploading || feedbackBusy || helpfulArticle) return
    if (!validTicketText(description, 20_000)) { setInvalid(true); return }
    setInvalid(false)
    setRelated([])
    setSearch({ q: Array.from(description.trim().replace(/\s+/g, ' ')).slice(0, 100).join(''), category, attempt: crypto.randomUUID() })
    setStage('suggestions')
  }
  const reviewed = (id: string) => setRelated(previous => previous.includes(id) || previous.length >= 10 ? previous : [...previous, id])
  const create = async () => {
    if (submitting.current || uploading || feedbackBusy || helpfulArticle || (!priorityReady && !settings.isError)) return
    if (!validTicketText(title, 250) || /[\u0000-\u001f\u007f]/.test(title) || !validTicketText(description, 20_000)) { setInvalid(true); return }
    const articleIDs = [...new Set([...related, ...(includeSource && source.data?.article.is_published && source.data.article.category_active ? [source.data.article.id] : [])])].slice(0, 10)
    setInvalid(false); setBusy(true); submitting.current = true
    try {
      const result = await api('/tickets', ticketResultSchema, { method: 'POST', body: { title: title.trim(), description: description.trim(), priority, category_id: category || null, article_ids: articleIDs, attachment_ids: attachments } })
      await cache.invalidateQueries({ queryKey: ['tickets'] })
      notify({ kind: 'success', message: t.ticketCreated }); navigate(`/tickets/${result.ticket.id}`)
    } catch (error) { notify({ kind: 'error', message: message(error) }) }
    finally { setBusy(false); submitting.current = false }
  }
  const disabled = busy || uploading || feedbackBusy
  const blocked = disabled || helpfulArticle !== ''
  const descriptionFields = <><FormField id="ticket-description" label={t.ticketDescription} error={invalid ? t.ticketValidation : undefined}><textarea id="ticket-description" rows={5} value={description} onChange={event => setDescription(event.target.value)} disabled={blocked} aria-invalid={invalid} aria-describedby={invalid ? 'ticket-description-error' : undefined} /></FormField><FormField id="ticket-category" label={t.category}><select id="ticket-category" value={category} onChange={event => setCategory(event.target.value)} disabled={blocked || categories.isPending || categories.isError}><option value="">{t.optionalCategory}</option>{categories.data?.map(item => <option key={item.id} value={item.id}>{locale === 'kk' ? item.name_kk : item.name_ru}</option>)}</select></FormField>{categories.isError && <p className="inline-error" role="alert">{message(categories.error)} <Button type="button" variant="ghost" size="sm" onClick={() => void categories.refetch()}>{t.retry}</Button></p>}</>
  if (stage === 'resolved') return <section className="content-card ticket-self-service" role="status"><CheckCircle2 size={40} aria-hidden="true" /><h1>{t.helpfulResolved}</h1><Button asChild><Link to="/knowledge">{t.backKnowledge}</Link></Button></section>
  return <><div className="article-navigation"><Link to="/tickets"><ArrowLeft size={16} aria-hidden="true" />{t.backTickets}</Link></div><div className="page-heading"><div><h1>{t.newTicket}</h1><p>{t.suggestionDescription}</p></div></div>{stage !== 'create' ? <><form className="article-editor content-card" onSubmit={event => { event.preventDefault(); find() }}>{descriptionFields}<Button type="submit" disabled={blocked || suggestions.isFetching}><Search size={17} aria-hidden="true" />{t.findSuggestions}</Button></form>{stage === 'suggestions' && <section className="ticket-suggestions" aria-labelledby="suggestions-title"><h2 id="suggestions-title">{t.suggestionTitle}</h2>{suggestions.isPending ? <p role="status">{t.loading}</p> : suggestions.isError ? <div className="inline-error" role="alert">{t.suggestionUnavailable} <Button variant="outline" onClick={() => void suggestions.refetch()}>{t.retry}</Button></div> : <><div className="article-grid">{suggestions.data?.items.filter(article => article.is_published && article.category_active).map(article => <SuggestedArticle key={`${search!.attempt}:${article.id}`} article={article} attempt={search!.attempt} disabled={disabled || (!!helpfulArticle && helpfulArticle !== article.id)} onResolved={() => setStage('resolved')} onReviewed={reviewed} onBusyChange={setFeedbackBusy} onChoice={helpful => { if (helpful) setHelpfulArticle(article.id) }} />)}</div>{suggestions.data?.items.length === 0 && <p className="table-state content-card">{t.noSuggestions}</p>}<div className="form-actions"><Button onClick={() => setStage('create')} disabled={blocked}>{t.unresolved}</Button></div></>}</section>}</> : <form className="article-editor content-card" onSubmit={event => { event.preventDefault(); void create() }} noValidate><FormField id="ticket-title" label={t.ticketTitle} error={invalid ? t.ticketValidation : undefined}><input id="ticket-title" value={title} onChange={event => setTitle(event.target.value)} disabled={blocked} aria-invalid={invalid} aria-describedby={invalid ? 'ticket-title-error' : undefined} /></FormField>{descriptionFields}{settings.isError && <p className="field-hint">{t.settingsUnavailableDefault}</p>}<FormField id="ticket-priority" label={t.priority}><select id="ticket-priority" value={priority} onChange={event => { priorityChosen.current = true; setPriorityReady(true); setPriority(event.target.value as TicketPriority) }} disabled={blocked}>{ticketPriorities.map(value => <option key={value} value={value}>{t[ticketPriorityKey[value]]}</option>)}</select></FormField>{(related.length > 0 || source.data) && <div className="ticket-related"><h2>{t.relatedArticles}</h2>{source.isError && <p role="alert" className="inline-error">{message(source.error)}</p>}{[...new Set([...related, ...(includeSource && source.data ? [source.data.article.id] : [])])].slice(0, 10).map(id => { const article = source.data?.article.id === id ? source.data.article : suggestions.data?.items.find(item => item.id === id); return article && <div className="ticket-related-item" key={id}><Link to={`/knowledge/articles/${id}`} target="_blank" rel="noopener noreferrer">{localizeArticle(article, locale).title} ↗</Link><Button type="button" size="icon" variant="ghost" disabled={blocked} aria-label={`${t.removeRelatedArticle}: ${localizeArticle(article, locale).title}`} onClick={() => { setRelated(previous => previous.filter(value => value !== id)); if (source.data?.article.id === id) setIncludeSource(false) }}><X size={14} aria-hidden="true" /></Button></div> })}</div>}<TicketUploads ids={attachments} onChange={setAttachments} disabled={busy} onBusyChange={setUploading} /><p className="field-hint">{t.articleSafety}</p><div className="form-actions"><Button type="button" variant="outline" disabled={blocked} onClick={() => setStage('suggestions')}>{t.tryAnotherGuide}</Button><Button type="submit" disabled={blocked || (!priorityReady && !settings.isError) || categories.isPending || categories.isError || (!!sourceID && source.isPending)}>{busy ? t.saving : t.createTicket}</Button></div></form>}</>
}

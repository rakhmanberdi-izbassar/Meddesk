import { useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, CheckCircle2, RefreshCw } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { FormField } from '../components/forms/FormField'
import { Modal } from '../components/forms/Modal'
import { TicketUploads } from '../components/TicketUploads'
import { KnowledgeState } from '../components/KnowledgeState'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { ApiError } from '../lib/api'
import { hasStaffRole, mediaURL } from '../lib/knowledge'
import { assigneesSchema, ticketCommentResultSchema, ticketDetailSchema, ticketPriorities, ticketPriorityKey, ticketResultSchema, ticketStatusKey, ticketTransitions, validTicketText, type Ticket, type TicketDetail as Detail, type TicketPriority, type TicketStatus } from '../lib/tickets'

function TicketContent({ data, reload }: { data: Detail; reload: () => Promise<Detail | undefined> }) {
  const { t, locale } = useLocale()
  const { api, user } = useAuth()
  const notify = useToast()
  const message = useErrorMessage()
  const cache = useQueryClient()
  // Inputs and version are one snapshot. Query refetches never replace this state.
  const [ticket, setTicket] = useState(data.ticket)
  const [priority, setPriority] = useState<TicketPriority>(data.ticket.priority)
  const [assignee, setAssignee] = useState(data.ticket.assignee_id ?? '')
  const [status, setStatus] = useState<TicketStatus | ''>('')
  const [reason, setReason] = useState('')
  const [comment, setComment] = useState('')
  const [internal, setInternal] = useState(false)
  const [attachments, setAttachments] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [commentInvalid, setCommentInvalid] = useState(false)
  const [reasonInvalid, setReasonInvalid] = useState(false)
  const pending = useRef(false)
  const staff = hasStaffRole(user?.role)
  const owner = user?.id === ticket.requester_id
  const closed = ticket.status === 'CLOSED'
  const disabled = busy || uploading || conflict
  const assignees = useQuery({ queryKey: ['tickets', 'assignees'], enabled: staff, queryFn: ({ signal }) => api('/tickets/assignees', assigneesSchema, { signal }) })
  const date = (value: string) => new Date(value).toLocaleString(locale === 'kk' ? 'kk-KZ' : 'ru-RU')
  const updated = async (next: Ticket) => {
    setTicket(next)
    cache.setQueryData<Detail>(['tickets', 'detail', ticket.id], previous => previous ? { ...previous, ticket: next } : previous)
    await cache.invalidateQueries({ queryKey: ['tickets'] })
  }
  const mutate = async (action: () => Promise<Ticket>, success: string, after?: () => void) => {
    if (pending.current || uploading || conflict) return
    pending.current = true; setBusy(true)
    try { const result = await action(); await updated(result); after?.(); notify({ kind: 'success', message: success }) }
    catch (error) {
      if (error instanceof ApiError && error.code === 'version_conflict') { setConflict(true); setConfirming(false) }
      else notify({ kind: 'error', message: message(error) })
    } finally { pending.current = false; setBusy(false) }
  }
  const management = () => void mutate(async () => (await api(`/tickets/${ticket.id}`, ticketResultSchema, { method: 'PATCH', body: { version: ticket.version, ...(priority !== ticket.priority ? { priority } : {}), ...((assignee || null) !== ticket.assignee_id ? { assignee_id: assignee || null } : {}) } })).ticket, t.ticketSaved)
  const transition = () => {
    if (!status || !validTicketText(reason, 2000, status === 'CLOSED')) { setReasonInvalid(true); return }
    setReasonInvalid(false)
    void mutate(async () => (await api(`/tickets/${ticket.id}`, ticketResultSchema, { method: 'PATCH', body: { version: ticket.version, status, reason: reason.trim() } })).ticket, t.ticketSaved, () => { setStatus(''); setReason('') })
  }
  const reopen = () => {
    if (!validTicketText(reason, 2000)) { setReasonInvalid(true); return }
    setReasonInvalid(false)
    void mutate(async () => (await api(`/tickets/${ticket.id}/reopen`, ticketResultSchema, { method: 'POST', body: { version: ticket.version, reason: reason.trim() } })).ticket, t.ticketSaved, () => setReason(''))
  }
  const send = () => {
    if (!validTicketText(comment, 10_000)) { setCommentInvalid(true); return }
    setCommentInvalid(false)
    void mutate(async () => {
      const result = await api(`/tickets/${ticket.id}/comments`, ticketCommentResultSchema, { method: 'POST', body: { version: ticket.version, body: comment.trim(), is_internal: staff && internal } })
      cache.setQueryData<Detail>(['tickets', 'detail', ticket.id], previous => previous ? { ...previous, comments: [...previous.comments, result.comment] } : previous)
      return result.ticket
    }, t.commentSaved, () => { setComment(''); setInternal(false) })
  }
  const adoptLatest = async () => {
    if (pending.current) return
    pending.current = true; setBusy(true)
    try {
      const latest = await reload()
      if (latest) {
        setTicket(latest.ticket); setPriority(latest.ticket.priority); setAssignee(latest.ticket.assignee_id ?? '')
        setStatus(''); setReason(''); setComment(''); setInternal(false); setAttachments([])
        setConflict(false); setRefreshing(false); setCommentInvalid(false); setReasonInvalid(false)
      }
    } catch (error) { notify({ kind: 'error', message: message(error) }) }
    finally { setBusy(false); pending.current = false }
  }
  const linkedArticle = ticket.converted_article_id ?? data.ticket.converted_article_id
  return <><div className="article-navigation"><Link to="/tickets"><ArrowLeft size={16} aria-hidden="true" />{t.backTickets}</Link><Button variant="outline" size="sm" disabled={busy || uploading} onClick={() => setRefreshing(true)}><RefreshCw size={14} aria-hidden="true" />{t.refreshTicket}</Button></div>{(conflict || data.ticket.version > ticket.version) && <div className="version-conflict" role="alert"><p>{conflict ? t.ticketVersionConflict : t.ticketNewVersion}</p><a href={`/tickets/${ticket.id}`} target="_blank" rel="noopener noreferrer">{t.viewLatest}</a><Button type="button" variant="outline" size="sm" disabled={busy || uploading} onClick={() => setRefreshing(true)}>{t.refreshTicket}</Button></div>}<div className="page-heading"><div><div className="eyebrow"><span />{t.ticketNumber}{ticket.number}</div><h1>{ticket.title}</h1><p>{t.articleSafety}</p></div><div className="ticket-heading-badges"><Badge className={`ticket-status ticket-status-${ticket.status.toLowerCase()}`}>{t[ticketStatusKey[ticket.status]]}</Badge><Badge className={`ticket-priority ticket-priority-${ticket.priority.toLowerCase()}`}>{t[ticketPriorityKey[ticket.priority]]}</Badge></div></div><div className="ticket-detail-grid"><div className="ticket-main"><section className="content-card ticket-description"><h2>{t.ticketDescription}</h2><p className="preserve-lines">{ticket.description}</p><dl className="ticket-metadata"><div><dt>{t.requester}</dt><dd>{ticket.requester_name}</dd></div><div><dt>{t.department}</dt><dd>{ticket.department_name}</dd></div><div><dt>{t.assignee}</dt><dd>{ticket.assignee_name || t.unassigned}</dd></div><div><dt>{t.created}</dt><dd>{date(ticket.created_at)}</dd></div><div><dt>{t.updated}</dt><dd>{date(ticket.updated_at)}</dd></div>{ticket.resolved_at && <div><dt>{t.ticketResolved}</dt><dd>{date(ticket.resolved_at)}</dd></div>}</dl>{ticket.article_ids.length > 0 && <div className="ticket-related"><h2>{t.relatedArticles}</h2>{ticket.article_ids.map((id, index) => <Link key={id} to={`/knowledge/articles/${id}`}>{t.openArticle} {index + 1}</Link>)}</div>}{ticket.attachment_ids.length > 0 && <><h2>{t.screenshots}</h2><div className="article-images">{ticket.attachment_ids.map((id, index) => <a key={id} href={mediaURL(id)} target="_blank" rel="noopener noreferrer"><img src={mediaURL(id)} loading="lazy" alt={`${t.screenshots} ${index + 1}`} /></a>)}</div></>}</section><section className="content-card ticket-comments"><h2>{t.comments}</h2>{data.comments.filter(item => staff || !item.is_internal).map(item => <article className={`ticket-comment ${item.is_internal ? 'internal' : ''}`} key={item.id}><div><strong>{item.author_name}</strong><time dateTime={item.created_at}>{date(item.created_at)}</time>{item.is_internal && <Badge>{t.internalLabel}</Badge>}</div><p className="preserve-lines">{item.body}</p></article>)}{data.comments.filter(item => staff || !item.is_internal).length === 0 && <p className="field-hint">{t.noComments}</p>}{!closed && <form onSubmit={event => { event.preventDefault(); send() }} className="editor-form"><FormField id="ticket-comment" label={t.commentBody} error={commentInvalid ? t.commentValidation : undefined}><textarea id="ticket-comment" rows={4} value={comment} onChange={event => setComment(event.target.value)} disabled={disabled} aria-invalid={commentInvalid} aria-describedby={commentInvalid ? 'ticket-comment-error' : undefined} /></FormField>{staff && <label className="checkbox-field"><input type="checkbox" checked={internal} onChange={event => setInternal(event.target.checked)} disabled={disabled} />{t.internalComment}</label>}<div className="form-actions"><Button type="submit" disabled={disabled}>{busy ? t.saving : t.sendComment}</Button></div></form>}</section>{!closed && (staff || owner) && <section className="content-card ticket-attachments"><TicketUploads ids={attachments} onChange={setAttachments} disabled={busy || conflict} existingCount={ticket.attachment_ids.length} onBusyChange={setUploading} /><div className="form-actions"><Button disabled={disabled || attachments.length === 0} onClick={() => void mutate(async () => (await api(`/tickets/${ticket.id}/attachments`, ticketResultSchema, { method: 'POST', body: { version: ticket.version, attachment_ids: attachments } })).ticket, t.ticketSaved, () => setAttachments([]))}>{t.saveAttachments}</Button></div></section>}</div><aside className="ticket-side">{owner && ticket.status === 'RESOLVED' && <section className="content-card ticket-confirm"><CheckCircle2 size={25} aria-hidden="true" /><h2>{t.confirmResolved}</h2><p>{t.confirmResolvedDescription}</p><Button disabled={disabled} onClick={() => setConfirming(true)}>{t.confirmResolved}</Button></section>}{staff && !closed && <section className="content-card ticket-management"><h2>{t.edit}</h2><form className="editor-form" onSubmit={event => { event.preventDefault(); management() }}><FormField id="manage-priority" label={t.priority}><select id="manage-priority" value={priority} onChange={event => setPriority(event.target.value as TicketPriority)} disabled={disabled}>{ticketPriorities.map(value => <option key={value} value={value}>{t[ticketPriorityKey[value]]}</option>)}</select></FormField><FormField id="manage-assignee" label={t.assignee}><select id="manage-assignee" value={assignee} onChange={event => setAssignee(event.target.value)} disabled={disabled || assignees.isPending || assignees.isError}><option value="">{t.unassigned}</option>{ticket.assignee_id && !assignees.data?.items.some(item => item.id === ticket.assignee_id) && <option value={ticket.assignee_id}>{ticket.assignee_name} ({t.inactive})</option>}{assignees.data?.items.map(item => <option key={item.id} value={item.id}>{item.display_name}</option>)}</select></FormField>{assignees.isError && <p className="inline-error" role="alert">{message(assignees.error)} <Button type="button" size="sm" variant="ghost" onClick={() => void assignees.refetch()}>{t.retry}</Button></p>}<Button type="button" size="sm" variant="outline" disabled={disabled || !assignees.data?.items.some(item => item.id === user?.id)} onClick={() => setAssignee(user!.id)}>{t.assignToMe}</Button><Button type="submit" disabled={disabled || assignees.isPending || assignees.isError || (priority === ticket.priority && (assignee || null) === ticket.assignee_id)}>{busy ? t.saving : t.save}</Button></form><hr /><h2>{t.ticketStatusHeading}</h2><p className="field-hint">{t.ticketStatusWorkflow}</p><form className="editor-form" onSubmit={event => { event.preventDefault(); transition() }}><FormField id="manage-status" label={t.nextStatus}><select id="manage-status" value={status} onChange={event => setStatus(event.target.value as TicketStatus | '')} disabled={disabled}><option value="">{t.changeStatus}</option>{ticketTransitions[ticket.status].map(value => <option key={value} value={value}>{t[ticketStatusKey[value]]}</option>)}</select></FormField><FormField id="manage-reason" label={t.transitionReason} hint={t.transitionReasonHint} error={reasonInvalid ? t.reasonRequired : undefined}><textarea id="manage-reason" rows={3} value={reason} onChange={event => setReason(event.target.value)} disabled={disabled} aria-invalid={reasonInvalid} aria-describedby={`manage-reason-hint${reasonInvalid ? ' manage-reason-error' : ''}`} /></FormField><Button type="submit" disabled={disabled || !status}>{t.changeStatus}</Button></form></section>}{closed && <section className="content-card ticket-management"><p>{t.closedTicketNote}</p>{user?.role === 'ADMIN' && <form className="editor-form" onSubmit={event => { event.preventDefault(); reopen() }}><FormField id="reopen-reason" label={t.transitionReason} error={reasonInvalid ? t.reasonRequired : undefined}><textarea id="reopen-reason" rows={3} value={reason} onChange={event => setReason(event.target.value)} disabled={disabled} aria-invalid={reasonInvalid} aria-describedby={reasonInvalid ? 'reopen-reason-error' : undefined} /></FormField><Button type="submit" disabled={disabled}>{t.reopenTicket}</Button></form>}</section>}{staff && (ticket.status === 'RESOLVED' || closed) && <section className="content-card ticket-management"><h2>{t.ticketToArticle}</h2><p>{t.ticketToArticleDescription}</p><Button asChild variant="outline"><Link to={linkedArticle ? `/engineer/knowledge/${linkedArticle}/edit` : `/engineer/knowledge/new?ticket_id=${encodeURIComponent(ticket.id)}`}>{linkedArticle ? t.openTicketDraft : t.ticketToArticle}</Link></Button></section>}<section className="content-card ticket-history"><h2>{t.ticketHistory}</h2><ol>{data.history.map(item => <li key={item.id}><strong>{item.from_status ? `${t[ticketStatusKey[item.from_status]]} → ` : ''}{t[ticketStatusKey[item.to_status]]}</strong><span>{item.actor_name} · {date(item.created_at)}</span>{item.reason && <p className="preserve-lines">{item.reason}</p>}</li>)}</ol></section></aside></div>{refreshing && <Modal title={t.refreshTicket} description={t.refreshTicketDescription} onClose={() => setRefreshing(false)} busy={busy}><div className="form-actions"><Button autoFocus variant="outline" disabled={busy} onClick={() => setRefreshing(false)}>{t.cancel}</Button><Button disabled={busy} onClick={() => void adoptLatest()}>{busy ? t.loading : t.refreshTicket}</Button></div></Modal>}{confirming && <Modal title={t.confirmResolved} description={t.confirmResolvedDescription} onClose={() => setConfirming(false)} busy={busy}><div className="form-actions"><Button autoFocus variant="outline" disabled={busy} onClick={() => setConfirming(false)}>{t.cancel}</Button><Button disabled={busy || conflict} onClick={() => void mutate(async () => (await api(`/tickets/${ticket.id}/confirm`, ticketResultSchema, { method: 'POST', body: { version: ticket.version } })).ticket, t.ticketSaved, () => setConfirming(false))}>{busy ? t.saving : t.closeTicket}</Button></div></Modal>}</>
}

export function TicketDetail() {
  const { id = '' } = useParams()
  const { api } = useAuth()
  const { t } = useLocale()
  const message = useErrorMessage()
  const detail = useQuery({ queryKey: ['tickets', 'detail', id], queryFn: ({ signal }) => api(`/tickets/${encodeURIComponent(id)}`, ticketDetailSchema, { signal }) })
  if (detail.error instanceof ApiError && detail.error.status === 404) return <div className="table-state"><p role="alert">{t.ticketMissing}</p><Button asChild variant="outline"><Link to="/tickets">{t.backTickets}</Link></Button></div>
  const transient = !!detail.data && detail.error instanceof ApiError && (detail.error.status === 0 || detail.error.status >= 500)
  return <>{transient && <p className="inline-error" role="alert">{message(detail.error)} <Button variant="ghost" size="sm" onClick={() => void detail.refetch()}>{t.retry}</Button></p>}<KnowledgeState pending={detail.isPending} error={transient ? undefined : detail.error} retry={() => void detail.refetch()}>{detail.data && <TicketContent key={detail.data.ticket.id} data={detail.data} reload={async () => { const result = await detail.refetch(); if (result.error) throw result.error; return result.data }} />}</KnowledgeState></>
}

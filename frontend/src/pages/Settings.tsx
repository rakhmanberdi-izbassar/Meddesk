import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { settingsQueryKey, settingsResultSchema, useSettings, type ApplicationSettings } from '../lib/settings'
import { formatTimestamp } from '../lib/analytics'
import { ticketPriorities, ticketPriorityKey, type TicketPriority, validTicketText } from '../lib/tickets'
import { FormField } from '../components/forms/FormField'
import { Modal } from '../components/forms/Modal'
import { Button } from '../components/ui/button'

export function Settings() {
  const { user, api } = useAuth()
  const { t, locale } = useLocale()
  const query = useSettings()
  const cache = useQueryClient()
  const notify = useToast()
  const message = useErrorMessage()
  const [snapshot, setSnapshot] = useState<ApplicationSettings | null>(null)
  const [draft, setDraft] = useState<ApplicationSettings | null>(null)
  const [invalid, setInvalid] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [dialog, setDialog] = useState<'save' | 'reload' | null>(null)
  const [busy, setBusy] = useState(false)
  const mounted = useRef(true)
  const submitting = useRef(false)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => { if (query.data && !snapshot) { setSnapshot(query.data.settings); setDraft(query.data.settings) } }, [query.data, snapshot])
  const validate = () => {
    let goodTimezone = false
    try { new Intl.DateTimeFormat('en', { timeZone: draft!.reporting_timezone.trim() }).format(); goodTimezone = draft!.reporting_timezone.trim().length > 0 } catch { /* Validation is shown below. */ }
    return validTicketText(draft!.clinic_name, 150) && !/[\u0000-\u001f\u007f]/.test(draft!.clinic_name) && goodTimezone
  }
  const submit = () => { if (!validate()) { setInvalid(true); return }; setInvalid(false); setDialog('save') }
  const save = async () => {
    if (!snapshot || !draft || submitting.current) return
    submitting.current = true; setBusy(true)
    try {
      const result = await api('/settings', settingsResultSchema, { method: 'PATCH', body: { version: snapshot.version, clinic_name: draft.clinic_name.trim(), default_ticket_priority: draft.default_ticket_priority, reporting_timezone: draft.reporting_timezone.trim() } })
      if (!mounted.current) return
      setSnapshot(result.settings); setDraft(result.settings); setDialog(null); setConflict(false)
      cache.setQueryData(settingsQueryKey(user?.id, user?.role), result)
      void cache.invalidateQueries({ queryKey: ['analytics'] }); void cache.invalidateQueries({ queryKey: ['admin', 'audit'] })
      notify({ kind: 'success', message: t.settingsSaved })
    } catch (error) {
      if (!mounted.current) return
      setDialog(null)
      if (error instanceof ApiError && error.status === 409) { setConflict(true); void query.refetch() }
      else notify({ kind: 'error', message: message(error) })
    } finally { submitting.current = false; if (mounted.current) setBusy(false) }
  }
  const reload = async () => {
    if (submitting.current) return
    submitting.current = true; setBusy(true)
    try { const result = await query.refetch(); if (!mounted.current) return; if (result.error || !result.data) throw result.error; setSnapshot(result.data.settings); setDraft(result.data.settings); setInvalid(false); setConflict(false); setDialog(null) }
    catch (error) { if (mounted.current) notify({ kind: 'error', message: message(error) }) }
    finally { submitting.current = false; if (mounted.current) setBusy(false) }
  }
  const dirty = draft && snapshot && (draft.clinic_name.trim() !== snapshot.clinic_name || draft.default_ticket_priority !== snapshot.default_ticket_priority || draft.reporting_timezone.trim() !== snapshot.reporting_timezone)
  const newer = !!snapshot && !!query.data && query.data.settings.version !== snapshot.version
  return <><div className="page-heading"><div><h1>{t.settings}</h1><p>{t.settingsDescription}</p></div></div>{query.isPending && !draft ? <p role="status" className="table-state">{t.loading}</p> : query.isError && !draft ? <div className="content-card table-state" role="alert"><p>{message(query.error)}</p><Button variant="outline" onClick={() => void query.refetch()}>{t.retry}</Button></div> : draft && snapshot && <form className="content-card settings-form" onSubmit={event => { event.preventDefault(); submit() }} noValidate><p className="settings-version">{t.settingsVersion}: {snapshot.version} · {t.settingsUpdated}: {formatTimestamp(snapshot.updated_at, locale, snapshot.reporting_timezone)}</p>{(newer || conflict) && <div className="version-notice" role="alert"><p>{conflict ? t.settingsConflict : t.settingsNewVersion}</p><Button type="button" variant="outline" disabled={busy} onClick={() => setDialog('reload')}>{t.settingsReload}</Button></div>}<FormField id="settings-clinic" label={t.clinicName}><input id="settings-clinic" value={draft.clinic_name} onChange={event => setDraft({ ...draft, clinic_name: event.target.value })} disabled={busy || dialog !== null} maxLength={150} aria-invalid={invalid} /></FormField><FormField id="settings-priority" label={t.defaultPriority} hint={t.settingsPriorityHint}><select id="settings-priority" value={draft.default_ticket_priority} onChange={event => setDraft({ ...draft, default_ticket_priority: event.target.value as TicketPriority })} disabled={busy || dialog !== null} aria-describedby="settings-priority-hint">{ticketPriorities.map(value => <option key={value} value={value}>{t[ticketPriorityKey[value]]}</option>)}</select></FormField><FormField id="settings-timezone" label={t.reportingTimezone} hint={`${t.timezoneHint} ${t.settingsTimezoneHint}`}><input id="settings-timezone" value={draft.reporting_timezone} onChange={event => setDraft({ ...draft, reporting_timezone: event.target.value })} disabled={busy || dialog !== null} maxLength={100} aria-invalid={invalid} aria-describedby="settings-timezone-hint" /></FormField>{invalid && <p className="inline-error" role="alert">{t.settingsValidation}</p>}<div className="form-actions"><Button type="button" variant="outline" disabled={busy} onClick={() => setDialog('reload')}>{t.settingsReload}</Button><Button type="submit" disabled={busy || !dirty || conflict}>{t.save}</Button></div></form>}{dialog && <Modal title={dialog === 'save' ? t.settingsConfirm : t.settingsReload} description={dialog === 'save' ? t.settingsConfirmDescription : t.settingsReloadDescription} onClose={() => setDialog(null)} busy={busy}>{dialog === 'save' && draft && <dl className="confirmation-values"><dt>{t.clinicName}</dt><dd>{draft.clinic_name.trim()}</dd><dt>{t.defaultPriority}</dt><dd>{t[ticketPriorityKey[draft.default_ticket_priority]]}</dd><dt>{t.reportingTimezone}</dt><dd>{draft.reporting_timezone.trim()}</dd></dl>}<div className="form-actions"><Button variant="outline" disabled={busy} onClick={() => setDialog(null)}>{t.cancel}</Button><Button disabled={busy} onClick={() => void (dialog === 'save' ? save() : reload())}>{busy ? t.saving : dialog === 'save' ? t.confirm : t.settingsReload}</Button></div></Modal>}</>
}

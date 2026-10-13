import { useEffect, useRef, useState } from 'react'
import { ImagePlus, X } from 'lucide-react'
import { Button } from './ui/button'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { mediaResultSchema, mediaURL, validMediaFile } from '../lib/knowledge'

export function TicketUploads({ ids, onChange, disabled, existingCount = 0, onBusyChange }: {
  ids: string[]; onChange: (ids: string[]) => void; disabled: boolean; existingCount?: number; onBusyChange: (busy: boolean) => void
}) {
  const { api } = useAuth()
  const { t } = useLocale()
  const notify = useToast()
  const message = useErrorMessage()
  const [uploading, setUploading] = useState(false)
  const controller = useRef(new AbortController())
  useEffect(() => { const current = new AbortController(); controller.current = current; return () => current.abort() }, [])
  const upload = async (file?: File) => {
    if (!file || disabled || uploading) return
    if (!validMediaFile(file, 'TICKET_ATTACHMENT') || ids.length + existingCount >= 10) { notify({ kind: 'error', message: t.fileInvalid }); return }
    setUploading(true); onBusyChange(true)
    try {
      const body = new FormData(); body.append('file', file)
      const result = await api('/media?purpose=TICKET_ATTACHMENT', mediaResultSchema, { method: 'POST', body, signal: controller.current.signal })
      if (!controller.current.signal.aborted) { onChange([...ids, result.media.id]); notify({ kind: 'success', message: t.ticketFileUploaded }) }
    } catch (error) { if (!controller.current.signal.aborted) notify({ kind: 'error', message: message(error) }) }
    finally { if (!controller.current.signal.aborted) { setUploading(false); onBusyChange(false) } }
  }
  return <section className="media-editor"><h2>{t.screenshots}</h2><p className="field-hint">{t.ticketImageRule}</p><label className={`upload-field ${disabled || uploading ? 'disabled' : ''}`}><ImagePlus size={18} aria-hidden="true" /><span>{uploading ? t.uploading : t.ticketScreenshot}</span><input type="file" accept="image/png,image/jpeg" aria-label={t.ticketScreenshot} disabled={disabled || uploading || ids.length + existingCount >= 10} onChange={event => { void upload(event.target.files?.[0]); event.target.value = '' }} /></label><div className="editor-images">{ids.map((id, index) => <div key={id}><img src={mediaURL(id)} alt={`${t.screenshots} ${existingCount + index + 1}`} /><Button type="button" variant="ghost" size="sm" disabled={disabled || uploading} onClick={() => onChange(ids.filter(value => value !== id))} aria-label={`${t.removeAttachment} ${index + 1}`}><X size={13} aria-hidden="true" />{t.removeAttachment}</Button></div>)}</div><p className="field-hint">{t.ticketAttachmentNote}</p></section>
}

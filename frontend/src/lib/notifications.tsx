import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { CheckCircle2, X, CircleAlert } from 'lucide-react'
import { ApiError } from './api'
import { useLocale } from './locale'

type Notice = { message: string; kind: 'success' | 'error' }
const ToastContext = createContext<((notice: Notice) => void) | null>(null)

export function ToastProvider({ children }: { children: ReactNode }) {
  const { t } = useLocale()
  const [notice, setNotice] = useState<Notice | null>(null)
  useEffect(() => { if (!notice) return; const id = window.setTimeout(() => setNotice(null), 6000); return () => window.clearTimeout(id) }, [notice])
  const Icon = notice?.kind === 'success' ? CheckCircle2 : CircleAlert
  return <ToastContext.Provider value={setNotice}>{children}{notice && <div className={`toast ${notice.kind}`} role="alert"><Icon size={20} aria-hidden="true" /><span>{notice.message}</span><button onClick={() => setNotice(null)} aria-label={t.close}><X size={18} aria-hidden="true" /></button></div>}</ToastContext.Provider>
}

export function useToast() {
  const notify = useContext(ToastContext)
  if (!notify) throw new Error('ToastProvider is required')
  return notify
}

export function useErrorMessage() {
  const { t } = useLocale()
  return (error: unknown) => {
    if (!(error instanceof ApiError)) return t.requestFailed
    const codes: Record<string, string> = {
      invalid_credentials: t.invalidCredentials, unauthenticated: t.sessionExpired,
      validation_failed: t.validationFailed, invalid_input: t.validationFailed,
      forbidden: t.permissionDenied, csrf_failed: t.sessionExpired, origin_forbidden: t.permissionDenied,
      rate_limited: t.rateLimited, conflict: t.conflict, inactive_department: t.inactiveDepartment,
      last_administrator: t.lastAdmin, department_has_active_users: t.departmentInUse,
      version_conflict: t.versionConflict, inactive_category: t.categoryRequired,
      category_has_published_articles: t.categoryDeactivate, unsupported_media_type: t.fileInvalid,
      invalid_transition: t.invalidTransition, invalid_assignee: t.invalidAssignee, ticket_closed: t.closedTicketNote, already_converted: t.ticketAlreadyConverted,
      file_too_large: t.fileInvalid, invalid_media: t.fileInvalid, invalid_file: t.fileInvalid,
      export_limit: t.exportLimit,
    }
    return codes[error.code] ?? (error.status === 401 ? t.sessionExpired : error.status === 403 ? t.permissionDenied : error.status === 409 ? t.conflict : t.requestFailed)
  }
}

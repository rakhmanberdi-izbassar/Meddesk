import type { ReactNode } from 'react'
import { Button } from './ui/button'
import { useLocale } from '../lib/locale'
import { useErrorMessage } from '../lib/notifications'

export function KnowledgeState({ pending, error, retry, children }: { pending: boolean; error?: unknown; retry: () => void; children: ReactNode }) {
  const { t } = useLocale()
  const message = useErrorMessage()
  if (pending) return <div className="table-state" role="status" aria-busy="true">{t.loading}</div>
  if (error) return <div className="table-state"><p role="alert">{message(error)}</p><Button variant="outline" onClick={retry}>{t.retry}</Button></div>
  return <>{children}</>
}

import { Button } from '../ui/button'
import { useLocale } from '../../lib/locale'

export function Pagination({ page, total, limit, busy, onChange }: { page: number; total: number; limit: number; busy: boolean; onChange: (page: number) => void }) {
  const { t } = useLocale()
  return <div className="table-pagination"><span>{t.records}: {total} · {t.page} {page + 1} / {Math.max(1, Math.ceil(total / limit))}</span><div><Button variant="outline" size="sm" disabled={page === 0 || busy} onClick={() => onChange(page - 1)}>{t.previous}</Button><Button variant="outline" size="sm" disabled={(page + 1) * limit >= total || busy} onClick={() => onChange(page + 1)}>{t.next}</Button></div></div>
}

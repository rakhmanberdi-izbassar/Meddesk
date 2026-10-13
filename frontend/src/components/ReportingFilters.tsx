import { FormField } from './forms/FormField'
import { useLocale } from '../lib/locale'
import type { ReportingWindow } from '../lib/analytics'

export function ReportingFilters({ value, onChange, invalid, prefix = 'report', disabled = false }: { value: ReportingWindow; onChange: (value: ReportingWindow) => void; invalid: boolean; prefix?: string; disabled?: boolean }) {
  const { t } = useLocale()
  return <><FormField id={`${prefix}-from`} label={t.dateFrom}><input id={`${prefix}-from`} type="date" value={value.from} onChange={event => onChange({ ...value, from: event.target.value })} disabled={disabled} aria-invalid={invalid} /></FormField><FormField id={`${prefix}-to`} label={t.dateTo}><input id={`${prefix}-to`} type="date" value={value.to} onChange={event => onChange({ ...value, to: event.target.value })} disabled={disabled} aria-invalid={invalid} /></FormField><FormField id={`${prefix}-timezone`} label={t.reportingTimezone} hint={t.timezoneHint}><input id={`${prefix}-timezone`} value={value.timezone} onChange={event => onChange({ ...value, timezone: event.target.value })} disabled={disabled} aria-invalid={invalid} aria-describedby={`${prefix}-timezone-hint`} maxLength={100} /></FormField></>
}

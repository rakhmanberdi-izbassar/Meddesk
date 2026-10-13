import { useLocale } from '../lib/locale'
import type { AnalyticsData } from '../lib/analytics'

export function AnalyticsSummary({ summary }: { summary: AnalyticsData['summary'] }) {
  const { t, locale } = useLocale()
  const number = new Intl.NumberFormat(locale === 'kk' ? 'kk-KZ' : 'ru-RU', { maximumFractionDigits: 1 })
  const cards = [
    { label: t.totalTickets, value: number.format(summary.tickets_total) },
    { label: t.openTickets, value: number.format(summary.tickets_open) },
    { label: t.resolvedTickets, value: number.format(summary.tickets_resolved) },
    { label: t.closedTickets, value: number.format(summary.tickets_closed) },
    { label: t.averageResolution, value: summary.average_resolution_seconds === null ? t.noMeasurement : `${number.format(summary.average_resolution_seconds / 3600)} ${t.hoursShort}` },
    { label: t.selfService, value: summary.self_service_rate === null ? t.noMeasurement : `${number.format(summary.self_service_rate * 100)}%`, note: `${t.helpfulAttempts}: ${number.format(summary.helpful_attempts)} / ${t.completedAttempts}: ${number.format(summary.feedback_attempts)}` },
  ]
  return <dl className="analytics-summary">{cards.map(card => <div className="metric-card" key={card.label}><dt>{card.label}</dt><dd>{card.value}</dd>{card.note && <p>{card.note}</p>}</div>)}</dl>
}

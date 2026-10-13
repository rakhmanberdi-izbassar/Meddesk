import { z } from 'zod'

const count = z.number().int().nonnegative()
const category = z.object({ id: z.string().nullable(), name_kk: z.string(), name_ru: z.string(), total: count, open: count, resolved: count })
export const analyticsSchema = z.object({
  period: z.object({ from: z.string(), to: z.string(), timezone: z.string(), start_at: z.string(), end_at: z.string() }),
  summary: z.object({ tickets_total: count, tickets_open: count, tickets_resolved: count, tickets_closed: count, average_resolution_seconds: z.number().nonnegative().nullable(), feedback_attempts: count, helpful_attempts: count, self_service_rate: z.number().nonnegative().nullable() }),
  departments: z.array(z.object({ id: z.string(), name: z.string(), total: count, open: count, resolved: count })),
  categories: z.array(category), common_issues: z.array(category),
  popular_articles: z.array(z.object({ id: z.string(), title: z.string(), view_count: count, helpful_count: count, not_helpful_count: count })),
  trend: z.array(z.object({ date: z.string(), tickets_created: count, tickets_resolved: count, feedback_attempts: count, helpful_attempts: count })),
})
export type AnalyticsData = z.infer<typeof analyticsSchema>
export const analyticsFiltersSchema = z.object({
  departments: z.array(z.object({ id: z.string(), name: z.string() })),
  categories: z.array(z.object({ id: z.string(), name_kk: z.string(), name_ru: z.string() })),
  assignees: z.array(z.object({ id: z.string(), display_name: z.string() })),
})
export const auditPageSchema = z.object({
  period: z.object({ from: z.string(), to: z.string(), timezone: z.string(), start_at: z.string(), end_at: z.string() }),
  items: z.array(z.object({ id: z.union([z.string(), z.number()]), actor_id: z.string().nullable(), actor_name: z.string().nullable(), action: z.string(), entity_type: z.string(), entity_id: z.string().nullable(), metadata: z.record(z.unknown()), created_at: z.string() })),
  total: count, limit: count, offset: count,
})
export const auditFiltersSchema = z.object({ actions: z.array(z.string()), entity_types: z.array(z.string()), actors: z.array(z.object({ id: z.string(), display_name: z.string() })) })
export type ReportingWindow = { from: string; to: string; timezone: string }
export function validReportingWindow(value: ReportingWindow) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value.from) || !/^\d{4}-\d{2}-\d{2}$/.test(value.to)) return false
  const start = Date.parse(`${value.from}T00:00:00Z`), end = Date.parse(`${value.to}T00:00:00Z`)
  if (!Number.isFinite(start) || !Number.isFinite(end) || new Date(start).toISOString().slice(0, 10) !== value.from || new Date(end).toISOString().slice(0, 10) !== value.to || end < start || (end - start) / 86_400_000 >= 366) return false
  try { new Intl.DateTimeFormat('en', { timeZone: value.timezone }).format(); return value.timezone.length > 0 } catch { return false }
}
export function defaultReportingWindow(timezone: string): ReportingWindow {
  const parts = new Intl.DateTimeFormat('en', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date())
  const get = (name: string) => parts.find(part => part.type === name)!.value
  const to = `${get('year')}-${get('month')}-${get('day')}`
  const start = new Date(`${to}T00:00:00Z`); start.setUTCDate(start.getUTCDate() - 29)
  return { from: start.toISOString().slice(0, 10), to, timezone }
}
export function formatTimestamp(value: string, locale: string, timezone: string) {
  try { return new Intl.DateTimeFormat(locale === 'kk' ? 'kk-KZ' : 'ru-RU', { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(new Date(value)) } catch { return value }
}

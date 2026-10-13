import { z } from 'zod'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from './auth'
import { ticketPrioritySchema } from './tickets'

export const settingsSchema = z.object({ clinic_name: z.string(), default_ticket_priority: ticketPrioritySchema, reporting_timezone: z.string(), version: z.number().int().positive(), updated_at: z.string() })
export const settingsResultSchema = z.object({ settings: settingsSchema })
export type ApplicationSettings = z.infer<typeof settingsSchema>
export const settingsQueryKey = (id?: string, role?: string) => ['application-settings', id, role] as const
export function useSettings() {
  const { user, status, api } = useAuth()
  return useQuery({ queryKey: settingsQueryKey(user?.id, user?.role), enabled: status === 'authenticated' && !!user, queryFn: ({ signal }) => api('/settings', settingsResultSchema, { signal }), staleTime: 60_000 })
}

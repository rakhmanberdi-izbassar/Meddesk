import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import { request } from './api'
import { useAuth } from './auth'
export const accessSchema = z.object({ allowed: z.boolean() })
export function useSchoolAccess() {
 const { user } = useAuth()
 return useQuery({ queryKey: ['school-access', user?.id], queryFn: ({ signal }) => request('/school/access', accessSchema, { signal }), enabled: !!user, retry: false })
}
export const assetSchema = z.object({ id: z.string(), name: z.string(), kind: z.string(), location: z.string(), model: z.string(), inventory_number: z.string(), ip_address: z.string().nullable(), notes: z.string(), version: z.number(), created_at: z.string(), updated_at: z.string() })
export const ticketSchema = z.object({ id: z.string(), number: z.number(), title: z.string(), description: z.string(), requester: z.string(), room: z.string(), category: z.string(), priority: z.string(), status: z.string(), lesson_blocked: z.boolean(), asset_id: z.string().nullable(), resolution: z.string(), public_reply: z.string().optional(), version: z.number(), created_at: z.string(), updated_at: z.string(), resolved_at: z.string().nullable() })
export type SchoolAsset = z.infer<typeof assetSchema>
export type SchoolTicket = z.infer<typeof ticketSchema>
export const assetsSchema = z.object({ items: z.array(assetSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const ticketsSchema = z.object({ items: z.array(ticketSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const categories = ['COMPUTERS', 'PRINTING', 'NETWORK', 'SURVEILLANCE', 'CLASSROOM', 'ACCOUNTS', 'OTHER'] as const
export const kinds = ['COMPUTER', 'PRINTER', 'CAMERA', 'RECORDER', 'ROUTER', 'SWITCH', 'PROJECTOR', 'OTHER'] as const
export const priorities = ['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const
export const statuses = ['NEW', 'IN_PROGRESS', 'WAITING_FOR_USER', 'RESOLVED', 'CLOSED'] as const
export const labels: Record<string, [string, string]> = {
 COMPUTERS: ['Компьютерлер', 'Компьютеры'], PRINTING: ['Басып шығару', 'Печать'], NETWORK: ['Интернет және Wi-Fi', 'Интернет и Wi-Fi'], SURVEILLANCE: ['Бейнебақылау', 'Видеонаблюдение'], CLASSROOM: ['Сынып жабдықтары', 'Оборудование класса'], ACCOUNTS: ['Аккаунттар', 'Учётные записи'], OTHER: ['Басқа', 'Другое'], COMPUTER: ['Компьютер', 'Компьютер'], PRINTER: ['Принтер', 'Принтер'], CAMERA: ['Камера', 'Камера'], RECORDER: ['Бейнетіркеуіш', 'Видеорегистратор'], ROUTER: ['Роутер', 'Роутер'], SWITCH: ['Коммутатор', 'Коммутатор'], PROJECTOR: ['Проектор / тақта', 'Проектор / доска'],
 LOW: ['Төмен', 'Низкий'], MEDIUM: ['Орташа', 'Средний'], HIGH: ['Жоғары', 'Высокий'], CRITICAL: ['Шұғыл', 'Критический'], NEW: ['Жаңа', 'Новая'], IN_PROGRESS: ['Орындалуда', 'В работе'], WAITING_FOR_USER: ['Жауап күтілуде', 'Ожидает ответа'], RESOLVED: ['Шешілді', 'Решена'], CLOSED: ['Жабылды', 'Закрыта'],
}
export const memberSchema = z.object({ id: z.string(), email: z.string(), display_name: z.string(), is_active: z.boolean(), version: z.number() })
export type SchoolMember = z.infer<typeof memberSchema>
export const membersSchema = z.object({ items: z.array(memberSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const schoolRequestSchema = z.object({ id: z.string(), number: z.number(), title: z.string(), description: z.string(), room: z.string(), category: z.string(), priority: z.string(), status: z.string(), lesson_blocked: z.boolean(), public_reply: z.string(), created_at: z.string(), updated_at: z.string(), resolved_at: z.string().nullable() })
export const schoolRequestsSchema = z.object({ items: z.array(schoolRequestSchema), total: z.number(), limit: z.number(), offset: z.number() })

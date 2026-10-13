import { z } from 'zod'

export const ticketStatusSchema = z.enum(['NEW', 'IN_PROGRESS', 'WAITING_FOR_USER', 'RESOLVED', 'CLOSED'])
export const ticketPrioritySchema = z.enum(['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'])
export type TicketStatus = z.infer<typeof ticketStatusSchema>
export type TicketPriority = z.infer<typeof ticketPrioritySchema>
export const ticketStatuses = ticketStatusSchema.options
export const ticketPriorities = ticketPrioritySchema.options
export const ticketSchema = z.object({
  id: z.string(), number: z.number().int(), title: z.string(), description: z.string(),
  requester_id: z.string(), requester_name: z.string(), department_id: z.string(), department_name: z.string(),
  category_id: z.string().nullable(), priority: ticketPrioritySchema, status: ticketStatusSchema,
  assignee_id: z.string().nullable(), assignee_name: z.string(), version: z.number().int(),
  created_at: z.string(), updated_at: z.string(), resolved_at: z.string().nullable(),
  article_ids: z.array(z.string()), attachment_ids: z.array(z.string()), converted_article_id: z.string().nullable(),
})
export type Ticket = z.infer<typeof ticketSchema>
export const ticketResultSchema = z.object({ ticket: ticketSchema })
export const ticketsPageSchema = z.object({ items: z.array(ticketSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const commentSchema = z.object({ id: z.string(), author_id: z.string(), author_name: z.string(), body: z.string(), is_internal: z.boolean(), created_at: z.string() })
export const ticketCommentResultSchema = z.object({ ticket: ticketSchema, comment: commentSchema })
export const ticketDetailSchema = z.object({
  ticket: ticketSchema, comments: z.array(commentSchema),
  history: z.array(z.object({ id: z.number(), from_status: ticketStatusSchema.nullable(), to_status: ticketStatusSchema, actor_name: z.string(), reason: z.string(), created_at: z.string() })),
})
export type TicketDetail = z.infer<typeof ticketDetailSchema>
export const ticketDepartmentsSchema = z.object({ items: z.array(z.object({ id: z.string(), name: z.string() })) })
export const assigneesSchema = z.object({ items: z.array(z.object({ id: z.string(), display_name: z.string() })) })
export const ticketTransitions: Record<TicketStatus, TicketStatus[]> = {
  NEW: ['IN_PROGRESS'], IN_PROGRESS: ['WAITING_FOR_USER', 'RESOLVED'],
  WAITING_FOR_USER: ['IN_PROGRESS', 'RESOLVED'], RESOLVED: ['IN_PROGRESS', 'CLOSED'], CLOSED: [],
}
export const ticketStatusKey = { NEW: 'ticketNew', IN_PROGRESS: 'ticketInProgress', WAITING_FOR_USER: 'ticketWaiting', RESOLVED: 'ticketResolved', CLOSED: 'ticketClosed' } as const
export const ticketPriorityKey = { LOW: 'priorityLow', MEDIUM: 'priorityMedium', HIGH: 'priorityHigh', CRITICAL: 'priorityCritical' } as const
export function validTicketText(value: string, max: number, required = true) {
  return (!required || value.trim().length > 0) && Array.from(value.trim()).length <= max && !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(value)
}

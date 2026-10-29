import { z } from 'zod'

const statusSchema = z.object({ status: z.string() })

export class ServiceUnavailableError extends Error {
  constructor() {
    super('Service unavailable')
    this.name = 'ServiceUnavailableError'
  }
}

export async function fetchOperationalStatus(
  endpoint: 'health' | 'ready',
  signal?: AbortSignal,
): Promise<{ status: string }> {
  try {
    const response = await fetch(`/api/v1/${endpoint}`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      credentials: 'same-origin',
      signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(8000)]) : AbortSignal.timeout(8000),
    })
    if (!response.ok) throw new ServiceUnavailableError()
    const result = statusSchema.parse(await response.json())
    const expected = endpoint === 'health' ? 'ok' : 'ready'
    if (result.status !== expected) throw new ServiceUnavailableError()
    return result
  } catch {
    // Never expose response bodies, network details, or configuration to the UI.
    throw new ServiceUnavailableError()
  }
}

export const roleSchema = z.enum(['EMPLOYEE', 'IT_ENGINEER', 'ADMIN'])
export type Role = z.infer<typeof roleSchema>
export const userSchema = z.object({
  id: z.string(), email: z.string(), display_name: z.string(), role: roleSchema, account_scope: z.enum(['CLINIC','SCHOOL']).optional(),
  department_id: z.string(), department_name: z.string(), is_active: z.boolean(),
})
export type User = z.infer<typeof userSchema>
export const sessionSchema = z.object({ user: userSchema, csrf_token: z.string().min(16) })
export type Session = z.infer<typeof sessionSchema>
export const departmentSchema = z.object({
  id: z.string(), name: z.string(), is_active: z.boolean(), created_at: z.string(), updated_at: z.string(),
})
export type Department = z.infer<typeof departmentSchema>
export const usersPageSchema = z.object({ items: z.array(userSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const departmentsPageSchema = z.object({ items: z.array(departmentSchema), total: z.number(), limit: z.number(), offset: z.number() })
export const userResultSchema = z.object({ user: userSchema })
export const departmentResultSchema = z.object({ department: departmentSchema })

export class ApiError extends Error {
  constructor(public readonly status: number, public readonly code: string) {
    super('API request failed')
    this.name = 'ApiError'
  }
}

export type RequestOptions = {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE'
  body?: unknown
  csrf?: string
  signal?: AbortSignal
}

async function responseError(response: Response) {
  let code = 'request_failed'
  try {
    const error = z.object({ error: z.object({ code: z.string().max(100) }) }).parse(await response.json())
    code = error.error.code.toLowerCase()
  } catch { /* Untrusted bodies never reach the interface. */ }
  return new ApiError(response.status, code)
}

export async function request<T>(path: string, schema: z.ZodType<T>, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET'
  const mutating = method !== 'GET' && path !== '/auth/login'
  if (mutating && !options.csrf) throw new ApiError(403, 'csrf_failed')
  const headers: Record<string, string> = { Accept: 'application/json' }
  const multipart = options.body instanceof FormData
  if (options.body !== undefined && !multipart) headers['Content-Type'] = 'application/json'
  if (mutating) headers['X-CSRF-Token'] = options.csrf!
  try {
    const response = await fetch(`/api/v1${path}`, {
      method, headers, credentials: 'same-origin',
      body: options.body === undefined ? undefined : multipart ? options.body as FormData : JSON.stringify(options.body),
      signal: options.signal ? AbortSignal.any([options.signal, AbortSignal.timeout(multipart ? 120_000 : 12_000)]) : AbortSignal.timeout(multipart ? 120_000 : 12_000),
    })
    if (!response.ok) throw await responseError(response)
    return schema.parse(response.status === 204 ? undefined : await response.json())
  } catch (error) {
    if (error instanceof ApiError) throw error
    throw new ApiError(0, 'request_failed')
  }
}

export type DownloadFormat = 'xlsx' | 'csv'
const downloadTypes: Record<DownloadFormat, string> = {
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  csv: 'text/csv',
}

export async function requestDownload(path: string, format: DownloadFormat, signal?: AbortSignal): Promise<Blob> {
  try {
    const response = await fetch(`/api/v1${path}`, {
      method: 'GET', credentials: 'include', headers: { Accept: downloadTypes[format] },
      signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(60_000)]) : AbortSignal.timeout(60_000),
    })
    if (!response.ok) throw await responseError(response)
    if (response.headers.get('Content-Type')?.split(';')[0].trim().toLowerCase() !== downloadTypes[format]) throw new ApiError(0, 'request_failed')
    return await response.blob()
  } catch (error) {
    if (error instanceof ApiError) throw error
    throw new ApiError(0, 'request_failed')
  }
}

export function saveDownload(blob: Blob, format: DownloadFormat) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  // A known filename and extension avoid trusting paths supplied by the server.
  anchor.download = `meddesk-tickets.${format}`
  try { document.body.append(anchor); anchor.click() }
  finally { anchor.remove(); window.setTimeout(() => URL.revokeObjectURL(url), 30_000) }
}

export function queryString(values: Record<string, string | number | undefined>) {
  const params = new URLSearchParams()
  Object.entries(values).forEach(([key, value]) => { if (value !== undefined && value !== '') params.set(key, String(value)) })
  return params.toString()
}

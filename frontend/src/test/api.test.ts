import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchOperationalStatus, ServiceUnavailableError } from '../lib/api'

afterEach(() => vi.unstubAllGlobals())

describe('operational API validation', () => {
  it.each(['health', 'ready'] as const)('rejects unrecognized %s status', async (endpoint) => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ status: 'unexpected' }), { status: 200 })))
    await expect(fetchOperationalStatus(endpoint)).rejects.toBeInstanceOf(ServiceUnavailableError)
  })

  it('does not leak raw network errors', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('postgres://secret@example.test') }))
    await expect(fetchOperationalStatus('health')).rejects.toThrow('Service unavailable')
  })

  it('rejects malformed response bodies', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>proxy error</html>', { status: 200 })))
    await expect(fetchOperationalStatus('ready')).rejects.toBeInstanceOf(ServiceUnavailableError)
  })
})

import { z } from 'zod'
import { ApiError, request, requestDownload, saveDownload, sessionSchema } from '../lib/api'
import { passwordIsValid } from '../lib/validation'

describe('authenticated API client', () => {
  it('requires CSRF before making any mutation', async () => {
    const mock = vi.fn(); vi.stubGlobal('fetch', mock)
    await expect(request('/profile', z.object({}), { method: 'PATCH', body: {} })).rejects.toMatchObject({ status: 403, code: 'csrf_failed' })
    expect(mock).not.toHaveBeenCalled()
  })
  it('preserves same-origin cookies and omits CSRF on login', async () => {
    const mock = vi.fn(async () => new Response('{}', { status: 200 })); vi.stubGlobal('fetch', mock)
    await request('/auth/login', z.object({}), { method: 'POST', body: {} })
    expect(mock).toHaveBeenCalledWith('/api/v1/auth/login', expect.objectContaining({ credentials: 'same-origin', headers: { Accept: 'application/json', 'Content-Type': 'application/json' } }))
  })
  it('rejects malformed session responses safely', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ user: { password_hash: 'private-test-hash' } }), { status: 200 })))
    await expect(request('/auth/me', sessionSchema)).rejects.toMatchObject({ status: 0, code: 'request_failed' })
  })
  it('sanitizes non-JSON failures', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('private backend detail', { status: 500 })))
    await expect(request('/auth/me', sessionSchema)).rejects.toBeInstanceOf(ApiError)
    await expect(request('/auth/me', sessionSchema)).rejects.toThrow('API request failed')
  })
  it('accepts 204 logout responses', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })))
    await expect(request('/auth/logout', z.undefined(), { method: 'POST', csrf: 'synthetic-csrf' })).resolves.toBeUndefined()
  })
  it('validates password bounds in UTF-8 bytes without trimming', () => {
    expect(passwordIsValid('Қазақша')).toBe(true)
    expect(passwordIsValid('short')).toBe(false)
    expect(passwordIsValid('a'.repeat(72))).toBe(true)
    expect(passwordIsValid('Қ'.repeat(37))).toBe(false)
    expect(passwordIsValid('            ')).toBe(true)
  })
  it('sends versioned deletion as JSON with CSRF', async () => {
    const mock = vi.fn(async () => new Response(null, { status: 204 })); vi.stubGlobal('fetch', mock)
    await request('/knowledge/articles/test-id', z.undefined(), { method: 'DELETE', body: { version: 3 }, csrf: 'synthetic-csrf' })
    expect(mock).toHaveBeenCalledWith('/api/v1/knowledge/articles/test-id', expect.objectContaining({ method: 'DELETE', body: '{"version":3}', headers: expect.objectContaining({ 'Content-Type': 'application/json', 'X-CSRF-Token': 'synthetic-csrf' }) }))
  })
  it('lets the browser set multipart boundaries and includes same-origin CSRF credentials', async () => {
    const mock = vi.fn(async () => new Response('{}')); vi.stubGlobal('fetch', mock)
    const form = new FormData(); form.append('file', new File(['synthetic'], 'image.png', { type: 'image/png' }))
    await request('/media?purpose=ARTICLE_IMAGE', z.object({}), { method: 'POST', body: form, csrf: 'synthetic-csrf' })
    expect(mock).toHaveBeenCalledWith('/api/v1/media?purpose=ARTICLE_IMAGE', expect.objectContaining({ body: form, credentials: 'same-origin', headers: { Accept: 'application/json', 'X-CSRF-Token': 'synthetic-csrf' } }))
  })
  it('blocks uploads before network access when the CSRF token is missing', async () => {
    const mock = vi.fn(); vi.stubGlobal('fetch', mock)
    await expect(request('/media?purpose=ARTICLE_VIDEO', z.object({}), { method: 'POST', body: new FormData() })).rejects.toMatchObject({ status: 403 })
    expect(mock).not.toHaveBeenCalled()
  })
})

describe('binary report downloads', () => {
  it.each([
    ['xlsx', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'],
    ['csv', 'text/csv'],
  ] as const)('downloads %s with cookies and the correct Accept header', async (format, contentType) => {
    const mock = vi.fn(async () => new Response('synthetic report', { headers: { 'Content-Type': `${contentType}; charset=utf-8` } }))
    vi.stubGlobal('fetch', mock)
    const blob = await requestDownload(`/tickets/export?format=${format}`, format)
    expect(await blob.text()).toBe('synthetic report')
    expect(mock).toHaveBeenCalledWith(`/api/v1/tickets/export?format=${format}`, expect.objectContaining({ method: 'GET', credentials: 'include', headers: { Accept: contentType } }))
  })
  it('recognizes JSON export-limit failures without exposing server messages', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'export_limit', message: 'private server details' } }), { status: 409 })))
    await expect(requestDownload('/tickets/export?format=csv', 'csv')).rejects.toMatchObject({ status: 409, code: 'export_limit', message: 'API request failed' })
  })
  it('safely rejects malformed failure bodies and success responses of the wrong type', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('private proxy detail', { status: 500 })))
    await expect(requestDownload('/tickets/export?format=csv', 'csv')).rejects.toMatchObject({ status: 500, code: 'request_failed', message: 'API request failed' })
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>private page</html>', { headers: { 'Content-Type': 'text/html' } })))
    await expect(requestDownload('/tickets/export?format=csv', 'csv')).rejects.toMatchObject({ status: 0, code: 'request_failed' })
  })
  it('uses a fixed filename, clicks an attached link, removes it and revokes the object URL', () => {
    vi.useFakeTimers()
    const create = vi.fn(() => 'blob:synthetic-report'), revoke = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: create })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revoke })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      expect(this).toHaveAttribute('download', 'meddesk-tickets.xlsx')
      expect(this).toHaveAttribute('href', 'blob:synthetic-report')
      expect(this.isConnected).toBe(true)
    })
    const blob = new Blob(['synthetic report'])
    saveDownload(blob, 'xlsx')
    expect(create).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalledOnce()
    expect(revoke).not.toHaveBeenCalled()
    expect(document.querySelector('a[download]')).toBeNull()
    vi.advanceTimersByTime(30_000)
    expect(revoke).toHaveBeenCalledWith('blob:synthetic-report')
    click.mockRestore()
    vi.useRealTimers()
  })
})

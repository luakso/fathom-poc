import { describe, it, expect, vi, afterEach } from 'vitest'
import { api, ApiError } from './api'

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })))
}
afterEach(() => vi.unstubAllGlobals())

describe('api client', () => {
  it('builds the neighbors URL and validates the payload', async () => {
    mockFetch(200, { address: '0x' + 'a'.repeat(40), lens: 'known' })
    const n = await api.neighbors('base', '0x' + 'a'.repeat(40), 'known', 8)
    expect(n.lens).toBe('known')
    const url = (fetch as ReturnType<typeof vi.fn>).mock.calls[0]![0] as string
    expect(url).toBe(`/api/base/entity/0x${'a'.repeat(40)}/neighbors?lens=known&limit=8`)
  })
  it('surfaces server error messages as ApiError with status', async () => {
    mockFetch(404, { error: 'not found' })
    await expect(api.entity('base', '0x' + 'a'.repeat(40))).rejects.toThrowError(ApiError)
    await expect(api.entity('base', '0x' + 'a'.repeat(40))).rejects.toThrow('not found')
  })
  it('rejects schema-invalid payloads', async () => {
    mockFetch(200, { dataMaxDay: 1 })
    await expect(api.meta()).rejects.toThrow()
  })
})

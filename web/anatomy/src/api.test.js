import { describe, it, expect, vi, afterEach } from 'vitest'
import { fetchTx } from './api.js'

afterEach(() => vi.restoreAllMocks())

describe('fetchTx', () => {
  it('returns parsed graph on 200', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({ chain: 'base', txHash: '0xabc', nodes: [], edges: [] }),
    })))
    const g = await fetchTx('base', '0xabc')
    expect(g.txHash).toBe('0xabc')
  })
  it('throws server error message on non-2xx', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      json: async () => ({ error: 'not found' }),
    })))
    await expect(fetchTx('base', '0xabc')).rejects.toThrow('not found')
  })
})

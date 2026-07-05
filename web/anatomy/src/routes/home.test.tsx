// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { newQueryClient } from '../App'
import { Home } from './Home'

const B = '0x' + 'b'.repeat(40)

function board(role: string, window: string, sort: string) {
  return {
    role, window, lens: 'known', sort,
    rows: [{ rank: 1, address: B, label: 'api.tollbit.com', txnCount: 20389, volumeUsdc: '38.36', distinctCounterparties: 214, firstSeen: '2026-04-08', lastSeen: '2026-06-06' }],
  }
}
afterEach(() => {
  vi.unstubAllGlobals()
  cleanup()
})

describe('Home', () => {
  it('fetches the payee/all/volume board by default and renders rows', async () => {
    const fetchMock = vi.fn(async (url: string) => {
      const u = new URL(String(url), 'http://x')
      return new Response(JSON.stringify(board(u.searchParams.get('role')!, u.searchParams.get('window')!, u.searchParams.get('sort')!)), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <MemoryRouter>
        <QueryClientProvider client={newQueryClient()}><Home /></QueryClientProvider>
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getByText('api.tollbit.com')).toBeTruthy())
    const url = String(fetchMock.mock.calls[0][0])
    expect(url).toContain('role=payee')
    expect(url).toContain('window=all')
    expect(url).toContain('sort=volume')
    expect(url).toContain('limit=500')
  })
  it('switching tab/window/sort refetches with new params', async () => {
    const fetchMock = vi.fn(async (_url: string) => new Response(JSON.stringify(board('payer', '7d', 'txns')), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    render(
      <MemoryRouter>
        <QueryClientProvider client={newQueryClient()}><Home /></QueryClientProvider>
      </MemoryRouter>,
    )
    fireEvent.click(screen.getByRole('button', { name: /payers/i }))
    fireEvent.click(screen.getByRole('button', { name: /7d/i }))
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((c) => String(c[0]))
      expect(urls.some((u) => u.includes('role=payer') && u.includes('window=7d'))).toBe(true)
    })
  })
})

// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { newQueryClient } from '../App'
import { Entity } from './Entity'

class RO {
  observe() {}
  unobserve() {}
  disconnect() {}
}
;(globalThis as Record<string, unknown>).ResizeObserver = (globalThis as Record<string, unknown>).ResizeObserver ?? RO

const A = '0x' + 'a'.repeat(40)
const B = '0x' + 'b'.repeat(40)
const C = '0x' + 'c'.repeat(40)

const entity = {
  chain: 'base', address: A, roles: ['payer'],
  summaries: { payer: {
    known: { txnCount: 20389, volumeUsdc: '38.36', firstDay: '2026-04-08', lastDay: '2026-06-06', activeDays: 59, distinctCounterparties: 1 },
    all: { txnCount: 20500, volumeUsdc: '40.00', activeDays: 60, distinctCounterparties: 3 },
  } },
}
const row = (address: string) => ({ address, txnCount: 100, volumeUsdc: '90', share: '0.900000', firstSeen: '2026-04-08', lastSeen: '2026-06-06' })
const knownNeighbors = { address: A, lens: 'known', payees: { total: 1, rows: [row(B)] } }
const allNeighbors = { address: A, lens: 'all', payees: { total: 2, rows: [row(B), row(C)] } }

function stubFetch() {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const u = String(url)
    if (u.includes('/neighbors')) {
      return new Response(JSON.stringify(u.includes('lens=all') ? allNeighbors : knownNeighbors), { status: 200 })
    }
    if (u.includes('/timeline')) return new Response(JSON.stringify({ address: A, lens: 'known', roles: {} }), { status: 200 })
    return new Response(JSON.stringify(entity), { status: 200 })
  }))
}
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function mount(path = `/base/address/${A}`) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={newQueryClient()}>
        <Routes><Route path="/base/address/:addr" element={<Entity />} /></Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('Entity route', () => {
  it('renders subject KPIs, counterparty node, and ghost from the all lens', async () => {
    stubFetch()
    mount()
    await waitFor(() => expect(screen.getByText('20,389')).toBeTruthy())
    await waitFor(() => expect(screen.getAllByText(/0xbbbbb…bbbb/).length).toBeGreaterThan(0))
    await waitFor(() => expect(screen.getAllByText(/0xccccc…cccc/).length).toBeGreaterThan(0)) // ghost
    expect(screen.getByText('outside lens')).toBeTruthy()
  })
  it('expand fetches the counterparty neighbors and grows the graph', async () => {
    stubFetch()
    mount()
    await waitFor(() => screen.getByText('+ expand'))
    fireEvent.click(screen.getByText('+ expand'))
    await waitFor(() => {
      const calls = (fetch as ReturnType<typeof vi.fn>).mock.calls.map((c) => String(c[0]))
      expect(calls.some((u) => u.includes(`/entity/${B}/neighbors`))).toBe(true)
    })
  })
  it('unknown address renders the explicit empty state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'not found' }), { status: 404 })))
    mount()
    await waitFor(() => expect(screen.getByText(/unknown address/i)).toBeTruthy(), { timeout: 3000 })
  })
})

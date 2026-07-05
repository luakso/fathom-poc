// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { newQueryClient } from '../App'
import { Tx } from './Tx'

class RO {
  observe() {}
  unobserve() {}
  disconnect() {}
}
;(globalThis as Record<string, unknown>).ResizeObserver = (globalThis as Record<string, unknown>).ResizeObserver ?? RO

const TX = '0x' + 'e'.repeat(64)
const A = '0x' + 'a'.repeat(40)

const g = {
  chain: 'base', txHash: TX, truncated: true,
  nodes: [
    { id: `tx:${TX}`, kind: 'transaction', label: TX, fields: { paid: '0.001881', method: 'transferWithAuthorization', decodable: 'true', status: 'success' } },
    { id: `addr:${A}`, kind: 'address', label: A, roles: ['payer'], fields: { address: A } },
  ],
  edges: [{ id: 'e1', source: `addr:${A}`, target: `tx:${TX}`, kind: 'pays' }],
}

const entityA = {
  chain: 'base', address: A, roles: ['payer'],
  summaries: { payer: {
    known: { txnCount: 42, volumeUsdc: '10.00', firstDay: '2026-04-08', lastDay: '2026-06-06', activeDays: 5, distinctCounterparties: 1 },
  } },
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function stubFetch() {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const u = String(url)
    if (u.includes('/tx/')) return new Response(JSON.stringify(g), { status: 200 })
    if (u.includes(`/entity/${A}`) && !u.includes('/neighbors') && !u.includes('/timeline') && !u.includes('/payments')) {
      return new Response(JSON.stringify(entityA), { status: 200 })
    }
    if (u.includes('/timeline')) return new Response(JSON.stringify({ address: A, lens: 'known', roles: {} }), { status: 200 })
    if (u.includes('/payments')) return new Response(JSON.stringify({ rows: [] }), { status: 200 })
    return new Response(JSON.stringify({ error: 'not found' }), { status: 404 })
  }))
}

describe('Tx route', () => {
  it('renders the dossier canvas, tx detail drawer, and truncation note', async () => {
    stubFetch()
    render(
      <MemoryRouter initialEntries={[`/base/tx/${TX}`]}>
        <QueryClientProvider client={newQueryClient()}>
          <Routes><Route path="/base/tx/:hash" element={<Tx />} /></Routes>
        </QueryClientProvider>
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText(/0xeeeee…eeee/).length).toBeGreaterThan(0))
    expect(screen.getByText(/showing first 128 events/i)).toBeTruthy()
    // "Overview" also labels the drawer tab button; scope to TxDetail's section title
    expect(screen.getByText('Overview', { selector: '.section-title' })).toBeTruthy()
  })
  it('selecting the payer address node remounts the drawer into entity tabs (no orphaned Tx tab)', async () => {
    stubFetch()
    render(
      <MemoryRouter initialEntries={[`/base/tx/${TX}`]}>
        <QueryClientProvider client={newQueryClient()}>
          <Routes><Route path="/base/tx/:hash" element={<Tx />} /></Routes>
        </QueryClientProvider>
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText(/0xeeeee…eeee/).length).toBeGreaterThan(0))
    // select the payer address card instead of the tx node
    fireEvent.click(screen.getAllByText(/0xaaaaa…aaaa/)[0])
    // the 'Tx' tab must be gone and the drawer must not be stuck rendering
    // nothing for a tab that no longer exists in this tab set
    await waitFor(() => expect(screen.getByRole('button', { name: 'Activity' })).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Tx' })).toBeNull()
    await waitFor(() => expect(
      screen.queryByText(/loading identity/i) || screen.queryByText('Identity signals', { selector: '.section-title' }),
    ).toBeTruthy())
  })
  it('unknown tx renders the explicit empty state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'not found' }), { status: 404 })))
    render(
      <MemoryRouter initialEntries={[`/base/tx/${TX}`]}>
        <QueryClientProvider client={newQueryClient()}>
          <Routes><Route path="/base/tx/:hash" element={<Tx />} /></Routes>
        </QueryClientProvider>
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getByText(/unknown transaction/i)).toBeTruthy())
  })
})

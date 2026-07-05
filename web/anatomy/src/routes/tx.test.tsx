// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
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

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('Tx route', () => {
  it('renders the dossier canvas, tx detail drawer, and truncation note', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) =>
      new Response(JSON.stringify(String(url).includes('/tx/') ? g : { error: 'not found' }), { status: String(url).includes('/tx/') ? 200 : 404 })))
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

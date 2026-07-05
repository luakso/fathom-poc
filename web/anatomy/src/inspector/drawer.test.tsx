// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { newQueryClient } from '../App'
import { Drawer } from './Drawer'

const A = '0x' + 'a'.repeat(40)

const entity = {
  chain: 'base', address: A, label: 'api.tollbit.com', labelSource: 'catalog',
  roles: ['payee'],
  signals: [{ source: 'catalog', kind: 'endpoint', value: 'https://api.tollbit.com' }],
  summaries: {
    payee: {
      known: { txnCount: 20389, volumeUsdc: '38.36', firstDay: '2026-04-08', lastDay: '2026-06-06', activeDays: 59, distinctCounterparties: 214 },
      all: { txnCount: 20500, volumeUsdc: '40.00', firstDay: '2026-04-08', lastDay: '2026-06-06', activeDays: 60, distinctCounterparties: 220 },
    },
  },
}
const timeline = { address: A, lens: 'known', roles: { payee: [
  { day: '2026-06-01', txnCount: 10, volumeUsdc: '1' },
  { day: '2026-06-03', txnCount: 30, volumeUsdc: '3' },
] } }

function mountDrawer() {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (String(url).includes('/timeline')) return new Response(JSON.stringify(timeline), { status: 200 })
    return new Response(JSON.stringify(entity), { status: 200 })
  }))
  return render(
    <MemoryRouter>
      <QueryClientProvider client={newQueryClient()}>
        <Drawer chain="base" address={A} lens="known" onNavigate={() => {}} />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}
afterEach(() => {
  vi.unstubAllGlobals()
  cleanup()
})

describe('Drawer', () => {
  it('renders header identity + meta from the entity payload', async () => {
    mountDrawer()
    await waitFor(() => expect(screen.getByText('api.tollbit.com')).toBeTruthy())
    expect(screen.getByText(/Apr 8, 2026/)).toBeTruthy()
    expect(screen.getByText(/catalog/)).toBeTruthy()
  })
  it('overview shows both lens totals', async () => {
    mountDrawer()
    await waitFor(() => expect(screen.getByText('20,389')).toBeTruthy())
    expect(screen.getByText('20,500')).toBeTruthy()
  })
  it('activity tab densifies and renders bars', async () => {
    mountDrawer()
    await waitFor(() => screen.getByText('api.tollbit.com'))
    fireEvent.click(screen.getByRole('button', { name: 'Activity' }))
    await waitFor(() => {
      const bars = document.querySelectorAll('svg rect')
      expect(bars.length).toBe(3) // Jun 1, Jun 2 (zero-filled), Jun 3
    })
  })
})

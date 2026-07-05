// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { AppRoutes, newQueryClient } from './App'

afterEach(() => {
  vi.unstubAllGlobals()
  cleanup()
})

describe('App boot', () => {
  it('renders the home route without crashing', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (String(url).includes('/api/meta')) {
        return new Response(JSON.stringify({
          dataMaxDay: '2026-06-06', builtAt: 'x', methodologyVersion: 2,
          totals: { known: { txnCount: 1, volumeUsdc: '1' }, all: { txnCount: 2, volumeUsdc: '2' } },
        }), { status: 200 })
      }
      return new Response(JSON.stringify({
        role: 'payee', window: 'all', lens: 'known', sort: 'volume', rows: [],
      }), { status: 200 })
    }))
    render(
      <MemoryRouter initialEntries={['/']}>
        <QueryClientProvider client={newQueryClient()}>
          <AppRoutes />
        </QueryClientProvider>
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getByText(/Live leaderboards/)).toBeTruthy())
  })
})

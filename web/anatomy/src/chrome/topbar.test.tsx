// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { newQueryClient } from '../App'
import { TopBar } from './TopBar'

function LocationProbe() {
  const loc = useLocation()
  return <div data-testid="loc">{loc.pathname + loc.search}</div>
}

function mount(initial = '/') {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
    dataMaxDay: '2026-06-06', builtAt: 'x', methodologyVersion: 2,
    totals: { known: { txnCount: 1, volumeUsdc: '1' }, all: { txnCount: 2, volumeUsdc: '2' } },
  }), { status: 200 })))
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <QueryClientProvider client={newQueryClient()}>
        <TopBar />
        <Routes><Route path="*" element={<LocationProbe />} /></Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}
afterEach(() => {
  vi.unstubAllGlobals()
  cleanup()
})

describe('TopBar', () => {
  it('routes a pasted address to the entity page preserving lens', () => {
    mount('/?lens=all')
    const input = screen.getByPlaceholderText(/address or tx hash/i)
    fireEvent.change(input, { target: { value: '0x' + 'A'.repeat(40) } })
    fireEvent.submit(input.closest('form')!)
    expect(screen.getByTestId('loc').textContent).toBe(`/base/address/0x${'a'.repeat(40)}?lens=all`)
  })
  it('routes a tx hash to the tx page', () => {
    mount()
    const input = screen.getByPlaceholderText(/address or tx hash/i)
    fireEvent.change(input, { target: { value: '0x' + 'b'.repeat(64) } })
    fireEvent.submit(input.closest('form')!)
    expect(screen.getByTestId('loc').textContent).toContain(`/base/tx/0x${'b'.repeat(64)}`)
  })
  it('shows inline error on junk', () => {
    mount()
    const input = screen.getByPlaceholderText(/address or tx hash/i)
    fireEvent.change(input, { target: { value: 'garbage' } })
    fireEvent.submit(input.closest('form')!)
    expect(screen.getByText(/not an address or tx hash/i)).toBeTruthy()
    expect(screen.getByTestId('loc').textContent).toBe('/')
  })
  it('lens toggle writes the lens search param in place', () => {
    mount('/base/address/0x' + 'a'.repeat(40))
    fireEvent.click(screen.getByRole('button', { name: /all eip-3009/i }))
    expect(screen.getByTestId('loc').textContent).toContain('lens=all')
  })
})

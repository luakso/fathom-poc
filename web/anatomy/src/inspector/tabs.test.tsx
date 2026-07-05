// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { newQueryClient } from '../App'
import { FingerprintTab } from './FingerprintTab'
import { CounterpartiesTab } from './CounterpartiesTab'
import { PaymentsTab } from './PaymentsTab'

const A = '0x' + 'a'.repeat(40)
const B = '0x' + 'b'.repeat(40)
const TX = '0x' + 'c'.repeat(64)

function withClient(ui: React.ReactElement) {
  return render(<QueryClientProvider client={newQueryClient()}>{ui}</QueryClientProvider>)
}
afterEach(() => {
  vi.unstubAllGlobals()
  cleanup()
})

describe('FingerprintTab', () => {
  it('renders cadence, price points with honesty figure, concentration', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      address: A, lens: 'known',
      roles: { payer: {
        activeDays: 59, spanDays: 60, medianTxnsPerDay: 346, topDayShare: '0.041000',
        pricePoints: [{ amountUsdc: '0.001881', txnCount: 20389 }],
        totalDistinctAmounts: 1, top1Share: '1.000000', top3Share: '1.000000',
      } },
    }), { status: 200 })))
    withClient(<FingerprintTab chain="base" address={A} lens="known" />)
    await waitFor(() => expect(screen.getByText('346')).toBeTruthy())
    expect(screen.getByText('$0.001881')).toBeTruthy()
    expect(screen.getByText(/of 1 distinct/)).toBeTruthy()
    expect(screen.getByText(/top-1 100%/i)).toBeTruthy()
  })
})

describe('CounterpartiesTab', () => {
  it('renders rows and navigates on address click', async () => {
    const page = {
      address: A, role: 'payee', lens: 'known', total: 1,
      rows: [{ address: B, txnCount: 100, volumeUsdc: '90', share: '0.900000', firstSeen: '2026-04-08', lastSeen: '2026-06-06' }],
    }
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(page), { status: 200 })))
    const onNavigate = vi.fn()
    withClient(<CounterpartiesTab chain="base" address={A} lens="known" onNavigate={onNavigate} defaultRole="payee" />)
    await waitFor(() => expect(screen.getByText('$90.00')).toBeTruthy())
    fireEvent.click(screen.getByText(/0xbbbbb…bbbb/))
    expect(onNavigate).toHaveBeenCalledWith(`/base/address/${B}`)
  })
})

describe('PaymentsTab', () => {
  it('renders payment rows and loads more with the cursor', async () => {
    const mk = (next?: string) => ({
      address: A, role: 'payer', lens: 'known', next,
      rows: [{ txHash: TX, logIndex: 1, blockNumber: 2, blockTimestamp: '2026-06-06T23:54:00Z', payer: A, payee: B, facilitator: B, amountUsdc: '0.001881', facilitatorKnown: true }],
    })
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(mk('123:1')), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(mk(undefined)), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const onNavigate = vi.fn()
    withClient(<PaymentsTab chain="base" address={A} lens="known" onNavigate={onNavigate} defaultRole="payer" />)
    await waitFor(() => expect(screen.getAllByText('$0.001881')).toHaveLength(1))
    fireEvent.click(screen.getByText(/load more/i))
    await waitFor(() => expect(screen.getAllByText('$0.001881')).toHaveLength(2))
    expect(String(fetchMock.mock.calls[1][0])).toContain('before=123%3A1')
    fireEvent.click(screen.getAllByText(/0xccccc…cccc/)[0])
    expect(onNavigate).toHaveBeenCalledWith(`/base/tx/${TX}`)
  })
})

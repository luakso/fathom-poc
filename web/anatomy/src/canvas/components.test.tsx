// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Canvas } from './Canvas'
import { buildEntityGraph } from './graph'
import type { Neighbors } from '../lib/schemas'

class RO {
  observe() {}
  unobserve() {}
  disconnect() {}
}
;(globalThis as Record<string, unknown>).ResizeObserver = (globalThis as Record<string, unknown>).ResizeObserver ?? RO

const A = '0x' + 'a'.repeat(40)
const B = '0x' + 'b'.repeat(40)

const neighbors: Neighbors = {
  address: A,
  lens: 'known',
  payees: { total: 1, rows: [{ address: B, txnCount: 20389, volumeUsdc: '38.36', share: '1.000000', firstSeen: '2026-04-08', lastSeen: '2026-06-06' }] },
}
const state = buildEntityGraph(
  { address: A, roles: ['payer'] },
  { payments: '20,389', volume: '$38.36', counterparties: '1' },
  neighbors,
  [],
)

describe('Canvas', () => {
  it('renders subject KPIs and counterparty card', () => {
    render(
      <div style={{ width: 1200, height: 800 }}>
        <Canvas nodes={state.nodes} edges={state.edges} selectedId={A} onSelect={() => {}} />
      </div>,
    )
    expect(screen.getByText('20,389')).toBeTruthy()
    expect(screen.getByText('$38.36')).toBeTruthy()
    expect(screen.getAllByText(/0xbbbbb…bbbb/)).not.toHaveLength(0)
  })
  it('select and expand callbacks fire', () => {
    const onSelect = vi.fn()
    const onExpand = vi.fn()
    render(
      <div style={{ width: 1200, height: 800 }}>
        <Canvas nodes={state.nodes} edges={state.edges} selectedId={A} onSelect={onSelect} onExpand={onExpand} />
      </div>,
    )
    fireEvent.click(screen.getByText('+ expand'))
    expect(onExpand).toHaveBeenCalledWith(B)
  })
})

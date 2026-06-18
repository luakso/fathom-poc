import { describe, it, expect } from 'vitest'
import { applyStats } from './adapter.js'

describe('applyStats', () => {
  const flow = {
    nodes: [
      { id: 'addr:0xp', type: 'address', position: { x: 0, y: 0 }, data: { id: 'addr:0xp', fields: {} } },
    ],
    edges: [],
  }
  it('merges stats into the matching node immutably', () => {
    const next = applyStats(flow, 'addr:0xp', { paymentCount: 5 })
    expect(next.nodes[0].data.stats.paymentCount).toBe(5)
    expect(flow.nodes[0].data.stats).toBeUndefined() // original untouched
  })
  it('leaves other nodes unchanged', () => {
    const next = applyStats(flow, 'addr:other', { paymentCount: 9 })
    expect(next.nodes[0].data.stats).toBeUndefined()
  })
})

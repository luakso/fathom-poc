import { describe, it, expect } from 'vitest'
import { txGraphToFlow } from './txgraph'
import { overlaps, NODE_W, NODE_H, GHOST_W, GHOST_H } from './layout'
import type { Graph } from '../lib/schemas'

const A = '0x' + 'a'.repeat(40)
const B = '0x' + 'b'.repeat(40)
const F = '0x' + 'f'.repeat(40)
const TX = '0x' + 'e'.repeat(64)

const g: Graph = {
  chain: 'base', txHash: TX,
  nodes: [
    { id: `tx:${TX}`, kind: 'transaction', label: TX, fields: { paid: '0.001881' } },
    { id: 'ev:1', kind: 'event', label: 'Transfer', fields: {} },
    { id: `addr:${A}`, kind: 'address', label: A, roles: ['payer'], fields: {} },
    { id: `addr:${B}`, kind: 'address', label: B, roles: ['payee'], fields: {} },
    { id: `addr:${F}`, kind: 'address', label: F, roles: ['facilitator'], fields: {} },
  ],
  edges: [
    { id: 'e1', source: `addr:${A}`, target: `tx:${TX}`, kind: 'pays', label: '0.001881 USDC' },
    { id: 'e2', source: `tx:${TX}`, target: `addr:${B}`, kind: 'pays' },
    { id: 'e3', source: `addr:${F}`, target: `tx:${TX}`, kind: 'settles' },
    { id: 'e4', source: `tx:${TX}`, target: 'ev:1', kind: 'emits' },
  ],
}

describe('txGraphToFlow', () => {
  const flow = txGraphToFlow(g)
  it('keeps all nodes and edges', () => {
    expect(flow.nodes).toHaveLength(5)
    expect(flow.edges).toHaveLength(4)
  })
  it('lanes: payer left, payee right, facilitator above, events below', () => {
    const pos = Object.fromEntries(flow.nodes.map((n) => [n.id, n.position]))
    const tx = pos[`tx:${TX}`]
    expect(pos[`addr:${A}`].x).toBeLessThan(tx.x)
    expect(pos[`addr:${B}`].x).toBeGreaterThan(tx.x)
    expect(pos[`addr:${F}`].y).toBeLessThan(tx.y)
    expect(pos['ev:1'].y).toBeGreaterThan(tx.y)
  })
  it('settles edges get the verb styling flag', () => {
    expect(flow.edges.find((e) => e.id === 'e3')?.data?.verb).toBe(true)
  })
  it('no overlapping boxes', () => {
    const rects = flow.nodes.map((n) => ({
      x: n.position.x, y: n.position.y,
      w: n.type === 'event' ? GHOST_W : NODE_W,
      h: n.type === 'event' ? GHOST_H : NODE_H,
    }))
    for (let i = 0; i < rects.length; i++)
      for (let j = i + 1; j < rects.length; j++)
        expect(overlaps(rects[i], rects[j])).toBe(false)
  })
})

import { describe, it, expect } from 'vitest'
import { graphToFlow, NODE_SIZE } from './adapter.js'

const sample = {
  chain: 'base',
  txHash: '0xabc',
  nodes: [
    { id: 'tx:0xabc', kind: 'transaction', label: '0xabc', fields: {} },
    { id: 'evt:0xabc:0', kind: 'event', label: 'log 0', fields: {} },
    { id: 'addr:0xp', kind: 'address', label: '0xp', roles: ['payer'], fields: {} },
  ],
  edges: [{ id: 'e1', source: 'tx:0xabc', target: 'evt:0xabc:0', kind: 'emits' }],
}

// A richer dossier: 2 events, distinct payer/payee/facilitator, plus a second
// payer — exercises multi-node lane stacking.
const rich = {
  chain: 'base',
  txHash: '0xdef',
  nodes: [
    { id: 'tx:0xdef', kind: 'transaction', label: '0xdef', fields: {} },
    { id: 'evt:0xdef:0', kind: 'event', label: 'log 0', fields: {} },
    { id: 'evt:0xdef:1', kind: 'event', label: 'log 1', fields: {} },
    { id: 'addr:p1', kind: 'address', label: 'p1', roles: ['payer'], fields: {} },
    { id: 'addr:p2', kind: 'address', label: 'p2', roles: ['payer'], fields: {} },
    { id: 'addr:y1', kind: 'address', label: 'y1', roles: ['payee'], fields: {} },
    { id: 'addr:f1', kind: 'address', label: 'f1', roles: ['facilitator'], fields: {} },
  ],
  edges: [],
}

function box(node) {
  const s = NODE_SIZE[node.type]
  return { x1: node.position.x, y1: node.position.y, x2: node.position.x + s.w, y2: node.position.y + s.h }
}
function overlaps(a, b) {
  return a.x1 < b.x2 && b.x1 < a.x2 && a.y1 < b.y2 && b.y1 < a.y2
}

describe('graphToFlow', () => {
  it('maps every node and edge', () => {
    const { nodes, edges } = graphToFlow(sample)
    expect(nodes).toHaveLength(3)
    expect(edges).toHaveLength(1)
  })
  it('assigns react-flow types from kind', () => {
    const { nodes } = graphToFlow(sample)
    expect(nodes.find((n) => n.id === 'tx:0xabc').type).toBe('transaction')
    expect(nodes.find((n) => n.id === 'addr:0xp').type).toBe('address')
  })
  it('is deterministic', () => {
    const a = graphToFlow(sample)
    const b = graphToFlow(sample)
    expect(a.nodes.map((n) => n.position)).toEqual(b.nodes.map((n) => n.position))
  })
})

describe('graphToFlow layout (role lanes)', () => {
  it('orders lanes left→right: facilitator < payer < tx < payee', () => {
    const { nodes } = graphToFlow(rich)
    const x = (id) => nodes.find((n) => n.id === id).position.x
    expect(x('addr:f1')).toBeLessThan(x('addr:p1'))
    expect(x('addr:p1')).toBeLessThan(x('tx:0xdef'))
    expect(x('tx:0xdef')).toBeLessThan(x('addr:y1'))
  })

  it('stacks events directly below the TX node, sharing its center lane', () => {
    const { nodes } = graphToFlow(rich)
    const center = (n) => n.position.x + NODE_SIZE[n.type].w / 2
    const tx = nodes.find((n) => n.id === 'tx:0xdef')
    const e0 = nodes.find((n) => n.id === 'evt:0xdef:0')
    const e1 = nodes.find((n) => n.id === 'evt:0xdef:1')
    expect(e0.position.y).toBeGreaterThan(tx.position.y)
    expect(e1.position.y).toBeGreaterThan(e0.position.y)
    expect(center(e0)).toBe(center(tx)) // same center lane
    expect(center(e1)).toBe(center(tx))
  })

  it('places the two payers in the same lane (same x), stacked', () => {
    const { nodes } = graphToFlow(rich)
    const p1 = nodes.find((n) => n.id === 'addr:p1')
    const p2 = nodes.find((n) => n.id === 'addr:p2')
    expect(p1.position.x).toBe(p2.position.x)
    expect(p1.position.y).not.toBe(p2.position.y)
  })

  it('produces no overlapping node boxes', () => {
    const { nodes } = graphToFlow(rich)
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        expect(overlaps(box(nodes[i]), box(nodes[j])), `${nodes[i].id} overlaps ${nodes[j].id}`).toBe(false)
      }
    }
  })
})

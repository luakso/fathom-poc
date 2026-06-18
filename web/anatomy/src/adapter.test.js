import { describe, it, expect } from 'vitest'
import { graphToFlow } from './adapter.js'

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

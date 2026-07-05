import { describe, it, expect } from 'vitest'
import { buildEntityGraph, expandNode, ghostRows } from './graph'
import { overlaps, NODE_W, NODE_H, GHOST_W, GHOST_H } from './layout'
import type { Neighbors, NeighborRow } from '../lib/schemas'

const A = '0x' + 'a'.repeat(40)
const B = '0x' + 'b'.repeat(40)
const C = '0x' + 'c'.repeat(40)
const D = '0x' + 'd'.repeat(40)
const F = '0x' + 'f'.repeat(40)

function row(address: string, share = '0.500000', volumeUsdc = '10', txnCount = 100): NeighborRow {
  return { address, txnCount, volumeUsdc, share, firstSeen: '2026-04-08', lastSeen: '2026-06-06' }
}
const subject = { address: A, roles: ['payer'] }
const kpis = { payments: '20,389', volume: '$38.36', counterparties: '1' }

function neighbors(partial: Partial<Neighbors>): Neighbors {
  return { address: A, lens: 'known', ...partial }
}

describe('buildEntityGraph', () => {
  const state = buildEntityGraph(subject, kpis, neighbors({
    payees: { total: 2, rows: [row(B, '0.900000', '90'), row(C, '0.100000', '10')] },
    facilitators: { total: 1, rows: [row(F, '1.000000')] },
  }), [row(D, '0.000100', '0.01', 1)])

  it('creates subject + payees + facilitator + ghost with address ids', () => {
    expect(state.nodes.map((n) => n.id).sort()).toEqual([A, B, C, D, F].sort())
    expect(state.nodes.find((n) => n.id === A)?.data.roleClass).toBe('subject')
    expect(state.nodes.find((n) => n.id === D)?.type).toBe('ghost')
  })
  it('payees sit right of subject, facilitator above', () => {
    const sub = state.nodes.find((n) => n.id === A)!
    expect(state.nodes.find((n) => n.id === B)!.position.x).toBeGreaterThan(sub.position.x)
    expect(state.nodes.find((n) => n.id === F)!.position.y).toBeLessThan(sub.position.y)
  })
  it('edge directions and kinds', () => {
    expect(state.edges.find((e) => e.id === `${A}->${B}`)).toBeTruthy()
    expect(state.edges.find((e) => e.id === `${F}=>${A}`)?.data?.label).toBe('settles')
  })
  it('exactly one primary edge, the top-share one', () => {
    const primaries = state.edges.filter((e) => e.data?.primary)
    expect(primaries).toHaveLength(1)
    expect(primaries[0].id).toBe(`${A}->${B}`)
  })
  it('edge label composes usd · txns · share', () => {
    expect(state.edges.find((e) => e.id === `${A}->${B}`)?.data?.label).toBe('$90.00 · 100 txns · 90%')
  })
  it('no two node boxes overlap', () => {
    const rects = state.nodes.map((n) => ({
      x: n.position.x, y: n.position.y,
      w: n.type === 'ghost' ? GHOST_W : NODE_W,
      h: n.type === 'ghost' ? GHOST_H : NODE_H + (n.data.roleClass === 'subject' ? 40 : 0),
    }))
    for (let i = 0; i < rects.length; i++)
      for (let j = i + 1; j < rects.length; j++)
        expect(overlaps(rects[i], rects[j])).toBe(false)
  })
})

describe('expandNode', () => {
  const base = buildEntityGraph(subject, kpis, neighbors({
    payees: { total: 1, rows: [row(B, '1.000000')] },
  }), [])
  const grown = expandNode(base, B, neighbors({
    address: B,
    payers: { total: 2, rows: [row(A, '0.700000'), row(C, '0.300000')] },
    payees: { total: 1, rows: [row(D, '1.000000')] },
  }))

  it('dedupes by address: A and B keep their positions', () => {
    for (const id of [A, B]) {
      expect(grown.nodes.find((n) => n.id === id)?.position)
        .toEqual(base.nodes.find((n) => n.id === id)?.position)
    }
    expect(grown.nodes.filter((n) => n.id === A)).toHaveLength(1)
  })
  it('adds new nodes and edges without duplicating existing edges', () => {
    expect(grown.nodes.map((n) => n.id).sort()).toEqual([A, B, C, D].sort())
    expect(grown.edges.filter((e) => e.id === `${A}->${B}`)).toHaveLength(1)
    expect(grown.edges.find((e) => e.id === `${C}->${B}`)).toBeTruthy()
    expect(grown.edges.find((e) => e.id === `${B}->${D}`)).toBeTruthy()
  })
  it('does not mutate the input state', () => {
    expect(base.nodes).toHaveLength(2)
    expect(base.edges).toHaveLength(1)
  })
  it('never promotes a new primary edge', () => {
    expect(grown.edges.filter((e) => e.data?.primary)).toHaveLength(1)
  })
})

describe('ghostRows', () => {
  it('returns all-lens rows missing from known, capped', () => {
    const known = [row(B)]
    const all = [row(B), row(C), row(D), row(F)]
    expect(ghostRows(known, all, 2).map((r) => r.address)).toEqual([C, D])
  })
})

import { describe, it, expect } from 'vitest'
import {
  MetaSchema, EntitySchema, NeighborsSchema, TimelineSchema,
  FingerprintSchema, CounterpartyPageSchema, PaymentPageSchema,
  LeaderboardSchema, GraphSchema,
} from './schemas'

const row = {
  address: '0x' + 'a'.repeat(40), txnCount: 20389, volumeUsdc: '38.36',
  share: '1.000000', firstSeen: '2026-04-08', lastSeen: '2026-06-06',
}

describe('schemas mirror the Go API', () => {
  it('meta', () => {
    const m = MetaSchema.parse({
      dataMaxDay: '2026-06-06', builtAt: '2026-07-05T00:00:00Z', methodologyVersion: 2,
      totals: { known: { txnCount: 21190000, volumeUsdc: '7540000.12' }, all: { txnCount: 22767310, volumeUsdc: '463900000' } },
    })
    expect(m.totals.known.volumeUsdc).toBe('7540000.12')
  })
  it('entity with omitted optionals', () => {
    const e = EntitySchema.parse({
      chain: 'base', address: row.address, roles: ['payer'],
      summaries: { payer: { known: { txnCount: 1, volumeUsdc: '1', activeDays: 1, distinctCounterparties: 1 } } },
    })
    expect(e.label).toBeUndefined()
    expect(e.summaries.payer.known.firstDay).toBeUndefined()
  })
  it('neighbors with omitted directions', () => {
    const n = NeighborsSchema.parse({ address: row.address, lens: 'known', payees: { total: 1, rows: [row] } })
    expect(n.payers).toBeUndefined()
    expect(n.payees?.rows[0].share).toBe('1.000000')
  })
  it('timeline / fingerprint', () => {
    TimelineSchema.parse({ address: row.address, lens: 'known', roles: { payer: [{ day: '2026-06-06', txnCount: 3, volumeUsdc: '0.01' }] } })
    const f = FingerprintSchema.parse({
      address: row.address, lens: 'all',
      roles: { payer: { activeDays: 59, spanDays: 60, medianTxnsPerDay: 346, topDayShare: '0.041000', pricePoints: [{ amountUsdc: '0.001881', txnCount: 20389 }], totalDistinctAmounts: null, top1Share: '1.000000', top3Share: '1.000000' } },
    })
    expect(f.roles.payer.totalDistinctAmounts).toBeNull()
  })
  it('pages + leaderboard + graph', () => {
    CounterpartyPageSchema.parse({ address: row.address, role: 'payee', lens: 'known', total: 1, rows: [row] })
    PaymentPageSchema.parse({ address: row.address, role: 'payer', lens: 'known', rows: [{ txHash: '0x' + 'b'.repeat(64), logIndex: 1, blockNumber: 2, blockTimestamp: '2026-06-06T23:54:00Z', payer: row.address, payee: row.address, facilitator: row.address, amountUsdc: '0.001881', facilitatorKnown: true }] })
    LeaderboardSchema.parse({ role: 'payee', window: 'all', lens: 'known', sort: 'volume', rows: [{ rank: 1, ...row, distinctCounterparties: 214 }] })
    const g = GraphSchema.parse({ chain: 'base', txHash: '0x' + 'b'.repeat(64), nodes: [{ id: 'tx:0xb', kind: 'transaction', label: 'tx', fields: { paid: '0.001881' } }], edges: [{ id: 'e1', source: 'a', target: 'b', kind: 'flow' }] })
    expect(g.truncated).toBeUndefined()
  })
  it('rejects a typo\'d field type loudly', () => {
    expect(() => MetaSchema.parse({ dataMaxDay: '2026-06-06', builtAt: 'x', methodologyVersion: '2', totals: {} })).toThrow()
  })
})

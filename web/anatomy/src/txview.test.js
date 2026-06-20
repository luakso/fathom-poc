import { describe, it, expect } from 'vitest'
import { buildTxView } from './txview.js'

const now = Date.parse('2026-06-15T10:00:00Z')

const base = {
  paid: '0.002000', tokenSymbol: 'USDC',
  totalFeeWei: '8574617501899',
  method: 'transferWithAuthorization', methodKind: 'v,r,s', methodId: '0xe3ee160e',
  block: '46958198', timestamp: '2026-06-01T10:00:00Z', blockHash: '0xblockhash',
  from: '0xfac', calledContract: '0x833589fcd6edb6e08f4c7c32d4f71b54bda02913',
  contractLabel: 'USDC · Circle', txValue: '0',
  gasUsed: '85720', gasLimit: '95307', effectiveGasPrice: '10000000',
  baseFee: '5000000', maxFee: '30000000', maxPriorityFee: '5000000',
  gasCostWei: '8572000000000', l1Fee: '2617501899', l1GasUsed: '3307', l1GasPrice: '123539468',
  txType: '2', txNonce: '1382712', transactionIndex: '51',
  tokenDecimals: '6', eventCount: '1', inputCalldata: '0xdeadbeef',
  status: 'success', explorerUrl: 'https://basescan.org/tx/0xtxE',
  decodable: 'true', dpFrom: '0xp', dpTo: '0xq', dpValue: '0.002000',
  dpValidAfter: '', dpValidBefore: '2026-06-01T11:00:00Z', dpNonce: '0xabcd',
}

describe('buildTxView', () => {
  it('builds the headline with paid leading and fee in ETH', () => {
    const v = buildTxView(base, now)
    expect(v.headline.paid).toBe('0.002000 USDC')
    expect(v.headline.fee).toBe('0.000008574617501899 Ξ')
    expect(v.headline.method).toBe('transferWithAuthorization')
    expect(v.headline.methodKind).toBe('v,r,s')
    expect(v.headline.age).toBe('14d ago')
    expect(v.headline.events).toBe('1')
  })

  it('labels the contract and gas utilization in sections', () => {
    const v = buildTxView(base, now)
    const overview = v.sections.find((s) => s.title === 'Overview')
    expect(overview.rows.find((r) => r.k === 'interacted with').v).toBe('USDC · Circle')
    expect(overview.rows.find((r) => r.k === 'block hash').v).toBe('0xblockhash')
    const gas = v.sections.find((s) => s.title === 'Gas & Fees')
    expect(gas.rows.find((r) => r.k === 'gas limit / used').v).toBe('95,307 / 85,720 (89.9%)')
  })

  it('exposes a decoded param table when decodable', () => {
    const v = buildTxView(base, now)
    expect(v.input.decodable).toBe(true)
    expect(v.input.params.find((p) => p.k === 'from').v).toBe('0xp')
    expect(v.input.params.find((p) => p.k === 'validAfter').v).toBe('—')
  })

  it('omits the param table when not decodable', () => {
    const v = buildTxView({ ...base, decodable: 'false', eventCount: '14', method: 'aggregate3', methodKind: 'multicall' }, now)
    expect(v.input.decodable).toBe(false)
    expect(v.input.params).toBeNull()
  })

  it('renders NULL fields as dash', () => {
    const v = buildTxView({ ...base, l1Fee: '', transactionIndex: '', blockHash: '' }, now)
    const gas = v.sections.find((s) => s.title === 'Gas & Fees')
    expect(gas.rows.find((r) => r.k === 'L1 fee').v).toBe('—')
    const attrs = v.sections.find((s) => s.title === 'Attributes')
    expect(attrs.rows.find((r) => r.k === 'position in block').v).toBe('—')
    const ov = v.sections.find((s) => s.title === 'Overview')
    expect(ov.rows.find((r) => r.k === 'block hash').v).toBe('—')
  })

  it('passes through the basescan url', () => {
    expect(buildTxView(base, now).basescanUrl).toBe('https://basescan.org/tx/0xtxE')
  })

  it('collapses fully-missing composite rows to a single dash', () => {
    const v = buildTxView({ ...base, gasLimit: '', gasUsed: '', baseFee: '', maxFee: '', maxPriorityFee: '', l1GasPrice: '', l1GasUsed: '' }, now)
    const gas = v.sections.find((s) => s.title === 'Gas & Fees')
    expect(gas.rows.find((r) => r.k === 'gas limit / used').v).toBe('—')
    expect(gas.rows.find((r) => r.k === 'base / max / prio').v).toBe('—')
    expect(gas.rows.find((r) => r.k === 'L1 gas price / used').v).toBe('—')
  })

  it('keeps composite rows when sub-values are present', () => {
    const v = buildTxView(base, now)
    const gas = v.sections.find((s) => s.title === 'Gas & Fees')
    expect(gas.rows.find((r) => r.k === 'base / max / prio').v).toBe('0.005 / 0.03 / 0.005 Gwei')
    expect(gas.rows.find((r) => r.k === 'L1 gas price / used').v).toBe('0.123539468 Gwei / 3,307')
  })
})

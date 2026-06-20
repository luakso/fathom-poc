import { weiToEth, gwei, groupDigits, relativeAge } from './format.js'

const dash = (s) => (s == null || s === '' ? '—' : s)
const eth = (wei) => (wei == null || wei === '' ? '—' : `${weiToEth(wei)} Ξ`)
const allDash = (parts) => parts.every((p) => p === '—')

function utilization(limit, used) {
  if (!limit || !used) return '—'
  const pct = ((Number(used) / Number(limit)) * 100).toFixed(1)
  return `${groupDigits(limit)} / ${groupDigits(used)} (${pct}%)`
}

// buildTxView maps a transaction node's Fields map to the view-model the headline
// and detail panel render. nowMs is injected for deterministic relative ages.
export function buildTxView(f, nowMs) {
  const sym = f.tokenSymbol || 'USDC'

  const headline = {
    paid: f.paid ? `${f.paid} ${sym}` : '—',
    fee: eth(f.totalFeeWei),
    method: f.method || '—',
    methodKind: f.methodKind || '',
    age: relativeAge(f.timestamp, nowMs),
    events: dash(f.eventCount),
  }

  const overview = {
    title: 'Overview',
    rows: [
      { k: 'status', v: f.status === 'success' ? 'Success' : dash(f.status), note: 'inferred — only settled payments are indexed' },
      { k: 'paid (total)', v: f.paid ? `${f.paid} ${sym}` : '—' },
      { k: 'block', v: groupDigits(f.block) },
      { k: 'block hash', v: dash(f.blockHash) },
      { k: 'timestamp', v: dash(f.timestamp) },
      { k: 'from (gas payer)', v: dash(f.from) },
      { k: 'interacted with', v: f.contractLabel || dash(f.calledContract) },
      { k: 'native value', v: eth(f.txValue), note: 'ETH sent — n/a for x402' },
    ],
  }

  const gas = {
    title: 'Gas & Fees',
    rows: [
      { k: 'total fee', v: eth(f.totalFeeWei) },
      { k: 'gas price', v: f.effectiveGasPrice ? `${gwei(f.effectiveGasPrice)} Gwei` : '—' },
      { k: 'gas limit / used', v: utilization(f.gasLimit, f.gasUsed) },
      { k: 'base / max / prio', v: (() => {
        const parts = [gwei(f.baseFee), gwei(f.maxFee), gwei(f.maxPriorityFee)]
        return allDash(parts) ? '—' : `${parts.join(' / ')} Gwei`
      })() },
      { k: 'L2 fee', v: eth(f.gasCostWei) },
      { k: 'L1 fee', v: eth(f.l1Fee) },
      { k: 'L1 gas price / used', v: (() => {
        const price = gwei(f.l1GasPrice)
        const used = groupDigits(f.l1GasUsed)
        return price === '—' && used === '—' ? '—' : `${price} Gwei / ${used}`
      })() },
    ],
  }

  const attributes = {
    title: 'Attributes',
    rows: [
      { k: 'txn type', v: f.txType === '2' ? '2 (EIP-1559)' : dash(f.txType) },
      { k: 'nonce', v: groupDigits(f.txNonce) },
      { k: 'position in block', v: dash(f.transactionIndex) },
    ],
  }

  const decodable = f.decodable === 'true'
  const input = {
    method: f.method || '—',
    methodId: dash(f.methodId),
    decodable,
    params: decodable
      ? [
          { k: 'from', v: dash(f.dpFrom) },
          { k: 'to', v: dash(f.dpTo) },
          { k: 'value', v: f.dpValue ? `${f.dpValue} ${sym}` : '—' },
          { k: 'validAfter', v: dash(f.dpValidAfter) },
          { k: 'validBefore', v: dash(f.dpValidBefore) },
          { k: 'nonce', v: dash(f.dpNonce) },
          { k: 'v / r / s', v: 'in raw calldata' },
        ]
      : null,
    rawCalldata: dash(f.inputCalldata),
  }

  return { headline, sections: [overview, gas, attributes], input, basescanUrl: f.explorerUrl || '' }
}

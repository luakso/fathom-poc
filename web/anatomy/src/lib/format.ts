// Pure display formatters. Each tolerates null/'' inputs. Money arrives as
// decimal strings from the API; never parseFloat for display arithmetic.

function fromBaseUnits(wei: string | null | undefined, decimals: number): string {
  if (wei == null || wei === '') return '—'
  const digits = String(wei).replace(/\D/g, '')
  if (digits === '') return '—'
  const padded = digits.padStart(decimals + 1, '0')
  const intPart = padded.slice(0, padded.length - decimals).replace(/^0+(?=\d)/, '')
  const frac = padded.slice(padded.length - decimals).replace(/0+$/, '')
  return frac ? `${intPart}.${frac}` : intPart
}

export function weiToEth(wei: string | null | undefined): string {
  return fromBaseUnits(wei, 18)
}

export function gwei(wei: string | null | undefined): string {
  return fromBaseUnits(wei, 9)
}

export function groupDigits(s: string | number | null | undefined): string {
  if (s == null || s === '') return '—'
  return String(s).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

export function relativeAge(iso: string | null | undefined, nowMs: number): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  const s = Math.max(0, Math.floor((nowMs - t) / 1000))
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

export function short(s: string | null | undefined): string {
  if (!s) return ''
  return s.length > 12 ? `${s.slice(0, 7)}…${s.slice(-4)}` : s
}

// usd renders an API decimal string as dollars: 2dp with digit grouping for
// >= $0.01, full precision below (micro-payment price points must stay exact).
export function usd(amount: string | null | undefined): string {
  if (amount == null || amount === '') return '—'
  const neg = amount.startsWith('-')
  const abs = neg ? amount.slice(1) : amount
  const [intRaw = '', fracRaw = ''] = abs.split('.')
  const int = intRaw.replace(/^0+(?=\d)/, '') || '0'
  const isSubCent = int === '0' && !/^[1-9]/.test(fracRaw.slice(0, 2))
  if (isSubCent && fracRaw.replace(/0+$/, '') !== '') {
    return `${neg ? '-' : ''}$0.${fracRaw.replace(/0+$/, '')}`
  }
  // round to 2dp using string math on the third fraction digit
  let cents = (fracRaw + '00').slice(0, 2)
  let intOut = int
  if ((fracRaw[2] ?? '0') >= '5') {
    const bumped = String(BigInt(int + cents) + 1n).padStart(3, '0')
    intOut = bumped.slice(0, -2)
    cents = bumped.slice(-2)
  }
  return `${neg ? '-' : ''}$${groupDigits(intOut)}.${cents}`
}

// pct renders a share fraction string ("0.415000") as a percentage.
export function pct(share: string | null | undefined): string {
  if (share == null || share === '') return '—'
  const n = Number(share)
  if (Number.isNaN(n)) return '—'
  const p = n * 100
  if (p >= 100) return '100%'
  if (p >= 10) return `${p.toFixed(1).replace(/\.0$/, '')}%`
  if (p >= 1) return `${p.toFixed(1).replace(/\.0$/, '')}%`
  return `${p.toFixed(2).replace(/0$/, '').replace(/\.$/, '')}%`
}

export function dayLabel(day: string | null | undefined): string {
  if (!day) return '—'
  const t = Date.parse(`${day}T00:00:00Z`)
  if (Number.isNaN(t)) return '—'
  return new Date(t).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' })
}

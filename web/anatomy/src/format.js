// Pure display formatters. Each tolerates null/"" inputs.

function fromBaseUnits(wei, decimals) {
  if (wei == null || wei === '') return '—'
  const digits = String(wei).replace(/\D/g, '')
  if (digits === '') return '—'
  const padded = digits.padStart(decimals + 1, '0')
  const intPart = padded.slice(0, padded.length - decimals).replace(/^0+(?=\d)/, '')
  const frac = padded.slice(padded.length - decimals).replace(/0+$/, '')
  return frac ? `${intPart}.${frac}` : intPart
}

export function weiToEth(wei) {
  return fromBaseUnits(wei, 18)
}

export function gwei(wei) {
  return fromBaseUnits(wei, 9)
}

export function groupDigits(s) {
  if (s == null || s === '') return '—'
  return String(s).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

export function relativeAge(iso, nowMs) {
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

export function short(s) {
  if (!s) return ''
  return s.length > 12 ? `${s.slice(0, 7)}…${s.slice(-4)}` : s
}

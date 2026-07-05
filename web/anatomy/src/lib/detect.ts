// Auto-detect what the user pasted into search (spec §2.1):
// 0x + 40 hex -> entity route, 0x + 64 hex -> tx route, else invalid.

export type Detected = { kind: 'address' | 'tx'; value: string } | { kind: 'invalid' }

export function detectQuery(raw: string): Detected {
  const s = raw.trim().toLowerCase()
  if (/^0x[0-9a-f]{40}$/.test(s)) return { kind: 'address', value: s }
  if (/^0x[0-9a-f]{64}$/.test(s)) return { kind: 'tx', value: s }
  return { kind: 'invalid' }
}

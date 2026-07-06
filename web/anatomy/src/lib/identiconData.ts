// Deterministic address fingerprint (pure, no deps). 5x5 symmetric on/off
// grid + hue derived from the address (same output as the v1 module).

function hash32(str: string): number {
  let h = 0x811c9dc5 // FNV-1a 32-bit
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

export function identiconData(address: string): { hue: number; cells: boolean[][] } {
  const h = hash32((address || '').toLowerCase())
  const hue = h % 360
  const cells: boolean[][] = []
  for (let r = 0; r < 5; r++) {
    const row: boolean[] = []
    for (let c = 0; c < 3; c++) {
      row.push(Boolean((h >>> ((r * 3 + c) % 31)) & 1))
    }
    row.push(row[1] ?? false)
    row.push(row[0] ?? false)
    cells.push(row)
  }
  return { hue, cells }
}

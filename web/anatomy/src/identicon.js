// Deterministic address fingerprint (pure, no deps). Produces a 5x5 symmetric
// on/off grid + a hue, both derived from the address — a GitHub-identicon-style
// visual so each address is recognizable at a glance.

function hash32(str) {
  let h = 0x811c9dc5 // FNV-1a 32-bit
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

// identiconData returns { hue, cells } where cells is a 5x5 boolean grid,
// left-right symmetric (col4=col0, col3=col1). Case-insensitive on the address.
export function identiconData(address) {
  const h = hash32((address || '').toLowerCase())
  const hue = h % 360
  const cells = []
  for (let r = 0; r < 5; r++) {
    const row = []
    for (let c = 0; c < 3; c++) {
      row.push(Boolean((h >>> ((r * 3 + c) % 31)) & 1))
    }
    row.push(row[1]) // mirror
    row.push(row[0])
    cells.push(row)
  }
  return { hue, cells }
}

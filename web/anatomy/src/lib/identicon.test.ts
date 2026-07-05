import { describe, it, expect } from 'vitest'
import { identiconData } from './identicon'

describe('identiconData', () => {
  const addr = '0x5b2b12aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaf2b1'
  it('is deterministic and case-insensitive', () => {
    expect(identiconData(addr)).toEqual(identiconData(addr.toUpperCase()))
  })
  it('is a symmetric 5x5 grid with hue in range', () => {
    const { hue, cells } = identiconData(addr)
    expect(hue).toBeGreaterThanOrEqual(0)
    expect(hue).toBeLessThan(360)
    expect(cells).toHaveLength(5)
    for (const row of cells) {
      expect(row).toHaveLength(5)
      expect(row[4]).toBe(row[0])
      expect(row[3]).toBe(row[1])
    }
  })
  it('differs across addresses', () => {
    expect(identiconData(addr)).not.toEqual(identiconData('0x' + '1'.repeat(40)))
  })
})

import { describe, it, expect } from 'vitest'
import { identiconData } from './identicon.js'

describe('identiconData', () => {
  it('is deterministic and case-insensitive', () => {
    expect(identiconData('0xAbC123')).toEqual(identiconData('0xabc123'))
  })
  it('produces a 5x5 left-right symmetric grid', () => {
    const { cells } = identiconData('0xdeadbeef')
    expect(cells).toHaveLength(5)
    for (const row of cells) {
      expect(row).toHaveLength(5)
      expect(row[0]).toBe(row[4])
      expect(row[1]).toBe(row[3])
    }
  })
  it('hue is within [0,360)', () => {
    const { hue } = identiconData('0xdeadbeef')
    expect(hue).toBeGreaterThanOrEqual(0)
    expect(hue).toBeLessThan(360)
  })
  it('different addresses yield different fingerprints', () => {
    const a = identiconData('0x1111111111111111111111111111111111111111')
    const b = identiconData('0x2222222222222222222222222222222222222222')
    expect(a.hue === b.hue && JSON.stringify(a.cells) === JSON.stringify(b.cells)).toBe(false)
  })
})

import { describe, it, expect } from 'vitest'
import { densify } from './series'

describe('densify', () => {
  it('fills gaps with zero days', () => {
    const out = densify([
      { day: '2026-06-01', txnCount: 5, volumeUsdc: '1.5' },
      { day: '2026-06-04', txnCount: 2, volumeUsdc: '0.2' },
    ])
    expect(out.map((p) => p.day)).toEqual(['2026-06-01', '2026-06-02', '2026-06-03', '2026-06-04'])
    expect(out[1]).toEqual({ day: '2026-06-02', txnCount: 0, volumeUsdc: '0' })
  })
  it('handles month boundary', () => {
    const out = densify([
      { day: '2026-05-31', txnCount: 1, volumeUsdc: '1' },
      { day: '2026-06-01', txnCount: 1, volumeUsdc: '1' },
    ])
    expect(out).toHaveLength(2)
  })
  it('sorts unsorted input and passes single point through', () => {
    expect(densify([{ day: '2026-06-01', txnCount: 1, volumeUsdc: '1' }])).toHaveLength(1)
    expect(densify([])).toEqual([])
  })
})

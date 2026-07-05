import { describe, it, expect } from 'vitest'
import { weiToEth, gwei, groupDigits, relativeAge, short, usd, pct, dayLabel } from './format'

describe('format (ported)', () => {
  it('weiToEth', () => {
    expect(weiToEth('1000000000000000000')).toBe('1')
    expect(weiToEth('1500000000000000')).toBe('0.0015')
    expect(weiToEth('')).toBe('—')
    expect(weiToEth(null)).toBe('—')
  })
  it('gwei', () => expect(gwei('2500000000')).toBe('2.5'))
  it('groupDigits', () => {
    expect(groupDigits('1234567')).toBe('1,234,567')
    expect(groupDigits('')).toBe('—')
  })
  it('relativeAge', () => {
    const now = Date.parse('2026-06-06T12:00:00Z')
    expect(relativeAge('2026-06-06T11:59:30Z', now)).toBe('30s ago')
    expect(relativeAge('2026-06-05T12:00:00Z', now)).toBe('1d ago')
    expect(relativeAge('', now)).toBe('—')
  })
  it('short', () => {
    expect(short('0x5b2b12aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaf2b1')).toBe('0x5b2b1…f2b1')
    expect(short('0xabc')).toBe('0xabc')
  })
})

describe('format (new)', () => {
  it('usd formats dollars with grouping at 2dp', () => {
    expect(usd('12345.678')).toBe('$12,345.68')
    expect(usd('38.36')).toBe('$38.36')
    expect(usd('0')).toBe('$0.00')
  })
  it('usd keeps sub-cent precision', () => {
    expect(usd('0.001881')).toBe('$0.001881')
  })
  it('usd tolerates empty', () => {
    expect(usd('')).toBe('—')
    expect(usd(null)).toBe('—')
  })
  it('pct', () => {
    expect(pct('0.415000')).toBe('41.5%')
    expect(pct('1')).toBe('100%')
    expect(pct('0.0007')).toBe('0.07%')
    expect(pct(null)).toBe('—')
  })
  it('dayLabel', () => {
    expect(dayLabel('2026-06-06')).toBe('Jun 6, 2026')
    expect(dayLabel('')).toBe('—')
  })
})

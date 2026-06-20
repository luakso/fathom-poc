import { describe, it, expect } from 'vitest'
import { weiToEth, gwei, groupDigits, relativeAge, short } from './format.js'

describe('weiToEth', () => {
  it('converts wei to a trimmed ETH string', () => {
    expect(weiToEth('8574617501899')).toBe('0.000008574617501899')
    expect(weiToEth('1000000000000000000')).toBe('1')
    expect(weiToEth('0')).toBe('0')
  })
  it('returns dash for empty/null', () => {
    expect(weiToEth('')).toBe('—')
    expect(weiToEth(null)).toBe('—')
  })
})

describe('gwei', () => {
  it('converts wei to gwei', () => {
    expect(gwei('5000000')).toBe('0.005')
    expect(gwei('1000000000')).toBe('1')
  })
  it('returns dash for empty', () => {
    expect(gwei('')).toBe('—')
  })
})

describe('groupDigits', () => {
  it('adds thousands separators', () => {
    expect(groupDigits('95307')).toBe('95,307')
    expect(groupDigits('51')).toBe('51')
  })
  it('returns dash for empty', () => {
    expect(groupDigits('')).toBe('—')
  })
})

describe('relativeAge', () => {
  const now = Date.parse('2026-06-15T10:00:00Z')
  it('formats seconds/minutes/hours/days', () => {
    expect(relativeAge('2026-06-15T09:59:30Z', now)).toBe('30s ago')
    expect(relativeAge('2026-06-15T09:30:00Z', now)).toBe('30m ago')
    expect(relativeAge('2026-06-15T07:00:00Z', now)).toBe('3h ago')
    expect(relativeAge('2026-06-01T10:00:00Z', now)).toBe('14d ago')
  })
  it('returns dash for empty/invalid', () => {
    expect(relativeAge('', now)).toBe('—')
    expect(relativeAge('not-a-date', now)).toBe('—')
  })
})

describe('short', () => {
  it('shortens long hashes', () => {
    expect(short('0x0e11881999fe9e23eefffc375f86e096')).toBe('0x0e118…e096')
  })
  it('leaves short strings intact', () => {
    expect(short('0xabc')).toBe('0xabc')
    expect(short('')).toBe('')
  })
})

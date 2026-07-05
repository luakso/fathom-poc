import { describe, it, expect } from 'vitest'
import { detectQuery } from './detect'

describe('detectQuery', () => {
  const addr = '0x' + 'a'.repeat(40)
  const tx = '0x' + 'b'.repeat(64)
  it('detects address', () => expect(detectQuery(addr)).toEqual({ kind: 'address', value: addr }))
  it('detects tx hash', () => expect(detectQuery(tx)).toEqual({ kind: 'tx', value: tx }))
  it('trims and lowercases', () => {
    expect(detectQuery('  0x' + 'A'.repeat(40) + ' ')).toEqual({ kind: 'address', value: addr })
  })
  it('rejects junk', () => {
    expect(detectQuery('hello')).toEqual({ kind: 'invalid' })
    expect(detectQuery('0x123')).toEqual({ kind: 'invalid' })
    expect(detectQuery('')).toEqual({ kind: 'invalid' })
  })
})

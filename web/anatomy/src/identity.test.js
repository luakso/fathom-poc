import { describe, it, expect } from 'vitest'
import { identityView } from './identity.js'

describe('identityView', () => {
  it('names a known facilitator', () => {
    const v = identityView({ entityLabel: 'Coinbase', facilitatorKnown: 'true', selfSettled: 'false' })
    expect(v.name).toBe('Coinbase')
    expect(v.knownLabel).toBe('known ✓')
    expect(v.knownTone).toBe('known')
    expect(v.selfSettled).toBe(false)
  })
  it('flags an unknown facilitator with no name', () => {
    const v = identityView({ facilitatorKnown: 'false' })
    expect(v.name).toBe('')
    expect(v.knownLabel).toBe('unknown ⚠')
    expect(v.knownTone).toBe('unknown')
    expect(v.selfSettled).toBe(false)
  })
  it('marks self-settled', () => {
    expect(identityView({ facilitatorKnown: 'true', selfSettled: 'true' }).selfSettled).toBe(true)
  })
  it('returns empty identity for a non-facilitator (payer/payee)', () => {
    const v = identityView({})
    expect(v).toEqual({ name: '', knownLabel: '', knownTone: '', selfSettled: false })
  })
})

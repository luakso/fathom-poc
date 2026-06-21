import { describe, it, expect } from 'vitest'
import { primaryRole, roleColor, ROLE_COLOR } from './roles.js'

describe('primaryRole', () => {
  it('prefers payer, then payee, then facilitator', () => {
    expect(primaryRole(['facilitator', 'payer'])).toBe('payer')
    expect(primaryRole(['facilitator', 'payee'])).toBe('payee')
    expect(primaryRole(['facilitator'])).toBe('facilitator')
  })
  it('falls back to facilitator for empty/unknown', () => {
    expect(primaryRole([])).toBe('facilitator')
    expect(primaryRole(undefined)).toBe('facilitator')
  })
})

describe('roleColor', () => {
  it('maps known roles to their palette color', () => {
    expect(roleColor('payer')).toBe(ROLE_COLOR.payer)
    expect(roleColor('payee')).toBe(ROLE_COLOR.payee)
    expect(roleColor('facilitator')).toBe(ROLE_COLOR.facilitator)
  })
  it('returns a neutral gray for unknown roles', () => {
    expect(roleColor('whatever')).toBe('#8fa098')
  })
})

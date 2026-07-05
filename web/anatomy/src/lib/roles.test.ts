import { describe, it, expect } from 'vitest'
import { primaryRole, roleColor } from './roles'

describe('roles', () => {
  it('primary role prefers payer > payee > facilitator', () => {
    expect(primaryRole(['facilitator', 'payer'])).toBe('payer')
    expect(primaryRole(['payee'])).toBe('payee')
    expect(primaryRole(['facilitator'])).toBe('facilitator')
    expect(primaryRole([])).toBe('facilitator')
    expect(primaryRole(undefined)).toBe('facilitator')
  })
  it('maps roles to sonar palette', () => {
    expect(roleColor('payee')).toBe('#3dd68c')
    expect(roleColor('facilitator')).toBe('#f2c14e')
    expect(roleColor('payer')).toBe('#4fd6e3')
    expect(roleColor('nope')).toBe('#8fa098')
  })
})

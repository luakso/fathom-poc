// Role -> color (sonar palette) and the primary-role rule. Cyan marks the
// payer/interactive family in chips; card accents are handled by CSS class.

// The three economic roles an entity can carry in the substrate.
export type Role = 'payer' | 'payee' | 'facilitator'

// Roles plus the two presentation-only classes a canvas node can take.
export type RoleClass = Role | 'subject' | 'ghost'

export const ROLE_COLOR: Record<RoleClass, string> = {
  payer: '#4fd6e3',
  payee: '#3dd68c',
  facilitator: '#f2c14e',
  subject: '#4fd6e3',
  ghost: '#8fa098',
}

const FALLBACK_COLOR = '#8fa098'

export type PrimaryRole = Role

export function primaryRole(roles: readonly string[] | undefined): PrimaryRole {
  const r = roles ?? []
  if (r.includes('payer')) return 'payer'
  if (r.includes('payee')) return 'payee'
  return 'facilitator'
}

export function roleColor(role: string): string {
  // Widen to a string-indexed view so an unknown role falls through cleanly
  // (noUncheckedIndexedAccess makes the lookup string | undefined).
  const colors: Record<string, string> = ROLE_COLOR
  return colors[role] ?? FALLBACK_COLOR
}

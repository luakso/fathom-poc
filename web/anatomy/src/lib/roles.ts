// Role -> color (sonar palette) and the primary-role rule. Cyan marks the
// payer/interactive family in chips; card accents are handled by CSS class.

export const ROLE_COLOR: Record<string, string> = {
  payer: '#4fd6e3',
  payee: '#3dd68c',
  facilitator: '#f2c14e',
}

export type PrimaryRole = 'payer' | 'payee' | 'facilitator'

export function primaryRole(roles: string[] | undefined): PrimaryRole {
  const r = roles ?? []
  if (r.includes('payer')) return 'payer'
  if (r.includes('payee')) return 'payee'
  return 'facilitator'
}

export function roleColor(role: string): string {
  return ROLE_COLOR[role] ?? '#8fa098'
}

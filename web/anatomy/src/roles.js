// Role → color, and the primary-role rule. The primary role mirrors the lane
// assignment in adapter.js (payer > payee > facilitator) so a card's accent
// color matches the lane it sits in.

export const ROLE_COLOR = {
  payer: '#5db3ff', // blue — money source
  payee: '#3dd68c', // green — money sink (matches --accent)
  facilitator: '#e0b341', // amber — settler / gas payer
}

export function primaryRole(roles) {
  const r = roles || []
  if (r.includes('payer')) return 'payer'
  if (r.includes('payee')) return 'payee'
  return 'facilitator'
}

export function roleColor(role) {
  return ROLE_COLOR[role] || '#8fa098'
}

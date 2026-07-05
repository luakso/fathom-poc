import type { Lens } from '../lib/api'

export function CounterpartiesTab(_props: {
  chain: string
  address: string
  lens: Lens
  onNavigate: (to: string) => void
  defaultRole: string
}) {
  return <div className="empty-state">counterparties</div>
}

import type { Lens } from '../lib/api'

export function PaymentsTab(_props: {
  chain: string
  address: string
  lens: Lens
  onNavigate: (to: string) => void
  defaultRole: string
}) {
  return <div className="empty-state">payments</div>
}

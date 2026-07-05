import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { usd, short } from '../lib/format'
import type { PaymentRow } from '../lib/schemas'

type Props = { chain: string; address: string; lens: Lens; onNavigate: (to: string) => void; defaultRole: string }

function timeLabel(iso: string): string {
  return `${iso.slice(5, 10)} ${iso.slice(11, 16)}`
}

export function PaymentsTab({ chain, address, lens, onNavigate, defaultRole }: Props) {
  const [role, setRole] = useState(defaultRole)
  const [extra, setExtra] = useState<PaymentRow[]>([])
  const [cursor, setCursor] = useState<string | undefined>()
  const first = useQuery({
    queryKey: ['payments', chain, address, role, lens],
    queryFn: () => api.payments(chain, address, { role, lens }),
  })
  const more = useMutation({
    mutationFn: (before: string) => api.payments(chain, address, { role, lens, before }),
    onSuccess: (page) => {
      setExtra((xs) => [...xs, ...page.rows])
      setCursor(page.next)
    },
  })
  const lensSuffix = lens === 'all' ? '?lens=all' : ''
  if (first.isLoading) return <div className="skeleton">loading payments…</div>
  if (first.isError) return <div className="banner-err">{String(first.error)}</div>
  const rows = [...(first.data?.rows ?? []), ...extra]
  const next = cursor ?? first.data?.next
  return (
    <div className="card-block" style={{ padding: '10px 8px' }}>
      <div className="seg" style={{ margin: '2px 6px 8px' }}>
        {['payer', 'payee', 'facilitator'].map((r) => (
          <button key={r} className={r === role ? 'active' : ''}
            onClick={() => { setRole(r); setExtra([]); setCursor(undefined) }}>as {r}</button>
        ))}
      </div>
      {rows.length === 0 && <div className="empty-state">no payments under this lens</div>}
      {rows.map((r) => (
        <div className="pay-row" key={`${r.txHash}:${r.logIndex}`} onClick={() => onNavigate(`/base/tx/${r.txHash}${lensSuffix}`)}>
          <span className="tx">{short(r.txHash)}</span>
          <span className="amt">{usd(r.amountUsdc)}</span>
          <span className="time">{timeLabel(r.blockTimestamp)}</span>
        </div>
      ))}
      {next && (
        <button className="tab" style={{ width: '100%', marginTop: 6 }} disabled={more.isPending}
          onClick={() => more.mutate(next)}>
          {more.isPending ? 'loading…' : 'load more'}
        </button>
      )}
    </div>
  )
}

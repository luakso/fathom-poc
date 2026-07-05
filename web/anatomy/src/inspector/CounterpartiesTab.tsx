import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { usd, groupDigits, pct, short, dayLabel } from '../lib/format'
import { Identicon } from '../lib/Identicon'

const PAGE = 50
const SORTS = ['volume', 'txns', 'last_seen'] as const

type Props = { chain: string; address: string; lens: Lens; onNavigate: (to: string) => void; defaultRole: string }

export function CounterpartiesTab({ chain, address, lens, onNavigate, defaultRole }: Props) {
  const [role, setRole] = useState(defaultRole)
  const [sort, setSort] = useState<string>('volume')
  const [offset, setOffset] = useState(0)
  const q = useQuery({
    queryKey: ['counterparties', chain, address, role, lens, sort, offset],
    queryFn: () => api.counterparties(chain, address, { role, lens, sort, limit: PAGE, offset }),
    placeholderData: (prev) => prev,
  })
  const lensSuffix = lens === 'all' ? '?lens=all' : ''
  if (q.isLoading) return <div className="skeleton">loading counterparties…</div>
  if (q.isError) return <div className="banner-err">{String(q.error)}</div>
  const page = q.data
  return (
    <div className="card-block" style={{ padding: '10px 8px' }}>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', margin: '2px 6px 8px' }}>
        <div className="seg">
          {['payer', 'payee', 'facilitator'].map((r) => (
            <button key={r} className={r === role ? 'active' : ''} onClick={() => { setRole(r); setOffset(0) }}>{r}s</button>
          ))}
        </div>
        <span className="section-title" style={{ marginLeft: 'auto' }}>
          <span className="hint">{groupDigits(page?.total ?? 0)} total</span>
        </span>
      </div>
      <table className="data-table">
        <thead>
          <tr>
            <th>counterparty</th>
            {SORTS.map((s) => (
              <th key={s} className={s === sort ? 'active' : ''} onClick={() => { setSort(s); setOffset(0) }}>
                {s === 'last_seen' ? 'last seen' : s}
              </th>
            ))}
            <th>share</th>
          </tr>
        </thead>
        <tbody>
          {(page?.rows ?? []).map((r) => (
            <tr key={r.address}>
              <td>
                <button className="tab" style={{ display: 'flex', gap: 6, alignItems: 'center', padding: 0 }}
                  onClick={() => onNavigate(`/base/address/${r.address}${lensSuffix}`)}>
                  <Identicon address={r.address} size={16} />
                  {r.label ?? short(r.address)}
                </button>
              </td>
              <td>{usd(r.volumeUsdc)}</td>
              <td>{groupDigits(r.txnCount)}</td>
              <td>{dayLabel(r.lastSeen)}</td>
              <td><span className="share-bar"><i style={{ width: pct(r.share) }} /></span></td>
            </tr>
          ))}
        </tbody>
      </table>
      {(page?.rows.length ?? 0) === 0 && <div className="empty-state">none under this lens</div>}
      <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 8 }}>
        <button className="tab" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>‹ prev</button>
        <button className="tab" disabled={offset + PAGE >= (page?.total ?? 0)} onClick={() => setOffset(offset + PAGE)}>next ›</button>
      </div>
    </div>
  )
}

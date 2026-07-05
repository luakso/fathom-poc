import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { usd, groupDigits, short, dayLabel } from '../lib/format'
import { Identicon } from '../lib/Identicon'

const WINDOWS = ['7d', '30d', 'all'] as const
const SORTS = [
  { key: 'volume', label: 'volume' },
  { key: 'txns', label: 'txns' },
  { key: 'counterparties', label: 'counterparties' },
] as const

export function Home() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const lens = (params.get('lens') === 'all' ? 'all' : 'known') as Lens
  const [role, setRole] = useState<'payee' | 'payer'>('payee')
  const [window_, setWindow] = useState<string>('all')
  const [sort, setSort] = useState<string>('volume')
  const q = useQuery({
    queryKey: ['leaderboard', role, window_, lens, sort],
    queryFn: () => api.leaderboard('base', { role, window: window_, lens, sort, limit: 500 }),
    placeholderData: (prev) => prev,
  })
  const lensSuffix = lens === 'all' ? '?lens=all' : ''
  return (
    <div style={{ flex: 1, overflowY: 'auto', padding: '28px 20px' }}>
      <div style={{ maxWidth: 980, margin: '0 auto', display: 'flex', flexDirection: 'column', gap: 14 }}>
        <div className="section-title">Live leaderboards <span className="hint">top 500 · window anchors to data, not wall clock</span></div>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          <div className="seg">
            <button className={role === 'payee' ? 'active' : ''} onClick={() => setRole('payee')}>payees</button>
            <button className={role === 'payer' ? 'active' : ''} onClick={() => setRole('payer')}>payers</button>
          </div>
          <div className="seg">
            {WINDOWS.map((w) => (
              <button key={w} className={w === window_ ? 'active' : ''} onClick={() => setWindow(w)}>{w}</button>
            ))}
          </div>
        </div>
        <div className="card-block" style={{ padding: '8px 10px' }}>
          {q.isLoading && <div className="skeleton">loading leaderboard…</div>}
          {q.isError && <div className="banner-err">{String(q.error)}</div>}
          {q.data && (
            <table className="data-table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>{role}</th>
                  {SORTS.map((s) => (
                    <th key={s.key} className={s.key === sort ? 'active' : ''} onClick={() => setSort(s.key)}>{s.label}</th>
                  ))}
                  <th>first seen</th>
                  <th>last seen</th>
                </tr>
              </thead>
              <tbody>
                {q.data.rows.map((r) => (
                  <tr key={r.address} style={{ cursor: 'pointer' }}
                    onClick={() => navigate(`/base/address/${r.address}${lensSuffix}`)}>
                    <td>{r.rank}</td>
                    <td>
                      <span style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                        <Identicon address={r.address} size={18} />
                        {r.label ?? short(r.address)}
                      </span>
                    </td>
                    <td>{usd(r.volumeUsdc)}</td>
                    <td>{groupDigits(r.txnCount)}</td>
                    <td>{groupDigits(r.distinctCounterparties)}</td>
                    <td>{dayLabel(r.firstSeen)}</td>
                    <td>{dayLabel(r.lastSeen)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {q.data && q.data.rows.length === 0 && <div className="empty-state">empty leaderboard - run `anatomy rollup`?</div>}
        </div>
      </div>
    </div>
  )
}

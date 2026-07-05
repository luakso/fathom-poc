import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { densify } from '../lib/series'
import { dayLabel, groupDigits } from '../lib/format'

export function ActivityTab({ chain, address, lens }: { chain: string; address: string; lens: Lens }) {
  const q = useQuery({
    queryKey: ['timeline', chain, address, lens],
    queryFn: () => api.timeline(chain, address, lens),
  })
  const roles = Object.keys(q.data?.roles ?? {})
  const defaultRole = useMemo(
    () => roles.sort((a, b) =>
      (q.data?.roles[b] ?? []).reduce((s, p) => s + p.txnCount, 0) -
      (q.data?.roles[a] ?? []).reduce((s, p) => s + p.txnCount, 0))[0],
    [q.data],
  )
  const [role, setRole] = useState<string | null>(null)
  const active = role ?? defaultRole
  if (q.isLoading) return <div className="skeleton">loading activity…</div>
  if (q.isError) return <div className="banner-err">{String(q.error)}</div>
  const points = densify(q.data?.roles[active] ?? [])
  if (points.length === 0) return <div className="empty-state">no activity under this lens</div>
  const max = Math.max(...points.map((p) => p.txnCount))
  const maxIdx = points.findIndex((p) => p.txnCount === max)
  const W = 456, H = 122, top = 14, bottom = 22
  const bw = Math.max(1, Math.min(8, (W - 16) / points.length - 2))
  const step = (W - 16) / points.length
  return (
    <div className="card-block">
      <div className="section-title" style={{ marginBottom: 8 }}>
        Daily payments <span className="hint">{dayLabel(points[0].day)} → {dayLabel(points[points.length - 1].day)}</span>
      </div>
      {roles.length > 1 && (
        <div className="seg" style={{ marginBottom: 8 }}>
          {roles.map((r) => (
            <button key={r} className={r === active ? 'active' : ''} onClick={() => setRole(r)}>{r}</button>
          ))}
        </div>
      )}
      <svg width="100%" viewBox={`0 0 ${W} ${H}`}>
        <text x={8 + maxIdx * step} y={10} fontSize={10} fontWeight={700} fill="#8ce9bb" fontFamily="var(--mono)">
          {groupDigits(max)}
        </text>
        {points.map((p, i) => {
          const h = max === 0 ? 0 : Math.round((p.txnCount / max) * (H - top - bottom))
          return p.txnCount === 0
            ? <rect key={p.day} x={8 + i * step} y={H - bottom - 1} width={bw} height={1} fill="rgba(143,160,152,.35)" />
            : <rect key={p.day} x={8 + i * step} y={H - bottom - h} width={bw} height={h} rx={1}
                fill={i === maxIdx ? '#3dd68c' : 'rgba(61,214,140,.5)'} />
        })}
        <line x1={6} y1={H - bottom + 0.5} x2={W - 4} y2={H - bottom + 0.5} stroke="rgba(42,54,47,.9)" />
      </svg>
    </div>
  )
}

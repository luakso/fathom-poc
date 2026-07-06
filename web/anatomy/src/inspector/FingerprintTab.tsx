import { useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { usdc, groupDigits, pct } from '../lib/format'

export function FingerprintTab({ chain, address, lens }: { chain: string; address: string; lens: Lens }) {
  const q = useQuery({
    queryKey: ['fingerprint', chain, address, lens],
    queryFn: () => api.fingerprint(chain, address, lens),
  })
  if (q.isLoading) return <div className="skeleton">loading fingerprint…</div>
  if (q.isError) return <div className="banner-err">{String(q.error)}</div>
  const roles = Object.entries(q.data?.roles ?? {})
  if (roles.length === 0) return <div className="empty-state">no activity under this lens</div>
  return (
    <>
      {roles.map(([role, fp]) => (
        <div key={role}>
          <div className="section-title" style={{ margin: '4px 0 9px' }}>
            {role} fingerprint
            {role === 'facilitator' && <span className="hint">settled traffic, not own spending</span>}
          </div>
          <div className="tiles">
            <div className="tile">
              <h4>Cadence</h4>
              <div className="big">{groupDigits(fp.medianTxnsPerDay)}<small>/day</small></div>
              <div className="sub">active <b>{String(fp.activeDays)}</b> of {String(fp.spanDays)} days · top day <b>{pct(fp.topDayShare)}</b> of total</div>
            </div>
            <div className="tile">
              <h4>Price points</h4>
              {fp.pricePoints.slice(0, 5).map((p) => (
                <div className="sub" key={p.amountUsdc}><b>{usdc(p.amountUsdc)}</b> × {groupDigits(p.txnCount)}</div>
              ))}
              <div className="sub">
                {fp.totalDistinctAmounts != null
                  ? `of ${groupDigits(fp.totalDistinctAmounts)} distinct amounts`
                  : 'distinct count n/a under all lens'}
              </div>
            </div>
            <div className="tile">
              <h4>Concentration</h4>
              <div className="sub">{`top-1 ${pct(fp.top1Share)} · top-3 ${pct(fp.top3Share)} of volume`}</div>
            </div>
          </div>
        </div>
      ))}
    </>
  )
}

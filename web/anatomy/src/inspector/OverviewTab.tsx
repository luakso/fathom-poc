import type { Entity } from '../lib/schemas'
import type { Lens } from '../lib/api'
import { usd, groupDigits } from '../lib/format'

export function OverviewTab({ entity }: { entity: Entity; lens: Lens }) {
  return (
    <>
      <div className="card-block">
        <div className="section-title" style={{ marginBottom: 8 }}>Identity signals</div>
        {(entity.signals ?? []).length === 0 && <div className="empty-state">no identity signals</div>}
        {(entity.signals ?? []).map((s) => (
          <div key={`${s.source}:${s.kind}:${s.value}`} style={{ display: 'flex', gap: 8, alignItems: 'center', padding: '5px 0', fontSize: 11.5 }}>
            <span className="badge">{s.source}</span>
            <span style={{ color: 'var(--dim)' }}>{s.kind}</span>
            {s.url ? <a href={s.url} target="_blank" rel="noreferrer" className="mono">{s.value}</a> : <span className="mono">{s.value}</span>}
          </div>
        ))}
      </div>
      {Object.entries(entity.summaries).map(([role, byLens]) => (
        <div className="card-block" key={role}>
          <div className="section-title" style={{ marginBottom: 8 }}>{role} totals <span className="hint">both lenses</span></div>
          <table className="data-table">
            <thead><tr><th>lens</th><th>txns</th><th>volume</th><th>counterparties</th><th>active days</th></tr></thead>
            <tbody>
              {(['known', 'all'] as const).map((l) => byLens[l] && (
                <tr key={l}>
                  <td>{l === 'known' ? 'verified x402' : 'all EIP-3009'}</td>
                  <td>{groupDigits(byLens[l].txnCount)}</td>
                  <td>{usd(byLens[l].volumeUsdc)}</td>
                  <td>{groupDigits(byLens[l].distinctCounterparties)}</td>
                  <td>{byLens[l].activeDays}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </>
  )
}

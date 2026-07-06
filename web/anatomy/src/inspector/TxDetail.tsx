import type { GraphNode } from '../lib/schemas'
import { buildTxView } from '../lib/txview'

export function TxDetail({ node }: { node: GraphNode }) {
  // Relative age is not rendered in the detail panel, so pass a fixed 0 rather
  // than reading the wall clock during render.
  const v = buildTxView(node.fields, 0)
  return (
    <>
      {v.sections.map((s) => (
        <div className="card-block" key={s.title}>
          <div className="section-title" style={{ marginBottom: 8 }}>{s.title}</div>
          <table className="data-table"><tbody>
            {s.rows.map((r) => (
              <tr key={r.k}>
                <td style={{ color: 'var(--dim)', fontFamily: 'var(--font)' }}>{r.k}</td>
                <td>{r.v}{r.note && <span style={{ color: 'var(--faint)', marginLeft: 6, fontFamily: 'var(--font)' }}>({r.note})</span>}</td>
              </tr>
            ))}
          </tbody></table>
        </div>
      ))}
      <div className="card-block">
        <div className="section-title" style={{ marginBottom: 8 }}>Input data <span className="hint">{v.input.methodId}</span></div>
        {v.input.params && (
          <table className="data-table"><tbody>
            {v.input.params.map((r) => (
              <tr key={r.k}><td style={{ color: 'var(--dim)', fontFamily: 'var(--font)' }}>{r.k}</td><td>{r.v}</td></tr>
            ))}
          </tbody></table>
        )}
        <div className="mono" style={{ marginTop: 8, fontSize: 10, color: 'var(--faint)', wordBreak: 'break-all', maxHeight: 120, overflowY: 'auto' }}>
          {v.input.rawCalldata}
        </div>
      </div>
      {v.basescanUrl && (
        <a href={v.basescanUrl} target="_blank" rel="noreferrer" className="badge" style={{ alignSelf: 'flex-start' }}>view on Basescan ↗</a>
      )}
    </>
  )
}

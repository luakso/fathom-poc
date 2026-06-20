import { useState } from 'react'
import { buildTxView } from '../txview.js'
import { short } from '../format.js'

export default function TxDetailPanel({ node, onClose }) {
  const [showRaw, setShowRaw] = useState(false)
  if (!node) return null
  const v = buildTxView(node.fields || {}, Date.now())
  return (
    <aside className="tx-panel">
      <header className="tx-panel-top">
        <span className="tx-panel-title">Transaction details</span>
        <button className="tx-panel-copy" title="copy tx hash" onClick={() => navigator.clipboard?.writeText(node.label)}>
          {short(node.label)} ⧉
        </button>
        <button className="tx-panel-x" onClick={onClose}>✕</button>
      </header>
      <div className="tx-panel-body">
        {v.sections.map((s) => (
          <section key={s.title}>
            <h4>{s.title}</h4>
            {s.rows.map((r) => (
              <div className="kv" key={r.k}>
                <span>{r.k}</span>
                <span>{r.v}{r.note ? <em className="note"> ({r.note})</em> : null}</span>
              </div>
            ))}
          </section>
        ))}

        <section>
          <h4>Input data — {v.input.method}</h4>
          <div className="kv"><span>method id</span><span>{v.input.methodId}</span></div>
          {v.input.decodable
            ? v.input.params.map((p) => (
                <div className="kv" key={p.k}><span>{p.k}</span><span>{p.v}</span></div>
              ))
            : <div className="note">decoded per-event — see event nodes</div>}
          <button className="tx-panel-toggle" onClick={() => setShowRaw((x) => !x)}>
            {showRaw ? 'hide raw calldata' : 'show raw calldata ▸'}
          </button>
          {showRaw && <pre className="tx-raw">{v.input.rawCalldata}</pre>}
        </section>

        {v.basescanUrl && (
          <section>
            <h4>Links</h4>
            <a className="tx-link" href={v.basescanUrl} target="_blank" rel="noreferrer">View on Basescan ↗</a>
          </section>
        )}
      </div>
    </aside>
  )
}

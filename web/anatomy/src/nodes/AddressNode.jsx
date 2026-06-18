import { Handle, Position } from '@xyflow/react'

export default function AddressNode({ data }) {
  const stats = data.stats // attached after expansion (Task 8)
  const onExpandStats = data.onExpandStats
  return (
    <div className="node-card address">
      <div className="title">{short(data.label)}</div>
      <div>{(data.roles || []).map((r) => <span key={r} className="role-badge">{r}</span>)}</div>
      {stats && (
        <>
          <div className="kv"><span>payments</span><span>{stats.paymentCount}</span></div>
          <div className="kv"><span>volume</span><span>${stats.volumeUsdc}</span></div>
          <div className="kv"><span>counterpts</span><span>{stats.distinctCounterparties}</span></div>
        </>
      )}
      {!stats && (data.providers || []).map((p) =>
        p.available ? (
          <button key={p.kind} className="role-badge" onClick={() => onExpandStats?.(data.id)}>+ {p.kind}</button>
        ) : (
          <span key={p.kind} className="provider-stub">{p.kind} (soon)</span>
        ),
      )}
      <Handle type="target" position={Position.Top} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

function short(s) {
  if (!s) return ''
  return s.length > 12 ? `${s.slice(0, 7)}…${s.slice(-4)}` : s
}

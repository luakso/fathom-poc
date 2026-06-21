import { Handle, Position } from '@xyflow/react'
import { primaryRole, roleColor } from '../roles.js'
import { short } from '../format.js'
import { identityView } from '../identity.js'
import Identicon from './Identicon.jsx'

export default function AddressNode({ data }) {
  const stats = data.stats // attached after expansion
  const onExpandStats = data.onExpandStats
  const roles = data.roles || []
  const accent = roleColor(primaryRole(roles))
  const id = identityView(data.fields || {})
  return (
    <div className="node-card address" style={{ borderColor: accent }}>
      <div className="addr-head">
        <Identicon address={data.label} size={22} />
        <span className="title" style={{ color: accent }}>{short(data.label)}</span>
      </div>
      {id.name && <div className="entity">{id.name}</div>}
      <div className="roles">
        {roles.map((r) => (
          <span key={r} className="role-badge" style={{ borderColor: roleColor(r), color: roleColor(r) }}>{r}</span>
        ))}
      </div>
      {(id.knownLabel || id.selfSettled) && (
        <div className="id-badges">
          {id.knownLabel && <span className={`id-badge ${id.knownTone}`}>{id.knownLabel}</span>}
          {id.selfSettled && <span className="id-badge self">self-settled</span>}
        </div>
      )}
      {stats && (
        <>
          <div className="kv"><span>payments</span><span>{stats.paymentCount}</span></div>
          <div className="kv"><span>volume</span><span>${stats.volumeUsdc}</span></div>
          <div className="kv"><span>counterpts</span><span>{stats.distinctCounterparties}</span></div>
        </>
      )}
      {!stats && (
        <div className="providers">
          {(data.providers || []).filter((p) => p.kind !== 'identity').map((p) =>
            p.available ? (
              <button key={p.kind} className="stat-btn" style={{ borderColor: accent, color: accent }} onClick={() => onExpandStats?.(data.id)}>+ {p.kind}</button>
            ) : (
              <span key={p.kind} className="provider-stub">{p.kind} (soon)</span>
            ),
          )}
        </div>
      )}
      <Handle type="target" position={Position.Top} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

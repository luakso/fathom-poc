import { EDGE_COLOR } from '../adapter.js'
import { ROLE_COLOR } from '../roles.js'

// Legend: a small fixed key for edge kinds and address role colors.
export default function Legend() {
  return (
    <div className="legend">
      <div className="legend-group">
        {[
          ['emits', EDGE_COLOR.emits],
          ['pays', EDGE_COLOR.pays],
          ['settles', EDGE_COLOR.settles],
        ].map(([label, color]) => (
          <span key={label} className="legend-item">
            <span className="legend-line" style={{ background: color }} />
            {label}
          </span>
        ))}
      </div>
      <div className="legend-group">
        {['payer', 'payee', 'facilitator'].map((role) => (
          <span key={role} className="legend-item">
            <span className="legend-dot" style={{ background: ROLE_COLOR[role] }} />
            {role}
          </span>
        ))}
      </div>
    </div>
  )
}

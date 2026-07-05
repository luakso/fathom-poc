import { Handle, Position, type NodeProps } from '@xyflow/react'
import { short } from '../lib/format'
import { buildTxView } from '../lib/txview'

export function TxNode({ data, selected }: NodeProps & { data: { label: string; fields: Record<string, string> } }) {
  const v = buildTxView(data.fields, Date.now())
  return (
    <div className={`node-card subject ${selected ? 'selected' : ''}`} style={{ width: 300 }}>
      <Handle id="l" type="target" position={Position.Left} style={{ opacity: 0 }} />
      <Handle id="t" type="target" position={Position.Top} style={{ opacity: 0 }} />
      <Handle id="r" type="source" position={Position.Right} style={{ opacity: 0 }} />
      <Handle id="b" type="source" position={Position.Bottom} style={{ opacity: 0 }} />
      <div className="n-title mono-title">{short(data.label)}</div>
      <div style={{ marginTop: 5, display: 'flex', gap: 6 }}>
        <span className="chip cyan">transaction</span>
        {v.headline.methodKind && <span className="chip green">{v.headline.method}</span>}
      </div>
      <div className="kpis">
        <div className="kpi"><b>{v.headline.paid}</b><span>paid</span></div>
        <div className="kpi"><b>{v.headline.fee}</b><span>fee</span></div>
        <div className="kpi"><b>{v.headline.events}</b><span>events</span></div>
      </div>
    </div>
  )
}

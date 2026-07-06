import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import { short } from '../lib/format'
import { buildTxView } from '../lib/txview'
import type { TxNodeData } from './graph'

export function TxNode({ data, selected }: NodeProps<Node<TxNodeData>> & { data: TxNodeData }) {
  // Relative age is not shown on the node card, so pass a fixed 0 rather than a
  // wall-clock read (keeps rendering deterministic / free of Date.now in render).
  const v = buildTxView(data.fields, 0)
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

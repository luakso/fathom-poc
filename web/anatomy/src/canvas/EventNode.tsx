import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import type { EventNodeData } from './graph'

export function EventNode({ data }: NodeProps<Node<EventNodeData>> & { data: EventNodeData }) {
  return (
    <div className="node-card ghost" style={{ width: 230 }}>
      <Handle id="t" type="target" position={Position.Top} style={{ opacity: 0 }} />
      <Handle id="r" type="source" position={Position.Right} style={{ opacity: 0 }} />
      <div className="n-title mono-title">{data.label}</div>
      {data.fields.amount && <div className="n-addr" style={{ marginTop: 3 }}>{data.fields.amount} USDC</div>}
      <span className="chip ghost-chip" style={{ marginTop: 6 }}>event</span>
    </div>
  )
}

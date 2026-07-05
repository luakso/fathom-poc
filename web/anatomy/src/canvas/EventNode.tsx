import { Handle, Position, type NodeProps } from '@xyflow/react'

export function EventNode({ data }: NodeProps & { data: { label: string; fields: Record<string, string> } }) {
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

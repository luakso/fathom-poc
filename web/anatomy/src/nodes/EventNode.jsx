import { Handle, Position } from '@xyflow/react'

export default function EventNode({ data }) {
  const f = data.fields || {}
  return (
    <div className="node-card event">
      <div className="title">{data.label}</div>
      <div className="kv"><span>amount</span><span>${f.amountUsdc}</span></div>
      <div className="kv"><span>asset</span><span>{f.tokenSymbol || f.asset}</span></div>
      <div className="kv"><span>settle</span><span>{f.settlementKind}</span></div>
      <Handle type="target" position={Position.Top} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

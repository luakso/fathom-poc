import { Handle, Position } from '@xyflow/react'

export default function TransactionNode({ data }) {
  const f = data.fields || {}
  return (
    <div className="node-card transaction">
      <div className="title">TX {short(data.label)}</div>
      <div className="kv"><span>block</span><span>{f.block}</span></div>
      <div className="kv"><span>gas used</span><span>{f.gasUsed}</span></div>
      <div className="kv"><span>method</span><span>{short(f.methodSelector)}</span></div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

function short(s) {
  if (!s) return ''
  return s.length > 12 ? `${s.slice(0, 7)}…${s.slice(-4)}` : s
}

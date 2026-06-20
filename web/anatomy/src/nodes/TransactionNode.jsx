import { Handle, Position } from '@xyflow/react'
import { buildTxView } from '../txview.js'
import { short } from '../format.js'

export default function TransactionNode({ data }) {
  const v = buildTxView(data.fields || {}, Date.now())
  const h = v.headline
  return (
    <div className="node-card transaction clickable">
      <div className="title">TX {short(data.label)}</div>
      <div className="kv lead"><span>paid</span><span>{h.paid}</span></div>
      <div className="kv"><span>fee</span><span>{h.fee}</span></div>
      <div className="kv">
        <span>method</span>
        <span>{h.method}{h.methodKind ? <em className="chip"> {h.methodKind}</em> : null}</span>
      </div>
      <div className="kv"><span>age</span><span>{h.age}</span></div>
      <div className="kv"><span>events</span><span>{h.events}</span></div>
      <div className="hint">click for details ▸</div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import { short } from '../lib/format'
import type { EntityNodeData } from './graph'

export function GhostNode({ data }: NodeProps<Node<EntityNodeData>> & { data: EntityNodeData }) {
  const onFocus = data.onFocus as ((addr: string) => void) | undefined
  return (
    <div className="node-card ghost" style={{ width: 230 }}>
      <Handle id="l" type="target" position={Position.Left} style={{ opacity: 0 }} />
      <Handle id="r" type="source" position={Position.Right} style={{ opacity: 0 }} />
      <div className="n-title mono-title">{short(data.address)}</div>
      <div className="n-addr" style={{ margin: '3px 0 6px' }}>unidentified</div>
      <span className="chip ghost-chip">outside lens</span>
      {onFocus && (
        <button className="expand-btn" onClick={(e) => { e.stopPropagation(); onFocus(data.address) }}>⌖ focus</button>
      )}
    </div>
  )
}

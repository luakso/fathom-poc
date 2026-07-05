import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import { Identicon } from '../lib/Identicon'
import { short } from '../lib/format'
import type { EntityNodeData } from './graph'

type Props = NodeProps<Node<EntityNodeData>> & { data: EntityNodeData }

export function EntityNode({ data, selected }: Props) {
  const title = data.label || short(data.address)
  const cls = ['node-card', data.roleClass, selected ? 'selected' : ''].join(' ').trim()
  const onExpand = data.onExpand as ((addr: string) => void) | undefined
  const onFocus = data.onFocus as ((addr: string) => void) | undefined
  return (
    <div className={cls} style={{ width: 300 }}>
      <Handle id="l" type="target" position={Position.Left} style={{ opacity: 0 }} />
      <Handle id="t" type="target" position={Position.Top} style={{ opacity: 0 }} />
      <Handle id="r" type="source" position={Position.Right} style={{ opacity: 0 }} />
      <Handle id="b" type="source" position={Position.Bottom} style={{ opacity: 0 }} />
      <div className="n-head">
        <Identicon address={data.address} />
        <div>
          <div className={data.label ? 'n-title' : 'n-title mono-title'}>{title}</div>
          {data.label && <div className="n-addr">{short(data.address)}</div>}
          <div style={{ marginTop: 5, display: 'flex', gap: 6 }}>
            {data.chips.map((c) => (
              <span key={c} className={`chip ${c === 'facilitator' ? 'amber' : c === 'payee' ? 'green' : 'cyan'}`}>{c}</span>
            ))}
          </div>
        </div>
      </div>
      {data.detail && <div style={{ fontSize: 11, color: 'var(--dim)', lineHeight: 1.5 }}>{data.detail}</div>}
      {data.kpis && (
        <div className="kpis">
          <div className="kpi"><b>{data.kpis.payments}</b><span>payments</span></div>
          <div className="kpi"><b>{data.kpis.volume}</b><span>volume</span></div>
          <div className="kpi"><b>{data.kpis.counterparties}</b><span>counterparties</span></div>
        </div>
      )}
      <div style={{ display: 'flex', gap: 8, justifyContent: 'center' }}>
        {data.expandable && onExpand && (
          <button className="expand-btn" onClick={(e) => { e.stopPropagation(); onExpand(data.address) }}>+ expand</button>
        )}
        {data.roleClass !== 'subject' && onFocus && (
          <button className="expand-btn" onClick={(e) => { e.stopPropagation(); onFocus(data.address) }}>⌖ focus</button>
        )}
      </div>
    </div>
  )
}

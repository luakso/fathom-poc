import { useMemo } from 'react'
import { ReactFlow, Background, BackgroundVariant, Controls, MiniMap } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { EntityNode } from './EntityNode'
import { GhostNode } from './GhostNode'
import { TxNode } from './TxNode'
import { EventNode } from './EventNode'
import { FlowEdge } from './FlowEdge'
import { roleColor } from '../lib/roles'
import type { FlowNode, FlowEdge as FlowEdgeType } from './graph'

const nodeTypes = { entity: EntityNode, ghost: GhostNode, tx: TxNode, event: EventNode }
const edgeTypes = { flow: FlowEdge }

type Props = {
  nodes: FlowNode[]
  edges: FlowEdgeType[]
  selectedId?: string
  onSelect: (id: string) => void
  onExpand?: (addr: string) => void
  onFocus?: (addr: string) => void
}

export function Canvas({ nodes, edges, selectedId, onSelect, onExpand, onFocus }: Props) {
  const decorated = useMemo(
    () => nodes.map((n) => {
      const selected = n.id === selectedId
      // onExpand/onFocus belong only to entity/ghost cards (their data carries
      // roleClass); tx/event cards are left untouched.
      return 'roleClass' in n.data
        ? { ...n, selected, data: { ...n.data, onExpand, onFocus } }
        : { ...n, selected }
    }),
    [nodes, selectedId, onExpand, onFocus],
  )
  return (
    <div className="canvas-wrap">
      <ReactFlow
        nodes={decorated}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodeClick={(_, node) => onSelect(node.id)}
        fitView
        minZoom={0.2}
        nodesDraggable
        proOptions={{ hideAttribution: false }}
      >
        <Background variant={BackgroundVariant.Dots} gap={24} size={1.2} color="rgba(214,226,218,.075)" />
        <Controls showInteractive={false} />
        <MiniMap
          pannable
          nodeColor={(n) => {
            if (n.type === 'ghost') return 'rgba(143,160,152,.25)'
            const roleClass = n.data['roleClass']
            return typeof roleClass === 'string' ? roleColor(roleClass) : '#8fa098'
          }}
          maskColor="rgba(7,11,9,.7)"
          style={{ background: 'rgba(12,17,14,.92)' }}
        />
      </ReactFlow>
    </div>
  )
}

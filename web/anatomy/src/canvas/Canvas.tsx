import { useMemo } from 'react'
import { ReactFlow, Background, BackgroundVariant, Controls, MiniMap, type Node, type Edge } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { EntityNode } from './EntityNode'
import { GhostNode } from './GhostNode'
import { FlowEdge } from './FlowEdge'
import { roleColor } from '../lib/roles'
import type { EntityNodeData } from './graph'

const nodeTypes = { entity: EntityNode, ghost: GhostNode }
const edgeTypes = { flow: FlowEdge }

type Props = {
  nodes: Node<EntityNodeData>[]
  edges: Edge[]
  selectedId?: string
  onSelect: (id: string) => void
  onExpand?: (addr: string) => void
  onFocus?: (addr: string) => void
}

export function Canvas({ nodes, edges, selectedId, onSelect, onExpand, onFocus }: Props) {
  const decorated = useMemo(
    () => nodes.map((n) => ({
      ...n,
      selected: n.id === selectedId,
      data: { ...n.data, onExpand, onFocus },
    })),
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
          nodeColor={(n) => (n.type === 'ghost' ? 'rgba(143,160,152,.25)' : roleColor((n.data as EntityNodeData).roleClass))}
          maskColor="rgba(7,11,9,.7)"
          style={{ background: 'rgba(12,17,14,.92)' }}
        />
      </ReactFlow>
    </div>
  )
}

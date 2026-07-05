// Dossier Graph -> flow, v1 lane logic re-expressed on layout.ts: payer lane
// left of the tx card, payee right, facilitator above, events stacked below.
import type { Node, Edge } from '@xyflow/react'
import type { Graph, GraphNode } from '../lib/schemas'
import { NODE_W, NODE_H, GHOST_W, GHOST_H, COL_GAP, placeColumn, type Rect } from './layout'
import type { FlowState, EntityNodeData } from './graph'

function laneOf(n: GraphNode): 'left' | 'right' | 'up' | 'down' {
  if (n.kind === 'event') return 'down'
  const roles = n.roles ?? []
  if (roles.includes('payer')) return 'left'
  if (roles.includes('facilitator') && !roles.includes('payee')) return 'up'
  return 'right'
}

export function txGraphToFlow(g: Graph): FlowState {
  const txNode = g.nodes.find((n) => n.kind === 'transaction')
  const nodes: Node[] = []
  const txRect: Rect = { x: 0, y: 0, w: NODE_W, h: NODE_H }
  if (txNode) {
    nodes.push({ id: txNode.id, type: 'tx', position: { x: 0, y: 0 }, data: { ...txNode } })
  }
  const lanes: Record<string, GraphNode[]> = { left: [], right: [], up: [], down: [] }
  for (const n of g.nodes) {
    if (n.kind === 'transaction') continue
    lanes[laneOf(n)].push(n)
  }
  const occupied: Rect[] = [txRect]
  const place = (list: GraphNode[], x: number, centerY: number, box: { w: number; h: number }) => {
    const rects = placeColumn(occupied, x, centerY, list.map(() => box))
    occupied.push(...rects)
    return rects
  }
  const addrBox = { w: NODE_W, h: NODE_H }
  const evBox = { w: GHOST_W, h: GHOST_H }
  const mk = (n: GraphNode, r: Rect): Node => n.kind === 'event'
    ? { id: n.id, type: 'event', position: { x: r.x, y: r.y }, data: { ...n } }
    : {
        id: n.id, type: 'entity', position: { x: r.x, y: r.y },
        data: {
          address: n.fields.address ?? n.label,
          label: n.fields.entityLabel || undefined,
          roleClass: (n.roles ?? []).includes('facilitator') ? 'facilitator' : (n.roles ?? []).includes('payer') ? 'payer' : 'payee',
          chips: n.roles ?? [],
          expandable: false,
        } satisfies EntityNodeData,
      }
  place(lanes.left, -(COL_GAP + NODE_W), NODE_H / 2, addrBox).forEach((r, i) => nodes.push(mk(lanes.left[i], r)))
  place(lanes.right, NODE_W + COL_GAP, NODE_H / 2, addrBox).forEach((r, i) => nodes.push(mk(lanes.right[i], r)))
  place(lanes.up, 40, -(COL_GAP + NODE_H), addrBox).forEach((r, i) => nodes.push(mk(lanes.up[i], r)))
  place(lanes.down, 35, NODE_H + COL_GAP + GHOST_H, evBox).forEach((r, i) => nodes.push(mk(lanes.down[i], r)))

  const edges: Edge[] = g.edges.map((e) => ({
    id: e.id, source: e.source, target: e.target, type: 'flow',
    data: { label: e.label, verb: e.kind === 'settles', share: 0.4 },
  }))
  return { nodes: nodes as FlowState['nodes'], edges }
}

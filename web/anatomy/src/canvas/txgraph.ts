// Dossier Graph -> flow, v1 lane logic re-expressed on layout.ts: payer lane
// left of the tx card, payee right, facilitator above, events stacked below.
import type { Graph, GraphNode } from '../lib/schemas'
import { NODE_W, NODE_H, GHOST_W, GHOST_H, COL_GAP, placeColumn, type Rect } from './layout'
import type { FlowState, FlowNode, EntityNodeData } from './graph'

type Lane = 'left' | 'right' | 'up' | 'down'

function laneOf(n: GraphNode): Lane {
  if (n.kind === 'event') return 'down'
  const roles = n.roles ?? []
  if (roles.includes('payer')) return 'left'
  if (roles.includes('facilitator') && !roles.includes('payee')) return 'up'
  return 'right'
}

function entityData(n: GraphNode): EntityNodeData {
  const roles = n.roles ?? []
  return {
    address: n.fields.address ?? n.label,
    label: n.fields.entityLabel || undefined,
    roleClass: roles.includes('facilitator') ? 'facilitator' : roles.includes('payer') ? 'payer' : 'payee',
    chips: roles,
    expandable: false,
  }
}

function mk(n: GraphNode, r: Rect): FlowNode {
  if (n.kind === 'event') {
    return { id: n.id, type: 'event', position: { x: r.x, y: r.y }, data: { label: n.label, fields: n.fields } }
  }
  return { id: n.id, type: 'entity', position: { x: r.x, y: r.y }, data: entityData(n) }
}

export function txGraphToFlow(g: Graph): FlowState {
  const txNode = g.nodes.find((n) => n.kind === 'transaction')
  const nodes: FlowNode[] = []
  const txRect: Rect = { x: 0, y: 0, w: NODE_W, h: NODE_H }
  if (txNode) {
    nodes.push({ id: txNode.id, type: 'tx', position: { x: 0, y: 0 }, data: { label: txNode.label, fields: txNode.fields } })
  }
  const lanes: Record<Lane, GraphNode[]> = { left: [], right: [], up: [], down: [] }
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
  const pushLane = (list: GraphNode[], rects: Rect[]) => {
    list.forEach((n, i) => {
      const r = rects[i]
      if (r) nodes.push(mk(n, r))
    })
  }
  pushLane(lanes.left, place(lanes.left, -(COL_GAP + NODE_W), NODE_H / 2, addrBox))
  pushLane(lanes.right, place(lanes.right, NODE_W + COL_GAP, NODE_H / 2, addrBox))
  pushLane(lanes.up, place(lanes.up, 40, -(COL_GAP + NODE_H), addrBox))
  pushLane(lanes.down, place(lanes.down, 35, NODE_H + COL_GAP + GHOST_H, evBox))

  const edges = g.edges.map((e) => ({
    id: e.id, source: e.source, target: e.target, type: 'flow',
    data: { label: e.label, verb: e.kind === 'settles', share: 0.4 },
  }))
  return { nodes, edges }
}
